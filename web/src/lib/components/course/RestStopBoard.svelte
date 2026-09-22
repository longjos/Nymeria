<script lang="ts">
	// Status per rest stop, sequence order (back of the course first — the
	// backend already returns Stations ordered by sequence asc). The summary
	// strip reads off `rideStops`, the SAME derivation the ride strip's B4
	// zone uses, and off STOP_GLYPH, the same glyph table — they previously
	// each rolled their own buckets ("sweep_passed" counted as open by one
	// and as awaiting-sweep by the other) and reported different numbers for
	// the same net at the same instant.
	import type { StationView, CourseConfig } from '$lib/types';
	import { rideStops } from '$lib/stores/ride';
	import { STOP_GLYPHS } from '$lib/rideMeta';
	import StationRow from './StationRow.svelte';

	let {
		stations, config, clearThroughSeq, canOperate, isNcs, ncsCallsign, onFlyTo
	}: {
		stations: StationView[];
		config: CourseConfig | null;
		clearThroughSeq: number;
		canOperate: boolean;
		isNcs: boolean;
		ncsCallsign: string;
		onFlyTo?: (lat: number, lon: number, zoom?: number) => void;
	} = $props();

	let sorted = $derived([...stations].sort((a, b) => a.sequenceNumber - b.sequenceNumber));
</script>

<div class="rsb">
	<div class="rsb-summary" role="status">
		<span>{STOP_GLYPHS.open} {$rideStops.open} open</span>
		<span>{STOP_GLYPHS.awaitingSweep} {$rideStops.awaitingSweep} awaiting sweep</span>
		<span>{STOP_GLYPHS.readyToClose} {$rideStops.readyToClose} ready to close</span>
		<span>{STOP_GLYPHS.closed} {$rideStops.closed} closed</span>
	</div>

	<div class="rsb-list">
		{#each sorted as station (station.checkpointId)}
			<StationRow
				{station}
				config={config ?? { netId: '', division: '', sweepLabel: 'SWEEP', leadLabel: 'LEAD', closeRequiresSweep: true, autoSweepFromPassage: true, updatedAt: '' }}
				cleared={station.sequenceNumber <= clearThroughSeq}
				{canOperate}
				{isNcs}
				{ncsCallsign}
				{onFlyTo}
			/>
		{/each}
	</div>
</div>

<style>
	.rsb {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.rsb-summary {
		display: flex;
		gap: var(--space-md);
		padding: var(--space-sm) var(--space-md);
		font-size: 0.75rem;
		color: var(--color-text-muted);
		border-bottom: 1px solid var(--color-primary);
		position: sticky;
		top: 0;
		background: var(--color-surface);
		z-index: 1;
	}

	.rsb-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: var(--space-sm) var(--space-md);
	}
</style>
