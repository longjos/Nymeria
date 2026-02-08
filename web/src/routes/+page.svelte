<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

	let status = $state('checking...');

	onMount(async () => {
		try {
			const health = await api.health();
			status = health.status;
		} catch {
			status = 'unreachable';
		}
	});
</script>

<div class="home">
	<h1>Nymeria</h1>
	<p class="subtitle">Amateur Radio APRS Client</p>

	<div class="status">
		<span class="label">Server:</span>
		<span class="value" class:ok={status === 'ok'} class:err={status !== 'ok' && status !== 'checking...'}>
			{status}
		</span>
	</div>

	<div class="features">
		<div class="card">
			<h3>Map</h3>
			<p>Real-time APRS station tracking on an interactive map</p>
		</div>
		<div class="card">
			<h3>Messages</h3>
			<p>Send and receive APRS messages</p>
		</div>
		<div class="card">
			<h3>Transports</h3>
			<p>Connect via APRS-IS, KISS TNC, or serial</p>
		</div>
	</div>
</div>

<style>
	.home {
		text-align: center;
		padding: 3rem 1rem;
	}

	h1 {
		font-size: 2.5rem;
		color: var(--color-accent);
	}

	.subtitle {
		color: var(--color-text-muted);
		margin-top: 0.5rem;
	}

	.status {
		margin: 2rem 0;
		font-size: 1.1rem;
	}

	.label {
		color: var(--color-text-muted);
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
		gap: 1rem;
		margin-top: 2rem;
		max-width: 700px;
		margin-left: auto;
		margin-right: auto;
	}

	.card {
		background: var(--color-surface);
		padding: 1.5rem;
		border-radius: 8px;
		border: 1px solid var(--color-primary);
	}

	.card h3 {
		color: var(--color-accent);
		margin-bottom: 0.5rem;
	}
</style>
