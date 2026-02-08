<script lang="ts">
	import { onMount } from 'svelte';
	import Map from '$lib/components/Map.svelte';
	import StationCard from '$lib/components/StationCard.svelte';
	import { stations, stationList, initStationStore } from '$lib/stores/stations';

	let search = $state('');
	let sidebarOpen = $state(true);

	let filteredStations = $derived.by(() => {
		const list = $stationList;
		if (!search) return list;
		const q = search.toUpperCase();
		return list.filter(
			(s) =>
				s.callsign.includes(q) ||
				(s.comment ?? '').toUpperCase().includes(q)
		);
	});

	let stationsWithPosition = $derived(
		$stationList.filter((s) => s.position)
	);

	let stationCount = $derived($stations.size);

	onMount(() => {
		initStationStore();
	});
</script>

<svelte:head>
	<link
		rel="stylesheet"
		href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css"
		crossorigin=""
	/>
</svelte:head>

<div class="map-page" class:sidebar-open={sidebarOpen}>
	<div class="map-area">
		<Map stations={stationsWithPosition} />
		<div class="map-overlay">
			<button class="toggle-sidebar" onclick={() => (sidebarOpen = !sidebarOpen)}>
				{sidebarOpen ? 'Hide' : 'Show'} Stations ({stationCount})
			</button>
		</div>
	</div>
	{#if sidebarOpen}
		<aside class="sidebar">
			<div class="sidebar-header">
				<input
					type="search"
					placeholder="Search stations..."
					bind:value={search}
				/>
			</div>
			<div class="station-list">
				{#each filteredStations as station (station.callsign + '-' + station.ssid)}
					<StationCard {station} compact />
				{:else}
					<p class="empty">
						{search ? 'No matching stations' : 'No stations yet'}
					</p>
				{/each}
			</div>
		</aside>
	{/if}
</div>

<style>
	.map-page {
		display: flex;
		height: calc(100vh - 60px);
		margin: -1.5rem;
	}

	.map-area {
		flex: 1;
		position: relative;
	}

	.map-overlay {
		position: absolute;
		top: 10px;
		right: 10px;
		z-index: 1000;
	}

	.toggle-sidebar {
		padding: 0.4rem 0.75rem;
		background: var(--color-surface);
		color: var(--color-text);
		border: 1px solid var(--color-primary);
		border-radius: 6px;
		font-size: 0.8rem;
		cursor: pointer;
	}

	.toggle-sidebar:hover {
		border-color: var(--color-accent);
	}

	.sidebar {
		width: 320px;
		flex-shrink: 0;
		background: var(--color-bg);
		border-left: 1px solid var(--color-primary);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.sidebar-header {
		padding: 0.75rem;
		border-bottom: 1px solid var(--color-primary);
	}

	.sidebar-header input {
		width: 100%;
		padding: 0.4rem 0.75rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 6px;
		color: var(--color-text);
		font-size: 0.85rem;
		outline: none;
	}

	.sidebar-header input:focus {
		border-color: var(--color-accent);
	}

	.station-list {
		flex: 1;
		overflow-y: auto;
		padding: 0.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.empty {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}

	@media (max-width: 768px) {
		.sidebar {
			width: 100%;
			position: absolute;
			right: 0;
			top: 0;
			bottom: 0;
			z-index: 1001;
		}
	}
</style>
