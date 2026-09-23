<script lang="ts">
	// Phone-only 44px peek line inside BottomSheet's peekContent (spec §1/§9).
	// NOT a strip — exactly three facts: sweep mile, next shutoff countdown,
	// and the highest open tier + count. Everything else is one tap into the
	// situation tab, rendered vertically by RideSituation.svelte.
	import { rideRail, rideSweep, rideShutoffs, rideTierCounts } from '$lib/stores/ride';
	import { openRideSituation } from '$lib/stores/ui';
	import { tierStyle } from '$lib/rideMeta';
	import RideTierGlyph from './RideTierGlyph.svelte';

	let topTier = $derived($rideTierCounts[0] ?? null);
	let borderVar = $derived(topTier ? tierStyle(topTier.tier).colorVar : '--color-primary');

	let sweepText = $derived($rideSweep.age === 'never' ? '▲ —' : `▲ ${$rideRail.sweepMileText}`);
	let shutoffText = $derived($rideShutoffs.next && $rideShutoffs.countdown ? `╫ −${$rideShutoffs.countdown}` : '');

	let ariaLabel = $derived(
		`Ride status: ${sweepText}${shutoffText ? `, next shutoff in ${$rideShutoffs.countdown}` : ''}${
			topTier ? `, ${topTier.count} ${topTier.tier.label} open` : ', nothing open'
		}. Open ride situation.`
	);
</script>

<button class="ride-peek" style="border-left-color: var({borderVar})" onclick={openRideSituation} aria-label={ariaLabel}>
	<span class="ride-peek-fact">{sweepText}</span>
	{#if shutoffText}<span class="ride-peek-fact">{shutoffText}</span>{/if}
	{#if topTier}
		<!-- Glyph + COUNT alone is one-and-a-half channels: rank 2 and rank 3
		     share amber and the same triangle family, so on a phone the only
		     thing separating PRIORITY from HIGH is a fill. The two-letter code
		     is the spec §10 text channel and it is not optional here. -->
		<span class="ride-peek-tier">
			<RideTierGlyph tier={topTier.tier} size={14} />
			<span class="ride-peek-code">{tierStyle(topTier.tier).code}</span>
			{topTier.count}
		</span>
	{/if}
	<span class="ride-peek-chevron" aria-hidden="true">›</span>
</button>

<style>
	.ride-peek {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		width: 100%;
		min-height: 44px;
		box-sizing: border-box;
		padding: 0 var(--space-sm);
		background: var(--color-surface);
		border: none;
		border-left: 3px solid;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
		font-variant-numeric: tabular-nums;
	}

	.ride-peek:hover,
	.ride-peek:focus-visible {
		background: var(--color-primary);
	}

	.ride-peek-fact {
		font-size: var(--ride-t-body);
		white-space: nowrap;
	}

	.ride-peek-tier {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		font-size: var(--ride-t-body);
		font-weight: 700;
	}

	.ride-peek-code {
		font-size: var(--ride-t-label);
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
	}

	.ride-peek-chevron {
		color: var(--color-text-muted);
		flex-shrink: 0;
		margin-left: auto;
	}
</style>
