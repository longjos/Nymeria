<script lang="ts">
	// Extracted from RouteProgressBar.svelte (panel density) and
	// dashboard/EventProgress.svelte, which duplicated the wxBrackets math,
	// elementColors table and dotSize function verbatim. This is now the ONE
	// copy; RideStrip.svelte's density="strip" would otherwise have been a
	// third. RouteProgressBar and EventProgress keep their own heading/detail
	// chrome and render this for the rail itself.
	import type { CheckpointWithPassages, ProgressElement, WxAlert, ShutoffPoint, StationClosure } from '$lib/types';
	import { statusColor } from '$lib/annotationMeta';
	import { STOP_GLYPHS } from '$lib/rideMeta';
	import WxTierGlyph from './WxTierGlyph.svelte';

	interface IncidentPin {
		id: string;
		mile?: number;
		checkpointId?: string;
		tier: { rank: number; label: string; id: string };
		label: string;
	}

	let {
		checkpoints,
		elements,
		shutoffs = [],
		closures,
		outOfOrder,
		wxAlerts = [],
		density = 'panel',
		scale = 'index',
		stopMiles,
		incidentPins = [],
		leadLabel = 'LEAD',
		sweepLabel = 'SWEEP',
		focusable = false,
		onStopActivate,
		onSweepMarkerActivate,
	}: {
		checkpoints: CheckpointWithPassages[];
		elements: ProgressElement[];
		shutoffs?: ShutoffPoint[];
		closures?: Map<string, StationClosure['state']>;
		/**
		 * store.StationView.outOfOrder, keyed by checkpoint annotation id — the
		 * SAME backend field StationRow badges "out of order" (spec finding
		 * 13). At `scale="mile"` (the strip's course rail) nodes are POSITIONED
		 * by projected route mile but always LABELED by sequence number; with
		 * mis-sequenced data those two orders disagree and the rail draws
		 * sequence numbers out of left-to-right order with nothing saying so.
		 * Reading the backend's own reconciliation — rather than re-deriving a
		 * mile-vs-sequence check here — keeps the rail and the course panel
		 * from ever disagreeing about which stop is out of order.
		 */
		outOfOrder?: Map<string, boolean>;
		wxAlerts?: WxAlert[];
		density?: 'panel' | 'strip';
		scale?: 'index' | 'mile';
		stopMiles?: Map<string, number>;
		incidentPins?: IncidentPin[];
		leadLabel?: string;
		sweepLabel?: string;
		focusable?: boolean;
		onStopActivate?: (cpId: string) => void;
		onSweepMarkerActivate?: () => void;
	} = $props();

	// Tier brackets over the affected checkpoint span (UX §8.6) — the ONE copy.
	let wxBrackets = $derived(
		wxAlerts
			.filter((a) => a.tier !== 'statement' && a.affects.checkpointSeqRange.length === 2)
			.map((a) => {
				const [loSeq, hiSeq] = a.affects.checkpointSeqRange;
				const total = checkpoints.length;
				const loIdx = checkpoints.findIndex((c) => c.meta.sequenceNumber === loSeq);
				const hiIdx = checkpoints.findIndex((c) => c.meta.sequenceNumber === hiSeq);
				if (loIdx < 0 || hiIdx < 0) return null;
				const pctStart = total > 1 ? (loIdx / (total - 1)) * 100 : 0;
				const pctEnd = total > 1 ? (hiIdx / (total - 1)) * 100 : 100;
				return { alert: a, pctStart, pctEnd, loSeq, hiSeq };
			})
			.filter((b): b is NonNullable<typeof b> => b !== null)
	);

	// Element colors by label. The three hexes this table inherited from
	// RouteProgressBar were literally --color-warning, --color-info and
	// --color-wx-statement written out longhand; they are tokens now, so the
	// rail can never drift from the rest of the palette.
	const elementColors: Record<string, string> = {
		tail: 'var(--color-warning)',
		'main pack': 'var(--color-info)',
	};
	const ELEMENT_COLOR_FALLBACK = 'var(--color-wx-statement)';

	function getElementColor(label: string): string {
		const lower = label.toLowerCase();
		if (lower === leadLabel.toLowerCase()) return 'var(--color-ride-lead)';
		if (lower === sweepLabel.toLowerCase()) return 'var(--color-ride-sweep)';
		return elementColors[lower] || ELEMENT_COLOR_FALLBACK;
	}

	function isLeadOrSweep(label: string): 'lead' | 'sweep' | null {
		const lower = label.toLowerCase();
		if (lower === leadLabel.toLowerCase()) return 'lead';
		if (lower === sweepLabel.toLowerCase()) return 'sweep';
		return null;
	}

	// presentationMode (EventProgress's larger dashboard look) is a scale
	// factor on the same function rather than a second dotSize copy.
	function dotSize(passageCount: number, presentationMode = false): number {
		return presentationMode
			? Math.min(24, Math.max(10, 10 + passageCount * 2))
			: Math.min(20, Math.max(8, 8 + passageCount * 2));
	}

	function pctFor(i: number, total: number, cpId: string): number {
		if (scale === 'mile' && stopMiles) {
			const totalMiles = Math.max(...Array.from(stopMiles.values()), 1);
			const mile = stopMiles.get(cpId);
			if (mile != null && totalMiles > 0) return Math.min(100, Math.max(0, (mile / totalMiles) * 100));
		}
		return total > 1 ? (i / (total - 1)) * 100 : 50;
	}

	function stopGlyph(cp: CheckpointWithPassages): { glyph: string; title: string } {
		const closure = closures?.get(cp.meta.annotationId);
		// STOP_GLYPHS is the one glyph table for this state across the strip,
		// the rail and the rest-stop board (U+23F3/U+231B are emoji-
		// presentation codepoints and render as tofu on the Linux font stack).
		if (closure === 'closed') return { glyph: STOP_GLYPHS.closed, title: 'Closed' };
		if (closure === 'sweep_passed') return { glyph: STOP_GLYPHS.readyToClose, title: 'Sweep passed — ready to close' };
		if (closure === 'riders_clear') return { glyph: STOP_GLYPHS.awaitingSweep, title: 'Awaiting sweep' };
		if (cp.annotation.status === 'at-capacity') return { glyph: STOP_GLYPHS.atCapacity, title: 'At capacity' };
		if (cp.annotation.status === 'open' || cp.annotation.status === 'active') return { glyph: STOP_GLYPHS.open, title: 'Open' };
		if (cp.annotation.status === 'closed') return { glyph: STOP_GLYPHS.closed, title: 'Closed' };
		return { glyph: STOP_GLYPHS.planned, title: 'Planned' };
	}

	function focusStopNode(idx: number) {
		const el = stopRefs[idx];
		el?.focus();
	}

	let stopRefs: (HTMLElement | undefined)[] = $state([]);
	let focusIdx = $state(0);

	// A course can shrink (a checkpoint deleted mid-net). Left unclamped the
	// rove index outruns the node list and no stop node holds tabindex 0,
	// which drops the rail out of the tab order entirely.
	$effect(() => {
		const n = checkpoints.length;
		if (n > 0 && focusIdx > n - 1) focusIdx = n - 1;
	});

	function handleRailKeydown(e: KeyboardEvent) {
		if (!focusable) return;
		const n = checkpoints.length;
		if (n === 0) return;
		if (e.key === 'ArrowRight') {
			focusIdx = Math.min(n - 1, focusIdx + 1);
			focusStopNode(focusIdx);
			e.preventDefault();
		} else if (e.key === 'ArrowLeft') {
			focusIdx = Math.max(0, focusIdx - 1);
			focusStopNode(focusIdx);
			e.preventDefault();
		}
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="course-rail density-{density}" class:has-wx={wxBrackets.length > 0} role="group" aria-label="Course rail" onkeydown={handleRailKeydown}>
	<!-- NWS alert tier brackets over the affected checkpoint span -->
	{#if wxBrackets.length > 0}
		<div class="cr-wx-layer">
			{#each wxBrackets as b, i (b.alert.id)}
				<div
					class="cr-wx"
					style="left: {b.pctStart}%; width: {b.pctEnd - b.pctStart}%; top: {2 + i * 12}px; --wx: var(--color-wx-{b.alert.tier})"
					title="{b.alert.event}: CP {b.loSeq}–CP {b.hiSeq}"
				>
					<span class="cr-wx-label"><WxTierGlyph tier={b.alert.tier} size={10} /> {b.alert.shortCode}</span>
				</div>
			{/each}
		</div>
	{/if}

	<!-- Connecting line -->
	<div class="cr-line"></div>

	<!-- Shutoff gates -->
	{#each shutoffs.filter((s) => s.status !== 'cancelled') as s (s.id)}
		{@const mile = stopMiles && s.routeMile != null ? s.routeMile : null}
		{@const pct = mile != null && stopMiles ? Math.min(100, Math.max(0, (mile / Math.max(...Array.from(stopMiles.values()), 1)) * 100)) : 50}
		<div class="cr-shutoff" class:fired={s.status === 'fired'} style="left: {pct}%" title="{s.name} — {s.status === 'fired' ? 'FIRED' : 'armed'}">
			<span class="cr-shutoff-gate" role="img" aria-label="Shutoff {s.name}, {s.status}">╫</span>
		</div>
	{/each}

	<!-- Incident pins -->
	{#each incidentPins as pin (pin.id)}
		{@const pct = pin.mile != null && stopMiles ? Math.min(100, Math.max(0, (pin.mile / Math.max(...Array.from(stopMiles.values()), 1)) * 100)) : 50}
		<div class="cr-incident" style="left: {pct}%">
			<span class="cr-incident-glyph" role="img" aria-label="{pin.label}">⬣</span>
		</div>
	{/each}

	<!-- Element markers (lead/sweep above/below the line; others above) -->
	{#if elements.length > 0}
		<div class="cr-elements">
			{#each elements as elem (elem.label)}
				{@const cpIndex = checkpoints.findIndex((c) => c.meta.annotationId === elem.lastCheckpointId)}
				{@const pct = pctFor(cpIndex, checkpoints.length, elem.lastCheckpointId)}
				{@const side = isLeadOrSweep(elem.label)}
				<div
					class="cr-element"
					class:below={side === 'sweep'}
					style="left: {pct}%; --elem-color: {getElementColor(elem.label)}"
					title="{elem.label} at CP{elem.lastCheckpointSeq}"
				>
					{#if side === 'sweep'}
						<span class="cr-element-label">{elem.label}</span>
						<span class="cr-element-dot cr-chevron-down"></span>
					{:else}
						<span class="cr-element-dot" class:cr-chevron-up={side === 'lead'}></span>
						<span class="cr-element-label">{elem.label}</span>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	<!-- Checkpoint dots -->
	<div class="cr-dots">
		{#each checkpoints as cp, i (cp.meta.annotationId)}
			{@const pct = pctFor(i, checkpoints.length, cp.meta.annotationId)}
			{@const size = dotSize(cp.passageCount)}
			{@const color = statusColor('checkpoint', cp.annotation.status)}
			{@const g = stopGlyph(cp)}
			{@const oo = outOfOrder?.get(cp.meta.annotationId) ?? false}
			<div class="cr-dot-wrapper" style="left: {pct}%">
				{#if density === 'strip'}
					<div
						class="cr-stop-node"
						class:cr-stop-node--oo={oo}
						role="button"
						tabindex={focusable ? (i === focusIdx ? 0 : -1) : -1}
						bind:this={stopRefs[i]}
						onclick={() => onStopActivate?.(cp.meta.annotationId)}
						onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onStopActivate?.(cp.meta.annotationId); } }}
						onfocus={() => (focusIdx = i)}
						title="{cp.annotation.label} — {g.title}{oo ? ' — out of order' : ''}"
						aria-label="{cp.annotation.label}, stop {cp.meta.sequenceNumber}{stopMiles?.get(cp.meta.annotationId) != null ? `, mile ${stopMiles.get(cp.meta.annotationId)!.toFixed(1)}` : ''}, {g.title}{oo ? ', out of order' : ''}"
					>
						{#if oo}
							<!-- Non-color channels (spec §10): a distinct triangle SHAPE
							     (never used by any stopGlyph, same filled-triangle-plus-
							     punched-mark family as rideMeta's PRIORITY tier — an SVG
							     path rather than a Unicode glyph so it can't render as
							     tofu on an unfamiliar font stack) plus the literal TEXT in
							     title/aria-label above. Quiet on purpose — a small corner
							     mark, not a full chip — since out-of-order is a data
							     problem, not an operational emergency. -->
							<svg class="cr-stop-oo" width="9" height="9" viewBox="0 0 16 16" role="img" aria-label="Out of order">
								<title>Out of order</title>
								<path d="M8 1.5l6.5 13H1.5L8 1.5z" fill="var(--color-warning)" />
								<path d="M8 6.2v3 M8 11.4v.1" stroke="var(--color-bg)" stroke-width="1.6" stroke-linecap="round" />
							</svg>
						{/if}
						<span class="cr-stop-glyph" style="color: {color}">{g.glyph}</span>
						<span class="cr-stop-seq">{cp.meta.sequenceNumber}</span>
					</div>
				{:else}
					<button
						class="cr-dot"
						style="width: {size}px; height: {size}px; background: {color}; border-color: {color}"
						onclick={() => onStopActivate?.(cp.meta.annotationId)}
						title="{cp.annotation.label} (#{cp.meta.sequenceNumber}) — {cp.passageCount} passages"
						aria-label="{cp.annotation.label}, stop {cp.meta.sequenceNumber}, {cp.passageCount} passages"
					>
						<span class="cr-dot-seq">{cp.meta.sequenceNumber}</span>
					</button>
					<span class="cr-dot-label">{cp.annotation.shortName || cp.annotation.label}</span>
					{#if cp.passageCount > 0}
						<span class="cr-dot-count">{cp.passageCount}</span>
					{/if}
				{/if}
			</div>
		{/each}
	</div>

	{#if density === 'strip' && onSweepMarkerActivate}
		<div class="cr-sweep-marker-wrap">
			<!-- A lone ▲ is not an accessible name; `title` is a weak and
			     inconsistently-announced fallback for the one control on the
			     rail that COMMITS (spec §5). -->
			<button
				class="cr-sweep-marker"
				onclick={onSweepMarkerActivate}
				title="Mark {sweepLabel.toLowerCase()} passed…"
				aria-label="Mark {sweepLabel.toLowerCase()} passed a stop"
			>
				▲
			</button>
		</div>
	{/if}
</div>

<style>
	.course-rail {
		position: relative;
		min-height: 72px;
		min-width: 200px;
		padding: 24px 16px 0;
	}

	.course-rail.has-wx {
		padding-top: 40px;
	}

	.course-rail.density-strip {
		min-height: 44px;
		min-width: 0;
		padding: 16px 12px 0;
	}

	.cr-wx-layer {
		position: absolute;
		top: 0;
		left: 16px;
		right: 16px;
		height: 40px;
	}

	.cr-wx {
		position: absolute;
		height: 8px;
		border: 2px solid var(--wx);
		border-bottom: none;
		border-radius: 3px 3px 0 0;
	}

	.cr-wx-label {
		position: absolute;
		top: -12px;
		left: 0;
		display: inline-flex;
		align-items: center;
		gap: var(--space-2xs);
		font-size: 0.6rem;
		font-weight: 700;
		color: var(--wx);
		white-space: nowrap;
	}

	.cr-line {
		position: absolute;
		top: 36px;
		left: 16px;
		right: 16px;
		height: 2px;
		background: var(--color-primary);
		opacity: 0.6;
	}

	.density-strip .cr-line {
		top: 22px;
	}

	.cr-shutoff {
		position: absolute;
		top: 26px;
		transform: translateX(-50%);
		color: var(--color-text-muted);
		z-index: 1;
	}

	.density-strip .cr-shutoff {
		top: 14px;
	}

	.cr-shutoff-gate {
		font-size: 1rem;
		font-weight: 700;
	}

	.cr-shutoff.fired .cr-shutoff-gate {
		color: var(--color-error);
	}

	.cr-incident {
		position: absolute;
		top: 20px;
		transform: translateX(-50%);
		z-index: 2;
	}

	.density-strip .cr-incident {
		top: 8px;
	}

	.cr-incident-glyph {
		font-size: 0.85rem;
		color: var(--color-error);
	}

	/* Element markers */
	.cr-elements {
		position: absolute;
		top: 4px;
		left: 16px;
		right: 16px;
		height: 20px;
	}

	.density-strip .cr-elements {
		top: 0;
		height: 44px;
	}

	.cr-element {
		position: absolute;
		transform: translateX(-50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1px;
	}

	.cr-element.below {
		top: 26px;
	}

	.density-strip .cr-element.below {
		top: 24px;
	}

	.cr-element-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--elem-color);
		box-shadow: 0 0 4px var(--elem-color);
	}

	.cr-chevron-up {
		width: 0;
		height: 0;
		background: none;
		box-shadow: none;
		border-left: 5px solid transparent;
		border-right: 5px solid transparent;
		border-bottom: 7px solid var(--elem-color);
		border-radius: 0;
	}

	.cr-chevron-down {
		width: 0;
		height: 0;
		background: none;
		box-shadow: none;
		border-left: 5px solid transparent;
		border-right: 5px solid transparent;
		border-top: 7px solid var(--elem-color);
		border-radius: 0;
	}

	.cr-element-label {
		font-size: 0.6rem;
		font-weight: 700;
		color: var(--elem-color);
		white-space: nowrap;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	/* Checkpoint dots */
	.cr-dots {
		position: absolute;
		top: 24px;
		left: 16px;
		right: 16px;
		height: 48px;
	}

	.density-strip .cr-dots {
		top: 16px;
		left: 4px;
		right: 4px;
		height: 24px;
	}

	.cr-dot-wrapper {
		position: absolute;
		transform: translateX(-50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-2xs);
	}

	.cr-dot {
		border: 2px solid;
		border-radius: 50%;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: transform var(--duration-fast), box-shadow var(--duration-fast);
		padding: 0;
		min-width: 0;
	}

	.cr-dot:hover {
		transform: scale(1.25);
		box-shadow: 0 0 8px rgba(255, 255, 255, 0.2);
	}

	.cr-dot-seq {
		font-size: 0.55rem;
		font-weight: 700;
		color: var(--color-on-accent);
		line-height: 1;
	}

	.cr-dot-label {
		font-size: 0.6rem;
		color: var(--color-text-muted);
		white-space: nowrap;
		max-width: 60px;
		overflow: hidden;
		text-overflow: ellipsis;
		text-align: center;
	}

	.cr-dot-count {
		font-size: 0.55rem;
		color: var(--color-text-muted);
		opacity: 0.7;
	}

	/* Strip stop nodes — sequence number only, full name in title/focus */
	.cr-stop-node {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		cursor: pointer;
		border-radius: var(--radius-sm);
		padding: 2px;
	}

	/* Out-of-order (finding 13): a quiet corner mark, not a full chip — see
	   the SVG's own comment for why it's a shape, not a color. */
	.cr-stop-node--oo {
		border: 1px dashed var(--color-warning);
	}

	.cr-stop-oo {
		position: absolute;
		top: -3px;
		right: -3px;
		z-index: 1;
	}

	.cr-stop-glyph {
		font-size: 0.85rem;
		line-height: 1;
	}

	.cr-stop-seq {
		font-size: 0.5rem;
		color: var(--color-text-muted);
		line-height: 1;
	}

	.cr-sweep-marker-wrap {
		position: absolute;
		right: 4px;
		bottom: 0;
	}

	.cr-sweep-marker {
		background: none;
		border: none;
		color: var(--color-ride-sweep);
		font-size: 0.9rem;
		cursor: pointer;
		min-width: 24px;
		min-height: 24px;
	}
</style>
