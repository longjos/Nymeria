<script lang="ts">
	import type { Station, WeatherAlertThreshold } from '$lib/types';
	import { isAlertTriggered, weatherUnits } from '$lib/stores/weather';
	import { formatTempShort, formatWindSpeed, formatPressure, formatRain, pressureLabel } from '$lib/units';

	let {
		station,
		alerts,
		onClick
	}: {
		station: Station;
		alerts?: Record<string, WeatherAlertThreshold>;
		onClick?: (callsign: string) => void;
	} = $props();

	let wx = $derived(station.weather);
	let key = $derived(station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign);
	let tempStr = $derived(formatTempShort(wx?.temperature, $weatherUnits));
	let windStr = $derived(formatWindSpeed(wx?.windSpeed, $weatherUnits));
	let humStr = $derived(wx?.humidity !== undefined ? `${wx.humidity}%` : '--');
	let pressStr = $derived(formatPressure(wx?.pressure, $weatherUnits));
	let ago = $derived.by(() => {
		const diff = Date.now() - new Date(station.lastHeard).getTime();
		const min = Math.floor(diff / 60000);
		if (min < 1) return 'just now';
		if (min < 60) return `${min}m ago`;
		const hrs = Math.floor(min / 60);
		if (hrs < 24) return `${hrs}h ago`;
		return `${Math.floor(hrs / 24)}d ago`;
	});

	let hasAlert = $derived.by(() => {
		if (!wx || !alerts) return false;
		return isAlertTriggered('temperature', wx.temperature, alerts) ||
			isAlertTriggered('windSpeed', wx.windSpeed !== undefined ? wx.windSpeed * 3.6 : undefined, alerts) ||
			isAlertTriggered('windGust', wx.windGust !== undefined ? wx.windGust * 3.6 : undefined, alerts);
	});

	function windArrowRotation(dir?: number): string {
		if (dir === undefined) return 'rotate(0deg)';
		return `rotate(${dir}deg)`;
	}
</script>

<button class="wx-card" class:alert={hasAlert} onclick={() => onClick?.(key)}>
	<div class="wx-card-header">
		<span class="wx-callsign">{key}</span>
		<span class="wx-ago">{ago}</span>
	</div>
	<div class="wx-card-body">
		<div class="wx-hero">
			<span class="wx-temp" class:alert-value={isAlertTriggered('temperature', wx?.temperature, alerts)}>
				{tempStr}
			</span>
		</div>
		<div class="wx-metrics">
			<div class="wx-metric">
				<svg class="wind-arrow" width="14" height="14" viewBox="0 0 14 14" style="transform: {windArrowRotation(wx?.windDir)}">
					<path d="M7 1L4 10h6L7 1z" fill="currentColor"/>
				</svg>
				<span class:alert-value={isAlertTriggered('windSpeed', wx?.windSpeed !== undefined ? wx.windSpeed * 3.6 : undefined, alerts)}>
					{windStr}
				</span>
			</div>
			<div class="wx-metric">
				<svg width="12" height="12" viewBox="0 0 12 12" fill="none">
					<path d="M6 1c-2.2 0-4 1.8-4 4 0 3 4 6 4 6s4-3 4-6c0-2.2-1.8-4-4-4z" stroke="currentColor" stroke-width="1.2"/>
				</svg>
				<span>{humStr}</span>
			</div>
			<div class="wx-metric">
				<span class="wx-metric-label">{pressureLabel($weatherUnits)}</span>
				<span>{pressStr}</span>
			</div>
			{#if wx?.rain1h !== undefined && wx.rain1h > 0}
				<div class="wx-metric rain">
					<svg width="12" height="12" viewBox="0 0 12 12" fill="none">
						<path d="M6 1L3 7h6L6 1z" stroke="currentColor" stroke-width="1"/>
						<path d="M4 9h4" stroke="currentColor" stroke-width="1" stroke-linecap="round"/>
					</svg>
					<span>{formatRain(wx.rain1h, $weatherUnits)}</span>
				</div>
			{/if}
		</div>
	</div>
</button>

<style>
	.wx-card {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 10px 12px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: background var(--duration-fast), border-color var(--duration-fast);
		text-align: left;
		color: var(--color-text);
		width: 100%;
	}

	.wx-card:hover {
		background: var(--color-primary);
	}

	.wx-card.alert {
		border-color: var(--color-warning);
		box-shadow: inset 0 0 0 1px rgba(245, 158, 11, 0.15);
	}

	.wx-card-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.wx-callsign {
		font-family: monospace;
		font-weight: 700;
		font-size: 0.85rem;
		color: var(--color-accent);
	}

	.wx-ago {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-card-body {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.wx-hero {
		flex-shrink: 0;
	}

	.wx-temp {
		font-size: 1.5rem;
		font-weight: 700;
		letter-spacing: -0.5px;
	}

	.wx-metrics {
		display: flex;
		flex-wrap: wrap;
		gap: 6px 10px;
		flex: 1;
	}

	.wx-metric {
		display: flex;
		align-items: center;
		gap: 3px;
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.wx-metric-label {
		font-size: 0.6rem;
		text-transform: uppercase;
		opacity: 0.6;
	}

	.wx-metric.rain {
		color: #60a5fa;
	}

	.wind-arrow {
		color: var(--color-text-muted);
		transition: transform 0.3s ease;
	}

	.alert-value {
		color: var(--color-warning);
		font-weight: 600;
	}
</style>
