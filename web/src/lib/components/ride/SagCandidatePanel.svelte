<script lang="ts">
	// Dispatch focus (docs/sag-map-spec.md §6, §8, §10) — the ranked candidate
	// list that replaces the dock's content while the operator is answering the
	// one question the whole feature exists for: which van do I send, and can
	// it carry them?
	//
	// Three rules this file must not break:
	//   1. NOTHING IS PRESELECTED. Ranking encodes distance, seats and
	//      staleness; it cannot encode "SAG 1's driver just radioed that he's
	//      stuck behind the parade". The list is ranked; the operator chooses.
	//   2. No distance renders without its direction word, and no crow-flies
	//      number renders without being labelled as one. A bare "1.8 mi" that
	//      silently means "backwards up a course full of oncoming riders" is
	//      the exact error this feature exists to prevent.
	//   3. NOT ONE LINE OF DISPATCH LOGIC IS REIMPLEMENTED. `Send` calls the
	//      same `api.dispatchSagLeg` with the same payload SagBoard builds, and
	//      the 409 seats/racks branches are that file's, verbatim.
	import { api, ApiError } from '$lib/api';
	import type { OverCapacityBody, SAGSlot } from '$lib/types';
	import { sagFocusedRequest, sagCandidates, sagVehicleGeo, sagRoute, sagAvgSpeedMph, focusSagRequest, clearSagFocus } from '$lib/stores/sagMap';
	import { rankCandidates, type RankResult, type SagCandidate } from '$lib/sagDispatch';
	import { rideLadder, navigateZone, upsertSagRequest } from '$lib/stores/ride';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { canOperate } from '$lib/stores/session';
	import { secondClock } from '$lib/stores/clock';
	import { showToast } from '$lib/stores/toast';
	import { tierById, tierStyle, ageText } from '$lib/rideMeta';
	import RideTierGlyph from '../RideTierGlyph.svelte';

	const METERS_PER_MILE = 1609.344;
	const RANK_CAPS = ['①', '②', '③'];

	let netId = $derived($activeNetId);
	let focused = $derived($sagFocusedRequest);
	let clock = $derived($secondClock);

	/**
	 * When the PICKUP itself cannot be placed, `sagCandidates` has nothing to
	 * rank against and returns null. The panel still works: the same ranking
	 * function is called with no pickup, which yields the same rows with no
	 * distances, and the header says why. Dispatch is never blocked by a
	 * missing coordinate — somebody is standing at the roadside either way.
	 */
	let result = $derived<RankResult | null>(
		$sagCandidates ??
			(focused
				? rankCandidates($sagVehicleGeo, {
						routeIndex: $sagRoute.index,
						pickup: null,
						needSeats: Math.max(1, focused.waiting),
						needRacks: focused.request.slots.filter(
							(s) => s.disposition === 'waiting' && !s.legId && s.bike === 'with_rider'
						).length,
						nowMs: clock,
						avgSpeedMph: $sagAvgSpeedMph
					})
				: null)
	);

	let rankedLive = $derived(result?.ranked ?? []);
	let excluded = $derived(result?.excluded ?? []);

	/**
	 * THE ROW ORDER IS FROZEN WHILE THE PANEL IS OPEN.
	 *
	 * Ranking recomputes on the shared second clock, so a van crossing the
	 * 20-minute staleness line re-sorts the list under a hand that is already
	 * moving toward `Send` — and the operator dispatches the vehicle that slid
	 * into the space the one they chose just left. Spec §12 rejected
	 * number-key shortcuts for exactly this reason; the mouse target moves too.
	 *
	 * The DATA in each row stays live (distances, ages and capacity all keep
	 * updating). Only the ORDER is held, and the operator is told when the
	 * ranking has moved and can take the new one deliberately.
	 */
	let lockedOrder = $state<string[]>([]);

	$effect(() => {
		const ids = rankedLive.map((c) => c.id);
		if (lockedOrder.length === 0 && ids.length > 0) lockedOrder = ids;
	});

	let ranked = $derived.by(() => {
		const live = rankedLive;
		if (lockedOrder.length === 0) return live;
		const pos = new Map(lockedOrder.map((id, i) => [id, i]));
		// A vehicle that appears after the list was locked (a late check-in)
		// goes to the BOTTOM rather than jumping into the ranked positions —
		// it has not been read yet, so it cannot have been aimed at.
		const held = live.filter((c) => pos.has(c.id)).sort((a, b) => pos.get(a.id)! - pos.get(b.id)!);
		const fresh = live.filter((c) => !pos.has(c.id));
		return [...held, ...fresh];
	});

	/** The live ranking now disagrees with what is on screen. */
	let orderMoved = $derived(
		rankedLive.length === ranked.length && rankedLive.some((c, i) => ranked[i]?.id !== c.id)
	);

	function resort(): void {
		lockedOrder = rankedLive.map((c) => c.id);
		highlight = 0;
	}
	let tier = $derived(focused ? (tierById($rideLadder, focused.request.priority) ?? null) : null);

	let waitingSlots = $derived<SAGSlot[]>(
		focused ? focused.request.slots.filter((s) => s.disposition === 'waiting' && !s.legId) : []
	);

	// --- rider selection ---------------------------------------------------
	// The default is byte-for-byte SagBoard.openDispatch()'s: every waiting,
	// unassigned slot. The disclosure exists only when there is something to
	// choose between.
	let selected = $state<Set<string>>(new Set());
	let chooserFor = $state<string | null>(null);
	let touched = $state(false);
	let lastSeededId = '';

	$effect(() => {
		const id = focused?.request.id ?? '';
		if (id === lastSeededId) return;
		lastSeededId = id;
		selected = new Set(waitingSlots.map((s) => s.id));
		touched = false;
		chooserFor = null;
		overcap = null;
		highlight = 0;
		lockedOrder = [];
	});

	function toggleSlot(id: string): void {
		const next = new Set(selected);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selected = next;
		touched = true;
	}

	/** The slots this row would actually send. Untouched, a partial-capacity
	 *  vehicle takes as many as it can seat; once the operator has picked, the
	 *  picks are honoured exactly — including a pick the server will refuse,
	 *  which it then explains. */
	function slotsFor(c: SagCandidate): string[] {
		const chosen = waitingSlots.filter((s) => selected.has(s.id)).map((s) => s.id);
		if (touched) return chosen;
		return chosen.slice(0, Math.max(1, c.seatsOffered));
	}

	function sendLabel(c: SagCandidate): string {
		const n = slotsFor(c).length;
		if (n === waitingSlots.length && c.eligibility === 'eligible') return 'Send';
		return `Send ${n} rider${n === 1 ? '' : 's'}`;
	}

	// --- dispatch (SagBoard.submitDispatch, unchanged) ---------------------
	let dispatching = $state<string | null>(null);
	let overcap = $state<{ id: string; body: OverCapacityBody } | null>(null);
	let announcement = $state('');

	async function send(c: SagCandidate, allowOvercommit = false): Promise<void> {
		if (!focused || dispatching) return;
		const slotIds = slotsFor(c);
		if (slotIds.length === 0) return;
		dispatching = c.id;
		try {
			const updated = await api.dispatchSagLeg(netId, focused.request.id, {
				vehicleCheckInId: c.id,
				slotIds,
				allowOvercommit
			});
			upsertSagRequest(updated);
			overcap = null;
			const label = vehicleLabel(c);
			showToast(`${label} dispatched to SAG ${focused.request.sequence}`, 'success');
			announcement = `${label} dispatched to SAG ${focused.request.sequence}, ${slotIds.length} rider${slotIds.length === 1 ? '' : 's'}.`;
			// The decision is made; the transient mode ends with it.
			focusSagRequest(focused.request.id, 'select');
		} catch (e) {
			if (e instanceof ApiError && e.status === 409) {
				overcap = { id: c.id, body: e.body as unknown as OverCapacityBody };
			} else {
				showToast(e instanceof ApiError ? e.message : 'Dispatch failed', 'error');
			}
		} finally {
			dispatching = null;
		}
	}

	/**
	 * A seat refusal STATES A LIMIT; a rack overflow ASKS A QUESTION. Two
	 * blocks, two colours, and only one of them has an "anyway" button —
	 * retrying a seat refusal with allowOvercommit is refused by the server
	 * again, so offering the button would be a lie. (SagBoard.svelte.)
	 */
	function seatRefusalText(b: OverCapacityBody, label: string): string {
		const would = b.committedSeats + b.needSeats;
		return `${label} has ${b.seats} seat${b.seats === 1 ? '' : 's'}; this would be ${would}.`;
	}

	function rackQuestionText(b: OverCapacityBody, label: string): string {
		const would = b.committedRacks + b.needRacks;
		return `${label} has ${b.rackSlots} rack${b.rackSlots === 1 ? '' : 's'}; this would be ${would}.`;
	}

	// --- display -----------------------------------------------------------

	function vehicleLabel(c: SagCandidate): string {
		return c.vehicle.tacticalCall || c.vehicle.callsign;
	}

	function miles(m: number): string {
		return (m / METERS_PER_MILE).toFixed(1);
	}

	/**
	 * Every distance carries its meaning. Road miles carry the direction word;
	 * crow-flies miles carry the word `direct`, and say `(off course)` when a
	 * course exists to be off — with no course at all, "off course" would be
	 * meaningless and the header has already said every number is straight-line.
	 */
	function distanceText(c: SagCandidate): string {
		if (c.ambiguous || c.distanceMeters == null) return '';
		if (c.distanceKind === 'direct') {
			return $sagRoute.index ? `${miles(c.distanceMeters)} mi direct` : `${miles(c.distanceMeters)} mi direct`;
		}
		return `${miles(c.distanceMeters)} mi ${c.direction}`;
	}

	/**
	 * The second number, shown only when the two disagree materially.
	 *
	 * The roads are two-way and drivers take side roads, so on an out-and-back
	 * a van on the opposite leg is often on the SAME road as the pickup. The
	 * app cannot know whether a cut-through exists; the driver does. So both
	 * numbers go on screen and the operator decides, rather than the app
	 * picking one and hiding the other.
	 */
	function shortcutText(c: SagCandidate): string {
		if (!c.shortcut || c.routeMeters == null || c.directMeters == null) return '';
		if (c.distanceKind === 'direct') return `${miles(c.routeMeters)} mi if they follow the course`;
		return `${miles(c.directMeters)} mi direct`;
	}

	function distanceSpoken(c: SagCandidate): string {
		if (c.ambiguous && c.ambiguityMiles) {
			return `two possible positions, mile ${c.ambiguityMiles[0].toFixed(0)} or mile ${c.ambiguityMiles[1].toFixed(0)}`;
		}
		if (c.distanceMeters == null) return 'distance unknown';
		const extra = c.shortcut && c.routeMeters != null ? `, or ${miles(c.routeMeters)} miles following the course` : '';
		if (c.distanceKind === 'direct') return `${miles(c.distanceMeters)} miles in a straight line${extra}`;
		return `${miles(c.distanceMeters)} miles ${c.direction} on the course`;
	}

	/** Always the tilde; past the cap, never a precise large number. */
	function etaText(c: SagCandidate): string {
		if (c.etaMinutes == null) return '';
		return c.etaCapped ? `~${c.etaMinutes}+ min` : `~${c.etaMinutes} min`;
	}

	function ambiguityText(c: SagCandidate): string {
		if (!c.ambiguityMiles) return 'two possible positions';
		return `two possible positions — could be mi ${c.ambiguityMiles[0].toFixed(0)} or mi ${c.ambiguityMiles[1].toFixed(0)}`;
	}

	const EXCLUDED_TEXT: Record<string, string> = {
		released: 'checked out',
		'no-seats': 'no seat',
		'no-position': 'position never set'
	};

	/** Segmented capacity, filled = committed. Above six segments the pip count
	 *  is unreadable and a numeral is more honest. */
	function pips(total: number, committed: number): boolean[] | null {
		if (total <= 0 || total > 6) return null;
		return Array.from({ length: total }, (_, i) => i < committed);
	}

	function rowAria(c: SagCandidate, i: number): string {
		const v = c.vehicle;
		const bits = [
			vehicleLabel(c),
			`rank ${i + 1}`,
			distanceSpoken(c),
			c.etaMinutes != null ? `about ${c.etaMinutes}${c.etaCapped ? ' or more' : ''} minutes` : '',
			`${v.availableSeats} of ${v.seats} seats free`,
			`${v.availableRacks} of ${v.rackSlots} racks free`,
			c.age === 'never' ? 'position never set' : `position ${ageText(lastHeardOf(c), clock)} old`,
			c.age === 'stale' ? 'stale' : '',
			c.eligibility === 'partial' ? `can take ${c.seatsOffered} of ${focused?.waiting ?? 0} riders` : '',
			c.rackShortfall > 0 ? `${c.rackShortfall} bike${c.rackShortfall === 1 ? '' : 's'} would ride in the bed` : ''
		];
		return bits.filter(Boolean).join(', ');
	}

	function lastHeardOf(c: SagCandidate): string | undefined {
		return $sagVehicleGeo.find((g) => g.vehicle.checkInId === c.id)?.lastHeard;
	}

	// --- headers (spec §10) -------------------------------------------------
	let noUnits = $derived(ranked.length === 0 && excluded.length === 0);
	let noPositions = $derived(
		ranked.length === 0 && excluded.length > 0 && excluded.every((c) => c.excludedReason === 'no-position')
	);
	let allStale = $derived(ranked.length > 0 && ranked.every((c) => c.age === 'stale' || c.age === 'never'));
	let pickupUnplaced = $derived(focused != null && !focused.pickup.placed);

	// --- keyboard (spec §8) -------------------------------------------------
	let highlight = $state(0);
	let listEl = $state<HTMLElement | undefined>();
	/**
	 * NOTHING IS PRESELECTED. The roving cursor has to start somewhere for
	 * `↑ ↓ ⏎` to work, but at rest that cursor must not be visible: a row
	 * wearing the selection treatment before the operator has touched anything
	 * is a recommendation, and `Enter` on a recommendation is a reflex confirm
	 * on a decision they have context for and the ranking does not. The cursor
	 * appears only once the list actually holds keyboard focus.
	 */
	let listFocused = $state(false);

	$effect(() => {
		const n = ranked.length;
		if (n > 0 && highlight > n - 1) highlight = n - 1;
	});

	function onKey(e: KeyboardEvent): void {
		const n = ranked.length;
		if (e.key === 'Escape') {
			// Exit dispatch focus first; clearing focus entirely is the next Esc.
			if (focused) focusSagRequest(focused.request.id, 'select');
			else clearSagFocus();
			e.preventDefault();
			e.stopPropagation();
			return;
		}
		if (n === 0) return;
		switch (e.key) {
			case 'ArrowDown':
				highlight = (highlight + 1) % n;
				break;
			case 'ArrowUp':
				highlight = (highlight - 1 + n) % n;
				break;
			case 'Home':
				highlight = 0;
				break;
			case 'End':
				highlight = n - 1;
				break;
			case 'Enter':
				if ($canOperate && !overcap) send(ranked[highlight]);
				break;
			case 'c':
			case 'C':
				if (waitingSlots.length > 1) {
					chooserFor = chooserFor === ranked[highlight].id ? null : ranked[highlight].id;
				}
				break;
			default:
				return;
		}
		e.preventDefault();
	}
</script>

{#if focused}
	<section class="cp" aria-label="SAG candidates" style="--sag-unit: var(--color-sag-unit, #f97316); --sag-unit-soft: var(--color-sag-unit-soft, rgba(249, 115, 22, 0.14));">
		<header class="cp-head" style={tier ? `--cp-tier: var(${tierStyle(tier).colorVar}); --cp-tier-text: var(${tierStyle(tier).textVar});` : ''}>
			<div class="cp-head-row">
				<span class="cp-kicker">Candidates for</span>
				{#if tier}<RideTierGlyph {tier} size={13} />{/if}
				<span class="cp-name">SAG {focused.request.sequence}</span>
			</div>
			<div class="cp-head-row cp-head-row--sub">
				<span>{focused.pickup.placed ? focused.pickup.label : 'not on the map'}</span>
				<span class="cp-sep">·</span>
				<span>{focused.waiting === 1 ? '1 rider' : `${focused.waiting} riders`}</span>
				<span class="cp-age">{ageText(focused.request.createdAt, clock)}</span>
			</div>
		</header>

		{#if pickupUnplaced}
			<p class="cp-notice">
				Distance unknown — the pickup isn't on the map. Place it, or add a mile marker, to rank by distance.
			</p>
		{:else if result?.degraded}
			<!-- Before any number, not after it. -->
			<p class="cp-notice">
				{#if $sagRoute.index}
					<!-- A course IS loaded; it is the pickup (a dropped pin, a free
					     coordinate) whose place along it is unknown — the projection
					     can land on either leg of a shared road, so we do not guess. -->
					The pickup isn't tied to a mile on the course — these are straight-line distances, not road miles.
					Give it a mile marker or a rest stop to rank by road.
				{:else}
					No course loaded — these are straight-line distances, not road miles.
					<button class="cp-act" onclick={() => navigateZone('course-import')}>Import a course</button>
				{/if}
			</p>
		{/if}

		{#if noPositions}
			<p class="cp-notice cp-notice--warn">
				No SAG unit has a position — ranking by distance is not possible. Place them on the map, or dispatch
				from the board.
			</p>
		{:else if allStale}
			<p class="cp-notice cp-notice--warn">Every SAG position is more than 20 minutes old.</p>
		{/if}

		{#if noUnits}
			<p class="cp-notice cp-notice--warn">
				<span class="cp-warn-head">⚠ No SAG units checked in</span>
				Check in a unit with category “sag”.
			</p>
		{:else}
			{#if orderMoved}
				<!-- The order on screen is deliberately held while the panel is
				     open (see lockedOrder). Saying so is the honest half of that
				     bargain: withholding a changed ranking silently would be its
				     own kind of lie. -->
				<div class="cp-resort">
					<span>Ranking has changed since this list opened.</span>
					<button class="cp-resort-btn" tabindex="-1" onclick={resort}>Re-sort</button>
				</div>
			{/if}
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<div
				class="cp-list"
				role="listbox"
				tabindex="0"
				aria-label="Ranked SAG vehicles"
				aria-activedescendant={listFocused && ranked[highlight] ? `cp-opt-${ranked[highlight].id}` : undefined}
				bind:this={listEl}
				onkeydown={onKey}
				onfocusin={() => (listFocused = true)}
				onfocusout={(e) => {
					if (!listEl?.contains(e.relatedTarget as Node)) listFocused = false;
				}}
			>
				{#each ranked as c, i (c.id)}
					<div
						class="cp-row"
						class:cp-row--on={listFocused && i === highlight}
						class:cp-row--partial={c.eligibility === 'partial'}
						class:cp-row--stale={c.age === 'stale' || c.age === 'never'}
						id={`cp-opt-${c.id}`}
						role="option"
						aria-selected={listFocused && i === highlight}
						aria-label={rowAria(c, i)}
					>
						<div class="cp-row-head">
							<span class="cp-rank" aria-hidden="true">{RANK_CAPS[i] ?? i + 1}</span>
							<span class="cp-unit">{vehicleLabel(c)}</span>
							{#if c.ambiguous}
								<span class="cp-badge cp-badge--q" aria-hidden="true">? {ambiguityText(c)}</span>
							{:else if distanceText(c)}
								<span class="cp-dist" aria-hidden="true">{distanceText(c)}</span>
							{:else}
								<span class="cp-badge" aria-hidden="true">distance unknown</span>
							{/if}
						</div>

						<div class="cp-row-sub" aria-hidden="true">
							{#if etaText(c)}<span class="cp-eta">{etaText(c)}</span>{/if}
							{#if shortcutText(c)}
								<span class="cp-alt">{shortcutText(c)}</span>
							{/if}
							{#if c.age === 'stale'}
								<span class="cp-stale">position {ageText(lastHeardOf(c), clock)} old — stale</span>
							{:else if c.age === 'aging'}
								<span class="cp-muted">{ageText(lastHeardOf(c), clock)}</span>
							{/if}
						</div>

						<div class="cp-caps" aria-hidden="true">
							<span class="cp-cap" class:cp-cap--full={c.vehicle.availableSeats <= 0}>
								{#if pips(c.vehicle.seats, c.vehicle.committedSeats)}
									<span class="cp-bar">
										{#each pips(c.vehicle.seats, c.vehicle.committedSeats) ?? [] as filled, k (k)}
											<i class="cp-seat" class:cp-on={filled}></i>
										{/each}
									</span>
								{:else}
									<span class="cp-bar cp-bar--prop"
										><i style={`width:${Math.min(100, (c.vehicle.committedSeats / Math.max(1, c.vehicle.seats)) * 100)}%`}></i></span
									>
									<span class="cp-num">{c.vehicle.committedSeats}/{c.vehicle.seats}</span>
								{/if}
								<span class="cp-cap-word">{c.vehicle.availableSeats} seats free</span>
							</span>
							<span class="cp-cap">
								{#if pips(c.vehicle.rackSlots, c.vehicle.committedRacks)}
									<span class="cp-bar">
										{#each pips(c.vehicle.rackSlots, c.vehicle.committedRacks) ?? [] as filled, k (k)}
											<i class="cp-rack" class:cp-on={filled}></i>
										{/each}
									</span>
								{:else}
									<span class="cp-bar cp-bar--prop"
										><i style={`width:${Math.min(100, (c.vehicle.committedRacks / Math.max(1, c.vehicle.rackSlots)) * 100)}%`}></i></span
									>
									<span class="cp-num">{c.vehicle.committedRacks}/{c.vehicle.rackSlots}</span>
								{/if}
								<span class="cp-cap-word">{c.vehicle.availableRacks} racks free</span>
							</span>
						</div>

						{#if c.eligibility === 'partial'}
							<p class="cp-partial">Seats {c.seatsOffered} of {focused.waiting} needed</p>
						{/if}
						{#if c.rackShortfall > 0}
							<p class="cp-muted">
								{c.rackShortfall} bike{c.rackShortfall === 1 ? '' : 's'} would ride in the bed
							</p>
						{/if}

						{#if overcap?.id === c.id && overcap.body.seatsExceeded}
							<p class="cp-refusal">
								<span class="cp-refusal-head">⊘ No seat</span>
								{seatRefusalText(overcap.body, vehicleLabel(c))}
								Send fewer riders, or pick another vehicle.
							</p>
							<div class="cp-acts">
								<button class="cp-act" onclick={() => (overcap = null)}>Change the selection</button>
							</div>
						{:else if overcap?.id === c.id}
							<p class="cp-overcap">
								<span class="cp-overcap-head">⚠ Not enough racks</span>
								{rackQuestionText(overcap.body, vehicleLabel(c))}
								Carry them anyway?
							</p>
							<div class="cp-acts">
								<button
									class="cp-act cp-act--primary"
									disabled={dispatching != null}
									aria-busy={dispatching === c.id}
									onclick={() => send(c, true)}>{dispatching === c.id ? 'Dispatching…' : 'Yes — bikes ride in the bed'}</button
								>
								<button class="cp-act" onclick={() => (overcap = null)}>Cancel</button>
							</div>
						{:else}
							<div class="cp-acts">
								<button
									class="cp-act cp-act--primary"
									tabindex="-1"
									disabled={!$canOperate || dispatching != null || slotsFor(c).length === 0}
									aria-busy={dispatching === c.id}
									onclick={() => send(c)}>{dispatching === c.id ? 'Dispatching…' : sendLabel(c)}</button
								>
								{#if waitingSlots.length > 1}
									<!-- With one rider there is nothing to choose, and no
									     disclosure is shown. -->
									<button
										class="cp-act"
										tabindex="-1"
										aria-expanded={chooserFor === c.id}
										onclick={() => (chooserFor = chooserFor === c.id ? null : c.id)}
										>Choose riders… <kbd>c</kbd></button
									>
								{/if}
							</div>
						{/if}

						{#if chooserFor === c.id}
							<div class="cp-chooser">
								{#each waitingSlots as s (s.id)}
									<label class="cp-check">
										<input type="checkbox" checked={selected.has(s.id)} onchange={() => toggleSlot(s.id)} />
										{s.bib ? `Bib ${s.bib}` : s.riderName || 'Rider'}
									</label>
								{/each}
							</div>
						{/if}
					</div>
				{/each}

				{#if excluded.length > 0}
					<!-- Never hidden. An absence the operator cannot explain is worse
					     than a greyed row. -->
					<hr class="cp-rule" />
					{#each excluded as c (c.id)}
						<div
							class="cp-row cp-row--out"
							id={`cp-opt-${c.id}`}
							role="option"
							aria-selected="false"
							aria-disabled="true"
							aria-label={`${vehicleLabel(c)}, not available: ${EXCLUDED_TEXT[c.excludedReason ?? ''] ?? 'unavailable'}`}
						>
							<div class="cp-row-head">
								<span class="cp-rank" aria-hidden="true">⊘</span>
								<span class="cp-unit">{vehicleLabel(c)}</span>
								<span class="cp-badge" aria-hidden="true">{EXCLUDED_TEXT[c.excludedReason ?? ''] ?? 'unavailable'}</span>
							</div>
						</div>
					{/each}
				{/if}
			</div>
		{/if}

		<footer class="cp-foot">
			<span><kbd>↑</kbd><kbd>↓</kbd> move · <kbd>⏎</kbd> send · <kbd>Esc</kbd> exit</span>
		</footer>

		<div class="sr-only" aria-live="polite">{announcement}</div>
	</section>
{/if}

<style>
	/* The second distance: quieter than the one being acted on, but present,
	   because whether a cut-through exists is the driver's knowledge and not
	   ours to decide by hiding a number. */
	.cp-alt {
		color: var(--color-text-muted);
		font-size: var(--ride-t-label);
		white-space: nowrap;
	}

	.cp-resort {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		flex-shrink: 0;
		margin: 0 var(--space-sm) var(--space-xs);
		padding: var(--space-2xs) var(--space-xs);
		border: 1px dashed var(--color-warning);
		border-radius: var(--radius-sm);
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.cp-resort-btn {
		margin-left: auto;
		flex-shrink: 0;
		padding: 0 var(--space-sm);
		border: 1px solid var(--color-warning);
		border-radius: var(--radius-sm);
		background: none;
		color: var(--color-warning);
		font: inherit;
		font-weight: 700;
		cursor: pointer;
		min-height: 36px;
	}

	.cp-resort-btn:hover {
		background: var(--color-raised);
	}

	.cp {
		display: flex;
		flex-direction: column;
		min-height: 0;
		max-height: 100%;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		box-shadow: var(--shadow-md);
		overflow: hidden;
	}

	.cp-head {
		border-bottom: 1px solid var(--color-primary);
		border-left: 3px solid var(--cp-tier, var(--color-primary));
	}

	.cp-head-row {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		font-size: var(--ride-t-body);
		min-height: 44px;
		padding: 0 var(--space-sm);
	}

	.cp-head-row--sub {
		/* Below the 44px title band, not inside it: that band is what keeps the
		   title's baseline identical whether the dock or this panel is mounted. */
		min-height: 0;
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		padding: 0 var(--space-sm) var(--space-xs);
	}

	.cp-kicker {
		font-size: var(--ride-t-label);
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
		text-transform: uppercase;
	}

	.cp-name {
		font-weight: 700;
		color: var(--cp-tier-text, var(--color-text));
	}

	.cp-sep {
		opacity: 0.6;
	}

	.cp-age {
		margin-left: auto;
		font-variant-numeric: tabular-nums;
	}

	.cp-notice {
		margin: var(--space-xs) var(--space-sm);
		font-size: var(--ride-t-label);
		line-height: 1.4;
		color: var(--color-text-muted);
	}

	.cp-notice--warn {
		color: var(--color-warning);
		padding: var(--space-xs) var(--space-sm);
		background: var(--color-ride-priority-soft);
		border-radius: var(--radius-sm);
	}

	.cp-warn-head {
		display: block;
		font-weight: 700;
	}

	.cp-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		padding: var(--space-xs) var(--space-sm) var(--space-sm);
		overflow-y: auto;
		min-height: 0;
	}

	.cp-list:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
	}

	.cp-row {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		/* Without this the rows are flex children that shrink below their own
		   content inside the phone sheet's constrained column, and every row's
		   Send button renders on top of the row beneath it — the button the
		   operator is about to press, over the vehicle they did not choose. */
		flex-shrink: 0;
	}

	.cp-row--on {
		border-color: var(--color-accent);
		background: var(--sag-unit-soft);
	}

	.cp-row--partial {
		border-left: 3px solid var(--color-warning);
	}

	.cp-row--stale {
		border-style: dashed;
	}

	/* Greyed, with a reason, never hidden. */
	.cp-row--out {
		border-style: dotted;
		color: var(--color-text-muted);
	}

	.cp-row-head {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		font-size: var(--ride-t-body);
		flex-wrap: wrap;
	}

	.cp-rank {
		font-weight: 700;
		color: var(--color-text-muted);
	}

	.cp-unit {
		font-weight: 700;
		color: var(--sag-unit);
	}

	.cp-dist {
		margin-left: auto;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		color: var(--color-text);
		white-space: nowrap;
	}

	.cp-badge {
		margin-left: auto;
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.cp-badge--q {
		color: var(--color-warning);
		margin-left: 0;
		flex-basis: 100%;
	}

	.cp-row-sub {
		display: flex;
		gap: var(--space-sm);
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.cp-eta {
		font-variant-numeric: tabular-nums;
		color: var(--color-text);
	}

	.cp-stale {
		color: var(--color-warning);
	}

	.cp-muted {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.cp-caps {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-sm);
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.cp-cap {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
	}

	.cp-bar {
		display: inline-flex;
		gap: var(--space-2xs);
		align-items: center;
	}

	/* Seats are a WALL: a full row closes its frame. Racks are a LINE you can
	   see something spilling over, so their frame never thickens. */
	.cp-cap--full .cp-bar {
		outline: 2px solid var(--color-text-muted);
		outline-offset: 2px;
		border-radius: 2px;
	}

	.cp-seat,
	.cp-rack {
		width: 6px;
		height: 10px;
		border: 1px solid var(--color-text-muted);
		border-radius: 1px;
		display: inline-block;
	}

	.cp-rack {
		height: 6px;
	}

	.cp-on {
		background: var(--color-text-muted);
	}

	.cp-bar--prop {
		width: 36px;
		height: 8px;
		border: 1px solid var(--color-text-muted);
		border-radius: 2px;
		overflow: hidden;
		display: inline-block;
	}

	.cp-bar--prop i {
		display: block;
		height: 100%;
		background: var(--color-text-muted);
	}

	.cp-num {
		font-variant-numeric: tabular-nums;
	}

	.cp-cap-word {
		color: var(--color-text);
	}

	.cp-partial {
		font-size: var(--ride-t-label);
		color: var(--color-warning);
	}

	.cp-refusal,
	.cp-overcap {
		font-size: var(--ride-t-label);
		line-height: 1.4;
		padding: var(--space-xs);
		border-radius: var(--radius-sm);
	}

	.cp-refusal {
		color: var(--color-ride-emergency-text);
		background: var(--color-ride-emergency-soft);
	}

	.cp-overcap {
		color: var(--color-warning);
		background: var(--color-ride-priority-soft);
	}

	.cp-refusal-head,
	.cp-overcap-head {
		display: block;
		font-weight: 700;
	}

	.cp-acts {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-xs);
	}

	.cp-act {
		min-height: 30px;
		padding: 0 var(--space-sm);
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: var(--ride-t-label);
		cursor: pointer;
	}

	.cp-act--primary {
		border-color: var(--color-accent);
		font-weight: 700;
	}

	.cp-act:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.cp-act kbd {
		margin-left: var(--space-xs);
		opacity: 0.65;
	}

	.cp-chooser {
		display: flex;
		flex-direction: column;
		gap: var(--space-2xs);
	}

	.cp-check {
		font-size: var(--ride-t-label);
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		min-height: 44px;
	}

	.cp-rule {
		border: none;
		border-top: 1px solid var(--color-primary);
		margin: var(--space-xs) 0;
	}

	.cp-foot {
		margin-top: auto;
		padding: var(--space-xs) var(--space-sm);
		border-top: 1px solid var(--color-primary);
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.cp-foot kbd {
		margin-right: var(--space-2xs);
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}

	@media (max-width: 1199px) {
		.cp-row {
			min-height: 56px;
			padding: var(--space-sm);
		}

		.cp-act {
			min-height: 40px;
		}
	}
</style>
