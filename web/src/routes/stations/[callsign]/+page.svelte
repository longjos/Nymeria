<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import APRSIcon from '$lib/components/APRSIcon.svelte';
	import { api } from '$lib/api';
	import { symbolInfo } from '$lib/symbols';
	import { timeAgo, formatCoord, formatSpeed, formatAltitude, formatCourse, stationDisplayName } from '$lib/utils';
	import type { Station } from '$lib/types';

	let station = $state<Station | null>(null);
	let error = $state('');
	let callsign = $derived($page.params.callsign);

	onMount(async () => {
		try {
			station = await api.station(callsign);
		} catch {
			error = 'Station not found';
		}
	});

	let displayName = $derived(station ? stationDisplayName(station.callsign, station.ssid) : callsign);
	let info = $derived(station ? symbolInfo(station.symbol) : null);
</script>

<svelte:head>
	<title>{displayName} - Nymeria</title>
</svelte:head>

<div class="detail-page">
	<a href="/stations" class="back">Back to Stations</a>

	{#if error}
		<div class="error-card">
			<p>{error}</p>
			<a href="/stations">Return to station list</a>
		</div>
	{:else if station}
		<div class="station-header">
			<APRSIcon symbol={station.symbol} size={48} />
			<div>
				<h2>{displayName}</h2>
				{#if info}
					<span class="symbol-label">{info.label}</span>
				{/if}
			</div>
			<span class="last-heard">{timeAgo(station.lastHeard)}</span>
		</div>

		<div class="info-grid">
			{#if station.position}
				<div class="info-card">
					<h3>Position</h3>
					<div class="info-row">
						<span class="label">Coordinates</span>
						<span>{formatCoord(station.position.lat, station.position.lon)}</span>
					</div>
					{#if station.position.altitude}
						<div class="info-row">
							<span class="label">Altitude</span>
							<span>{formatAltitude(station.position.altitude)}</span>
						</div>
					{/if}
					{#if station.position.speed}
						<div class="info-row">
							<span class="label">Speed</span>
							<span>{formatSpeed(station.position.speed)}</span>
						</div>
					{/if}
					{#if station.position.course}
						<div class="info-row">
							<span class="label">Course</span>
							<span>{formatCourse(station.position.course)}</span>
						</div>
					{/if}
				</div>
			{/if}

			<div class="info-card">
				<h3>Info</h3>
				<div class="info-row">
					<span class="label">Source</span>
					<span>{station.source}</span>
				</div>
				{#if station.comment}
					<div class="info-row">
						<span class="label">Comment</span>
						<span>{station.comment}</span>
					</div>
				{/if}
				<div class="info-row">
					<span class="label">Track Points</span>
					<span>{station.track?.length ?? 0}</span>
				</div>
			</div>
		</div>

		{#if station.track && station.track.length > 0}
			<div class="info-card">
				<h3>Track History</h3>
				<div class="track-list">
					{#each station.track.slice().reverse().slice(0, 20) as tp}
						<div class="track-row">
							<span class="coords">{formatCoord(tp.lat, tp.lon)}</span>
							<span class="time">{timeAgo(tp.time)}</span>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<div class="actions">
			<a href="/messages/{station.callsign}" class="btn">Send Message</a>
			<a href="/map" class="btn btn-secondary">View on Map</a>
		</div>
	{:else}
		<div class="loading">Loading...</div>
	{/if}
</div>

<style>
	.detail-page {
		max-width: 700px;
	}

	.back {
		font-size: 0.85rem;
		color: var(--color-text-muted);
		text-decoration: none;
		display: inline-block;
		margin-bottom: 1rem;
	}

	.back:hover {
		color: var(--color-accent);
	}

	.station-header {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	.station-header h2 {
		font-family: monospace;
		font-size: 1.5rem;
	}

	.symbol-label {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}

	.last-heard {
		margin-left: auto;
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}

	.info-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.info-card {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 8px;
		padding: 1rem;
		margin-bottom: 1rem;
	}

	.info-card h3 {
		font-size: 0.85rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-bottom: 0.75rem;
	}

	.info-row {
		display: flex;
		justify-content: space-between;
		padding: 0.375rem 0;
		font-size: 0.9rem;
	}

	.info-row + .info-row {
		border-top: 1px solid var(--color-primary);
	}

	.info-row .label {
		color: var(--color-text-muted);
	}

	.track-list {
		max-height: 300px;
		overflow-y: auto;
	}

	.track-row {
		display: flex;
		justify-content: space-between;
		padding: 0.25rem 0;
		font-size: 0.8rem;
		font-family: monospace;
	}

	.track-row .time {
		color: var(--color-text-muted);
	}

	.actions {
		display: flex;
		gap: 0.75rem;
		margin-top: 1.5rem;
	}

	.btn {
		padding: 0.5rem 1.25rem;
		border-radius: 6px;
		text-decoration: none;
		font-size: 0.9rem;
		font-weight: 600;
		background: var(--color-accent);
		color: white;
	}

	.btn-secondary {
		background: var(--color-primary);
	}

	.error-card {
		background: var(--color-surface);
		border: 1px solid var(--color-accent);
		border-radius: 8px;
		padding: 2rem;
		text-align: center;
	}

	.error-card a {
		display: inline-block;
		margin-top: 1rem;
	}

	.loading {
		text-align: center;
		padding: 3rem;
		color: var(--color-text-muted);
	}
</style>
