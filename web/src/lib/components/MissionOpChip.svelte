<script lang="ts">
	/**
	 * One assigned-operator chip. Rendered identically in two places:
	 *  - the mission card's assigned-operators row
	 *  - the mission create form's "picked so far" row
	 * That equivalence is the point — what you pick while composing looks
	 * exactly like what you get once the mission exists.
	 */
	let {
		callsign,
		status,
		color,
		removable = false,
		onRemove
	}: {
		callsign: string;
		status: string;
		color: string;
		removable?: boolean;
		onRemove?: () => void;
	} = $props();
</script>

<div class="mission-op-chip">
	<span class="mission-op-dot" style="background: {color}"></span>
	<span class="mission-op-call">{callsign}</span>
	<span class="mission-op-status">{status}</span>
	{#if removable}
		<button
			type="button"
			class="mission-op-remove"
			title="Unassign {callsign}"
			aria-label="Unassign {callsign}"
			onclick={(e) => { e.stopPropagation(); onRemove?.(); }}
		>✕</button>
	{/if}
</div>

<style>
	.mission-op-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 4px 8px;
		font-size: 0.72rem;
		min-height: 28px;
	}

	.mission-op-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.mission-op-call {
		font-family: monospace;
		font-weight: 600;
	}

	.mission-op-status {
		color: var(--color-text-muted);
		font-size: 0.65rem;
		text-transform: uppercase;
	}

	.mission-op-remove {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		padding: 2px 6px;
		cursor: pointer;
		line-height: 1;
		min-width: 28px;
		min-height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.mission-op-remove:hover {
		color: #ef4444;
	}

	.mission-op-remove:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
		border-radius: var(--radius-sm);
	}
</style>
