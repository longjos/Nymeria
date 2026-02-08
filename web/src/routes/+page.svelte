<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { initStationStore, stations } from '$lib/stores/stations';

	let status = $state('checking...');
	let transportCount = $state(0);
	let stationCount = $derived($stations.size);

	onMount(async () => {
		initStationStore();
		try {
			const health = await api.health();
			status = health.status;
		} catch {
			status = 'unreachable';
		}
		try {
			const transports = await api.transports();
			transportCount = transports.length;
		} catch { /* ignore */ }
	});
</script>

<svelte:head>
	<title>Nymeria - APRS Client</title>
</svelte:head>

<div class="home">
	<div class="hero">
		<h1>Nymeria</h1>
		<p class="subtitle">Amateur Radio APRS Client</p>
	</div>

	<div class="status-bar">
		<div class="status-item">
			<span class="status-label">Server</span>
			<span class="status-value" class:ok={status === 'ok'} class:err={status === 'unreachable'}>
				{status}
			</span>
		</div>
		<div class="status-item">
			<span class="status-label">Transports</span>
			<span class="status-value">{transportCount}</span>
		</div>
		<div class="status-item">
			<span class="status-label">Stations</span>
			<span class="status-value">{stationCount}</span>
		</div>
	</div>

	<div class="features">
		<a href="/map" class="card">
			<h3>Map</h3>
			<p>Real-time APRS station tracking on an interactive map with track history</p>
		</a>
		<a href="/stations" class="card">
			<h3>Stations</h3>
			<p>Browse and search all heard stations with position, symbol, and status</p>
		</a>
		<a href="/messages" class="card">
			<h3>Messages</h3>
			<p>Send and receive APRS messages with delivery confirmation</p>
		</a>
	</div>
</div>

<style>
	.home {
		max-width: 700px;
		margin: 0 auto;
	}

	.hero {
		text-align: center;
		padding: 2.5rem 0 1.5rem;
	}

	h1 {
		font-size: 2.25rem;
		color: var(--color-accent);
		letter-spacing: 0.02em;
	}

	.subtitle {
		color: var(--color-text-muted);
		margin-top: 0.375rem;
		font-size: 1rem;
	}

	.status-bar {
		display: flex;
		justify-content: center;
		gap: 2rem;
		padding: 0.75rem 1rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 8px;
		margin-bottom: 2rem;
	}

	.status-item {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.125rem;
	}

	.status-label {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}

	.status-value {
		font-size: 1rem;
		font-weight: 600;
		font-family: monospace;
	}

	.ok {
		color: #4ade80;
	}

	.err {
		color: var(--color-accent);
	}

	.features {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 0.75rem;
	}

	.card {
		background: var(--color-surface);
		padding: 1.25rem;
		border-radius: 8px;
		border: 1px solid var(--color-primary);
		text-decoration: none;
		color: var(--color-text);
		transition: border-color 0.15s;
	}

	.card:hover {
		border-color: var(--color-accent);
	}

	.card h3 {
		color: var(--color-accent);
		margin-bottom: 0.375rem;
		font-size: 1.05rem;
	}

	.card p {
		font-size: 0.85rem;
		color: var(--color-text-muted);
		line-height: 1.4;
	}
</style>
