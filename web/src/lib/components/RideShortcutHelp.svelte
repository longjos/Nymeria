<script lang="ts">
	// '?' inside ride mode — a small overlay listing the ride status strip's
	// keyboard accelerators (spec §5).
	let { onClose }: { onClose: () => void } = $props();

	const rows: [string, string][] = [
		['q', 'Seed the palette with "lead " (LEAD passage)'],
		['w', 'Seed the palette with "sweep " (SWEEP passage)'],
		['a', 'Acknowledge the top unacknowledged item'],
		['r', 'Open the pending / awaiting-reply list'],
		['x', 'Open the shift-relief briefing'],
		['e', 'Seed the palette locked to the rank-1 tier (incident)'],
		['n / s / c', 'Seed the palette: incident / sag / close'],
		['Esc', 'Close a dialog (never dismisses the interrupt banner)']
	];

	function handleKeydown(e: KeyboardEvent): void {
		if (e.key === 'Escape') onClose();
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="rsh-backdrop" role="presentation" onclick={onClose}>
	<div class="rsh" role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="rsh-title" data-blocks-escape="true" onclick={(e) => e.stopPropagation()}>
		<div class="rsh-head">
			<h2 id="rsh-title" class="rsh-title">Ride strip shortcuts</h2>
			<button class="rsh-close" onclick={onClose} aria-label="Close">&times;</button>
		</div>
		<div class="rsh-list">
			{#each rows as [key, desc] (key)}
				<div class="rsh-row">
					<kbd class="rsh-key">{key}</kbd>
					<span class="rsh-desc">{desc}</span>
				</div>
			{/each}
		</div>
	</div>
</div>

<style>
	.rsh-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.rsh {
		width: 100%;
		max-width: 360px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		padding: var(--space-md);
	}

	.rsh-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: var(--space-sm);
	}

	.rsh-title {
		font-size: 1rem;
		font-weight: 700;
	}

	.rsh-close {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.2rem;
		cursor: pointer;
		min-width: 32px;
		min-height: 32px;
	}

	.rsh-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.rsh-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.rsh-key {
		min-width: 28px;
		text-align: center;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 2px 6px;
		font-size: 0.75rem;
		font-weight: 700;
	}

	.rsh-desc {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}
</style>
