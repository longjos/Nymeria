<script lang="ts">
	// The sweep-gated three-fact closure ladder (governing fact 4 / #5):
	// riders-clear, sweep-passed and close are three separate, timestamped
	// facts, never one toggle. ErrSweepNotPassed is surfaced here as an
	// explanation ("can't close yet — here's why, here's the NCS override"),
	// never as a bare failure toast.
	import type { StationView, CourseConfig } from '$lib/types';
	import { stationLadderMeta } from '$lib/courseMeta';
	import { reportRidersClear, markSweepPassed, closeStationChecked, reopenStation } from '$lib/stores/course';
	import { clock } from '$lib/wxAlertTime';
	import { showToast } from '$lib/stores/toast';
	import { netAnnotations } from '$lib/stores/netcontrol';
	import { annotationCentroid } from '$lib/geo';

	let {
		station, config, cleared, canOperate, isNcs, ncsCallsign, onFlyTo
	}: {
		station: StationView;
		config: CourseConfig;
		cleared: boolean;
		canOperate: boolean;
		isNcs: boolean;
		ncsCallsign: string;
		onFlyTo?: (lat: number, lon: number, zoom?: number) => void;
	} = $props();

	let c = $derived(station.closure);
	let meta = $derived(stationLadderMeta[c.state]);
	let willGate = $derived(config.closeRequiresSweep && !c.sweepPassedAt);
	let centroid = $derived.by(() => {
		const ann = $netAnnotations.find((a) => a.id === station.checkpointId);
		return ann ? annotationCentroid([ann]) : null;
	});

	function flyToStation(): void {
		if (onFlyTo && centroid) onFlyTo(centroid.lat, centroid.lon, 15);
	}

	let pending = $state<null | 'clear' | 'sweep' | 'close' | 'reopen'>(null);
	let gateOpen = $state(false);
	let overrideOpen = $state(false);
	let overrideReason = $state('');
	let overrideError = $state<string | null>(null);
	let reopenOpen = $state(false);
	let reopenReason = $state('');

	async function doClear(): Promise<void> {
		pending = 'clear';
		try {
			await reportRidersClear(station.checkpointId);
		} catch {
			/* toasted by the store action */
		} finally {
			pending = null;
		}
	}

	async function doSweepPassed(): Promise<void> {
		pending = 'sweep';
		try {
			await markSweepPassed(station.checkpointId);
		} catch {
			/* toasted by markSweepPassed */
		} finally {
			pending = null;
		}
	}

	async function tryClose(): Promise<void> {
		pending = 'close';
		const r = await closeStationChecked(station.checkpointId);
		pending = null;
		if (r.ok) return;
		if (r.gate === 'sweep_not_passed') gateOpen = true;
	}

	async function submitOverride(): Promise<void> {
		if (overrideReason.trim() === '') return;
		pending = 'close';
		overrideError = null;
		const r = await closeStationChecked(station.checkpointId, { override: true, reason: overrideReason.trim() });
		pending = null;
		if (r.ok) {
			// closeStation() already toasts the override success — one action,
			// one confirmation.
			overrideOpen = false;
			gateOpen = false;
			overrideReason = '';
		} else if (r.gate === 'ncs_required') {
			overrideError = 'Net control has changed — you can no longer override.';
		} else if (r.gate === 'other') {
			overrideError = r.message;
		}
	}

	async function submitReopen(): Promise<void> {
		if (reopenReason.trim() === '') return;
		pending = 'reopen';
		try {
			await reopenStation(station.checkpointId, reopenReason.trim());
			reopenOpen = false;
			reopenReason = '';
		} catch {
			/* toasted by the store action */
		} finally {
			pending = null;
		}
	}
</script>

<div class="station-row" class:cleared class:out-of-order={station.outOfOrder} role="group" aria-labelledby="st-{station.checkpointId}-name">
	<div class="station-head">
		<span class="seq" aria-hidden="true">#{station.sequenceNumber}</span>
		{#if onFlyTo && centroid}
			<button class="station-name" id="st-{station.checkpointId}-name" onclick={flyToStation}>{station.label}</button>
		{:else}
			<span class="station-name" id="st-{station.checkpointId}-name">{station.label}</span>
		{/if}
		<span class="state-chip" style="color: var({meta.colorVar})">
			<svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true">
				<title>{meta.word}</title>
				<path d={meta.glyph} fill={meta.glyphFill ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.5" />
			</svg>
			<span class="state-word">{meta.word}</span>
		</span>
		{#if station.outOfOrder}
			<span class="oo-chip" title="Closed ahead of an earlier stop — reported, not blocked">out of order</span>
		{/if}
	</div>

	<ol class="ladder" aria-label="Closure steps for {station.label}">
		<li class="step" class:done={!!c.ridersClearAt} aria-current={c.state === 'open' ? 'step' : undefined}>
			<span class="step-mark" aria-hidden="true">{c.ridersClearAt ? '✓' : '1'}</span>
			<span class="step-label">Riders clear</span>
			{#if c.ridersClearAt}
				<span class="step-when">{clock(c.ridersClearAt)}{c.ridersClearBy ? ` · ${c.ridersClearBy}` : ''}</span>
			{:else if canOperate && c.state === 'open'}
				<button class="step-btn" onclick={doClear} aria-busy={pending === 'clear'} disabled={pending !== null}>Report clear</button>
			{/if}
		</li>
		<li class="step" class:done={!!c.sweepPassedAt} aria-current={c.state === 'riders_clear' ? 'step' : undefined}>
			<span class="step-mark" aria-hidden="true">{c.sweepPassedAt ? '✓' : '2'}</span>
			<span class="step-label">Sweep passed</span>
			{#if c.sweepPassedAt}
				<span class="step-when">{clock(c.sweepPassedAt)} · {c.sweepPassageId ? 'from passage log' : c.sweepPassedBy}</span>
			{:else if canOperate && c.state !== 'closed'}
				<button class="step-btn" onclick={doSweepPassed} aria-busy={pending === 'sweep'} disabled={pending !== null}>Mark passed</button>
				{#if config.autoSweepFromPassage}<span class="step-hint">auto when {config.sweepLabel} is logged here</span>{/if}
			{/if}
		</li>
		<li class="step" class:done={c.state === 'closed'} class:blocked={willGate && c.state !== 'closed'}>
			<span class="step-mark" aria-hidden="true">{c.state === 'closed' ? '✓' : '3'}</span>
			<span class="step-label">Close</span>
			{#if c.state === 'closed'}
				<span class="step-when">{clock(c.closedAt)}{c.closedBy ? ` · ${c.closedBy}` : ''}{c.closedByOverride ? ' · override' : ''}</span>
			{:else if canOperate}
				<button
					class="step-btn step-btn-close"
					onclick={tryClose}
					aria-busy={pending === 'close'}
					disabled={pending !== null}
					aria-describedby={willGate ? `st-${station.checkpointId}-gate-hint` : undefined}
				>Close stop</button>
				{#if willGate}<span class="step-hint" id="st-{station.checkpointId}-gate-hint">needs sweep first</span>{/if}
			{/if}
		</li>
	</ol>

	{#if gateOpen}
		<div class="gate-note" role="status">
			<span class="gate-glyph" aria-hidden="true">&#8987;</span>
			<div class="gate-body">
				<p><strong>{station.label} can't close yet</strong> &mdash; the sweep hasn't passed it. Riders behind the sweep would lose support.</p>
				{#if overrideOpen}
					<label class="gate-reason-label" for="st-{station.checkpointId}-override-reason">Override reason (required &mdash; written to the timeline)</label>
					<textarea id="st-{station.checkpointId}-override-reason" class="gate-reason" rows="2" bind:value={overrideReason} placeholder="Why close before sweep?"></textarea>
					{#if overrideError}<p class="gate-error" role="alert">{overrideError}</p>{/if}
					<div class="gate-actions">
						<button class="btn-secondary" onclick={() => { overrideOpen = false; overrideReason = ''; overrideError = null; }} disabled={pending !== null}>Cancel</button>
						<button class="btn-danger" onclick={submitOverride} disabled={pending !== null || overrideReason.trim() === ''} aria-busy={pending === 'close'}>Close anyway</button>
					</div>
				{:else}
					<div class="gate-actions">
						<button class="btn-secondary" onclick={() => (gateOpen = false)}>Wait for sweep</button>
						{#if isNcs}
							<button class="btn-secondary btn-override" onclick={() => (overrideOpen = true)}>Close anyway&hellip;</button>
						{:else}
							<span class="muted">Only net control{ncsCallsign ? ` (${ncsCallsign})` : ''} or an admin can override.</span>
						{/if}
					</div>
				{/if}
			</div>
		</div>
	{/if}

	{#if canOperate && c.state !== 'open'}
		<div class="reopen-row">
			{#if reopenOpen}
				<label class="sr-only" for="st-{station.checkpointId}-reopen-reason">Reopen reason (required)</label>
				<input id="st-{station.checkpointId}-reopen-reason" class="reopen-input" type="text" bind:value={reopenReason} placeholder="Why reopen?" />
				<button class="link-btn" onclick={submitReopen} disabled={reopenReason.trim() === '' || pending !== null} aria-busy={pending === 'reopen'}>Confirm reopen</button>
				<button class="link-btn" onclick={() => { reopenOpen = false; reopenReason = ''; }}>Cancel</button>
			{:else}
				<button class="link-btn" onclick={() => (reopenOpen = true)}>Reopen&hellip;</button>
			{/if}
		</div>
	{/if}
</div>

<style>
	.station-row {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.station-row.cleared {
		border-left: 3px solid var(--color-success);
		background: color-mix(in srgb, var(--color-success) 6%, transparent);
	}

	.station-head {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		flex-wrap: wrap;
	}

	.seq {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
	}

	.station-name {
		flex: 1;
		min-width: 0;
		font-weight: 700;
		font-size: 0.88rem;
		background: none;
		border: none;
		color: var(--color-text);
		text-align: left;
		padding: 0;
		cursor: pointer;
	}

	span.station-name {
		cursor: default;
	}

	.state-chip {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		font-size: 0.75rem;
		font-weight: 600;
		white-space: nowrap;
	}

	.oo-chip {
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding: 1px 6px;
		border: 1px solid var(--color-text-muted);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
	}

	.ladder {
		list-style: none;
		display: grid;
		grid-template-columns: 20px 1fr auto;
		row-gap: var(--space-xs);
		column-gap: var(--space-sm);
		margin: 0;
		padding: 0;
	}

	.step {
		display: contents;
	}

	.step-mark {
		grid-column: 1;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: var(--color-raised-strong);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.65rem;
		color: var(--color-text-muted);
	}

	.step.done .step-mark {
		color: var(--color-success);
	}

	.step-label {
		grid-column: 2;
		font-size: 0.8rem;
		color: var(--color-text);
	}

	.step.blocked .step-label {
		color: var(--color-text-muted);
	}

	.step-when {
		grid-column: 3;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		white-space: nowrap;
		align-self: center;
	}

	.step-btn {
		grid-column: 3;
		min-height: 32px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.72rem;
		font-weight: 700;
		cursor: pointer;
	}

	.step-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.step-btn-close {
		border-color: var(--color-accent);
	}

	.step-hint {
		grid-column: 2 / 4;
		font-size: 0.68rem;
		color: var(--color-warning);
	}

	.gate-note {
		display: flex;
		gap: var(--space-sm);
		padding: var(--space-sm);
		border-radius: var(--radius-sm);
		border-left: 3px solid var(--color-warning);
		background: color-mix(in srgb, var(--color-warning) 12%, transparent);
	}

	.gate-glyph {
		font-size: 1rem;
		line-height: 1.4;
	}

	.gate-body {
		flex: 1;
		min-width: 0;
		font-size: 0.8rem;
	}

	.gate-body p {
		margin: 0 0 6px;
	}

	.gate-actions {
		display: flex;
		gap: var(--space-sm);
		align-items: center;
		flex-wrap: wrap;
	}

	.gate-reason-label {
		display: block;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		margin-bottom: 2px;
	}

	.gate-reason {
		width: 100%;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		font-size: 0.8rem;
		padding: var(--space-sm);
		resize: vertical;
		margin-bottom: 6px;
	}

	.gate-error {
		color: var(--color-error-text);
		font-size: 0.75rem;
	}

	.btn-secondary {
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-weight: 600;
		font-size: 0.78rem;
		cursor: pointer;
	}

	.btn-override {
		border-color: var(--color-warning);
		color: var(--color-warning);
	}

	.btn-danger {
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-error);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-on-accent);
		font-weight: 700;
		font-size: 0.78rem;
		cursor: pointer;
	}

	.btn-danger:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.muted {
		color: var(--color-text-muted);
		font-size: 0.75rem;
	}

	.reopen-row {
		display: flex;
		align-items: center;
		gap: 6px;
	}

	.reopen-input {
		flex: 1;
		min-height: 32px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		font-size: 0.75rem;
		padding: 0 var(--space-sm);
	}

	.link-btn {
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
		padding: 4px;
	}

	.link-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}
</style>
