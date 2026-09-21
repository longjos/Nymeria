<script lang="ts">
	import { weatherStations, weatherConfig, selectedWeatherStation, initWeatherStore } from '$lib/stores/weather';
	import { stations } from '$lib/stores/stations';
	import { wxPanelTab } from '$lib/stores/ui';
	import { wxUnackedCount, wxLinkStatus, wxSelectedAlertId } from '$lib/stores/wxAlerts';
	import type { Station } from '$lib/types';
	import WeatherStationCard from './WeatherStationCard.svelte';
	import WeatherDetail from './WeatherDetail.svelte';
	import WxAlertPanel from './WxAlertPanel.svelte';

	let {
		onFlyTo
	}: {
		onFlyTo?: (lat: number, lon: number) => void;
	} = $props();

	// Ensure weather config is loaded
	initWeatherStore();

	const TABS: Array<{ key: 'stations' | 'alerts'; id: string; pane: string }> = [
		{ key: 'stations', id: 'wx-tab-stations', pane: 'wx-pane-stations' },
		{ key: 'alerts', id: 'wx-tab-alerts', pane: 'wx-pane-alerts' }
	];

	function handleTabKeydown(e: KeyboardEvent): void {
		if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
		e.preventDefault();
		const idx = TABS.findIndex((t) => t.key === $wxPanelTab);
		const next = e.key === 'ArrowRight' ? (idx + 1) % TABS.length : (idx - 1 + TABS.length) % TABS.length;
		wxPanelTab.set(TABS[next].key);
		(document.getElementById(TABS[next].id) as HTMLElement | null)?.focus();
	}

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
	<div class="wx-panel-header">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<circle cx="8" cy="8" r="3" stroke="currentColor" stroke-width="1.5"/>
			<path d="M8 1v2M8 13v2M1 8h2M13 8h2M3 3l1.5 1.5M11.5 11.5L13 13M13 3l-1.5 1.5M4.5 11.5L3 13" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
		</svg>
		<h2 class="panel-title">Weather</h2>
	</div>

	<div class="wx-tabs" role="tablist" aria-label="Weather sections" tabindex="-1" onkeydown={handleTabKeydown}>
		<button
			class="wx-tab tab"
			role="tab"
			id="wx-tab-stations"
			tabindex={$wxPanelTab === 'stations' ? 0 : -1}
			aria-selected={$wxPanelTab === 'stations'}
			aria-controls="wx-pane-stations"
			class:active={$wxPanelTab === 'stations'}
			onclick={() => wxPanelTab.set('stations')}
		>
			<span class="tab-label">Stations</span>
			<span class="tab-count">{$weatherStations.length}</span>
		</button>
		<button
			class="wx-tab tab"
			role="tab"
			id="wx-tab-alerts"
			tabindex={$wxPanelTab === 'alerts' ? 0 : -1}
			aria-selected={$wxPanelTab === 'alerts'}
			aria-controls="wx-pane-alerts"
			class:active={$wxPanelTab === 'alerts'}
			onclick={() => {
				// Clicking the tab you are already on is the conventional "go
				// back to the top of this section" gesture; it used to be inert
				// while an alert detail was open.
				if ($wxPanelTab === 'alerts') wxSelectedAlertId.set(null);
				wxPanelTab.set('alerts');
			}}
		>
			<span class="tab-label">NWS Alerts</span>
			{#if $wxUnackedCount > 0}
				<span class="tab-count tab-count-alert" aria-label="{$wxUnackedCount} unacknowledged">{$wxUnackedCount}</span>
			{/if}
			{#if $wxLinkStatus.state === 'down'}
				<span class="wx-tab-dot down" aria-hidden="true"></span>
			{/if}
		</button>
	</div>

	{#if $wxPanelTab === 'alerts'}
		<div id="wx-pane-alerts" role="tabpanel" aria-labelledby="wx-tab-alerts" tabindex="0">
			<WxAlertPanel {onFlyTo} />
		</div>
	{:else if selectedStation}
		<WeatherDetail
			station={selectedStation}
			config={$weatherConfig}
			onBack={handleBack}
			{onFlyTo}
		/>
	{:else}
		<div id="wx-pane-stations" role="tabpanel" aria-labelledby="wx-tab-stations" tabindex="0">
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
		</div>
	{/if}
</div>

<style>
	/* The header keeps its own horizontal padding; .wx-panel itself carries
	   none, so #wx-pane-alerts (WxAlertPanel — sticky provenance strip,
	   full-bleed rows) can reach the panel edge while #wx-pane-stations adds
	   its own padding back. */
	.wx-panel {
		display: flex;
		flex-direction: column;
	}

	.wx-panel-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-md) var(--space-md) 0;
		margin-bottom: var(--space-sm);
		color: var(--color-text);
	}

	.panel-title {
		font-size: 0.95rem;
		font-weight: 700;
		flex: 1;
	}

	/* Tabs — copied from NetControlPanel's .tabs/.tab/.tab-count recipe. */
	.wx-tabs {
		display: flex;
		align-items: stretch;
		gap: 2px;
		padding: 0 var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		margin-bottom: var(--space-sm);
	}

	.wx-tab {
		flex: 1 0 auto;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		min-height: 44px;
		padding: 0 var(--space-sm);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		font-weight: 500;
		letter-spacing: 0.01em;
		white-space: nowrap;
		cursor: pointer;
		transition: color var(--duration-fast), border-color var(--duration-fast);
	}

	.wx-tab:hover { color: var(--color-text); }
	.wx-tab:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
		border-radius: var(--radius-sm);
	}
	.wx-tab.active {
		color: var(--color-text);
		border-bottom-color: var(--color-accent);
	}

	.tab-count {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 18px;
		height: 18px;
		padding: 0 5px;
		border-radius: var(--radius-full);
		background: var(--color-primary);
		color: var(--color-text-muted);
		font-size: 0.68rem;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		line-height: 1;
		transition: background var(--duration-fast), color var(--duration-fast);
	}
	.wx-tab.active .tab-count {
		background: var(--color-accent);
		color: #fff;
	}

	.tab-count-alert,
	.wx-tab.active .tab-count-alert {
		background: var(--color-wx-watch);
		color: #000;
		animation: wx-tab-alert-pulse 2s ease-in-out infinite;
	}
	@keyframes wx-tab-alert-pulse {
		0%, 100% { box-shadow: 0 0 0 0 rgba(245, 158, 11, 0.55); }
		50% { box-shadow: 0 0 0 4px rgba(245, 158, 11, 0); }
	}
	@media (prefers-reduced-motion: reduce) {
		.tab-count-alert { animation: none; }
	}

	.wx-tab-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		flex-shrink: 0;
	}
	.wx-tab-dot.down { background: var(--color-wx-warning); }

	#wx-pane-stations {
		padding: 0 var(--space-md) var(--space-md);
	}

	#wx-pane-alerts {
		flex: 1;
		min-height: 0;
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
