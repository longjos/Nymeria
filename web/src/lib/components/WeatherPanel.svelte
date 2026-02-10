<script lang="ts">
	import { weatherStations, weatherConfig, selectedWeatherStation, initWeatherStore } from '$lib/stores/weather';
	import { stations } from '$lib/stores/stations';
	import type { Station } from '$lib/types';
	import WeatherStationCard from './WeatherStationCard.svelte';
	import WeatherDetail from './WeatherDetail.svelte';

	let {
		onFlyTo
	}: {
		onFlyTo?: (lat: number, lon: number) => void;
	} = $props();

	// Ensure weather config is loaded
	initWeatherStore();

	let selectedStation = $derived.by(() => {
		const callsign = $selectedWeatherStation;
		if (!callsign) return null;
		// Try to find the station by key in main stations map
		const s = $stations.get(callsign);
		if (s?.weather) return s;
		// Try with weather stations list
		return $weatherStations.find(
			(s) => (s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign) === callsign
		) ?? null;
	});

	function handleStationClick(callsign: string) {
		selectedWeatherStation.set(callsign);
	}

	function handleBack() {
		selectedWeatherStation.set(null);
	}
</script>

<div class="wx-panel">
	{#if selectedStation}
		<WeatherDetail
			station={selectedStation}
			config={$weatherConfig}
			onBack={handleBack}
			{onFlyTo}
		/>
	{:else}
		<div class="wx-panel-header">
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<circle cx="8" cy="8" r="3" stroke="currentColor" stroke-width="1.5"/>
				<path d="M8 1v2M8 13v2M1 8h2M13 8h2M3 3l1.5 1.5M11.5 11.5L13 13M13 3l-1.5 1.5M4.5 11.5L3 13" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
			</svg>
			<h2 class="panel-title">Weather</h2>
			<span class="station-count">{$weatherStations.length}</span>
		</div>

		{#if $weatherStations.length === 0}
			<div class="wx-empty">
				<p>No weather stations heard yet.</p>
				<p class="wx-empty-hint">Weather data will appear automatically when stations with weather sensors report in via APRS.</p>
			</div>
		{:else}
			<div class="wx-list">
				{#each $weatherStations as station (station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign)}
					<WeatherStationCard
						{station}
						alerts={$weatherConfig.alerts}
						onClick={handleStationClick}
					/>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.wx-panel {
		padding: var(--space-md);
	}

	.wx-panel-header {
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

	.wx-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.wx-empty {
		text-align: center;
		padding: var(--space-2xl) var(--space-md);
		color: var(--color-text-muted);
	}

	.wx-empty p {
		font-size: 0.85rem;
		margin-bottom: var(--space-sm);
	}

	.wx-empty-hint {
		font-size: 0.75rem;
		opacity: 0.7;
	}
</style>
