<script lang="ts">
	// The rail's "Sweep passed…" confirm (spec §5): a wrong sweep passage
	// corrupts the most-asked-about number in the operation, so this is a
	// cheap insurance tap rather than a blind commit.
	//
	// Every stop is a large button, not an <option> in a dropdown: this is
	// used one-handed in a moving vehicle, and a native select is a small
	// target that opens a second, platform-styled picker on top of this one.
	// The next stop the sweep is heading for is preselected, so the common
	// case is a single tap on the commit button. A search field appears only
	// when the stops do not all fit on this screen — a short course never
	// shows an empty box to type into.
	import RideDialog from './ride/RideDialog.svelte';
	import { sweepCandidates, filterStops } from '$lib/sweepPick';
	import type { StationView } from '$lib/types';

	let {
		stations,
		defaultCheckpointId,
		sweepLabel = 'SWEEP',
		stopMiles,
		onConfirm,
		onCancel
	}: {
		stations: StationView[];
		defaultCheckpointId: string;
		/** The net's configured name for the sweep ("SWEEP", "TAIL", …). */
		sweepLabel?: string;
		/** Course mile of each stop by id, when a course line is loaded. */
		stopMiles?: Map<string, number>;
		onConfirm: (cpId: string) => Promise<void>;
		onCancel: () => void;
	} = $props();

	const sweepName = $derived(sweepLabel.charAt(0).toUpperCase() + sweepLabel.slice(1).toLowerCase());

	let candidates = $derived(sweepCandidates(stations));
	let query = $state('');
	let shown = $derived(filterStops(candidates, query));

	// Preselect the stop the sweep is heading for, else the first one left.
	let selected = $state('');
	$effect(() => {
		if (selected && candidates.some((s) => s.checkpointId === selected)) return;
		selected = nextId;
	});

	// Typing a stop down to exactly one match selects it, so "3 ⏎" commits.
	$effect(() => {
		if (query.trim() && shown.length === 1) selected = shown[0].checkpointId;
	});

	/** The stop the sweep is heading for — the backend's answer when it has
	 *  one, else the first stop not yet passed, which is the same thing on a
	 *  course walked in order. Tagged in the list and preselected. */
	let nextId = $derived(
		candidates.find((s) => s.checkpointId === defaultCheckpointId)?.checkpointId ?? candidates[0]?.checkpointId ?? ''
	);

	let selectedStop = $derived(candidates.find((s) => s.checkpointId === selected) ?? null);
	let submitting = $state(false);

	async function confirm() {
		if (!selected || submitting) return;
		submitting = true;
		try {
			await onConfirm(selected);
		} catch {
			// markSweepPassed has already said why in a toast; stay open so the
			// operator can retry or pick a different stop.
		} finally {
			submitting = false;
		}
	}

	// ---- the search field appears only when the list overflows ----
	let listEl = $state<HTMLElement | null>(null);
	let needsSearch = $state(false);
	$effect(() => {
		const list = listEl;
		const body = list?.closest<HTMLElement>('.rd-body');
		if (!list || !body) return;
		// Sticky: once shown it stays, so the field cannot vanish under the
		// operator's thumb as their typing shortens the list.
		const measure = () => {
			if (!needsSearch && body.scrollHeight > body.clientHeight + 1) needsSearch = true;
		};
		const ro = new ResizeObserver(measure);
		ro.observe(body);
		measure();
		return () => ro.disconnect();
	});

	// ---- radio group keyboard model: one tab stop, arrows move the choice ----
	function focusStop(id: string) {
		listEl?.querySelector<HTMLElement>(`[data-cp="${CSS.escape(id)}"]`)?.focus();
	}

	function handleListKey(e: KeyboardEvent) {
		const i = shown.findIndex((s) => s.checkpointId === selected);
		let next = -1;
		if (e.key === 'ArrowDown' || e.key === 'ArrowRight') next = Math.min(shown.length - 1, i + 1);
		else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') next = Math.max(0, i - 1);
		else if (e.key === 'Home') next = 0;
		else if (e.key === 'End') next = shown.length - 1;
		else if (e.key === 'Enter') {
			e.preventDefault();
			confirm();
			return;
		}
		if (next < 0 || !shown[next]) return;
		e.preventDefault();
		selected = shown[next].checkpointId;
		focusStop(selected);
	}

	function handleSearchKey(e: KeyboardEvent) {
		if (e.key === 'Enter' && selectedStop && shown.includes(selectedStop)) {
			e.preventDefault();
			confirm();
		} else if (e.key === 'ArrowDown' && shown[0]) {
			e.preventDefault();
			if (!shown.some((s) => s.checkpointId === selected)) selected = shown[0].checkpointId;
			focusStop(selected);
		}
	}

	// Open with the preselected stop focused and in view.
	let didFocus = false;
	$effect(() => {
		if (didFocus || !listEl || !selected) return;
		didFocus = true;
		requestAnimationFrame(() => {
			const el = listEl?.querySelector<HTMLElement>(`[data-cp="${CSS.escape(selected)}"]`);
			el?.focus({ preventScroll: true });
			el?.scrollIntoView({ block: 'nearest' });
		});
	});

	function stateText(s: StationView): string {
		return s.closure.state === 'riders_clear' ? 'riders clear' : 'open';
	}

	function mileText(s: StationView): string {
		const m = stopMiles?.get(s.checkpointId);
		return m == null ? '' : `mi ${m.toFixed(1)}`;
	}
</script>

<RideDialog title="Mark {sweepName.toLowerCase()} passed" titleId="spc-title" onClose={onCancel} closeDisabled={submitting}>
	{#if candidates.length === 0}
		<!-- Every stop is already past the sweep: an empty list over a
		     permanently-disabled commit reads as a broken dialog. -->
		<p class="spc-empty">{sweepName} has already passed every stop on the course.</p>
	{:else}
		<p class="spc-lede" id="spc-lede">Which stop has {sweepName.toLowerCase()} just passed?</p>

		{#if needsSearch}
			<div class="spc-search">
				<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" aria-hidden="true"><circle cx="7" cy="7" r="4.5" /><path d="M10.5 10.5L14 14" /></svg>
				<input
					type="search"
					class="spc-search-input"
					placeholder="Stop name or number"
					aria-label="Find a stop by name or number"
					autocomplete="off"
					enterkeyhint="done"
					bind:value={query}
					onkeydown={handleSearchKey}
				/>
			</div>
		{/if}

		<!-- svelte-ignore a11y_interactive_supports_focus -->
		<div class="spc-list" role="radiogroup" aria-labelledby="spc-lede" bind:this={listEl} onkeydown={handleListKey}>
			{#each shown as s (s.checkpointId)}
				{@const on = s.checkpointId === selected}
				{@const mile = mileText(s)}
				<button
					type="button"
					class="spc-stop"
					class:spc-stop--on={on}
					role="radio"
					aria-checked={on}
					tabindex={on || (!shown.some((x) => x.checkpointId === selected) && s === shown[0]) ? 0 : -1}
					data-cp={s.checkpointId}
					onclick={() => (selected = s.checkpointId)}
				>
					<span class="spc-seq" aria-hidden="true">{s.sequenceNumber}</span>
					<span class="spc-main">
						<span class="spc-name"><span class="sr-only">Stop {s.sequenceNumber}, </span>{s.label}</span>
						<span class="spc-meta">
							{#if s.checkpointId === nextId}<span class="spc-next">Next for {sweepName.toLowerCase()}</span>{/if}
							{#if mile}<span>{mile}</span>{/if}
							<span class:spc-clear={s.closure.state === 'riders_clear'}>{stateText(s)}</span>
						</span>
					</span>
					<span class="spc-check" aria-hidden="true">
						{#if on}
							<svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 9.5l3.2 3L14 5.5" /></svg>
						{/if}
					</span>
				</button>
			{:else}
				<p class="spc-none">No stop matches “{query.trim()}”.</p>
			{/each}
		</div>
	{/if}

	{#snippet footer()}
		{#if candidates.length === 0}
			<button type="button" class="spc-btn spc-cancel" onclick={onCancel}>Close</button>
		{:else}
			<button type="button" class="spc-btn spc-cancel" onclick={onCancel} disabled={submitting}>Cancel</button>
			<button
				type="button"
				class="spc-btn spc-confirm"
				disabled={submitting || !selectedStop}
				aria-busy={submitting}
				onclick={confirm}
			>
				{#if submitting}
					Saving…
				{:else if selectedStop}
					{sweepName} passed #{selectedStop.sequenceNumber}<span class="sr-only"> {selectedStop.label}</span>
				{:else}
					Pick a stop
				{/if}
			</button>
		{/if}
	{/snippet}
</RideDialog>

<style>
	.spc-lede,
	.spc-empty {
		margin: 0;
		font-size: var(--font-sm, 0.875rem);
		color: var(--color-text-muted);
	}

	/* Pinned to the top of the dialog's scrolling body so it stays reachable
	   however far down the list the operator has scrolled. */
	.spc-search {
		position: sticky;
		top: calc(var(--space-md) * -1);
		z-index: 1;
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		min-height: 48px;
		margin: 0 calc(var(--space-md) * -1);
		padding: var(--space-sm) var(--space-md);
		background: var(--color-surface);
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text-muted);
	}

	.spc-search-input {
		flex: 1;
		min-width: 0;
		min-height: 44px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		/* 16px: anything smaller makes iOS zoom the page on focus. */
		font: inherit;
		font-size: 16px;
	}

	.spc-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.spc-stop {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		min-height: 56px;
		padding: var(--space-xs) var(--space-sm);
		background: var(--color-bg);
		border: 1px solid color-mix(in srgb, var(--color-text) 16%, transparent);
		border-radius: var(--radius-md);
		color: var(--color-text);
		font: inherit;
		text-align: left;
		cursor: pointer;
		transition: border-color var(--duration-fast, 0.15s), background var(--duration-fast, 0.15s);
	}

	.spc-stop:hover {
		border-color: color-mix(in srgb, var(--color-text) 40%, transparent);
	}

	/* Selected: a sweep-coloured 2px ring AND a check glyph — never colour
	   alone, since this is read in sunlight. The inset shadow draws the second
	   pixel so the row does not shift by a pixel when it is chosen. */
	.spc-stop--on {
		border-color: var(--color-ride-sweep);
		box-shadow: inset 0 0 0 1px var(--color-ride-sweep);
		background: color-mix(in srgb, var(--color-ride-sweep) 12%, var(--color-bg));
	}

	.spc-seq {
		flex: 0 0 auto;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 32px;
		height: 32px;
		padding: 0 6px;
		border-radius: var(--radius-full, 999px);
		border: 1.5px solid currentColor;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		color: var(--color-text-muted);
	}

	.spc-stop--on .spc-seq {
		color: var(--color-ride-sweep);
	}

	.spc-main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.spc-name {
		font-weight: 600;
		line-height: 1.25;
		overflow-wrap: anywhere;
	}

	.spc-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0 var(--space-sm);
		font-size: var(--ride-t-label, 0.75rem);
		font-variant-numeric: tabular-nums;
		color: var(--color-text-muted);
	}

	.spc-next {
		font-weight: 700;
		color: var(--color-ride-sweep);
	}

	.spc-clear {
		color: var(--color-success);
	}

	.spc-check {
		flex: 0 0 24px;
		display: inline-flex;
		justify-content: center;
		color: var(--color-ride-sweep);
	}

	.spc-none {
		margin: 0;
		padding: var(--space-md) 0;
		text-align: center;
		color: var(--color-text-muted);
	}

	.spc-btn {
		min-height: 44px;
		padding: 0 var(--space-md);
		border-radius: var(--radius-sm);
		font: inherit;
		font-weight: 700;
		cursor: pointer;
	}

	.spc-cancel {
		background: none;
		border: 1px solid var(--color-primary);
		color: var(--color-text);
	}

	.spc-confirm {
		min-width: 9rem;
		background: var(--color-ride-sweep);
		border: none;
		color: var(--color-on-accent);
	}

	.spc-confirm:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* Phone: the dialog is a bottom sheet; the two buttons share the width so
	   the commit is a thumb-sized target, not a corner. */
	@media (max-width: 640px) {
		.spc-btn {
			flex: 1;
			min-height: 48px;
		}
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
</style>
