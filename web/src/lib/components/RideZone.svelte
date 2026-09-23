<script lang="ts">
	// One tile in the ride status strip's zone row (spec §1/§5/§10). NOT a
	// <button> — B6 (Acknowledge) and B1 (the phase chip) each render a real
	// inner <button> via `children`, and a button inside a button is a Svelte 5
	// build error. The whole tile is a `div role="button"` instead.
	import type { Snippet } from 'svelte';
	import type { ZoneLine, RideZoneId } from '$lib/stores/ride';

	let {
		id,
		label,
		lines,
		span = 1,
		tone = 'normal',
		borderVar,
		ariaSentence,
		focused,
		onActivate,
		children
	}: {
		id: RideZoneId;
		label: string;
		lines: ZoneLine[];
		span?: 1 | 2 | 3;
		tone?: 'normal' | 'muted' | 'warning' | 'success';
		borderVar?: string;
		ariaSentence: string;
		focused: boolean;
		onActivate: () => void;
		children?: Snippet;
	} = $props();

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			onActivate();
		}
	}
</script>

<div class="ride-zone tone-{tone}" class:ride-zone--emergency={id === 'traffic' && span === 3} style="grid-column: span {span}" role="group" aria-labelledby="{id}-label">
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<div
		class="ride-zone-hit"
		role="button"
		tabindex={focused ? 0 : -1}
		data-border={borderVar ? 'true' : undefined}
		style={borderVar ? `--zone-border: var(${borderVar})` : undefined}
		onclick={onActivate}
		onkeydown={handleKeydown}
		aria-label={ariaSentence}
	>
		<span id="{id}-label" class="ride-zone-label">{label}</span>
		{#each lines as line, i (i)}
			<span class="ride-zone-{line.size === 'label' ? 'meta' : line.size} tone-{line.tone ?? 'normal'}">{line.text}</span>
		{/each}
	</div>
	{#if children}
		<div class="ride-zone-extra">
			{@render children()}
		</div>
	{/if}
</div>

<style>
	.ride-zone {
		position: relative;
		border-radius: var(--radius-sm);
		background: var(--color-bg);
		min-width: 0;
		min-height: 0;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	/* Row B is a 66px contract. Label + value + body + meta at the previous
	   line-heights/padding measured 82px, so every tile's third line (the
	   shutoff countdown, `! N unacked`, the relief countdown) was being
	   amputated by the strip's overflow:hidden. These are the tightest
	   metrics that still clear 4.5:1 and keep the value line at 1.375rem. */
	.ride-zone-hit {
		flex: 1 1 auto;
		min-height: 0;
		display: flex;
		flex-direction: column;
		justify-content: center;
		gap: 0;
		padding: var(--space-2xs) var(--space-sm);
		cursor: pointer;
		border-radius: var(--radius-sm);
		border-left: 3px solid transparent;
		min-width: 0;
	}

	.ride-zone-hit[data-border] {
		border-left-color: var(--zone-border);
	}

	.ride-zone-hit:hover,
	.ride-zone-hit:focus-visible {
		background: var(--color-primary);
	}

	.ride-zone-label {
		font-size: var(--ride-t-label);
		line-height: 1.15;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.ride-zone-value {
		font-size: var(--ride-t-value);
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		line-height: 1.05;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--color-text);
	}

	.ride-zone-body {
		font-size: var(--ride-t-body);
		line-height: 1.2;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--color-text);
	}

	/* A third-line VALUE ("11.4mph · 6m ago", "seats 6/9"), not a zone
	   header. It previously reused .ride-zone-label, whose uppercase turned
	   "6m ago" into "6M" — read as six MILES on a tile headlined "mi 17.4". */
	.ride-zone-meta {
		font-size: var(--ride-t-label);
		line-height: 1.2;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--color-text-muted);
	}

	.ride-zone-label:empty,
	.ride-zone-meta:empty,
	.ride-zone-body:empty {
		display: none;
	}

	.tone-muted {
		color: var(--color-text-muted);
	}

	.tone-warning {
		color: var(--color-warning);
	}

	.tone-success {
		color: var(--color-success);
	}

	/* The extra slot (phase chip, Acknowledge) used to stack UNDER the lines
	   and push the tile past the row height. It is now a sibling column, so
	   the emergency Acknowledge button sits to the right of the incident
	   text exactly as the spec's EMERGENCY wireframe draws it. */
	.ride-zone-extra {
		display: flex;
		align-items: center;
		flex: 0 0 auto;
		min-width: 0;
		padding: 0 var(--space-sm) 0 0;
	}

	.ride-zone:has(.ride-zone-extra) {
		flex-direction: row;
		align-items: stretch;
	}
</style>
