<script lang="ts">
	import type { Station, WeatherReading, WeatherConfig } from '$lib/types';
	import { weatherReadings, weatherReadingsLoading, loadWeatherReadings, isAlertTriggered } from '$lib/stores/weather';
	import { formatTemp, formatWindSpeed, formatWindSpeedValue, formatPressure, formatRain,
		convertTemp, convertWindSpeed, convertPressure, convertRain,
		tempUnit, windUnit, pressureUnit, pressureLabel, rainUnit } from '$lib/units';
	import type { UnitSystem } from '$lib/units';
	import WeatherChart from './WeatherChart.svelte';

	let {
		station,
		config,
		onBack,
		onFlyTo
	}: {
		station: Station;
		config: WeatherConfig;
		onBack?: () => void;
		onFlyTo?: (lat: number, lon: number) => void;
	} = $props();

	let wx = $derived(station.weather);
	let key = $derived(station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign);
	let units = $derived((config.units || 'metric') as UnitSystem);

	let timeRange = $state<'24h' | '7d'>('24h');

	$effect(() => {
		const since = new Date(Date.now() - (timeRange === '24h' ? 24 * 3600000 : 7 * 24 * 3600000)).toISOString();
		loadWeatherReadings(key, since);
	});

	// Prepare chart data from readings (oldest first), applying unit conversion
	function chartData(readings: WeatherReading[], field: keyof WeatherReading, convert?: (v: number) => number): Array<{ time: string; value: number }> {
		const result: Array<{ time: string; value: number }> = [];
		for (let i = readings.length - 1; i >= 0; i--) {
			const r = readings[i];
			const val = r[field];
			if (val !== undefined && val !== null && typeof val === 'number') {
				result.push({ time: r.timestamp, value: convert ? convert(val) : val });
			}
		}
		return result;
	}

	let tempData = $derived(chartData($weatherReadings, 'temperature', (v) => convertTemp(v, units)));
	let windData = $derived(chartData($weatherReadings, 'windSpeed', (v) => convertWindSpeed(v, units)));
	let gustData = $derived(chartData($weatherReadings, 'windGust', (v) => convertWindSpeed(v, units)));
	let pressData = $derived(chartData($weatherReadings, 'pressure', (v) => convertPressure(v, units)));
	let humData = $derived(chartData($weatherReadings, 'humidity'));
	let rainData = $derived(chartData($weatherReadings, 'rain1h', (v) => convertRain(v, units)));

	function windDirLabel(dir?: number): string {
		if (dir === undefined) return '--';
		const dirs = ['N', 'NNE', 'NE', 'ENE', 'E', 'ESE', 'SE', 'SSE', 'S', 'SSW', 'SW', 'WSW', 'W', 'WNW', 'NW', 'NNW'];
		return dirs[Math.round(dir / 22.5) % 16];
	}
</script>

<div class="wx-detail">
	<div class="wx-detail-header">
		<button class="back-btn" onclick={onBack} title="Back">
			<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
				<path d="M10 3L5 8l5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
			</svg>
		</button>
		<span class="wx-detail-call">{key}</span>
		{#if station.position}
			<button class="fly-btn" onclick={() => onFlyTo?.(station.position!.lat, station.position!.lon)} title="Fly to station">
				<svg width="14" height="14" viewBox="0 0 14 14" fill="none">
					<path d="M7 1L2 13l5-3 5 3L7 1z" fill="currentColor"/>
				</svg>
			</button>
		{/if}
	</div>

	<!-- Current conditions -->
	<div class="wx-current">
		<div class="wx-hero-section">
			<div class="wx-hero-temp" class:alert={isAlertTriggered('temperature', wx?.temperature, config.alerts)}>
				{formatTemp(wx?.temperature, units)}
			</div>
			{#if wx?.windDir !== undefined}
				<div class="wx-wind-compass">
					<svg width="48" height="48" viewBox="0 0 48 48">
						<circle cx="24" cy="24" r="22" stroke="var(--color-primary)" stroke-width="1.5" fill="none"/>
						<text x="24" y="10" text-anchor="middle" fill="var(--color-text-muted)" font-size="8">N</text>
						<text x="24" y="44" text-anchor="middle" fill="var(--color-text-muted)" font-size="8">S</text>
						<text x="6" y="27" text-anchor="middle" fill="var(--color-text-muted)" font-size="8">W</text>
						<text x="42" y="27" text-anchor="middle" fill="var(--color-text-muted)" font-size="8">E</text>
						<g transform="rotate({wx.windDir} 24 24)">
							<path d="M24 8L21 18h6L24 8z" fill="var(--color-accent)"/>
							<line x1="24" y1="18" x2="24" y2="36" stroke="var(--color-accent)" stroke-width="2"/>
						</g>
					</svg>
					<span class="wx-wind-label">{windDirLabel(wx.windDir)} {formatWindSpeed(wx.windSpeed, units)}</span>
				</div>
			{/if}
		</div>

		<div class="wx-metrics-grid">
			{#if wx?.windGust !== undefined}
				<div class="wx-metric-box" class:alert={isAlertTriggered('windGust', wx.windGust * 3.6, config.alerts)}>
					<span class="wx-metric-val">{formatWindSpeed(wx.windGust, units)}</span>
					<span class="wx-metric-lbl">Gust</span>
				</div>
			{/if}
			{#if wx?.humidity !== undefined}
				<div class="wx-metric-box">
					<div class="hum-bar">
						<div class="hum-fill" style="width: {wx.humidity}%"></div>
					</div>
					<span class="wx-metric-val">{wx.humidity}%</span>
					<span class="wx-metric-lbl">Humidity</span>
				</div>
			{/if}
			{#if wx?.pressure !== undefined}
				<div class="wx-metric-box">
					<span class="wx-metric-val">{formatPressure(wx.pressure, units)}</span>
					<span class="wx-metric-lbl">{pressureLabel(units)}</span>
				</div>
			{/if}
			{#if wx?.rain1h !== undefined}
				<div class="wx-metric-box rain">
					<span class="wx-metric-val">{formatRain(wx.rain1h, units)}</span>
					<span class="wx-metric-lbl">Rain 1h</span>
				</div>
			{/if}
			{#if wx?.rain24h !== undefined}
				<div class="wx-metric-box rain">
					<span class="wx-metric-val">{formatRain(wx.rain24h, units)}</span>
					<span class="wx-metric-lbl">Rain 24h</span>
				</div>
			{/if}
			{#if wx?.luminosity !== undefined}
				<div class="wx-metric-box">
					<span class="wx-metric-val">{wx.luminosity}</span>
					<span class="wx-metric-lbl">Lux</span>
				</div>
			{/if}
		</div>
	</div>

	<!-- Historical charts -->
	<div class="wx-charts-section">
		<div class="wx-charts-header">
			<span class="section-title">History</span>
			<div class="time-toggle">
				<button class:active={timeRange === '24h'} onclick={() => timeRange = '24h'}>24h</button>
				<button class:active={timeRange === '7d'} onclick={() => timeRange = '7d'}>7d</button>
			</div>
		</div>

		{#if $weatherReadingsLoading}
			<div class="loading">Loading...</div>
		{:else}
			<div class="wx-charts-grid">
				{#if tempData.length > 0}
					<WeatherChart data={tempData} title="Temperature" unit={tempUnit(units)} color="#f59e0b" />
				{/if}
				{#if windData.length > 0}
					<WeatherChart data={windData} title="Wind Speed" unit={windUnit(units)} color="#60a5fa" />
				{/if}
				{#if gustData.length > 0}
					<WeatherChart data={gustData} title="Wind Gust" unit={windUnit(units)} color="#818cf8" />
				{/if}
				{#if pressData.length > 0}
					<WeatherChart data={pressData} title="Pressure" unit={pressureUnit(units)} color="#a78bfa" />
				{/if}
				{#if humData.length > 0}
					<WeatherChart data={humData} title="Humidity" unit="%" color="#34d399" />
				{/if}
				{#if rainData.length > 0}
					<WeatherChart data={rainData} title="Rain" unit={rainUnit(units)} color="#38bdf8" />
				{/if}
			</div>
		{/if}
	</div>
</div>

<style>
	.wx-detail {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
		padding-bottom: var(--space-lg);
	}

	.wx-detail-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding-bottom: var(--space-sm);
		border-bottom: 1px solid var(--color-primary);
	}

	.back-btn, .fly-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.back-btn:hover, .fly-btn:hover {
		color: var(--color-text);
		background: var(--color-primary);
	}

	.wx-detail-call {
		font-family: monospace;
		font-weight: 700;
		font-size: 1rem;
		flex: 1;
	}

	.wx-current {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.wx-hero-section {
		display: flex;
		align-items: center;
		gap: var(--space-lg);
	}

	.wx-hero-temp {
		font-size: 2.5rem;
		font-weight: 800;
		letter-spacing: -1px;
		line-height: 1;
	}

	.wx-hero-temp.alert {
		color: var(--color-warning);
	}

	.wx-wind-compass {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
	}

	.wx-wind-label {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		font-weight: 500;
	}

	.wx-metrics-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: var(--space-sm);
	}

	.wx-metric-box {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		padding: 8px;
		background: var(--color-primary);
		border-radius: var(--radius-sm);
	}

	.wx-metric-box.alert {
		border: 1px solid var(--color-warning);
	}

	.wx-metric-box.rain {
		color: #60a5fa;
	}

	.wx-metric-val {
		font-size: 0.9rem;
		font-weight: 700;
		font-family: monospace;
	}

	.wx-metric-lbl {
		font-size: 0.6rem;
		text-transform: uppercase;
		color: var(--color-text-muted);
		letter-spacing: 0.04em;
	}

	.hum-bar {
		width: 100%;
		height: 4px;
		background: rgba(255,255,255,0.1);
		border-radius: 2px;
		overflow: hidden;
	}

	.hum-fill {
		height: 100%;
		background: #34d399;
		border-radius: 2px;
		transition: width 0.3s;
	}

	.wx-charts-section {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.wx-charts-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.section-title {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
	}

	.time-toggle {
		display: flex;
		gap: 2px;
		background: var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 2px;
	}

	.time-toggle button {
		padding: 3px 10px;
		font-size: 0.7rem;
		font-weight: 600;
		background: none;
		border: none;
		border-radius: 4px;
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.time-toggle button.active {
		background: var(--color-accent);
		color: white;
	}

	.wx-charts-grid {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.loading {
		text-align: center;
		padding: var(--space-lg);
		color: var(--color-text-muted);
		font-size: 0.8rem;
	}
</style>
