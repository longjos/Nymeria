<script lang="ts">
	import { stationList } from '$lib/stores/stations';
	import { stationKey as getStationKey } from '$lib/utils';
	import ConnectionStatus from './ConnectionStatus.svelte';

	let {
		unreadCount = 0,
		onSearchOpen,
		onStationsOpen,
		onMessagesOpen,
		onSelectStation
	}: {
		unreadCount?: number;
		onSearchOpen?: () => void;
		onStationsOpen?: () => void;
		onMessagesOpen?: () => void;
		onSelectStation?: (key: string) => void;
	} = $props();

	let searchFocused = $state(false);
	let searchQuery = $state('');
	let stationCount = $derived($stationList.length);

	let searchResults = $derived.by(() => {
		if (!searchQuery) return [];
		const q = searchQuery.toUpperCase();
		return $stationList
			.filter((s) => s.callsign.includes(q) || (s.comment ?? '').toUpperCase().includes(q))
			.slice(0, 8);
	});

	let showDropdown = $derived(searchFocused && searchQuery.length > 0 && searchResults.length > 0);

	function handleSelect(key: string) {
		searchQuery = '';
		searchFocused = false;
		onSelectStation?.(key);
	}
</script>

<!-- Desktop toolbar -->
<div class="toolbar desktop-only">
	<div class="search-group">
		<div class="search-wrap">
			<svg class="search-icon" width="14" height="14" viewBox="0 0 16 16" fill="none">
				<circle cx="6.5" cy="6.5" r="5.5" stroke="currentColor" stroke-width="1.5"/>
				<path d="M10.5 10.5L15 15" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
			</svg>
			<input
				type="search"
				placeholder="Search stations..."
				bind:value={searchQuery}
				onfocus={() => (searchFocused = true)}
				onblur={() => setTimeout(() => (searchFocused = false), 200)}
			/>
		</div>
		{#if showDropdown}
			<div class="search-dropdown">
				{#each searchResults as station}
					{@const key = getStationKey(station)}
					<button class="search-result" onmousedown={() => handleSelect(key)}>
						<span class="result-call">{station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign}</span>
						{#if station.comment}
							<span class="result-comment">{station.comment}</span>
						{/if}
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<button class="toolbar-btn" onclick={onStationsOpen} title="Stations">
		<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
			<circle cx="8" cy="5" r="3" stroke="currentColor" stroke-width="1.5"/>
			<path d="M2 14c0-3.3 2.7-6 6-6s6 2.7 6 6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
		</svg>
		<span>{stationCount}</span>
	</button>

	<button class="toolbar-btn" onclick={onMessagesOpen} title="Messages">
		<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
			<path d="M2 3h12v8H4l-2 2V3z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
		</svg>
		{#if unreadCount > 0}
			<span class="unread-badge">{unreadCount}</span>
		{/if}
	</button>

	<ConnectionStatus />
</div>

<!-- Mobile floating buttons -->
<div class="mobile-toolbar mobile-only">
	<button class="fab" onclick={onSearchOpen} title="Search">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<circle cx="6.5" cy="6.5" r="5.5" stroke="currentColor" stroke-width="1.5"/>
			<path d="M10.5 10.5L15 15" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
		</svg>
	</button>
	<button class="fab" onclick={onMessagesOpen} title="Messages">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<path d="M2 3h12v8H4l-2 2V3z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
		</svg>
		{#if unreadCount > 0}
			<span class="fab-badge">{unreadCount}</span>
		{/if}
	</button>
</div>

<style>
	/* Desktop */
	.toolbar {
		position: fixed;
		top: var(--space-md);
		left: var(--space-md);
		z-index: var(--z-toolbar);
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		pointer-events: auto;
	}

	.search-group {
		position: relative;
	}

	.search-wrap {
		display: flex;
		align-items: center;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		padding: 0 10px;
		transition: border-color var(--duration-fast);
	}

	.search-wrap:focus-within {
		border-color: var(--color-accent);
	}

	.search-icon {
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.search-wrap input {
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 0.85rem;
		padding: 6px 8px;
		width: 180px;
		outline: none;
	}

	.search-wrap input::placeholder {
		color: var(--color-text-muted);
	}

	.search-dropdown {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-md);
		overflow: hidden;
		max-height: 320px;
		overflow-y: auto;
	}

	.search-result {
		display: flex;
		flex-direction: column;
		gap: 1px;
		width: 100%;
		padding: 8px 12px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
		transition: background var(--duration-fast);
	}

	.search-result:last-child {
		border-bottom: none;
	}

	.search-result:hover {
		background: var(--color-primary);
	}

	.result-call {
		font-family: monospace;
		font-weight: 600;
		font-size: 0.85rem;
	}

	.result-comment {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.toolbar-btn {
		display: flex;
		align-items: center;
		gap: 5px;
		padding: 6px 10px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		font-size: 0.8rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
		pointer-events: auto;
	}

	.toolbar-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.unread-badge {
		background: var(--color-accent);
		color: white;
		font-size: 0.6rem;
		font-weight: 700;
		padding: 1px 5px;
		border-radius: 10px;
	}

	/* Mobile */
	.mobile-toolbar {
		position: fixed;
		top: var(--space-md);
		right: var(--space-md);
		z-index: var(--z-toolbar);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		pointer-events: auto;
	}

	.fab {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 50%;
		color: var(--color-text);
		cursor: pointer;
		box-shadow: var(--shadow-md);
		position: relative;
	}

	.fab-badge {
		position: absolute;
		top: -2px;
		right: -2px;
		background: var(--color-accent);
		color: white;
		font-size: 0.55rem;
		font-weight: 700;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}
</style>
