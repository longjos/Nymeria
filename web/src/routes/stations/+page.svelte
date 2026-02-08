<script lang="ts">
	import { onMount } from 'svelte';
	import StationCard from '$lib/components/StationCard.svelte';
	import { stationList, initStationStore } from '$lib/stores/stations';

	let search = $state('');

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

	onMount(() => {
		initStationStore();
	});
</script>

<svelte:head>
	<title>Stations - Nymeria</title>
</svelte:head>

<div class="stations-page">
	<div class="page-header">
		<h2>Stations</h2>
		<input
			type="search"
			placeholder="Search by callsign..."
			bind:value={search}
		/>
	</div>

	<div class="station-grid">
		{#each filteredStations as station (station.callsign + '-' + station.ssid)}
			<StationCard {station} />
		{:else}
			<p class="empty">
				{search ? 'No matching stations' : 'No stations heard yet. Connect a transport to start receiving.'}
			</p>
		{/each}
	</div>
</div>

<style>
	.stations-page {
		max-width: 800px;
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
		gap: 1rem;
	}

	h2 {
		font-size: 1.25rem;
		color: var(--color-text);
	}

	input {
		padding: 0.4rem 0.75rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 6px;
		color: var(--color-text);
		font-size: 0.85rem;
		outline: none;
		width: 240px;
	}

	input:focus {
		border-color: var(--color-accent);
	}

	.station-grid {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.empty {
		padding: 3rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
	}
</style>
