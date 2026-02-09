<script lang="ts">
	import { stationList } from '$lib/stores/stations';
	import { stationKey as getStationKey } from '$lib/utils';
	import { activeNet, activeCheckIns } from '$lib/stores/netcontrol';
	import { connectionState } from '$lib/stores/ui';
	import type { PanelMode } from '$lib/stores/ui';
	import UserMenu from './UserMenu.svelte';

	let {
		panelMode = 'closed',
		unreadCount = 0,
		onToggle,
		onSelectStation
	}: {
		panelMode?: PanelMode;
		unreadCount?: number;
		onToggle?: (mode: PanelMode) => void;
		onSelectStation?: (key: string) => void;
	} = $props();

	let stationCount = $derived($stationList.length);
	let netActive = $derived($activeNet?.status === 'open');
	let netOpCount = $derived($activeCheckIns.length);

	let connState = $derived($connectionState);
	let dotColor = $derived.by(() => {
		switch (connState) {
			case 'connected': return 'var(--color-connected)';
			case 'disconnected': return 'var(--color-disconnected)';
			case 'reconnecting': return 'var(--color-reconnecting)';
		}
	});
	let connLabel = $derived.by(() => {
		switch (connState) {
			case 'connected': return 'Connected';
			case 'disconnected': return 'Disconnected';
			case 'reconnecting': return 'Reconnecting';
		}
	});

	/* Search popover */
	let searchOpen = $state(false);
	let searchFocused = $state(false);
	let searchQuery = $state('');
	let inputEl = $state<HTMLInputElement>();

	let searchResults = $derived.by(() => {
		if (!searchQuery) return [];
		const q = searchQuery.toUpperCase();
		return $stationList
			.filter((s) => s.callsign.includes(q) || (s.comment ?? '').toUpperCase().includes(q))
			.slice(0, 8);
	});

	let showDropdown = $derived(searchFocused && searchQuery.length > 0 && searchResults.length > 0);

	function toggleSearch() {
		searchOpen = !searchOpen;
		if (searchOpen) {
			// Wait a tick for the input to render, then focus
			requestAnimationFrame(() => inputEl?.focus());
		} else {
			searchQuery = '';
		}
	}

	function handleSelect(key: string) {
		searchQuery = '';
		searchOpen = false;
		searchFocused = false;
		onSelectStation?.(key);
	}

	function handleSearchBlur() {
		setTimeout(() => {
			searchFocused = false;
			if (!searchQuery) searchOpen = false;
		}, 200);
	}

	function handleSearchKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			searchQuery = '';
			searchOpen = false;
			searchFocused = false;
		}
	}
</script>

<nav class="activity-rail desktop-only" aria-label="Navigation rail">
	<div class="rail-top">
		<!-- Search -->
		<div class="search-container">
			<button
				class="rail-btn"
				class:active={searchOpen}
				onclick={toggleSearch}
				title="Search stations"
				aria-label="Search stations"
			>
				<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
					<circle cx="6.5" cy="6.5" r="5.5" stroke="currentColor" stroke-width="1.5"/>
					<path d="M10.5 10.5L15 15" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
				</svg>
			</button>

			{#if searchOpen}
				<div class="search-popover">
					<input
						bind:this={inputEl}
						type="search"
						placeholder="Search stations..."
						bind:value={searchQuery}
						onfocus={() => (searchFocused = true)}
						onblur={handleSearchBlur}
						onkeydown={handleSearchKeydown}
					/>
					{#if showDropdown}
						<div class="search-results">
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
			{/if}
		</div>

		<div class="rail-divider"></div>

		<!-- Navigation icons -->
		<button
			class="rail-btn"
			class:active={panelMode === 'stations'}
			onclick={() => onToggle?.('stations')}
			title="Stations"
			aria-label="Stations ({stationCount})"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<circle cx="8" cy="5" r="3" stroke="currentColor" stroke-width="1.5"/>
				<path d="M2 14c0-3.3 2.7-6 6-6s6 2.7 6 6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
			</svg>
			{#if stationCount > 0}
				<span class="badge">{stationCount}</span>
			{/if}
		</button>

		<button
			class="rail-btn"
			class:active={panelMode === 'messages'}
			onclick={() => onToggle?.('messages')}
			title="Messages"
			aria-label="Messages{unreadCount > 0 ? ` (${unreadCount} unread)` : ''}"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M2 3h12v8H4l-2 2V3z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
			</svg>
			{#if unreadCount > 0}
				<span class="badge accent">{unreadCount}</span>
			{/if}
		</button>

		<button
			class="rail-btn"
			class:active={panelMode === 'netcontrol'}
			class:net-active={netActive}
			onclick={() => onToggle?.('netcontrol')}
			title="Net Control"
			aria-label="Net Control{netActive ? ` (${netOpCount} operators)` : ''}"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M8 1v4M4.5 3L6 6M11.5 3L10 6M8 6v5M5 11h6M3 14h10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
			</svg>
			{#if netActive}
				<span class="badge net">{netOpCount}</span>
			{/if}
		</button>

		<button
			class="rail-btn"
			class:active={panelMode === 'annotations'}
			onclick={() => onToggle?.('annotations')}
			title="Annotations"
			aria-label="Annotations"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M8 1a5 5 0 00-5 5c0 4 5 9 5 9s5-5 5-9a5 5 0 00-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z" stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
			</svg>
		</button>

		<button
			class="rail-btn"
			class:active={panelMode === 'transports'}
			onclick={() => onToggle?.('transports')}
			title="Transports"
			aria-label="Transports"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M8 1v4M8 11v4M1 8h4M11 8h4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
				<circle cx="8" cy="8" r="3" stroke="currentColor" stroke-width="1.5"/>
			</svg>
		</button>

		<button
			class="rail-btn"
			class:active={panelMode === 'activity'}
			onclick={() => onToggle?.('activity')}
			title="Activity Log"
			aria-label="Activity Log"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M2 2v12h12M5 10l3-3 2 2 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
			</svg>
		</button>
	</div>

	<div class="rail-bottom">
		<div class="conn-dot" title={connLabel}>
			<span class="dot" class:pulse={connState === 'reconnecting'} style="background: {dotColor}"></span>
		</div>
		<UserMenu position="rail" />
	</div>
</nav>

<style>
	.activity-rail {
		position: fixed;
		top: 0;
		right: 0;
		bottom: 0;
		width: var(--rail-width);
		background: var(--color-surface);
		border-left: 1px solid var(--color-primary);
		z-index: var(--z-toolbar);
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: var(--space-sm) 0;
	}

	.rail-top {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		flex: 1;
	}

	.rail-bottom {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-md);
		padding-bottom: var(--space-sm);
	}

	.rail-divider {
		width: 24px;
		height: 1px;
		background: var(--color-primary);
		margin: 4px 0;
	}

	.rail-btn {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: color var(--duration-fast), background var(--duration-fast);
	}

	.rail-btn:hover {
		color: var(--color-text);
		background: var(--color-primary);
	}

	.rail-btn.active {
		color: var(--color-accent);
		background: var(--color-primary);
		box-shadow: inset 3px 0 0 var(--color-accent);
	}

	.rail-btn.net-active {
		color: #22c55e;
	}

	.rail-btn.net-active.active {
		color: #22c55e;
		box-shadow: inset 3px 0 0 #22c55e;
	}

	.badge {
		position: absolute;
		top: 4px;
		right: 2px;
		font-size: 0.5rem;
		font-weight: 700;
		padding: 1px 4px;
		border-radius: 10px;
		background: var(--color-primary);
		color: var(--color-text);
		line-height: 1.2;
		min-width: 14px;
		text-align: center;
	}

	.badge.accent {
		background: var(--color-accent);
		color: white;
	}

	.badge.net {
		background: #22c55e;
		color: #000;
	}

	/* Search popover */
	.search-container {
		position: relative;
	}

	.search-popover {
		position: absolute;
		top: 0;
		right: calc(100% + 8px);
		width: 240px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-md);
		overflow: hidden;
	}

	.search-popover input {
		width: 100%;
		padding: 8px 12px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		font-size: 0.85rem;
		outline: none;
	}

	.search-popover input::placeholder {
		color: var(--color-text-muted);
	}

	.search-results {
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

	/* Connection dot */
	.conn-dot {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 24px;
		cursor: default;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}

	.dot.pulse {
		animation: pulse 1.5s ease-in-out infinite;
	}

	@keyframes pulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.3; }
	}
</style>
