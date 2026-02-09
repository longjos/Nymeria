<script lang="ts">
	import StationCard from './StationCard.svelte';
	import { stationList } from '$lib/stores/stations';
	import { getTacticalAlias } from '$lib/stores/tactical';
	import { stationKey } from '$lib/utils';

	let {
		onSelect,
		selectedKey
	}: {
		onSelect?: (key: string) => void;
		selectedKey?: string | null;
	} = $props();

	let search = $state('');

	let filteredStations = $derived.by(() => {
		const list = $stationList;
		if (!search) return list;
		const q = search.toUpperCase();
		const lookup = $getTacticalAlias;
		return list.filter(
			(s) =>
				s.callsign.includes(q) ||
				(s.comment ?? '').toUpperCase().includes(q) ||
				(lookup(stationKey(s)) ?? '').toUpperCase().includes(q)
		);
	});
</script>

<div class="station-list">
	<div class="list-header">
		<span class="count">{$stationList.length} stations</span>
		<input
			type="search"
			placeholder="Filter..."
			bind:value={search}
		/>
	</div>
	<div class="list-body">
		{#each filteredStations as station (station.callsign + '-' + station.ssid)}
			<StationCard
				{station}
				compact
				selected={selectedKey === (station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign)}
				onclick={() => {
					const key = station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign;
					onSelect?.(key);
				}}
			/>
		{:else}
			<p class="empty">
				{search ? 'No matching stations' : 'No stations heard yet'}
			</p>
		{/each}
	</div>
</div>

<style>
	.station-list {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.list-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.count {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	input {
		flex: 1;
		padding: 0.35rem 0.6rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
		outline: none;
		min-width: 0;
	}

	input:focus {
		border-color: var(--color-accent);
	}

	.list-body {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.empty {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
</style>
