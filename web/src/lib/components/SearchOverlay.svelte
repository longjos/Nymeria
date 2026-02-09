<script lang="ts">
	import { stationList } from '$lib/stores/stations';
	import { stationKey as getStationKey } from '$lib/utils';
	import { getTacticalAlias } from '$lib/stores/tactical';

	let {
		onClose,
		onSelect
	}: {
		onClose?: () => void;
		onSelect?: (key: string) => void;
	} = $props();

	let query = $state('');
	let inputEl: HTMLInputElement;

	let results = $derived.by(() => {
		if (!query) return $stationList.slice(0, 20);
		const q = query.toUpperCase();
		const lookup = $getTacticalAlias;
		return $stationList
			.filter((s) =>
				s.callsign.includes(q) ||
				(s.comment ?? '').toUpperCase().includes(q) ||
				(lookup(getStationKey(s)) ?? '').toUpperCase().includes(q)
			)
			.slice(0, 20);
	});

	function handleSelect(key: string) {
		onSelect?.(key);
		onClose?.();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onClose?.();
	}

	import { onMount } from 'svelte';
	onMount(() => {
		inputEl?.focus();
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="search-overlay mobile-only">
	<div class="search-header">
		<input
			type="search"
			placeholder="Search stations..."
			bind:value={query}
			bind:this={inputEl}
		/>
		<button class="cancel-btn" onclick={onClose}>Cancel</button>
	</div>
	<div class="search-results">
		{#each results as station (station.callsign + '-' + station.ssid)}
			{@const key = getStationKey(station)}
			{@const alias = $getTacticalAlias(key)}
			<button class="result-row" onclick={() => handleSelect(key)}>
				{#if alias}
					<span class="result-call"><span class="result-alias">{alias}</span> <span class="result-secondary">{station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign}</span></span>
				{:else}
					<span class="result-call">{station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign}</span>
				{/if}
				{#if station.comment}
					<span class="result-comment">{station.comment}</span>
				{/if}
			</button>
		{:else}
			<p class="empty">{query ? 'No results' : 'No stations heard yet'}</p>
		{/each}
	</div>
</div>

<style>
	.search-overlay {
		position: fixed;
		inset: 0;
		background: var(--color-bg);
		z-index: var(--z-overlay);
		display: flex;
		flex-direction: column;
	}

	.search-header {
		display: flex;
		gap: var(--space-sm);
		padding: var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	input {
		flex: 1;
		padding: 0.5rem 0.75rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.9rem;
		outline: none;
	}

	input:focus {
		border-color: var(--color-accent);
	}

	.cancel-btn {
		padding: 0.5rem 0.75rem;
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: 0.9rem;
		cursor: pointer;
	}

	.search-results {
		flex: 1;
		overflow-y: auto;
	}

	.result-row {
		display: flex;
		flex-direction: column;
		gap: 2px;
		width: 100%;
		padding: 0.75rem var(--space-md);
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
	}

	.result-row:active {
		background: var(--color-surface);
	}

	.result-call {
		font-family: monospace;
		font-weight: 600;
		font-size: 0.95rem;
	}

	.result-alias {
		color: var(--color-accent);
	}

	.result-secondary {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		font-weight: 400;
	}

	.result-comment {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--color-text-muted);
	}
</style>
