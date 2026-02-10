<script lang="ts">
	let {
		data = [],
		title = '',
		unit = '',
		color = 'var(--color-accent)',
		height = 120
	}: {
		data?: Array<{ time: string; value: number }>;
		title?: string;
		unit?: string;
		color?: string;
		height?: number;
	} = $props();

	let width = $state(300);
	let containerEl = $state<HTMLDivElement>();

	import { onMount } from 'svelte';

	onMount(() => {
		if (containerEl) {
			const ro = new ResizeObserver((entries) => {
				for (const entry of entries) {
					width = entry.contentRect.width;
				}
			});
			ro.observe(containerEl);
			return () => ro.disconnect();
		}
	});

	let padding = { top: 20, right: 8, bottom: 24, left: 40 };
	let chartWidth = $derived(width - padding.left - padding.right);
	let chartHeight = $derived(height - padding.top - padding.bottom);

	let minVal = $derived(data.length > 0 ? Math.min(...data.map(d => d.value)) : 0);
	let maxVal = $derived(data.length > 0 ? Math.max(...data.map(d => d.value)) : 1);
	let range = $derived(maxVal - minVal || 1);

	let points = $derived.by(() => {
		if (data.length === 0 || chartWidth <= 0) return '';
		return data.map((d, i) => {
			const x = padding.left + (i / Math.max(data.length - 1, 1)) * chartWidth;
			const y = padding.top + chartHeight - ((d.value - minVal) / range) * chartHeight;
			return `${x},${y}`;
		}).join(' ');
	});

	let areaPath = $derived.by(() => {
		if (data.length === 0 || chartWidth <= 0) return '';
		const pts = data.map((d, i) => {
			const x = padding.left + (i / Math.max(data.length - 1, 1)) * chartWidth;
			const y = padding.top + chartHeight - ((d.value - minVal) / range) * chartHeight;
			return `${x},${y}`;
		});
		const bottom = padding.top + chartHeight;
		const firstX = padding.left;
		const lastX = padding.left + chartWidth;
		return `M${firstX},${bottom} L${pts.join(' L')} L${lastX},${bottom} Z`;
	});

	let yTicks = $derived.by(() => {
		const ticks: Array<{ value: number; y: number }> = [];
		const step = range / 3;
		for (let i = 0; i <= 3; i++) {
			const v = minVal + step * i;
			const y = padding.top + chartHeight - (i / 3) * chartHeight;
			ticks.push({ value: v, y });
		}
		return ticks;
	});

	let xLabels = $derived.by(() => {
		if (data.length < 2) return [];
		const labels: Array<{ text: string; x: number }> = [];
		const indices = [0, Math.floor(data.length / 2), data.length - 1];
		for (const i of indices) {
			const d = new Date(data[i].time);
			const h = d.getHours().toString().padStart(2, '0');
			const m = d.getMinutes().toString().padStart(2, '0');
			const x = padding.left + (i / Math.max(data.length - 1, 1)) * chartWidth;
			labels.push({ text: `${h}:${m}`, x });
		}
		return labels;
	});

	let lastValue = $derived(data.length > 0 ? data[data.length - 1].value : null);
</script>

<div class="wx-chart" bind:this={containerEl}>
	<div class="wx-chart-header">
		<span class="wx-chart-title">{title}</span>
		{#if lastValue !== null}
			<span class="wx-chart-current" style="color: {color}">{lastValue.toFixed(1)}{unit}</span>
		{/if}
	</div>
	{#if data.length > 1}
		<svg {width} {height}>
			<!-- Y-axis grid -->
			{#each yTicks as tick}
				<line
					x1={padding.left} y1={tick.y}
					x2={width - padding.right} y2={tick.y}
					stroke="var(--color-primary)" stroke-width="1"
				/>
				<text x={padding.left - 4} y={tick.y + 3} text-anchor="end"
					fill="var(--color-text-muted)" font-size="9">
					{tick.value.toFixed(tick.value % 1 === 0 ? 0 : 1)}
				</text>
			{/each}
			<!-- Area fill -->
			<path d={areaPath} fill={color} opacity="0.1" />
			<!-- Line -->
			<polyline points={points} fill="none" stroke={color} stroke-width="1.5" stroke-linejoin="round" />
			<!-- X labels -->
			{#each xLabels as label}
				<text x={label.x} y={height - 4} text-anchor="middle"
					fill="var(--color-text-muted)" font-size="9">
					{label.text}
				</text>
			{/each}
		</svg>
	{:else}
		<div class="wx-chart-empty">No data</div>
	{/if}
</div>

<style>
	.wx-chart {
		width: 100%;
	}

	.wx-chart-header {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		margin-bottom: 4px;
		padding: 0 2px;
	}

	.wx-chart-title {
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
	}

	.wx-chart-current {
		font-size: 0.85rem;
		font-weight: 700;
		font-family: monospace;
	}

	svg {
		display: block;
	}

	.wx-chart-empty {
		height: 60px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}
</style>
