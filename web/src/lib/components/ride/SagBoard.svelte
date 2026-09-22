<script lang="ts">
	// The SAG board (WP7, part 1): open requests with status/rider count/
	// oldest age/pickup position, a per-vehicle capacity strip, and the full
	// dispatch flow (assign vehicle -> load -> deliver -> release) expanded
	// inline per request. Slots and legs are the backend's real model — see
	// internal/ride/types.go — so a request with a partial pickup shows two
	// legs, not a flattened "in progress" blob.
	import { api, ApiError } from '$lib/api';
	import type { SAGRequest, SAGLeg, SAGLocation, SAGVehicleStatus, OverCapacityBody } from '$lib/types';
	import {
		sagBoard, rideLadder, sagComposerSeed, upsertSagRequest, upsertSagVehicle
	} from '$lib/stores/ride';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { canOperate } from '$lib/stores/session';
	import { secondClock } from '$lib/stores/clock';
	import { showToast } from '$lib/stores/toast';
	import {
		tierById, tierStyle, ageText, enumLabel,
		SAG_STATUS_LABELS, SAG_TERMINAL_STATUSES, SAG_DISPOSITION_LABELS, SAG_LEG_STATUS_LABELS,
		SAG_LOCATION_KIND_LABELS, SAG_REASON_LABELS
	} from '$lib/rideMeta';
	import RideTierGlyph from '../RideTierGlyph.svelte';
	import SagRequestComposer from './SagRequestComposer.svelte';
	import ReasonDialog from '../ReasonDialog.svelte';

	let netId = $derived($activeNetId);

	let showAll = $state(false);
	let expandedId = $state<string | null>(null);
	let composerOpen = $state(false);
	let composerEditing = $state<SAGRequest | null>(null);
	let composerReason = $state('');

	// Command palette handoff (NetControlPanel's `case 'sag':`).
	$effect(() => {
		const seed = $sagComposerSeed;
		if (seed) {
			composerEditing = null;
			composerReason = seed.reason ?? '';
			composerOpen = true;
			sagComposerSeed.set(null);
		}
	});

	function openCreate(): void {
		composerEditing = null;
		composerReason = '';
		composerOpen = true;
	}

	function openEdit(r: SAGRequest): void {
		composerEditing = r;
		composerOpen = true;
	}

	let requests = $derived(
		($sagBoard?.requests ?? [])
			.filter((r) => showAll || !SAG_TERMINAL_STATUSES.has(r.status))
			.sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
	);
	let vehicles = $derived($sagBoard?.vehicles ?? []);
	/**
	 * The dispatch picker's actual candidate list. Kept as one derived value
	 * (rather than re-filtering inline in three places) so the empty-state
	 * message and the `<select>`'s options can never drift apart — that drift
	 * is exactly what left the picker rendering a dead dropdown with no
	 * explanation when every checked-in SAG unit had checked back out.
	 */
	let availableVehicles = $derived(vehicles.filter((v) => v.checkInStatus !== 'released'));

	function pickupText(loc: SAGLocation): string {
		if (loc.mileMarker != null) return `mi ${loc.mileMarker.toFixed(1)}`;
		if (loc.milesRemaining != null) return `${loc.milesRemaining.toFixed(1)} to go`;
		// Never fall through to the raw enum: a row was reading "reststop".
		return loc.description || loc.route || enumLabel(loc.kind, SAG_LOCATION_KIND_LABELS);
	}

	function riderCountText(r: SAGRequest): string {
		const waiting = r.slots.filter((s) => s.disposition === 'waiting').length;
		return waiting > 0 ? `${r.slots.length} (${waiting} waiting)` : `${r.slots.length}`;
	}

	// Both reasons below are written to the timeline + ICS-214 and used to be
	// collected with window.prompt(): unstyled, unvalidated, no pending state,
	// and suppressed outright in some embedded webviews.
	let cancelTarget = $state<SAGRequest | null>(null);
	let releaseTarget = $state<{ r: SAGRequest; leg: SAGLeg } | null>(null);

	async function submitCancelRequest(reason: string): Promise<void> {
		const r = cancelTarget;
		if (!r) return;
		try {
			upsertSagRequest(await api.cancelSagRequest(netId, r.id, reason));
			cancelTarget = null;
			showToast(`SAG ${r.sequence} cancelled`, 'success');
		} catch (e) {
			throw new Error(e instanceof ApiError ? e.message : 'Cancel failed');
		}
	}

	async function submitReleaseLeg(reason: string): Promise<void> {
		const t = releaseTarget;
		if (!t) return;
		try {
			upsertSagRequest(await api.releaseSagLeg(netId, t.r.id, t.leg.id, reason));
			releaseTarget = null;
			showToast(`${t.leg.vehicleLabel} released`, 'success');
		} catch (e) {
			throw new Error(e instanceof ApiError ? e.message : 'Could not release vehicle');
		}
	}

	/**
	 * One in-flight action at a time, named so each button can show its own
	 * pending state. Before this, Add rider / Mark en route / Mark on scene /
	 * Confirm load / Confirm delivery / Resolve fired with NO feedback at all:
	 * on a slow link the button just sat there and an operator double-tapped
	 * it, dispatching the same leg twice. Matches the course/* convention
	 * (aria-busy + a gerund label).
	 */
	let busy = $state<string | null>(null);

	async function run(key: string, fn: () => Promise<void>): Promise<void> {
		if (busy) return;
		busy = key;
		try {
			await fn();
		} finally {
			busy = null;
		}
	}

	// --- add rider ---

	let addRiderOpenFor = $state<string | null>(null);
	let newBib = $state('');
	let newName = $state('');
	let newNote = $state('');
	let newHasBike = $state(true);

	function openAddRider(id: string): void {
		addRiderOpenFor = id;
		newBib = '';
		newName = '';
		newNote = '';
		newHasBike = true;
	}

	async function submitAddRider(r: SAGRequest): Promise<void> {
		try {
			const updated = await api.addSagSlot(netId, r.id, { bib: newBib.trim(), riderName: newName.trim(), note: newNote.trim(), hasBike: newHasBike });
			upsertSagRequest(updated);
			addRiderOpenFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not add rider', 'error');
		}
	}

	// --- slot resolution ---

	function legalResolutions(disposition: string): string[] {
		if (disposition === 'waiting') return ['self_resolved', 'declined', 'not_found', 'handed_off', 'cancelled'];
		if (disposition === 'loaded') return ['handed_off'];
		return [];
	}

	async function resolveSlot(r: SAGRequest, slotId: string, disposition: string): Promise<void> {
		if (!disposition) return;
		try {
			upsertSagRequest(await api.resolveSagSlot(netId, r.id, slotId, { disposition }));
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not resolve rider', 'error');
		}
	}

	// --- dispatch a new leg ---

	let dispatchOpenFor = $state<string | null>(null);
	let dispatchVehicleId = $state('');
	let dispatchSlotIds = $state<Set<string>>(new Set());
	let dispatchOvercapacity = $state<OverCapacityBody | null>(null);
	let dispatching = $state(false);

	function openDispatch(r: SAGRequest): void {
		dispatchOpenFor = r.id;
		dispatchOvercapacity = null;
		dispatchVehicleId = availableVehicles[0]?.checkInId ?? '';
		dispatchSlotIds = new Set(r.slots.filter((s) => s.disposition === 'waiting' && !s.legId).map((s) => s.id));
	}

	function toggleDispatchSlot(id: string): void {
		const next = new Set(dispatchSlotIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		dispatchSlotIds = next;
	}

	async function submitDispatch(r: SAGRequest, allowOvercommit = false): Promise<void> {
		if (!dispatchVehicleId || dispatchSlotIds.size === 0) return;
		dispatching = true;
		try {
			const updated = await api.dispatchSagLeg(netId, r.id, {
				vehicleCheckInId: dispatchVehicleId,
				slotIds: Array.from(dispatchSlotIds),
				allowOvercommit
			});
			upsertSagRequest(updated);
			dispatchOpenFor = null;
			dispatchOvercapacity = null;
		} catch (e) {
			if (e instanceof ApiError && e.status === 409) {
				dispatchOvercapacity = e.body as unknown as OverCapacityBody;
			} else {
				showToast(e instanceof ApiError ? e.message : 'Dispatch failed', 'error');
			}
		} finally {
			dispatching = false;
		}
	}

	// --- leg lifecycle ---

	async function advanceLeg(r: SAGRequest, leg: SAGLeg, status: 'enroute' | 'onscene'): Promise<void> {
		try {
			upsertSagRequest(await api.advanceSagLeg(netId, r.id, leg.id, status));
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not update leg', 'error');
		}
	}

	let loadOpenFor = $state<string | null>(null);
	let loadSlotIds = $state<Set<string>>(new Set());

	function openLoad(leg: SAGLeg): void {
		loadOpenFor = leg.id;
		loadSlotIds = new Set(leg.slotIds);
	}

	function toggleLoadSlot(id: string): void {
		const next = new Set(loadSlotIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		loadSlotIds = next;
	}

	async function submitLoad(r: SAGRequest, leg: SAGLeg): Promise<void> {
		try {
			upsertSagRequest(await api.loadSagSlots(netId, r.id, leg.id, Array.from(loadSlotIds)));
			loadOpenFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not record load', 'error');
		}
	}

	let deliverOpenFor = $state<string | null>(null);
	let deliverSlotIds = $state<Set<string>>(new Set());
	let deliverDifferentDest = $state(false);
	let deliverDest = $state<SAGLocation>({ kind: 'next_reststop' });

	function openDeliver(r: SAGRequest, leg: SAGLeg): void {
		deliverOpenFor = leg.id;
		deliverSlotIds = new Set(leg.slotIds.filter((sid) => r.slots.find((s) => s.id === sid)?.disposition === 'loaded'));
		deliverDifferentDest = false;
		deliverDest = { ...r.dropoff };
	}

	function toggleDeliverSlot(id: string): void {
		const next = new Set(deliverSlotIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		deliverSlotIds = next;
	}

	async function submitDeliver(r: SAGRequest, leg: SAGLeg): Promise<void> {
		try {
			const updated = await api.deliverSagSlots(netId, r.id, leg.id, {
				slotIds: Array.from(deliverSlotIds),
				destination: deliverDifferentDest ? deliverDest : undefined
			});
			upsertSagRequest(updated);
			deliverOpenFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not record delivery', 'error');
		}
	}

	// --- vehicle capacity edit ---

	let vehicleEditFor = $state<string | null>(null);
	let vehicleSeats = $state(0);
	let vehicleRacks = $state(0);

	function openVehicleEdit(v: SAGVehicleStatus): void {
		vehicleEditFor = v.checkInId;
		vehicleSeats = v.seats;
		vehicleRacks = v.rackSlots;
	}

	async function submitVehicle(checkInId: string): Promise<void> {
		try {
			const status = await api.setSagVehicle(netId, checkInId, { seats: vehicleSeats, rackSlots: vehicleRacks });
			upsertSagVehicle(status);
			vehicleEditFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not update vehicle', 'error');
		}
	}

	function slotLabel(r: SAGRequest, slotId: string): string {
		const s = r.slots.find((x) => x.id === slotId);
		if (!s) return slotId.slice(0, 6);
		return s.bib ? `bib ${s.bib}` : s.riderName || 'rider';
	}
</script>

<div class="sb">
	<div class="sb-toolbar">
		<button class="sb-new" onclick={openCreate} disabled={!$canOperate}>+ New SAG request</button>
		<label class="sb-showall">
			<input type="checkbox" bind:checked={showAll} /> Show complete/cancelled
		</label>
	</div>

	{#if vehicles.length > 0}
		<div class="sb-vehicles">
			{#each vehicles as v (v.checkInId)}
				<div class="sb-vehicle" class:released={v.checkInStatus === 'released'}>
					<span class="sb-vehicle-label">{v.tacticalCall || v.callsign}</span>
					{#if vehicleEditFor === v.checkInId}
						<input type="number" min="0" class="sb-vehicle-input" bind:value={vehicleSeats} aria-label="Seats" />
						<span class="sb-vehicle-sep">seats</span>
						<input type="number" min="0" class="sb-vehicle-input" bind:value={vehicleRacks} aria-label="Rack slots" />
						<span class="sb-vehicle-sep">racks</span>
						<button class="sb-vehicle-save" onclick={() => submitVehicle(v.checkInId)}>Save</button>
						<button class="sb-vehicle-cancel" onclick={() => (vehicleEditFor = null)}>Cancel</button>
					{:else}
						<span class="sb-vehicle-cap" class:warn={v.availableSeats < 0}>seats {v.availableSeats}/{v.seats}</span>
						<span class="sb-vehicle-cap" class:warn={v.availableRacks < 0}>racks {v.availableRacks}/{v.rackSlots}</span>
						{#if $canOperate}<button class="sb-vehicle-edit" onclick={() => openVehicleEdit(v)}>Edit</button>{/if}
					{/if}
				</div>
			{/each}
		</div>
	{:else}
		<p class="sb-no-vehicles">No SAG vehicles checked in — check in a unit with category "sag" to dispatch.</p>
	{/if}

	{#if requests.length === 0}
		<p class="sb-empty">No {showAll ? '' : 'open '}SAG requests.</p>
	{/if}

	<div class="sb-list">
		{#each requests as r (r.id)}
			{@const tier = tierById($rideLadder, r.priority)}
			{@const expanded = expandedId === r.id}
			<div class="sb-card" class:expanded>
				<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
				<div class="sb-row" onclick={() => (expandedId = expanded ? null : r.id)} role="button" tabindex="0" onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); expandedId = expanded ? null : r.id; } }}>
					<span class="sb-seq">SAG {r.sequence}</span>
					{#if tier}
						<span class="sb-tier" title={tier.label}>
							<RideTierGlyph {tier} size={16} title={tier.label} />
							<span class="sb-tier-code">{tierStyle(tier).code}</span>
						</span>
					{/if}
					<span class="sb-status" data-status={r.status}>{SAG_STATUS_LABELS[r.status] ?? r.status}</span>
					<span class="sb-riders">{riderCountText(r)} rider{r.slots.length === 1 ? '' : 's'}</span>
					<span class="sb-age" title={new Date(r.createdAt).toLocaleString()}>{ageText(r.createdAt, $secondClock)} old</span>
					<span class="sb-pickup">{pickupText(r.pickup)}</span>
					<span class="sb-chevron" aria-hidden="true">{expanded ? '▾' : '▸'}</span>
				</div>

				{#if expanded}
					<div class="sb-detail">
						<div class="sb-detail-grid">
							<div><span class="sb-detail-label">Pickup</span><span>{pickupText(r.pickup)}{r.pickup.description ? ` — ${r.pickup.description}` : ''}</span></div>
							<div><span class="sb-detail-label">Dropoff</span><span>{r.dropoff.description || enumLabel(r.dropoff.kind, SAG_LOCATION_KIND_LABELS)}</span></div>
							<div><span class="sb-detail-label">Reason</span><span>{enumLabel(r.reason, SAG_REASON_LABELS) || '—'}</span></div>
							<div><span class="sb-detail-label">Requested by</span><span>{r.requestedBy || '—'}</span></div>
						</div>
						{#if r.notes}<p class="sb-notes">{r.notes}</p>{/if}
						{#if r.cancelReason}<p class="sb-cancel-reason">Cancelled: {r.cancelReason}</p>{/if}

						<div class="sb-actions-row">
							{#if $canOperate && !SAG_TERMINAL_STATUSES.has(r.status)}
								<button class="sb-action" onclick={() => openEdit(r)}>Edit</button>
								<button class="sb-action sb-action-danger" onclick={() => (cancelTarget = r)}>Cancel request</button>
							{/if}
						</div>

						<h4 class="sb-section-h">Riders</h4>
						<div class="sb-slots">
							{#each r.slots as s (s.id)}
								<div class="sb-slot">
									<span class="sb-slot-label">{s.bib ? `Bib ${s.bib}` : s.riderName || 'Rider'}{s.note ? ` — ${s.note}` : ''}{s.hasBike ? '' : ' (no bike)'}</span>
									<span class="sb-slot-disp" data-disp={s.disposition}>{SAG_DISPOSITION_LABELS[s.disposition] ?? s.disposition}</span>
									{#if $canOperate && legalResolutions(s.disposition).length > 0}
										<select class="sb-slot-resolve" onchange={(e) => { resolveSlot(r, s.id, (e.target as HTMLSelectElement).value); (e.target as HTMLSelectElement).value = ''; }}>
											<option value="">Resolve…</option>
											{#each legalResolutions(s.disposition) as d (d)}
												<option value={d}>{SAG_DISPOSITION_LABELS[d]}</option>
											{/each}
										</select>
									{/if}
								</div>
							{/each}
							{#if $canOperate && r.status !== 'cancelled'}
								{#if addRiderOpenFor === r.id}
									<div class="sb-add-rider">
										<input type="text" placeholder="bib" bind:value={newBib} aria-label="New rider bib" />
										<input type="text" placeholder="name" bind:value={newName} aria-label="New rider name" />
										<input type="text" placeholder="note" bind:value={newNote} aria-label="New rider note" />
										<label class="sb-bike"><input type="checkbox" bind:checked={newHasBike} /> bike</label>
										<button class="sb-action" disabled={busy !== null} aria-busy={busy === 'add-rider'} onclick={() => run('add-rider', () => submitAddRider(r))}>{busy === 'add-rider' ? 'Adding…' : 'Add'}</button>
										<button class="sb-action" disabled={busy !== null} onclick={() => (addRiderOpenFor = null)}>Cancel</button>
									</div>
								{:else}
									<button class="sb-add-rider-btn" onclick={() => openAddRider(r.id)}>+ Add rider</button>
								{/if}
							{/if}
						</div>

						<h4 class="sb-section-h">Legs</h4>
						{#if r.legs.length === 0}
							<p class="sb-no-legs">No vehicle dispatched yet.</p>
						{/if}
						{#each r.legs as leg (leg.id)}
							<div class="sb-leg">
								<div class="sb-leg-head">
									<span class="sb-leg-vehicle">{leg.vehicleLabel}</span>
									<span class="sb-leg-status" data-legstatus={leg.status}>{SAG_LEG_STATUS_LABELS[leg.status] ?? leg.status}</span>
									{#if leg.overcommitted}<span class="sb-leg-over">OVER CAPACITY</span>{/if}
									<span class="sb-leg-slots">{leg.slotIds.map((sid) => slotLabel(r, sid)).join(', ')}</span>
								</div>
								{#if $canOperate}
									<div class="sb-leg-actions">
										{#if leg.status === 'dispatched'}
											<button class="sb-action" disabled={busy !== null} aria-busy={busy === `enroute-${leg.id}`} onclick={() => run(`enroute-${leg.id}`, () => advanceLeg(r, leg, 'enroute'))}>{busy === `enroute-${leg.id}` ? 'Saving…' : 'Mark en route'}</button>
											<button class="sb-action" disabled={busy !== null} aria-busy={busy === `onscene-${leg.id}`} onclick={() => run(`onscene-${leg.id}`, () => advanceLeg(r, leg, 'onscene'))}>{busy === `onscene-${leg.id}` ? 'Saving…' : 'Mark on scene'}</button>
										{:else if leg.status === 'enroute'}
											<button class="sb-action" disabled={busy !== null} aria-busy={busy === `onscene-${leg.id}`} onclick={() => run(`onscene-${leg.id}`, () => advanceLeg(r, leg, 'onscene'))}>{busy === `onscene-${leg.id}` ? 'Saving…' : 'Mark on scene'}</button>
										{/if}
										{#if leg.status === 'dispatched' || leg.status === 'enroute' || leg.status === 'onscene'}
											<button class="sb-action" disabled={busy !== null} onclick={() => openLoad(leg)}>Load…</button>
											<button class="sb-action sb-action-danger" disabled={busy !== null} onclick={() => (releaseTarget = { r, leg })}>Release</button>
										{:else if leg.status === 'loaded'}
											<button class="sb-action" disabled={busy !== null} onclick={() => openDeliver(r, leg)}>Deliver…</button>
										{/if}
									</div>
								{/if}

								{#if loadOpenFor === leg.id}
									<div class="sb-inline-form">
										<span class="sb-inline-label">Who was actually loaded?</span>
										{#each leg.slotIds as sid (sid)}
											<label class="sb-checkline"><input type="checkbox" checked={loadSlotIds.has(sid)} onchange={() => toggleLoadSlot(sid)} /> {slotLabel(r, sid)}</label>
										{/each}
										<div class="sb-inline-actions">
											<button class="sb-action" disabled={busy !== null} aria-busy={busy === 'load'} onclick={() => run('load', () => submitLoad(r, leg))}>{busy === 'load' ? 'Recording…' : 'Confirm load'}</button>
											<button class="sb-action" disabled={busy !== null} onclick={() => (loadOpenFor = null)}>Cancel</button>
										</div>
									</div>
								{/if}

								{#if deliverOpenFor === leg.id}
									<div class="sb-inline-form">
										<span class="sb-inline-label">Deliver which riders?</span>
										{#each leg.slotIds.filter((sid) => r.slots.find((s) => s.id === sid)?.disposition === 'loaded') as sid (sid)}
											<label class="sb-checkline"><input type="checkbox" checked={deliverSlotIds.has(sid)} onchange={() => toggleDeliverSlot(sid)} /> {slotLabel(r, sid)}</label>
										{/each}
										<label class="sb-checkline"><input type="checkbox" bind:checked={deliverDifferentDest} /> Different destination than planned ({r.dropoff.description || enumLabel(r.dropoff.kind, SAG_LOCATION_KIND_LABELS)})</label>
										{#if deliverDifferentDest}
											<select bind:value={deliverDest.kind} class="sb-dest-select">
												<option value="next_reststop">Next rest stop</option>
												<option value="reststop">Rest stop</option>
												<option value="finish">Finish</option>
												<option value="start">Start</option>
												<option value="hospital">Hospital</option>
												<option value="other">Other</option>
											</select>
										{/if}
										<div class="sb-inline-actions">
											<button class="sb-action" onclick={() => run('deliver', () => submitDeliver(r, leg))} disabled={deliverSlotIds.size === 0 || busy !== null} aria-busy={busy === 'deliver'}>{busy === 'deliver' ? 'Recording…' : 'Confirm delivery'}</button>
											<button class="sb-action" disabled={busy !== null} onclick={() => (deliverOpenFor = null)}>Cancel</button>
										</div>
									</div>
								{/if}
							</div>
						{/each}

						{#if $canOperate && r.needsVehicle}
							{#if dispatchOpenFor === r.id}
								<div class="sb-inline-form sb-dispatch">
									{#if availableVehicles.length === 0}
										<!-- Two distinct causes, two distinct remedies (never
										     collapsed into one message) — see sb-no-vehicles
										     above for the roster-empty case; this is the same
										     wording so the two surfaces agree. -->
										<p class="sb-dispatch-empty">
											{#if vehicles.length === 0}
												No SAG vehicles on the roster. Set a station's category to SAG in the roster to dispatch it.
											{:else}
												All SAG vehicles have checked out.
											{/if}
										</p>
										<div class="sb-inline-actions">
											<button class="sb-action" onclick={() => (dispatchOpenFor = null)}>Close</button>
										</div>
									{:else}
										<span class="sb-inline-label">Dispatch which vehicle?</span>
										<select bind:value={dispatchVehicleId} class="sb-dest-select">
											<option value="">Choose a vehicle…</option>
											{#each availableVehicles as v (v.checkInId)}
												<option value={v.checkInId}>{v.tacticalCall || v.callsign} — {v.availableSeats}/{v.seats} seats, {v.availableRacks}/{v.rackSlots} racks</option>
											{/each}
										</select>
										<span class="sb-inline-label">Which riders?</span>
										{#each r.slots.filter((s) => s.disposition === 'waiting' && !s.legId) as s (s.id)}
											<label class="sb-checkline"><input type="checkbox" checked={dispatchSlotIds.has(s.id)} onchange={() => toggleDispatchSlot(s.id)} /> {s.bib ? `Bib ${s.bib}` : s.riderName || 'Rider'}</label>
										{/each}
										{#if dispatchOvercapacity}
											<p class="sb-overcap">
												{dispatchOvercapacity.error}: {dispatchOvercapacity.committedSeats}/{dispatchOvercapacity.seats} seats and {dispatchOvercapacity.committedRacks}/{dispatchOvercapacity.rackSlots} racks already committed.
											</p>
											<div class="sb-inline-actions">
												<button class="sb-action sb-action-danger" disabled={dispatching} aria-busy={dispatching} onclick={() => submitDispatch(r, true)}>{dispatching ? 'Dispatching…' : 'Dispatch anyway (over capacity)'}</button>
												<button class="sb-action" onclick={() => (dispatchOpenFor = null)}>Cancel</button>
											</div>
										{:else}
											<div class="sb-inline-actions">
												<button class="sb-action" disabled={dispatching || !dispatchVehicleId || dispatchSlotIds.size === 0} aria-busy={dispatching} onclick={() => submitDispatch(r, false)}>{dispatching ? 'Dispatching…' : 'Dispatch'}</button>
												<button class="sb-action" onclick={() => (dispatchOpenFor = null)}>Cancel</button>
											</div>
										{/if}
									{/if}
								</div>
							{:else}
								<button class="sb-dispatch-btn" onclick={() => openDispatch(r)}>+ Dispatch vehicle</button>
							{/if}
						{/if}
					</div>
				{/if}
			</div>
		{/each}
	</div>
</div>

{#if composerOpen}
	<SagRequestComposer {netId} editing={composerEditing} initialReason={composerReason} onClose={() => (composerOpen = false)} />
{/if}

{#if cancelTarget}
	<ReasonDialog
		title="Cancel SAG {cancelTarget.sequence}"
		confirmLabel="Cancel request"
		danger
		onConfirm={submitCancelRequest}
		onCancel={() => (cancelTarget = null)}
	/>
{/if}

{#if releaseTarget}
	<ReasonDialog
		title="Release {releaseTarget.leg.vehicleLabel}"
		confirmLabel="Release"
		required={false}
		onConfirm={submitReleaseLeg}
		onCancel={() => (releaseTarget = null)}
	/>
{/if}

<style>
	.sb {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		padding: var(--space-md);
	}

	.sb-toolbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
	}

	.sb-new {
		min-height: 40px;
		padding: 0 var(--space-md);
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-on-accent);
		font-weight: 700;
		cursor: pointer;
	}

	.sb-new:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.sb-showall {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.78rem;
		color: var(--color-text-muted);
	}

	.sb-vehicles {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-sm);
		padding: var(--space-sm);
		background: var(--color-bg);
		border-radius: var(--radius-sm);
	}

	.sb-vehicle {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.78rem;
		padding: 4px 8px;
		border-radius: var(--radius-sm);
		background: var(--color-surface);
	}

	.sb-vehicle.released {
		opacity: 0.5;
	}

	.sb-vehicle-label {
		font-weight: 700;
		color: var(--color-text);
	}

	.sb-vehicle-cap {
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
	}

	.sb-vehicle-cap.warn {
		color: var(--color-warning);
	}

	.sb-vehicle-input {
		width: 40px;
		min-height: 28px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		text-align: center;
	}

	.sb-vehicle-sep {
		font-size: 0.68rem;
		color: var(--color-text-muted);
	}

	.sb-vehicle-edit,
	.sb-vehicle-save,
	.sb-vehicle-cancel {
		min-height: 28px;
		padding: 0 8px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.7rem;
		cursor: pointer;
	}

	.sb-no-vehicles,
	.sb-dispatch-empty,
	.sb-empty,
	.sb-no-legs {
		color: var(--color-warning);
		font-size: 0.82rem;
		padding: var(--space-sm);
		background: var(--color-ride-priority-soft);
		border-radius: var(--radius-sm);
	}

	.sb-empty,
	.sb-no-legs {
		color: var(--color-text-muted);
		background: none;
		padding: var(--space-sm) 0;
	}

	.sb-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.sb-card {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		overflow: hidden;
	}

	.sb-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		min-height: 44px;
		padding: 6px var(--space-sm);
		cursor: pointer;
	}

	.sb-row:hover,
	.sb-row:focus-visible {
		background: var(--color-raised);
	}

	.sb-seq {
		font-weight: 700;
		font-size: 0.82rem;
		flex-shrink: 0;
	}

	.sb-status {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.sb-status[data-status='complete'] {
		color: var(--color-success);
	}

	.sb-status[data-status='cancelled'] {
		color: var(--color-text-muted);
		text-decoration: line-through;
	}

	.sb-riders,
	.sb-age,
	.sb-pickup {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.sb-pickup {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		text-align: right;
	}

	.sb-chevron {
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.sb-detail {
		padding: var(--space-sm) var(--space-md) var(--space-md);
		border-top: 1px solid var(--color-primary);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.sb-detail-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-sm);
		font-size: 0.8rem;
	}

	.sb-detail-grid > div {
		display: flex;
		flex-direction: column;
		gap: var(--space-2xs);
	}

	.sb-detail-label {
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
	}

	.sb-notes {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	.sb-tier {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2xs);
		flex-shrink: 0;
	}

	/* The code is the non-colour channel: PRIORITY and HIGH share amber and
	   the same triangle family, so the glyph alone cannot separate them. */
	.sb-tier-code {
		font-size: 0.625rem;
		font-weight: 800;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
	}

	.sb-cancel-reason {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.sb-actions-row {
		display: flex;
		gap: var(--space-sm);
	}

	.sb-action {
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
	}

	.sb-action:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.sb-action-danger {
		color: var(--color-error-text);
		border-color: var(--color-error);
	}

	.sb-section-h {
		font-size: 0.68rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
		margin: var(--space-xs) 0 0;
	}

	.sb-slots {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.sb-slot {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		font-size: 0.78rem;
		padding: 4px 0;
	}

	.sb-slot-label {
		flex: 1;
		min-width: 0;
	}

	.sb-slot-disp {
		font-size: 0.68rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.sb-slot-disp[data-disp='delivered'],
	.sb-slot-disp[data-disp='self_resolved'] {
		color: var(--color-success);
	}

	.sb-slot-resolve {
		min-height: 30px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.72rem;
	}

	.sb-add-rider,
	.sb-add-rider-btn {
		display: flex;
		gap: 6px;
		align-items: center;
	}

	.sb-add-rider input[type='text'] {
		min-height: 32px;
		padding: 0 8px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		width: 90px;
	}

	.sb-bike {
		display: flex;
		align-items: center;
		gap: var(--space-2xs);
		font-size: 0.7rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.sb-add-rider-btn {
		align-self: flex-start;
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: 0.78rem;
		cursor: pointer;
		min-height: 32px;
	}

	.sb-leg {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.sb-leg-head {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		flex-wrap: wrap;
		font-size: 0.78rem;
	}

	.sb-leg-vehicle {
		font-weight: 700;
	}

	.sb-leg-status {
		font-size: 0.68rem;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	.sb-leg-status[data-legstatus='delivered'] {
		color: var(--color-success);
	}

	.sb-leg-over {
		color: var(--color-error-text);
		font-size: 0.68rem;
		font-weight: 700;
	}

	.sb-leg-slots {
		flex: 1;
		min-width: 0;
		color: var(--color-text-muted);
		font-size: 0.72rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sb-leg-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.sb-inline-form {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: var(--space-sm);
		background: var(--color-bg);
		border-radius: var(--radius-sm);
	}

	.sb-inline-label {
		font-size: 0.68rem;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	.sb-checkline {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.78rem;
	}

	.sb-dest-select {
		min-height: 34px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
	}

	.sb-inline-actions {
		display: flex;
		gap: 6px;
		margin-top: 4px;
	}

	.sb-overcap {
		font-size: 0.78rem;
		color: var(--color-warning);
	}

	.sb-dispatch-btn {
		align-self: flex-start;
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-ride-priority-soft);
		border: 1px solid var(--color-warning);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.78rem;
		font-weight: 600;
		cursor: pointer;
	}
</style>
