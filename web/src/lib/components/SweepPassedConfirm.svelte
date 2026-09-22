<script lang="ts">
	// One-field confirm popover for the rail's sweep marker (spec §5): a wrong
	// sweep passage corrupts the most-asked-about number in the operation, so
	// this is a cheap insurance click rather than a blind commit.
	import type { StationView } from '$lib/types';

	let {
		stations,
		defaultCheckpointId,
		onConfirm,
		onCancel
	}: {
		stations: StationView[];
		defaultCheckpointId: string;
		onConfirm: (cpId: string) => Promise<void>;
		onCancel: () => void;
	} = $props();

	let candidates = $derived(
		[...stations].filter((s) => s.closure.state !== 'sweep_passed' && s.closure.state !== 'closed').sort((a, b) => a.sequenceNumber - b.sequenceNumber)
	);
	let selected = $state(defaultCheckpointId);
	let submitting = $state(false);

	async function confirm() {
		if (!selected || submitting) return;
		submitting = true;
		try {
			await onConfirm(selected);
		} finally {
			submitting = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onCancel();
	}

	let el = $state<HTMLElement | null>(null);
	$effect(() => {
		el?.querySelector<HTMLElement>('select')?.focus();
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="spc-backdrop" role="presentation" onclick={onCancel}>
	<div class="spc" bind:this={el} role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="spc-title" data-blocks-escape="true" onclick={(e) => e.stopPropagation()}>
		<h3 id="spc-title" class="spc-title">Mark sweep passed…</h3>
		{#if candidates.length === 0}
			<!-- Every stop is already past the sweep: an empty <select> over a
			     permanently-disabled Confirm reads as a broken dialog. -->
			<p class="spc-empty">The sweep has already passed every stop on the course.</p>
			<div class="spc-actions">
				<button class="spc-btn spc-cancel" onclick={onCancel}>Close</button>
			</div>
		{:else}
			<select class="spc-select" bind:value={selected}>
				{#each candidates as s (s.checkpointId)}
					<option value={s.checkpointId}>#{s.sequenceNumber} {s.label}</option>
				{/each}
			</select>
			<div class="spc-actions">
				<button class="spc-btn spc-cancel" onclick={onCancel}>Cancel</button>
				<button class="spc-btn spc-confirm" disabled={submitting || !selected} aria-busy={submitting} onclick={confirm}>{submitting ? 'Saving…' : 'Confirm'}</button>
			</div>
		{/if}
	</div>
</div>

<style>
	.spc-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.spc {
		width: 100%;
		max-width: 300px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.spc-title {
		font-size: 0.95rem;
		font-weight: 700;
	}

	.spc-empty {
		font-size: 0.82rem;
		color: var(--color-text-muted);
	}

	.spc-select {
		width: 100%;
		min-height: 40px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: 0 var(--space-sm);
		font: inherit;
	}

	.spc-actions {
		display: flex;
		gap: var(--space-sm);
		justify-content: flex-end;
	}

	.spc-btn {
		min-height: 40px;
		padding: 0 var(--space-md);
		border-radius: var(--radius-sm);
		font-weight: 700;
		cursor: pointer;
	}

	.spc-cancel {
		background: none;
		border: 1px solid var(--color-primary);
		color: var(--color-text);
	}

	.spc-confirm {
		background: var(--color-ride-sweep);
		border: none;
		color: var(--color-on-accent);
	}

	.spc-confirm:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
