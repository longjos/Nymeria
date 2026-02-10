<script lang="ts">
	import type { Station, TelemetryReading } from '$lib/types';
	import { telemetryReadings, telemetryReadingsLoading, telemetryParams,
		loadTelemetryReadings, applyEquation, channelName, channelUnit } from '$lib/stores/telemetry';
	import WeatherChart from './WeatherChart.svelte';

	let {
		station,
		onBack,
		onFlyTo
	}: {
		station: Station;
		onBack?: () => void;
		onFlyTo?: (lat: number, lon: number) => void;
	} = $props();

	let tel = $derived(station.telemetry);
	let params = $derived($telemetryParams ?? station.telemetryParams ?? null);
	let key = $derived(station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign);

	let timeRange = $state<'24h' | '7d'>('24h');

	$effect(() => {
		const since = new Date(Date.now() - (timeRange === '24h' ? 24 * 3600000 : 7 * 24 * 3600000)).toISOString();
		loadTelemetryReadings(key, since);
	});

	function fmtVal(channel: number, raw: number): string {
		const val = applyEquation(params, channel, raw);
		return val === Math.floor(val) ? val.toString() : val.toFixed(2);
	}

	function digitalBits(d: number): boolean[] {
		const bits: boolean[] = [];
		for (let i = 0; i < 8; i++) {
			bits.push(((d >> i) & 1) === 1);
		}
		return bits;
	}

	function bitLabel(i: number): string {
		if (params?.bitLabels[i]) return params.bitLabels[i];
		return `B${i}`;
	}

	// Chart data helpers (oldest first)
	const channelColors = ['#f59e0b', '#60a5fa', '#a78bfa', '#34d399', '#f472b6'];

	function channelChartData(readings: TelemetryReading[], channel: number): Array<{ time: string; value: number }> {
		const result: Array<{ time: string; value: number }> = [];
		const field = `analog${channel + 1}` as keyof TelemetryReading;
		for (let i = readings.length - 1; i >= 0; i--) {
			const r = readings[i];
			const raw = r[field] as number;
			if (raw !== undefined && raw !== null) {
				result.push({ time: r.timestamp, value: applyEquation(params, channel, raw) });
			}
		}
		return result;
	}

	let chartDataSets = $derived(
		[0, 1, 2, 3, 4].map((ch) => ({
			data: channelChartData($telemetryReadings, ch),
			title: channelName(params, ch),
			unit: channelUnit(params, ch),
			color: channelColors[ch]
		}))
	);
</script>

<div class="tel-detail">
	<div class="tel-detail-header">
		<button class="back-btn" onclick={onBack} title="Back">
			<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
				<path d="M10 3L5 8l5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
			</svg>
		</button>
		<span class="tel-detail-call">{key}</span>
		{#if station.position}
			<button class="fly-btn" onclick={() => onFlyTo?.(station.position!.lat, station.position!.lon)} title="Fly to station">
				<svg width="14" height="14" viewBox="0 0 14 14" fill="none">
					<path d="M7 1L2 13l5-3 5 3L7 1z" fill="currentColor"/>
				</svg>
			</button>
		{/if}
	</div>

	{#if params?.projectTitle}
		<div class="project-title">{params.projectTitle}</div>
	{/if}

	<!-- Current values -->
	{#if tel}
		<div class="tel-current">
			<span class="section-title">Current Values</span>
			<span class="tel-current-seq">Seq #{tel.seq}</span>
		</div>

		<div class="tel-values-grid">
			{#each [0, 1, 2, 3, 4] as ch}
				<div class="tel-value-box">
					<span class="tel-value-val" style="color: {channelColors[ch]}">{fmtVal(ch, tel.analog[ch])}</span>
					{#if channelUnit(params, ch)}
						<span class="tel-value-unit">{channelUnit(params, ch)}</span>
					{/if}
					<span class="tel-value-name">{channelName(params, ch)}</span>
				</div>
			{/each}
		</div>

		<!-- Digital channels -->
		<div class="tel-digital-section">
			<span class="section-title">Digital Channels</span>
			<div class="tel-digital-grid">
				{#each digitalBits(tel.digital) as bit, i}
					<div class="tel-digital-item" class:on={bit}>
						<span class="tel-digital-dot" class:on={bit}></span>
						<span class="tel-digital-label">{bitLabel(i)}</span>
						<span class="tel-digital-state">{bit ? 'ON' : 'OFF'}</span>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Historical charts -->
	<div class="tel-charts-section">
		<div class="tel-charts-header">
			<span class="section-title">History</span>
			<div class="time-toggle">
				<button class:active={timeRange === '24h'} onclick={() => timeRange = '24h'}>24h</button>
				<button class:active={timeRange === '7d'} onclick={() => timeRange = '7d'}>7d</button>
			</div>
		</div>

		{#if $telemetryReadingsLoading}
			<div class="loading">Loading...</div>
		{:else}
			<div class="tel-charts-grid">
				{#each chartDataSets as { data, title, unit, color }}
					{#if data.length > 0}
						<WeatherChart {data} {title} {unit} {color} />
					{/if}
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	.tel-detail {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
		padding-bottom: var(--space-lg);
	}

	.tel-detail-header {
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

	.tel-detail-call {
		font-family: monospace;
		font-weight: 700;
		font-size: 1rem;
		flex: 1;
	}

	.project-title {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	.section-title {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
	}

	.tel-current {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.tel-current-seq {
		font-family: monospace;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.tel-values-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: var(--space-sm);
	}

	.tel-value-box {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		padding: 8px;
		background: var(--color-primary);
		border-radius: var(--radius-sm);
	}

	.tel-value-val {
		font-size: 1.1rem;
		font-weight: 700;
		font-family: monospace;
	}

	.tel-value-unit {
		font-size: 0.6rem;
		color: var(--color-text-muted);
	}

	.tel-value-name {
		font-size: 0.65rem;
		text-transform: uppercase;
		color: var(--color-text-muted);
		letter-spacing: 0.04em;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 100%;
		text-align: center;
	}

	/* Digital channels */
	.tel-digital-section {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.tel-digital-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: var(--space-xs);
	}

	.tel-digital-item {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		padding: 6px 4px;
		background: var(--color-primary);
		border-radius: var(--radius-sm);
	}

	.tel-digital-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background: rgba(255, 255, 255, 0.1);
		border: 1px solid rgba(255, 255, 255, 0.15);
		transition: all 0.2s;
	}

	.tel-digital-dot.on {
		background: #22c55e;
		border-color: #22c55e;
		box-shadow: 0 0 6px rgba(34, 197, 94, 0.6);
	}

	.tel-digital-label {
		font-size: 0.6rem;
		font-weight: 600;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 100%;
	}

	.tel-digital-state {
		font-size: 0.55rem;
		font-family: monospace;
		color: var(--color-text-muted);
	}

	.tel-digital-item.on .tel-digital-state {
		color: #22c55e;
	}

	/* Charts */
	.tel-charts-section {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.tel-charts-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
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

	.tel-charts-grid {
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
