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
		sagBoard, rideLadder, rideSagSummary, sagComposerSeed, upsertSagRequest, upsertSagVehicle
	} from '$lib/stores/ride';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { canOperate } from '$lib/stores/session';
	import { secondClock } from '$lib/stores/clock';
	import { showToast } from '$lib/stores/toast';
	import {
		tierById, tierStyle, ageText, enumLabel,
		SAG_STATUS_LABELS, SAG_TERMINAL_STATUSES, SAG_DISPOSITION_LABELS, SAG_LEG_STATUS_LABELS,
		SAG_LOCATION_KIND_LABELS, SAG_REASON_LABELS,
		SAG_BIKE_LABELS, SAG_BIKE_SHORT, SAG_BIKE_GLYPHS,
		slotBike, unassignedRiders, activeUnloadedLegs, BIKE_ORDER
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

	/**
	 * Order is "what is blocked on ME, worst first", not "newest first".
	 * Newest-first buries the request that has had nobody coming for it for
	 * forty minutes under the one logged ten seconds ago, which is exactly
	 * backwards: a job a van is already driving needs nothing from NCS.
	 * 1. needs a vehicle  2. priority rank  3. oldest first  4. everything else.
	 */
	function sortKey(r: SAGRequest): [number, number, number] {
		const rank = tierById($rideLadder, r.priority)?.rank ?? 99;
		const blocked = r.needsVehicle && !SAG_TERMINAL_STATUSES.has(r.status) ? 0 : 1;
		return [blocked, rank, Date.parse(r.createdAt)];
	}

	let requests = $derived(
		($sagBoard?.requests ?? [])
			.filter((r) => showAll || !SAG_TERMINAL_STATUSES.has(r.status))
			.sort((a, b) => {
				const ka = sortKey(a);
				const kb = sortKey(b);
				return ka[0] - kb[0] || ka[1] - kb[1] || ka[2] - kb[2];
			})
	);

	/** Requests blocked on NCS finding a vehicle — the board's whole headline. */
	let blockedRequests = $derived(
		($sagBoard?.requests ?? []).filter((r) => !SAG_TERMINAL_STATUSES.has(r.status) && r.needsVehicle)
	);
	let ridersRoadside = $derived(blockedRequests.reduce((n, r) => n + unassignedRiders(r), 0));
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

	/**
	 * The full one-line form, mile marker AND place name, with no repeat: the
	 * detail grid used to render "Rest Stop Maxwell Chapel — Rest Stop Maxwell
	 * Chapel" because it appended the description to a value that had already
	 * fallen through to the description.
	 */
	function locationText(loc: SAGLocation): string {
		const head = pickupText(loc);
		const desc = loc.description?.trim();
		return desc && desc !== head ? `${head} — ${desc}` : head;
	}

	/** Bibs are the on-air identifier. The board has to answer "where is 334?". */
	function bibText(r: SAGRequest): string {
		const labels = r.slots
			.filter((s) => s.disposition !== 'cancelled')
			.map((s) => s.bib || s.riderName || '?')
			.filter(Boolean);
		if (labels.length === 0) return '';
		if (labels.length <= 3) return labels.join(' · ');
		return `${labels.slice(0, 3).join(' · ')} +${labels.length - 3}`;
	}

	/** The vehicle(s) currently carrying or fetching this request. */
	function vehicleText(r: SAGRequest): string {
		const live = r.legs.filter((l) => l.status !== 'released' && l.status !== 'delivered');
		return Array.from(new Set(live.map((l) => l.vehicleLabel))).join(', ');
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
	let newBike = $state('with_rider');
	/** '' = leave unassigned; otherwise the leg the rider joins. */
	let newAttachLegId = $state('');
	let addRiderOvercapacity = $state<OverCapacityBody | null>(null);

	function openAddRider(r: SAGRequest): void {
		addRiderOpenFor = r.id;
		newBib = '';
		newName = '';
		newNote = '';
		newBike = 'with_rider';
		addRiderOvercapacity = null;
		// "SAG 4, make that TWO riders at Maxwell" is the common case, so the
		// van already on its way is preselected. Exactly one live leg means
		// there is nothing to choose; more than one and the operator picks.
		const live = activeUnloadedLegs(r.legs);
		newAttachLegId = live.length === 1 ? live[0].id : '';
	}

	async function submitAddRider(r: SAGRequest, allowOvercommit = false): Promise<void> {
		try {
			const updated = await api.addSagSlot(netId, r.id, {
				bib: newBib.trim(),
				riderName: newName.trim(),
				note: newNote.trim(),
				bike: newBike,
				attachToLegId: newAttachLegId || undefined,
				allowOvercommit
			});
			upsertSagRequest(updated);
			addRiderOpenFor = null;
			addRiderOvercapacity = null;
		} catch (e) {
			if (e instanceof ApiError && e.status === 409) {
				addRiderOvercapacity = e.body as unknown as OverCapacityBody;
			} else {
				showToast(e instanceof ApiError ? e.message : 'Could not add rider', 'error');
			}
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
	/**
	 * slotId -> bike disposition, as the DRIVER reports it at the scene. This
	 * is the only moment anybody knows whether the bike actually went on the
	 * rack: the request-time checkbox was the caller's guess. It seeds from
	 * whatever the request assumed, so the common case is still zero clicks.
	 */
	let loadBike = $state<Record<string, string>>({});

	function openLoad(r: SAGRequest, leg: SAGLeg): void {
		loadOpenFor = leg.id;
		loadSlotIds = new Set(leg.slotIds);
		const seed: Record<string, string> = {};
		for (const sid of leg.slotIds) {
			const slot = r.slots.find((x) => x.id === sid);
			if (slot) seed[sid] = slotBike(slot);
		}
		loadBike = seed;
	}

	function setLoadBike(sid: string, value: string): void {
		loadBike = { ...loadBike, [sid]: value };
	}

	function toggleLoadSlot(id: string): void {
		const next = new Set(loadSlotIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		loadSlotIds = next;
	}

	async function submitLoad(r: SAGRequest, leg: SAGLeg): Promise<void> {
		try {
			const ids = Array.from(loadSlotIds);
			const bike: Record<string, string> = {};
			for (const sid of ids) if (loadBike[sid]) bike[sid] = loadBike[sid];
			upsertSagRequest(await api.loadSagSlots(netId, r.id, leg.id, { slotIds: ids, bike }));
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

	/** Change a bike disposition after the fact — a wrong one is one select away. */
	async function setSlotBike(r: SAGRequest, slotId: string, bike: string): Promise<void> {
		const s = r.slots.find((x) => x.id === slotId);
		if (!s || !bike) return;
		try {
			upsertSagRequest(
				await api.updateSagSlot(netId, r.id, slotId, {
					bib: s.bib ?? '', riderName: s.riderName ?? '', note: s.note ?? '', bike
				})
			);
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not update the bike', 'error');
		}
	}


	/**
	 * A seat refusal STATES A LIMIT; a rack overflow ASKS A QUESTION. They are
	 * rendered by different blocks, in different colours, and only one of them
	 * has an "anyway" button — retrying a seat refusal with allowOvercommit is
	 * refused by the server again, so offering the button would be a lie.
	 */
	function seatRefusalText(b: OverCapacityBody, vehicleLabel: string): string {
		const would = b.committedSeats + b.needSeats;
		return `${vehicleLabel} has ${b.seats} seat${b.seats === 1 ? '' : 's'}; this would be ${would}.`;
	}

	function rackQuestionText(b: OverCapacityBody, vehicleLabel: string): string {
		const would = b.committedRacks + b.needRacks;
		return `${vehicleLabel} has ${b.rackSlots} rack${b.rackSlots === 1 ? '' : 's'}; this would be ${would}.`;
	}

	/** The label of the vehicle a refusal is about, for either flow. */
	function dispatchVehicleLabel(): string {
		const v = availableVehicles.find((x) => x.checkInId === dispatchVehicleId);
		return v ? v.tacticalCall || v.callsign : 'That vehicle';
	}

	function attachVehicleLabel(r: SAGRequest): string {
		return r.legs.find((l) => l.id === newAttachLegId)?.vehicleLabel ?? 'That vehicle';
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
						{#if v.checkInStatus === 'released'}
							<!-- A checked-out unit used to be told apart by opacity alone,
							     while still advertising its full seat count. -->
							<span class="sb-vehicle-out">CHECKED OUT</span>
						{:else}
							<span class="sb-vehicle-cap" class:warn={v.availableSeats <= 0}>
								{v.availableSeats} free of {v.seats} seats
							</span>
							<span class="sb-vehicle-cap" class:warn={v.availableRacks <= 0}>
								{v.availableRacks} of {v.rackSlots} racks
							</span>
							{#if v.availableSeats <= 0 || v.availableRacks <= 0}
								<span class="sb-vehicle-full">{v.availableSeats < 0 || v.availableRacks < 0 ? 'OVER' : 'FULL'}</span>
							{/if}
						{/if}
						{#if $canOperate && v.checkInStatus !== 'released'}<button class="sb-vehicle-edit" onclick={() => openVehicleEdit(v)}>Edit</button>{/if}
					{/if}
				</div>
			{/each}
		</div>
	{:else}
		<p class="sb-no-vehicles">No SAG vehicles checked in — check in a unit with category "sag" to dispatch.</p>
	{/if}

	<!-- The board's single most important sentence, and the one that used to be
	     missing entirely: how many people are standing at the roadside with
	     nobody coming for them. Word + count + glyph, never colour alone. -->
	{#if blockedRequests.length > 0}
		<p class="sb-blocked" role="status">
			⬒ {ridersRoadside} rider{ridersRoadside === 1 ? '' : 's'} waiting for a vehicle
			across {blockedRequests.length} request{blockedRequests.length === 1 ? '' : 's'}
			{#if $rideSagSummary.seatsAvail <= 0 && availableVehicles.length > 0}· no free seats{/if}
		</p>
	{/if}

	{#if requests.length === 0}
		<p class="sb-empty">
			{#if showAll}
				Nothing logged yet. A SAG request is a transport job — log one when a rider needs a lift.
			{:else}
				Nobody is waiting for a ride.{#if $rideSagSummary.inMotion > 0}
					{$rideSagSummary.inMotion} job{$rideSagSummary.inMotion === 1 ? '' : 's'} in motion.
				{/if}
			{/if}
		</p>
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
					{#if r.needsVehicle && !SAG_TERMINAL_STATUSES.has(r.status)}
						{@const n = unassignedRiders(r)}
						<!-- The one thing the board exists to answer. Word + glyph, never
						     colour alone, and it says the NUMBER of people standing there. -->
						<span class="sb-needs">⬒ NEEDS VEHICLE · {n} rider{n === 1 ? '' : 's'}</span>
					{:else if vehicleText(r)}
						<span class="sb-onveh">{vehicleText(r)}</span>
					{/if}
					{#if bibText(r)}<span class="sb-bibs" title="Bibs on this request">{bibText(r)}</span>{/if}
					<span class="sb-age" title={new Date(r.createdAt).toLocaleString()}>{ageText(r.createdAt, $secondClock)} old</span>
					<span class="sb-pickup">{pickupText(r.pickup)} → {pickupText(r.dropoff)}</span>
					<span class="sb-chevron" aria-hidden="true">{expanded ? '▾' : '▸'}</span>
				</div>

				{#if expanded}
					<div class="sb-detail">
						<div class="sb-detail-grid">
							<div><span class="sb-detail-label">Pickup</span><span>{locationText(r.pickup)}</span></div>
							<div><span class="sb-detail-label">Dropoff</span><span>{locationText(r.dropoff)}</span></div>
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
								{@const bike = slotBike(s)}
								<div class="sb-slot">
									<span class="sb-slot-label">{s.bib ? `Bib ${s.bib}` : s.riderName || 'Rider'}{s.note ? ` — ${s.note}` : ''}</span>
									<!-- The bike is its own axis: a rider can be transported
									     while the bike stays behind or rides another vehicle. -->
									{#if $canOperate && s.disposition !== 'cancelled'}
										<select
											class="sb-slot-bike"
											value={bike}
											aria-label="Bike for {s.bib ? `bib ${s.bib}` : 'this rider'}"
											onchange={(e) => setSlotBike(r, s.id, (e.target as HTMLSelectElement).value)}
										>
											{#each BIKE_ORDER as b (b)}
												<option value={b}>{SAG_BIKE_GLYPHS[b]} {SAG_BIKE_LABELS[b]}</option>
											{/each}
										</select>
									{:else}
										<span class="sb-slot-bike-ro">{SAG_BIKE_GLYPHS[bike]} {SAG_BIKE_SHORT[bike]}</span>
									{/if}
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
									{@const live = activeUnloadedLegs(r.legs)}
									<div class="sb-add-rider">
										<input type="text" placeholder="bib" bind:value={newBib} aria-label="New rider bib" />
										<input type="text" placeholder="name" bind:value={newName} aria-label="New rider name" />
										<input type="text" placeholder="note" bind:value={newNote} aria-label="New rider note" />
										<select class="sb-slot-bike" bind:value={newBike} aria-label="Bike for the new rider">
											{#each BIKE_ORDER as b (b)}
												<option value={b}>{SAG_BIKE_GLYPHS[b]} {SAG_BIKE_LABELS[b]}</option>
											{/each}
										</select>
										<button class="sb-action" disabled={busy !== null} aria-busy={busy === 'add-rider'} onclick={() => run('add-rider', () => submitAddRider(r))}>{busy === 'add-rider' ? 'Adding…' : 'Add'}</button>
										<button class="sb-action" disabled={busy !== null} onclick={() => (addRiderOpenFor = null)}>Cancel</button>
									</div>
									<!-- "SAG 4, make that TWO riders at Maxwell": the van is
									     already rolling, so the rider joins its leg and the seat
									     count moves. A van that has already LOADED has left, so
									     it is not offered and the rider stays unassigned. -->
									{#if live.length > 0}
										<div class="sb-attach">
											<span class="sb-attach-label">Put them on</span>
											<select bind:value={newAttachLegId} class="sb-dest-select" aria-label="Vehicle for the new rider">
												{#each live as l (l.id)}
													<option value={l.id}>{l.vehicleLabel} ({SAG_LEG_STATUS_LABELS[l.status] ?? l.status})</option>
												{/each}
												<option value="">Nobody yet — needs a vehicle</option>
											</select>
										</div>
									{:else}
										<p class="sb-attach-none">No vehicle is on its way to this request — the rider will show as waiting.</p>
									{/if}
									{#if addRiderOvercapacity?.seatsExceeded}
										<!-- A REFUSAL. States the limit. No override: a seatbelt
										     count is not a judgement call, and the server would
										     refuse the retry anyway. -->
										<p class="sb-refusal">
											<span class="sb-refusal-head">⊘ No seat</span>
											{seatRefusalText(addRiderOvercapacity, attachVehicleLabel(r))}
											Add them unassigned, or dispatch another vehicle.
										</p>
										<div class="sb-inline-actions">
											<button class="sb-action" disabled={busy !== null} aria-busy={busy === 'add-rider-unassigned'} onclick={() => run('add-rider-unassigned', async () => { newAttachLegId = ''; addRiderOvercapacity = null; await submitAddRider(r); })}>Add as waiting</button>
											<button class="sb-action" onclick={() => { addRiderOpenFor = null; addRiderOvercapacity = null; }}>Cancel</button>
										</div>
									{:else if addRiderOvercapacity}
										<!-- A QUESTION. The bike can go in the bed of a truck. -->
										<p class="sb-overcap">
											<span class="sb-overcap-head">⚠ Not enough racks</span>
											{rackQuestionText(addRiderOvercapacity, attachVehicleLabel(r))}
											Carry it anyway?
										</p>
										<div class="sb-inline-actions">
											<button class="sb-action" disabled={busy !== null} aria-busy={busy === 'add-rider-over'} onclick={() => run('add-rider-over', () => submitAddRider(r, true))}>Yes — bike rides in the bed</button>
											<button class="sb-action" onclick={() => { newAttachLegId = ''; addRiderOvercapacity = null; }}>Leave them waiting instead</button>
										</div>
									{/if}
								{:else}
									<button class="sb-add-rider-btn" onclick={() => openAddRider(r)}>+ Add rider</button>
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
									<!-- Overcommitted can only mean bikes: a seat overflow is
									     refused outright, so it can never reach a leg. -->
									{#if leg.overcommitted}<span class="sb-leg-over">BIKES OVER RACKS</span>{/if}
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
											<button class="sb-action" disabled={busy !== null} onclick={() => openLoad(r, leg)}>Load…</button>
											<button class="sb-action sb-action-danger" disabled={busy !== null} onclick={() => (releaseTarget = { r, leg })}>Release</button>
										{:else if leg.status === 'loaded'}
											<button class="sb-action" disabled={busy !== null} onclick={() => openDeliver(r, leg)}>Deliver…</button>
										{/if}
									</div>
								{/if}

								{#if loadOpenFor === leg.id}
									<div class="sb-inline-form">
										<span class="sb-inline-label">Who was actually loaded — and did the bike go?</span>
										{#each leg.slotIds as sid (sid)}
											<div class="sb-loadline">
												<label class="sb-checkline"><input type="checkbox" checked={loadSlotIds.has(sid)} onchange={() => toggleLoadSlot(sid)} /> {slotLabel(r, sid)}</label>
												<!-- Whether the bike is physically on the rack is known
												     HERE, by the driver, not by whoever called it in. -->
												<select
													class="sb-slot-bike"
													value={loadBike[sid] ?? 'with_rider'}
													disabled={!loadSlotIds.has(sid)}
													aria-label="Bike for {slotLabel(r, sid)}"
													onchange={(e) => setLoadBike(sid, (e.target as HTMLSelectElement).value)}
												>
													{#each BIKE_ORDER as b (b)}
														<option value={b}>{SAG_BIKE_GLYPHS[b]} {SAG_BIKE_LABELS[b]}</option>
													{/each}
												</select>
											</div>
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
												<option value={v.checkInId}>
													{v.tacticalCall || v.callsign} — {v.availableSeats} free of {v.seats} seats, {v.availableRacks} of {v.rackSlots} racks{v.availableSeats <= 0 ? ' — NO SEATS' : ''}
												</option>
											{/each}
										</select>
										<span class="sb-inline-label">Which riders?</span>
										{#each r.slots.filter((s) => s.disposition === 'waiting' && !s.legId) as s (s.id)}
											<label class="sb-checkline"><input type="checkbox" checked={dispatchSlotIds.has(s.id)} onchange={() => toggleDispatchSlot(s.id)} /> {s.bib ? `Bib ${s.bib}` : s.riderName || 'Rider'}</label>
										{/each}
										{#if dispatchOvercapacity?.seatsExceeded}
											<p class="sb-refusal">
												<span class="sb-refusal-head">⊘ No seat</span>
												{seatRefusalText(dispatchOvercapacity, dispatchVehicleLabel())}
												Send fewer riders, or pick another vehicle.
											</p>
											<div class="sb-inline-actions">
												<button class="sb-action" onclick={() => (dispatchOvercapacity = null)}>Change the selection</button>
												<button class="sb-action" onclick={() => (dispatchOpenFor = null)}>Cancel</button>
											</div>
										{:else if dispatchOvercapacity}
											<p class="sb-overcap">
												<span class="sb-overcap-head">⚠ Not enough racks</span>
												{rackQuestionText(dispatchOvercapacity, dispatchVehicleLabel())}
												Carry them anyway?
											</p>
											<div class="sb-inline-actions">
												<button class="sb-action" disabled={dispatching} aria-busy={dispatching} onclick={() => submitDispatch(r, true)}>{dispatching ? 'Dispatching…' : 'Yes — bikes ride in the bed'}</button>
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

	.sb-vehicle-out,
	.sb-vehicle-full {
		font-size: 0.64rem;
		font-weight: 700;
		letter-spacing: 0.06em;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		background: var(--color-ride-priority-soft);
		color: var(--color-warning);
	}

	.sb-vehicle-out {
		background: var(--color-raised);
		color: var(--color-text-muted);
	}

	.sb-blocked {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.85rem;
		font-weight: 700;
		color: var(--color-warning);
		padding: var(--space-sm);
		border-left: 3px solid var(--color-warning);
		background: var(--color-ride-priority-soft);
		border-radius: var(--radius-sm);
	}

	.sb-needs {
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		white-space: nowrap;
		padding: 2px 6px;
		border-radius: var(--radius-sm);
		background: var(--color-ride-priority-soft);
		color: var(--color-warning);
		flex-shrink: 0;
	}

	.sb-onveh {
		font-size: 0.72rem;
		font-weight: 700;
		color: var(--color-text);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.sb-bibs {
		font-size: 0.75rem;
		font-variant-numeric: tabular-nums;
		color: var(--color-text);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.sb-slot-bike,
	.sb-slot-bike-ro {
		font-size: 0.72rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.sb-slot-bike {
		min-height: 32px;
		max-width: 160px;
		padding: 0 4px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
	}

	.sb-slot-bike:disabled {
		opacity: 0.4;
	}

	.sb-loadline {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		flex-wrap: wrap;
	}

	.sb-attach {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		margin-top: var(--space-xs);
	}

	.sb-attach-label {
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
	}

	.sb-attach-none {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		margin-top: var(--space-xs);
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
		flex-wrap: wrap;
		gap: 4px var(--space-sm);
		min-height: 44px;
		padding: 6px var(--space-sm);
		cursor: pointer;
	}

	/* The where-to-where must never be the thing that gets ellipsed away: it
	   wraps to its own line in a narrow panel rather than shrinking to "m…". */
	.sb-row .sb-pickup {
		flex: 1 1 100%;
		text-align: left;
		order: 9;
	}

	@media (min-width: 1500px) {
		.sb-row .sb-pickup {
			flex: 1 1 auto;
			text-align: right;
			order: 0;
		}
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
		flex-wrap: wrap;
		gap: 4px var(--space-sm);
		font-size: 0.78rem;
		padding: 4px 0;
	}

	.sb-slot-label {
		flex: 1 1 auto;
		min-width: 0;
		/* "Bib 700" must never break across two lines: the bib is what is read
		   back on the air. */
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
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

	/* A QUESTION the operator may answer yes to: amber, the same tone as every
	   other "are you sure" on this board. */
	.sb-overcap {
		font-size: 0.78rem;
		color: var(--color-warning);
		padding: var(--space-sm);
		border-left: 3px solid var(--color-warning);
		background: var(--color-ride-priority-soft);
		border-radius: var(--radius-sm);
	}

	/* A REFUSAL, not a question — deliberately NOT the same component or the
	   same colour as .sb-overcap, so the two never read as interchangeable.
	   --color-error-text, never --color-error: the latter is 4.16:1 on
	   --color-surface and fails AA at body size (strip spec section 3). */
	.sb-refusal {
		font-size: 0.78rem;
		color: var(--color-error-text);
		padding: var(--space-sm);
		border: 1px solid var(--color-error-text);
		background: var(--color-ride-emergency-soft);
		border-radius: var(--radius-sm);
	}

	.sb-refusal-head,
	.sb-overcap-head {
		display: block;
		font-weight: 700;
		letter-spacing: 0.03em;
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
