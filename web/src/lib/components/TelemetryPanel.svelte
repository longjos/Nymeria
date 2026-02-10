<script lang="ts">
	import { telemetryStations, selectedTelemetryStation } from '$lib/stores/telemetry';
	import { stations } from '$lib/stores/stations';
	import type { Station } from '$lib/types';
	import TelemetryStationCard from './TelemetryStationCard.svelte';
	import TelemetryDetail from './TelemetryDetail.svelte';

	let {
		onFlyTo
	}: {
		onFlyTo?: (lat: number, lon: number) => void;
	} = $props();

	let selectedStation = $derived.by(() => {
		const callsign = $selectedTelemetryStation;
		if (!callsign) return null;
		const s = $stations.get(callsign);
		if (s?.telemetry) return s;
		return $telemetryStations.find(
			(s) => (s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign) === callsign
		) ?? null;
	});

	function handleStationClick(callsign: string) {
		selectedTelemetryStation.set(callsign);
	}

	function handleBack() {
		selectedTelemetryStation.set(null);
	}
</script>

<div class="tel-panel">
	{#if selectedStation}
		<TelemetryDetail
			station={selectedStation}
			onBack={handleBack}
			{onFlyTo}
		/>
	{:else}
		<div class="tel-panel-header">
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M1 12l3-4 3 2 4-6 4 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
			</svg>
			<h2 class="panel-title">Telemetry</h2>
			<span class="station-count">{$telemetryStations.length}</span>
		</div>

		{#if $telemetryStations.length === 0}
			<div class="tel-empty">
				<p>No telemetry stations heard yet.</p>
				<p class="tel-empty-hint">Telemetry data will appear automatically when stations transmit T# packets via APRS.</p>
			</div>
		{:else}
			<div class="tel-list">
				{#each $telemetryStations as station (station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign)}
					<TelemetryStationCard
						{station}
						onClick={handleStationClick}
					/>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.tel-panel {
		padding: var(--space-md);
	}

	.tel-panel-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-bottom: var(--space-md);
		color: var(--color-text);
	}

	.panel-title {
		font-size: 0.95rem;
		font-weight: 700;
		flex: 1;
	}

	.station-count {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-text-muted);
		background: var(--color-primary);
		padding: 2px 8px;
		border-radius: var(--radius-full);
	}

	.tel-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.tel-empty {
		text-align: center;
		padding: var(--space-2xl) var(--space-md);
		color: var(--color-text-muted);
	}

	.tel-empty p {
		font-size: 0.85rem;
		margin-bottom: var(--space-sm);
	}

	.tel-empty-hint {
		font-size: 0.75rem;
		opacity: 0.7;
	}
</style>
