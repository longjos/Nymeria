<script lang="ts">
	// The SAG dock (docs/sag-map-spec.md §1, §2, §8, §10) — the persistent list
	// beside the map on desktop/tablet, and the bottom sheet's `sag` content on
	// a phone.
	//
	// The dock's most important job is the group it renders FIRST: a request
	// whose pickup cannot be drawn is invisible on every other surface in the
	// app, so `⚑ NOT ON THE MAP` sits above everything, whatever the tier of
	// what is below it. A placeable EMERGENCY is already screaming on the map;
	// an unplaceable PRIORITY has nowhere else to be seen. [spec §2]
	//
	// Keyboard model is the ride strip's verbatim: ONE tab stop, roving
	// tabindex, `role="toolbar"`. Thirty cards must not be thirty tab stops
	// between the map and the panel, so the per-card action buttons are
	// `tabindex="-1"` and are reached by key (p / m / i) with the keys named in
	// each card's screen-reader description.
	import { sagPlaced, sagUnplaced, sagVehicleGeo, sagCourseNotice, sagRoute, sagFocus, focusSagRequest, startDispatchFocus, type PlacedRequest } from '$lib/stores/sagMap';
	import { sagBoard, rideLadder, navigateZone, sagEditorSeed } from '$lib/stores/ride';
	import { sagFocusPresetOn } from '$lib/stores/mapSettings';
	import { secondClock } from '$lib/stores/clock';
	import { canOperate } from '$lib/stores/session';
	import { tierById, tierStyle, ageText, ageState, SAG_STATUS_LABELS, enumLabel } from '$lib/rideMeta';
	import type { SagVehicleGeo } from '$lib/sagDispatch';
	import RideTierGlyph from '../RideTierGlyph.svelte';

	let {
		onPlaceRequest,
		onPlaceVehicle,
		onSagFocusPreset,
		onCollapse,
		collapsed = false,
		onExpand
	}: {
		/** Tablet (769-1199px): the dock collapses to a 44px rail of
		 *  tier-coloured stubs so the map keeps the width it needs, and expands
		 *  as an overlay on a tap (spec §11). */
		collapsed?: boolean;
		onExpand?: () => void;
		/** Enter the map's existing pick mode for this request's pickup/dropoff.
		 *  The dock never implements picking; it only asks for it. */
		onPlaceRequest?: (requestId: string, which: 'pickup' | 'dropoff') => void;
		/** Same, for a checked-in vehicle that has never reported a position. */
		onPlaceVehicle?: (checkInId: string) => void;
		/** The map's one-click `SAG focus` preset (spec §4). Rendered only when
		 *  the host wires it — a button that does nothing is worse than none. */
		onSagFocusPreset?: () => void;
		onCollapse?: () => void;
	} = $props();

	const METERS_PER_MILE = 1609.344;

	function rank(p: PlacedRequest): number {
		return tierById($rideLadder, p.request.priority)?.rank ?? 99;
	}

	/** "What is blocked on ME, worst first" — the same ordering SagBoard uses,
	 *  so the two surfaces can never disagree about which request is worst. */
	function byUrgency(a: PlacedRequest, b: PlacedRequest): number {
		const blocked = (p: PlacedRequest) => (p.waiting > 0 ? 0 : 1);
		return (
			blocked(a) - blocked(b) ||
			rank(a) - rank(b) ||
			Date.parse(a.request.createdAt) - Date.parse(b.request.createdAt)
		);
	}

	let unplaced = $derived([...$sagUnplaced].sort(byUrgency));
	let waiting = $derived($sagPlaced.filter((p) => p.pickup.placed && p.waiting > 0).sort(byUrgency));
	let inMotion = $derived($sagPlaced.filter((p) => p.pickup.placed && p.waiting === 0).sort(byUrgency));
	/** Checked-in SAG units that have never reported a position. They are absent
	 *  from the map entirely (spec §3.5), so this group is their only home. */
	let noPosition = $derived(
		$sagVehicleGeo.filter((g) => g.vehicle.checkInStatus !== 'released' && (g.lat == null || g.lon == null))
	);
	let vehicleCount = $derived(($sagBoard?.vehicles ?? []).filter((v) => v.checkInStatus !== 'released').length);
	let ridersWaiting = $derived($sagPlaced.reduce((n, p) => n + p.waiting, 0));

	let motionOpen = $state(false);

	/** The rail's stubs: one per tier that actually has somebody waiting, worst
	 *  first, plus the unplaceable group — which keeps its position above every
	 *  tier here for the same reason it does in the full dock. Tier names are
	 *  never hardcoded; the ladder is agency-configurable. */
	let railStubs = $derived.by(() => {
		const byTier = new Map<string, { code: string; colorVar: string; rank: number; n: number }>();
		for (const p of waiting) {
			const t = tierById($rideLadder, p.request.priority);
			if (!t) continue;
			const st = tierStyle(t);
			const cur = byTier.get(t.id);
			if (cur) cur.n += 1;
			else byTier.set(t.id, { code: st.code, colorVar: st.colorVar, rank: t.rank, n: 1 });
		}
		return [...byTier.values()].sort((a, b) => a.rank - b.rank);
	});

	type DockItem =
		| { kind: 'request'; id: string; group: 'unplaced' | 'waiting' | 'motion'; p: PlacedRequest }
		| { kind: 'motion-toggle'; id: string }
		| { kind: 'vehicle'; id: string; g: SagVehicleGeo };

	/** The roving order, flattened exactly as the groups render below — the
	 *  arrow keys walk across group boundaries, so index and DOM order must be
	 *  the same list. */
	let items = $derived<DockItem[]>([
		...unplaced.map((p) => ({ kind: 'request' as const, id: p.request.id, group: 'unplaced' as const, p })),
		...waiting.map((p) => ({ kind: 'request' as const, id: p.request.id, group: 'waiting' as const, p })),
		...(inMotion.length > 0 ? [{ kind: 'motion-toggle' as const, id: '__motion' }] : []),
		...(motionOpen ? inMotion.map((p) => ({ kind: 'request' as const, id: p.request.id, group: 'motion' as const, p })) : []),
		...noPosition.map((g) => ({ kind: 'vehicle' as const, id: g.vehicle.checkInId, g }))
	]);

	let focusIdx = $state(0);
	let listEl = $state<HTMLElement | undefined>();

	// A roving index that outruns its list leaves NO item at tabindex 0 and the
	// whole dock silently drops out of the tab order — the exact failure the
	// single-tab-stop pattern exists to prevent. (RideStrip.svelte, same bug.)
	$effect(() => {
		const n = items.length;
		if (n > 0 && focusIdx > n - 1) focusIdx = n - 1;
	});

	function hits(): HTMLElement[] {
		return Array.from(listEl?.querySelectorAll<HTMLElement>('.sd-item') ?? []);
	}

	function moveTo(i: number): void {
		focusIdx = i;
		hits()[i]?.focus();
	}

	function onFocusIn(e: FocusEvent): void {
		const all = hits();
		const idx = all.indexOf((e.target as HTMLElement)?.closest('.sd-item') as HTMLElement);
		if (idx >= 0) focusIdx = idx;
	}

	function activate(item: DockItem): void {
		if (item.kind === 'motion-toggle') {
			motionOpen = !motionOpen;
			return;
		}
		if (item.kind === 'request') focusSagRequest(item.p.request.id, 'select');
	}

	function onKey(e: KeyboardEvent): void {
		const n = items.length;
		if (n === 0) return;
		const item = items[focusIdx];
		switch (e.key) {
			case 'ArrowDown':
				moveTo((focusIdx + 1) % n);
				break;
			case 'ArrowUp':
				moveTo((focusIdx - 1 + n) % n);
				break;
			case 'Home':
				moveTo(0);
				break;
			case 'End':
				moveTo(n - 1);
				break;
			case 'Enter':
			case ' ':
				if (item) activate(item);
				break;
			case 'd':
			case 'D':
				if (item?.kind === 'request' && item.p.waiting > 0) startDispatchFocus(item.p.request.id);
				break;
			case 'p':
			case 'P':
				if (item?.kind === 'request') onPlaceRequest?.(item.p.request.id, 'pickup');
				else if (item?.kind === 'vehicle') onPlaceVehicle?.(item.g.vehicle.checkInId);
				break;
			case 'm':
			case 'M':
				if (item?.kind === 'request') addMile(item.p);
				break;
			case 'i':
			case 'I':
				if (item?.kind === 'request') navigateZone('course-import');
				break;
			// The collapse control is tabindex="-1" like every other action in
			// this single-tab-stop toolbar, so without a key a keyboard-only
			// operator on a tablet could OPEN the dock and never shut it.
			case 'Escape':
				if (!onCollapse) return;
				onCollapse();
				break;
			default:
				return;
		}
		e.preventDefault();
	}

	/** `Add mile` — the faster repair when the caller is still on the air
	 *  saying a number. The mile-marker field lives in the request editor on
	 *  the SAG board, so this seeds that editor open on this request with the
	 *  caret already in the mile input, then navigates to the board. */
	function addMile(p: PlacedRequest): void {
		focusSagRequest(p.request.id, 'select');
		sagEditorSeed.set({ requestId: p.request.id, focusField: 'pickupMile' });
		navigateZone('sag');
	}

	function riderWord(n: number): string {
		return n === 1 ? '1 rider' : `${n} riders`;
	}

	function reqLabel(p: PlacedRequest): string {
		return `SAG ${p.request.sequence}`;
	}

	function courseMiles(): string {
		const total = $sagRoute.index?.totalMeters;
		return total ? (total / METERS_PER_MILE).toFixed(0) : '';
	}

	/** §2's per-reason copy. The card states the reason IN WORDS — "not drawn"
	 *  is never left to be inferred from an absence. */
	function reasonText(p: PlacedRequest): string {
		if (p.pickup.placed) return '';
		const label = p.pickup.label;
		switch (p.pickup.reason) {
			case 'no-location':
				return 'Not on the map — no mile marker and no coordinate';
			case 'no-course':
				return `No course loaded — ${label || 'this mile marker'} can't be drawn`;
			case 'route-unknown':
				return p.request.pickup.route
					? `Route "${p.request.pickup.route}" isn't loaded`
					: `Several courses are loaded and nothing says which one ${label || 'this'} is on`;
			case 'off-route': {
				const len = courseMiles();
				return len
					? `${label} is past the end of the ${len} mi course`
					: `${label} is past the end of the course`;
			}
			default:
				return 'Not on the map';
		}
	}

	/** Which of §2's actions this reason offers, in the spec's order. */
	function actionsFor(p: PlacedRequest): ('place' | 'mile' | 'import')[] {
		if (p.pickup.placed) return [];
		switch (p.pickup.reason) {
			case 'no-course':
			case 'route-unknown':
				return ['import', 'place'];
			case 'off-route':
				return ['mile', 'place'];
			default:
				return ['place', 'mile'];
		}
	}

	function requestAria(p: PlacedRequest, group: string): string {
		const tier = tierById($rideLadder, p.request.priority);
		const parts = [
			tier ? tierStyle(tier).ariaWord : '',
			reqLabel(p),
			p.pickup.placed ? p.pickup.label : 'not on the map',
			p.waiting > 0 ? `${riderWord(p.waiting)} needing a vehicle` : enumLabel(p.request.status, SAG_STATUS_LABELS),
			`reported ${ageText(p.request.createdAt, clock)} ago`
		];
		if (!p.pickup.placed) parts.push(reasonText(p));
		if (group === 'unplaced') parts.push('Press p to place it on the map, m to add a mile marker');
		else if (p.waiting > 0) parts.push('Press d to find a driver');
		return parts.filter(Boolean).join(', ');
	}

	let clock = $derived($secondClock);

	// --- one polite live region (spec §8) ---
	// Only two things are ever announced: a NEW request that needs a vehicle,
	// and nothing else from this surface. Batched to one announcement a minute,
	// and never `assertive` — assertive belongs solely to the interrupt banner.
	let announcement = $state('');
	let lastAnnouncedAt = 0;
	let seen = new Set<string>();
	let primed = false;

	$effect(() => {
		const needy = $sagPlaced.filter((p) => p.waiting > 0).map((p) => p.request.id);
		if (!primed) {
			// The first pass is the existing board, not news.
			seen = new Set(needy);
			primed = true;
			return;
		}
		const fresh = needy.filter((id) => !seen.has(id));
		seen = new Set(needy);
		if (fresh.length === 0) return;
		const now = Date.now();
		if (now - lastAnnouncedAt < 60_000) return;
		lastAnnouncedAt = now;
		announcement =
			fresh.length === 1
				? `New SAG request needing a vehicle: ${fresh.length} waiting.`
				: `${fresh.length} new SAG requests needing a vehicle.`;
	});
</script>

{#if collapsed}
	<!-- The tablet rail (spec §11). It carries COUNTS, not content: enough to
	     know whether opening it is worth the width, never enough to make a
	     dispatch decision from — which is why the whole stub is one button that
	     opens the real dock. -->
	<button
		class="sd-rail"
		style="--sag-unit: var(--color-sag-unit, #f97316);"
		onclick={onExpand}
		aria-label={`Open the SAG dock. ${ridersWaiting === 0 ? 'Nobody waiting' : `${ridersWaiting} waiting`}${unplaced.length > 0 ? `, ${unplaced.length} not on the map` : ''}.`}
	>
		<span class="sd-rail-title" aria-hidden="true">SAG</span>
		{#if unplaced.length > 0}
			<span class="sd-rail-stub sd-rail-stub--flag" aria-hidden="true">
				<span class="sd-rail-code">⚑</span><span class="sd-rail-n">{unplaced.length}</span>
			</span>
		{/if}
		{#each railStubs as st (st.code)}
			<span class="sd-rail-stub" style={`--sd-tier: var(${st.colorVar});`} aria-hidden="true">
				<span class="sd-rail-code">{st.code}</span><span class="sd-rail-n">{st.n}</span>
			</span>
		{/each}
		{#if railStubs.length === 0 && unplaced.length === 0}
			<span class="sd-rail-quiet" aria-hidden="true">—</span>
		{/if}
		<span class="sd-rail-grip" aria-hidden="true">›</span>
	</button>
{:else}
<section class="sd" aria-label="SAG dock" style="--sag-unit: var(--color-sag-unit, #f97316); --sag-unit-soft: var(--color-sag-unit-soft, rgba(249, 115, 22, 0.14));">
	<header class="sd-head">
		<span class="sd-title">SAG</span>
		{#if $sagBoard}
			<span class="sd-count">{ridersWaiting > 0 ? `${ridersWaiting} waiting` : 'nobody waiting'}</span>
		{/if}
		{#if onCollapse}
			<button
				class="sd-collapse"
				tabindex="-1"
				onclick={onCollapse}
				aria-label="Collapse the SAG dock (Escape)"
				title="Collapse (Esc)"
			>⌄</button>
		{/if}
	</header>

	{#if $sagCourseNotice}
		<p class="sd-notice">{$sagCourseNotice}</p>
	{/if}

	{#if !$sagBoard}
		<p class="sd-muted">Loading SAG…</p>
	{:else}
		{#if vehicleCount === 0}
			<!-- An absence that matters: no units means every request below is
			     unanswerable, and that is a fact, not a quiet empty list. -->
			<p class="sd-warn">
				<span class="sd-warn-head">⚠ No SAG units checked in</span>
				Check in a unit with category “sag”.
			</p>
		{/if}

		{#if items.length === 0}
			<!-- Muted, never green: absence is not verified good. -->
			<p class="sd-muted">Nobody is waiting for a ride.</p>
		{/if}

		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div
			class="sd-list"
			role="toolbar"
			aria-label="SAG requests and vehicles"
			aria-orientation="vertical"
			tabindex="-1"
			bind:this={listEl}
			onkeydown={onKey}
			onfocusin={onFocusIn}
		>
			{#if unplaced.length > 0}
				<h3 class="sd-group sd-group--flag">⚑ NOT ON THE MAP <span class="sd-group-n">({unplaced.length})</span></h3>
				{#each unplaced as p (p.request.id)}
					{@const tier = tierById($rideLadder, p.request.priority)}
					{@const idx = items.findIndex((it) => it.id === p.request.id && it.kind === 'request')}
					<div
						class="sd-item sd-card sd-card--unplaced"
						role="button"
						tabindex={idx === focusIdx ? 0 : -1}
						aria-label={requestAria(p, 'unplaced')}
						onclick={() => focusSagRequest(p.request.id, 'select')}
						onkeydown={() => {}}
						class:sd-card--focused={$sagFocus?.requestId === p.request.id}
						style={tier ? `--sd-tier: var(${tierStyle(tier).colorVar}); --sd-tier-text: var(${tierStyle(tier).textVar});` : ''}
					>
						<div class="sd-row">
							{#if tier}
								<RideTierGlyph {tier} size={13} />
								<span class="sd-code">{tierStyle(tier).code}</span>
							{/if}
							<span class="sd-name">{reqLabel(p)}</span>
							<span class="sd-sep">·</span>
							<span class="sd-riders">{riderWord(p.waiting)}</span>
							<span class="sd-age">{ageText(p.request.createdAt, clock)}</span>
						</div>
						{#if p.request.pickup.description?.trim()}
							<!-- The caller's own words, verbatim and quoted. This is the
							     only description of where they are that exists. -->
							<p class="sd-quote">“{p.request.pickup.description.trim()}”</p>
						{/if}
						<p class="sd-reason">{reasonText(p)}</p>
						<div class="sd-actions">
							{#each actionsFor(p) as a (a)}
								{#if a === 'place'}
									<button class="sd-act sd-act--primary" tabindex="-1" disabled={!$canOperate || !onPlaceRequest} onclick={(e) => { e.stopPropagation(); onPlaceRequest?.(p.request.id, 'pickup'); }}>
										Place on map <kbd>p</kbd>
									</button>
								{:else if a === 'mile'}
									<button class="sd-act" tabindex="-1" onclick={(e) => { e.stopPropagation(); addMile(p); }}>
										Add mile <kbd>m</kbd>
									</button>
								{:else}
									<button class="sd-act" tabindex="-1" onclick={(e) => { e.stopPropagation(); navigateZone('course-import'); }}>
										Import course <kbd>i</kbd>
									</button>
								{/if}
							{/each}
						</div>
					</div>
				{/each}
			{/if}

			{#if waiting.length > 0}
				<h3 class="sd-group">⬒ WAITING <span class="sd-group-n">({waiting.length})</span></h3>
				{#each waiting as p (p.request.id)}
					{@const tier = tierById($rideLadder, p.request.priority)}
					{@const idx = items.findIndex((it) => it.id === p.request.id && it.kind === 'request')}
					<div
						class="sd-item sd-card"
						role="button"
						tabindex={idx === focusIdx ? 0 : -1}
						aria-label={requestAria(p, 'waiting')}
						onclick={() => focusSagRequest(p.request.id, 'select')}
						onkeydown={() => {}}
						class:sd-card--focused={$sagFocus?.requestId === p.request.id}
						style={tier ? `--sd-tier: var(${tierStyle(tier).colorVar}); --sd-tier-text: var(${tierStyle(tier).textVar});` : ''}
					>
						<div class="sd-row">
							{#if tier}
								<RideTierGlyph {tier} size={13} />
								<span class="sd-code">{tierStyle(tier).code}</span>
							{/if}
							<span class="sd-name">{reqLabel(p)}</span>
							<span class="sd-sep">·</span>
							<span class="sd-where">{p.pickup.label}</span>
							<span class="sd-age">{ageText(p.request.createdAt, clock)}</span>
						</div>
						<div class="sd-row sd-row--sub">
							<span class="sd-riders">{riderWord(p.waiting)}</span>
							{#if p.request.needsVehicle}<span class="sd-needs">needs a vehicle</span>{/if}
						</div>
						<div class="sd-actions">
							<button class="sd-act sd-act--primary" tabindex="-1" disabled={!$canOperate} onclick={(e) => { e.stopPropagation(); startDispatchFocus(p.request.id); }}>
								Find a driver <kbd>d</kbd>
							</button>
						</div>
					</div>
				{/each}
			{/if}

			{#if inMotion.length > 0}
				<div
					class="sd-item sd-group sd-group--toggle"
					role="button"
					tabindex={items.findIndex((it) => it.kind === 'motion-toggle') === focusIdx ? 0 : -1}
					aria-expanded={motionOpen}
					aria-label={`In motion, ${inMotion.length} request${inMotion.length === 1 ? '' : 's'}, nobody waiting`}
					onclick={() => (motionOpen = !motionOpen)}
					onkeydown={() => {}}
				>
					<span aria-hidden="true">{motionOpen ? '▾' : '▸'}</span> in motion
					<span class="sd-group-n">({inMotion.length})</span>
				</div>
				{#if motionOpen}
					{#each inMotion as p (p.request.id)}
						{@const tier = tierById($rideLadder, p.request.priority)}
						{@const idx = items.findIndex((it) => it.id === p.request.id && it.kind === 'request')}
						<div
							class="sd-item sd-card sd-card--motion"
							role="button"
							tabindex={idx === focusIdx ? 0 : -1}
							aria-label={requestAria(p, 'motion')}
							onclick={() => focusSagRequest(p.request.id, 'select')}
							onkeydown={() => {}}
							class:sd-card--focused={$sagFocus?.requestId === p.request.id}
							style={tier ? `--sd-tier: var(${tierStyle(tier).colorVar}); --sd-tier-text: var(${tierStyle(tier).textVar});` : ''}
						>
							<div class="sd-row">
								{#if tier}<span class="sd-code">{tierStyle(tier).code}</span>{/if}
								<span class="sd-name">{reqLabel(p)}</span>
								<span class="sd-sep">·</span>
								<span class="sd-where">{p.pickup.label}</span>
								<span class="sd-age">{enumLabel(p.request.status, SAG_STATUS_LABELS)}</span>
							</div>
						</div>
					{/each}
				{/if}
			{/if}

			{#if noPosition.length > 0}
				<h3 class="sd-group">🚐 NO POSITION <span class="sd-group-n">({noPosition.length})</span></h3>
				{#each noPosition as g (g.vehicle.checkInId)}
					{@const idx = items.findIndex((it) => it.kind === 'vehicle' && it.id === g.vehicle.checkInId)}
					<div
						class="sd-item sd-card sd-card--unit"
						role="button"
						tabindex={idx === focusIdx ? 0 : -1}
						aria-label={`${g.vehicle.tacticalCall || g.vehicle.callsign}, ${ageState(g.lastHeard, clock) === 'never' ? 'position never set' : 'no position'}, ${g.vehicle.availableSeats} of ${g.vehicle.seats} seats free. Press p to place it on the map.`}
						onclick={() => onPlaceVehicle?.(g.vehicle.checkInId)}
						onkeydown={() => {}}
					>
						<div class="sd-row">
							<span class="sd-unit">{g.vehicle.tacticalCall || g.vehicle.callsign}</span>
							<span class="sd-sep">·</span>
							<span class="sd-reason sd-reason--inline">voice check-in, position never set</span>
						</div>
						{#if onPlaceVehicle}
							<div class="sd-actions">
								<button class="sd-act sd-act--primary" tabindex="-1" disabled={!$canOperate} onclick={(e) => { e.stopPropagation(); onPlaceVehicle?.(g.vehicle.checkInId); }}>
									Place on map <kbd>p</kbd>
								</button>
							</div>
						{/if}
					</div>
				{/each}
			{/if}
		</div>
	{/if}

	{#if onSagFocusPreset}
		<footer class="sd-foot">
			<!-- The label says what the NEXT press does. An operator looking at a
			     map they have quietly stripped of tracks, weather and DF has to
			     be able to see, from here, that they did it. -->
			<button
				class="sd-act"
				class:sd-act--on={$sagFocusPresetOn}
				aria-pressed={$sagFocusPresetOn}
				onclick={onSagFocusPreset}>{$sagFocusPresetOn ? 'Restore my layers' : 'SAG focus'}</button
			>
		</footer>
	{/if}

	<div class="sr-only" aria-live="polite">{announcement}</div>
</section>
{/if}

<style>
	/* ---- tablet rail (spec §11) ---- */
	.sd-act--on {
		border-color: var(--color-sag-unit);
		color: var(--color-sag-unit);
	}

	.sd-rail {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-xs);
		width: 44px;
		padding: var(--space-xs) 0;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		box-shadow: var(--shadow-md);
		color: var(--color-text);
		cursor: pointer;
	}

	.sd-rail-title {
		font-size: 9px;
		font-weight: 800;
		letter-spacing: 0.08em;
		color: var(--sag-unit);
	}

	.sd-rail-stub {
		display: flex;
		flex-direction: column;
		align-items: center;
		width: 32px;
		padding: 2px 0;
		border-left: 3px solid var(--sd-tier, var(--color-text-muted));
		border-radius: 2px;
		background: var(--color-bg);
	}

	.sd-rail-stub--flag {
		border-left-color: var(--sag-unit);
	}

	.sd-rail-code {
		font-size: 10px;
		font-weight: 800;
		/* Never the raw tier colour as text: the emergency/medium tokens alias
		   --color-error/--color-info and fail AA on --color-surface. */
		color: var(--color-text);
	}

	.sd-rail-n {
		font-size: 12px;
		font-weight: 800;
		font-variant-numeric: tabular-nums;
		color: var(--color-text);
	}

	.sd-rail-quiet,
	.sd-rail-grip {
		font-size: 12px;
		color: var(--color-text-muted);
	}

	.sd-rail-grip {
		margin-top: auto;
	}

	.sd {
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

	.sd-head {
		display: flex;
		align-items: baseline;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-sm) var(--space-xs);
		border-bottom: 1px solid var(--color-primary);
	}

	.sd-title {
		font-size: var(--ride-t-body);
		font-weight: 800;
		letter-spacing: 0.06em;
		color: var(--sag-unit);
	}

	.sd-count {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.sd-collapse {
		margin-left: auto;
		min-width: 28px;
		min-height: 28px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.sd-notice,
	.sd-warn,
	.sd-muted {
		margin: var(--space-xs) var(--space-sm);
		font-size: var(--ride-t-label);
		line-height: 1.35;
	}

	.sd-notice {
		color: var(--color-text-muted);
	}

	.sd-muted {
		color: var(--color-text-muted);
	}

	.sd-warn {
		color: var(--color-warning);
		padding: var(--space-xs) var(--space-sm);
		background: var(--color-ride-priority-soft);
		border-radius: var(--radius-sm);
	}

	.sd-warn-head {
		display: block;
		font-weight: 700;
	}

	.sd-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: var(--space-xs) var(--space-sm) var(--space-sm);
		overflow-y: auto;
		min-height: 0;
	}

	.sd-group {
		margin-top: var(--space-xs);
		font-size: var(--ride-t-label);
		font-weight: 800;
		letter-spacing: 0.08em;
		color: var(--color-text-muted);
	}

	/* The only group that is never muted: it is the one nothing else renders. */
	.sd-group--flag {
		color: var(--color-warning);
		margin-top: 0;
	}

	.sd-group--toggle {
		cursor: pointer;
		background: none;
		border: none;
		text-align: left;
		padding: 2px 0;
	}

	.sd-group-n {
		font-weight: 600;
		opacity: 0.8;
	}

	.sd-card {
		border: 1px solid var(--color-primary);
		border-left: 3px solid var(--sd-tier, var(--color-primary));
		border-radius: var(--radius-sm);
		padding: 6px var(--space-sm);
		cursor: pointer;
		display: flex;
		flex-direction: column;
		gap: 3px;
		/* A card must never shrink below its own content: inside the phone
		   sheet's constrained column that puts each card's action button on
		   top of the card beneath it. */
		flex-shrink: 0;
	}

	.sd-card--unplaced {
		border-left-style: dashed;
		background: var(--color-ride-priority-soft);
	}

	.sd-card--motion {
		opacity: 0.75;
	}

	.sd-card--unit {
		border-left-color: var(--sag-unit);
	}

	.sd-card--focused {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
	}

	.sd-item:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}

	.sd-row {
		display: flex;
		align-items: center;
		gap: 5px;
		font-size: var(--ride-t-body);
		min-width: 0;
	}

	.sd-row--sub {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	/* Tier as TEXT always uses the -text variant: --color-ride-emergency and
	   --color-ride-medium alias --color-error / --color-info and fail AA on
	   --color-surface at normal text size. */
	.sd-code {
		font-weight: 800;
		letter-spacing: 0.04em;
		color: var(--sd-tier-text, var(--color-text));
	}

	.sd-name {
		font-weight: 700;
		color: var(--color-text);
		white-space: nowrap;
	}

	.sd-unit {
		font-weight: 700;
		color: var(--sag-unit);
	}

	.sd-sep {
		color: var(--color-text-muted);
	}

	.sd-where,
	.sd-riders {
		color: var(--color-text);
		font-variant-numeric: tabular-nums;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sd-age {
		margin-left: auto;
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.sd-needs {
		font-weight: 700;
		letter-spacing: 0.04em;
		color: var(--sd-tier-text, var(--color-text-muted));
	}

	.sd-quote {
		font-size: var(--ride-t-label);
		font-style: italic;
		color: var(--color-text);
		line-height: 1.35;
	}

	.sd-reason {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		line-height: 1.35;
	}

	.sd-reason--inline {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sd-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		margin-top: 2px;
	}

	.sd-act {
		min-height: 30px;
		padding: 0 8px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: var(--ride-t-label);
		cursor: pointer;
	}

	.sd-act--primary {
		border-color: var(--color-accent);
		font-weight: 700;
	}

	.sd-act:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.sd-act kbd {
		margin-left: 4px;
		opacity: 0.65;
		font-size: 0.9em;
	}

	.sd-foot {
		margin-top: auto;
		padding: var(--space-xs) var(--space-sm);
		border-top: 1px solid var(--color-primary);
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}

	/* Touch sizes: a 1024px tablet is a touch device (spec §11). */
	@media (max-width: 1199px) {
		.sd-card {
			min-height: 56px;
			padding: var(--space-sm);
		}

		.sd-act {
			min-height: 40px;
		}
	}
</style>
