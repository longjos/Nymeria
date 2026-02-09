<script lang="ts">
	import type { Station } from '$lib/types';
	import APRSIcon from './APRSIcon.svelte';
	import { timeAgo, stationDisplayName, stationKey, formatCoord, formatSpeed } from '$lib/utils';
	import { symbolInfo } from '$lib/symbols';
	import { getTacticalAlias } from '$lib/stores/tactical';

	let {
		station,
		compact = false,
		selected = false,
		onclick
	}: {
		station: Station;
		compact?: boolean;
		selected?: boolean;
		onclick?: (e: MouseEvent) => void;
	} = $props();

	let key = $derived(stationKey(station));
	let displayName = $derived(stationDisplayName(station.callsign, station.ssid));
	let tacAlias = $derived($getTacticalAlias(key));
	let info = $derived(symbolInfo(station.symbol));
	let coords = $derived(
		station.position ? formatCoord(station.position.lat, station.position.lon) : ''
	);
	let speed = $derived(station.position ? formatSpeed(station.position.speed) : '');
</script>

{#if onclick}
	<button class="station-card" class:compact class:selected onclick={onclick}>
		<div class="card-left">
			<APRSIcon symbol={station.symbol} size={compact ? 28 : 36} />
		</div>
		<div class="card-body">
			<div class="card-header">
				{#if tacAlias}
					<span class="callsign"><span class="tac-alias">{tacAlias}</span> <span class="tac-callsign">{displayName}</span></span>
				{:else}
					<span class="callsign">{displayName}</span>
				{/if}
				<span class="time">{timeAgo(station.lastHeard)}</span>
			</div>
			{#if !compact}
				<div class="card-meta">
					<span class="symbol-label">{info.label}</span>
					{#if coords}
						<span class="coords">{coords}</span>
					{/if}
					{#if speed}
						<span class="speed">{speed}</span>
					{/if}
				</div>
				{#if station.comment}
					<div class="comment">{station.comment}</div>
				{/if}
			{/if}
		</div>
	</button>
{:else}
	<a href="/stations/{key}" class="station-card" class:compact class:selected>
		<div class="card-left">
			<APRSIcon symbol={station.symbol} size={compact ? 28 : 36} />
		</div>
		<div class="card-body">
			<div class="card-header">
				{#if tacAlias}
					<span class="callsign"><span class="tac-alias">{tacAlias}</span> <span class="tac-callsign">{displayName}</span></span>
				{:else}
					<span class="callsign">{displayName}</span>
				{/if}
				<span class="time">{timeAgo(station.lastHeard)}</span>
			</div>
			{#if !compact}
				<div class="card-meta">
					<span class="symbol-label">{info.label}</span>
					{#if coords}
						<span class="coords">{coords}</span>
					{/if}
					{#if speed}
						<span class="speed">{speed}</span>
					{/if}
				</div>
				{#if station.comment}
					<div class="comment">{station.comment}</div>
				{/if}
			{/if}
		</div>
	</a>
{/if}

<style>
	.station-card {
		display: flex;
		gap: 0.75rem;
		padding: 0.75rem 1rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 8px;
		text-decoration: none;
		color: var(--color-text);
		transition: border-color 0.15s;
		width: 100%;
		text-align: left;
		font: inherit;
		cursor: pointer;
	}

	.station-card:hover {
		border-color: var(--color-accent);
	}

	.station-card.compact {
		padding: 0.5rem 0.75rem;
	}

	.station-card.selected {
		border-color: var(--color-accent);
		background: rgba(233, 69, 96, 0.08);
	}

	.card-left {
		display: flex;
		align-items: flex-start;
		padding-top: 2px;
	}

	.card-body {
		flex: 1;
		min-width: 0;
	}

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}

	.callsign {
		font-weight: 600;
		font-size: 0.95rem;
		font-family: monospace;
	}

	.tac-alias {
		color: var(--color-accent);
	}

	.tac-callsign {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		font-weight: 400;
	}

	.time {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.card-meta {
		display: flex;
		gap: 0.75rem;
		margin-top: 0.25rem;
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.comment {
		margin-top: 0.25rem;
		font-size: 0.8rem;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
