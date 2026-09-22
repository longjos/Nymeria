<script lang="ts">
	// The B1/NET zone's phase chip (spec §8). Phase is ALWAYS operator-set —
	// this chip opens PhaseChangeDialog; it never switches the phase itself.
	import type { RidePhaseStatus } from '$lib/types';
	import { phaseLabel } from '$lib/rideMeta';

	let {
		state,
		canSet,
		onOpen
	}: {
		state: RidePhaseStatus | null;
		canSet: boolean;
		onOpen: () => void;
	} = $props();

	let suggestion = $derived(state?.suggestion.phase ?? '');
</script>

<button
	class="phase-chip"
	class:has-suggestion={!!suggestion}
	disabled={!canSet}
	title={canSet ? 'Change ride phase' : 'Only net control or an admin can change the ride phase'}
	onclick={(e) => {
		e.stopPropagation();
		onOpen();
	}}
>
	{phaseLabel(state?.phase ?? 'pre-start')}
	{#if suggestion}
		<span class="phase-suggest">⟳ {phaseLabel(suggestion)}?</span>
	{/if}
</button>

<style>
	.phase-chip {
		min-height: 22px;
		padding: 2px var(--space-sm);
		background: var(--color-primary);
		border: none;
		border-radius: var(--radius-full);
		color: var(--color-text);
		font-size: var(--ride-t-label);
		font-weight: 700;
		letter-spacing: 0.03em;
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		width: fit-content;
	}

	.phase-chip:disabled {
		cursor: default;
		opacity: 0.6;
	}

	.phase-chip:not(:disabled):hover,
	.phase-chip:not(:disabled):focus-visible {
		background: var(--color-accent);
	}

	.phase-suggest {
		color: var(--color-warning);
		animation: phase-pulse 1.2s ease-in-out 1;
	}

	@keyframes phase-pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.4;
		}
	}
</style>
