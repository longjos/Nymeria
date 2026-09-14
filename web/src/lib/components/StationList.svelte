<script lang="ts">
	import StationCard from './StationCard.svelte';
	import { getTacticalAlias } from '$lib/stores/tactical';
	import { rosterScopedStations } from '$lib/stores/rosterScope';
	import { stationKey } from '$lib/utils';

	let {
		onSelect,
		selectedKey
	}: {
		onSelect?: (key: string) => void;
		selectedKey?: string | null;
	} = $props();

	// The app-wide roster scope (#106): while a net is running and "Roster only" is
	// on, this list shows exactly what the map shows. `hidden` drives the banner —
	// a list that silently drops 288 stations mid-incident is worse than the bug.
	let scope = $derived($rosterScopedStations);
	let scopedStations = $derived(scope.stations);

	let search = $state('');
	// Empty string means "all transports". Otherwise holds a transport
	// display name (custom name if configured, else the transport type).
	let sourceFilter = $state('');

	// Friendly labels for bare transport type names, used when a transport has
	// no custom name configured. Custom names pass through unchanged, and any
	// unknown type falls back to its raw value, so the UI stays generic across
	// deployments and never hardcodes deployment-specific transport instances.
	const SOURCE_LABELS: Record<string, string> = {
		aprsis: 'Internet',
		kisstcp: 'KISS TCP',
		serial: 'Serial'
	};
	function sourceLabel(type: string): string {
		return SOURCE_LABELS[type] ?? type;
	}

	// The set of transports a station has been heard on. Prefers the new
	// `sources` array (transport display names), falling back to splitting the
	// legacy `source` summary (which may be a single value or a '+'-joined set).
	function stationSources(s: { sources?: string[]; source?: string }): string[] {
		if (s.sources && s.sources.length) return s.sources;
		if (s.source) return s.source.split('+');
		return [];
	}

	// Transport types actually present in the current roster, sorted, so the
	// chip row only shows filters that can match something.
	let availableSources = $derived.by(() => {
		const seen = new Set<string>();
		for (const s of scopedStations) {
			for (const t of stationSources(s)) seen.add(t);
		}
		return Array.from(seen).sort();
	});

	let filteredStations = $derived.by(() => {
		let list = scopedStations;
		if (sourceFilter) {
			list = list.filter((s) => stationSources(s).includes(sourceFilter));
		}
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
		<span class="count">{filteredStations.length} stations</span>
		<input
			type="search"
			placeholder="Filter..."
			bind:value={search}
		/>
	</div>
	{#if scope.scoped}
		<p class="scope-note">
			<span class="scope-tag">ROSTER ONLY</span>
			<span>{scope.hidden} other station{scope.hidden === 1 ? '' : 's'} hidden</span>
		</p>
	{/if}
	{#if availableSources.length > 1}
		<div class="source-chips" role="group" aria-label="Filter by transport">
			<button
				class="chip"
				class:active={sourceFilter === ''}
				onclick={() => (sourceFilter = '')}
			>All</button>
			{#each availableSources as type (type)}
				<button
					class="chip"
					class:active={sourceFilter === type}
					onclick={() => (sourceFilter = sourceFilter === type ? '' : type)}
				>{sourceLabel(type)}</button>
			{/each}
		</div>
	{/if}
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
				{search || sourceFilter ? 'No matching stations' : 'No stations heard yet'}
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

	/* Roster-scope banner: text-first, never colour alone — it has to read in
	   direct sun on a phone held one-handed. */
	.scope-note {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		flex-wrap: wrap;
		margin: 0;
		padding: var(--space-xs) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		background: rgba(233, 69, 96, 0.12);
		color: var(--color-text);
		font-size: 0.75rem;
		flex-shrink: 0;
	}

	.scope-tag {
		padding: 0.1rem 0.4rem;
		border: 1px solid var(--color-accent);
		border-radius: var(--radius-sm);
		color: var(--color-accent);
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		white-space: nowrap;
	}

	.source-chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-xs);
		padding: var(--space-xs) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.chip {
		padding: 0.2rem 0.6rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 999px;
		color: var(--color-text-muted);
		font-size: 0.72rem;
		cursor: pointer;
		transition: color 0.12s, border-color 0.12s, background 0.12s;
	}

	.chip:hover {
		color: var(--color-text);
		border-color: var(--color-accent);
	}

	.chip.active {
		color: var(--color-bg, #0c0f1a);
		background: var(--color-accent);
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
