<script lang="ts">
	import { stationList } from '$lib/stores/stations';
	import { stationKey as getStationKey } from '$lib/utils';
	import { getTacticalAlias } from '$lib/stores/tactical';
	import { activeNet, activeCheckIns } from '$lib/stores/netcontrol';
	import { connectionState } from '$lib/stores/ui';
	import type { PanelMode } from '$lib/stores/ui';
	import { canAdmin, pendingRequests } from '$lib/stores/session';
	import UserMenu from './UserMenu.svelte';

	let {
		panelMode = 'closed',
		unreadCount = 0,
		onToggle,
		onSelectStation,
		onCommandPalette
	}: {
		panelMode?: PanelMode;
		unreadCount?: number;
		onToggle?: (mode: PanelMode) => void;
		onSelectStation?: (key: string) => void;
		onCommandPalette?: () => void;
	} = $props();

	let isMac = $state(false);
	import { onMount } from 'svelte';
	onMount(() => {
		isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0;
	});

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

	/* Help popover */
	let helpOpen = $state(false);

	function toggleHelp() {
		helpOpen = !helpOpen;
	}

	function handleHelpBlur() {
		setTimeout(() => { helpOpen = false; }, 200);
	}

	/* Search popover */
	let searchOpen = $state(false);
	let searchFocused = $state(false);
	let searchQuery = $state('');
	let inputEl = $state<HTMLInputElement>();

	let searchResults = $derived.by(() => {
		if (!searchQuery) return [];
		const q = searchQuery.toUpperCase();
		const lookup = $getTacticalAlias;
		return $stationList
			.filter((s) =>
				s.callsign.includes(q) ||
				(s.comment ?? '').toUpperCase().includes(q) ||
				(lookup(getStationKey(s)) ?? '').toUpperCase().includes(q)
			)
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
		<!-- Logo -->
		<div class="rail-logo">
			<img src="/nymeria-logo.png" alt="Nymeria" width="28" height="28" />
		</div>

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
								{@const alias = $getTacticalAlias(key)}
								<button class="search-result" onmousedown={() => handleSelect(key)}>
									{#if alias}
										<span class="result-call"><span class="result-alias">{alias}</span> <span class="result-secondary">{station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign}</span></span>
									{:else}
										<span class="result-call">{station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign}</span>
									{/if}
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

		<!-- Command Palette -->
		<button
			class="rail-btn cmd-palette-btn"
			onclick={onCommandPalette}
			title={isMac ? 'Command Palette (⌘K)' : 'Command Palette (Ctrl+K)'}
			aria-label="Command Palette"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M4 1v4H1M12 1v4h3M4 15v-4H1M12 15v-4h3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>
				<rect x="5" y="5" width="6" height="6" rx="1" stroke="currentColor" stroke-width="1.3"/>
			</svg>
			<span class="kbd-hint">{isMac ? '⌘K' : '^K'}</span>
		</button>

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
			class:active={panelMode === 'bulletins'}
			onclick={() => onToggle?.('bulletins')}
			title="Bulletin Board"
			aria-label="Bulletin Board"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M2 2h12v11H2z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/>
				<path d="M5 5.5h6M5 8h4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
			</svg>
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
			class:active={panelMode === 'weather'}
			onclick={() => onToggle?.('weather')}
			title="Weather"
			aria-label="Weather"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<circle cx="8" cy="8" r="3" stroke="currentColor" stroke-width="1.5"/>
				<path d="M8 1v2M8 13v2M1 8h2M13 8h2M3 3l1.5 1.5M11.5 11.5L13 13M13 3l-1.5 1.5M4.5 11.5L3 13" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
			</svg>
		</button>

		<button
			class="rail-btn"
			class:active={panelMode === 'telemetry'}
			onclick={() => onToggle?.('telemetry')}
			title="Telemetry"
			aria-label="Telemetry"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M1 12l3-4 3 2 4-6 4 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
			</svg>
		</button>

		<button
			class="rail-btn"
			class:active={panelMode === 'df'}
			onclick={() => onToggle?.('df')}
			title="Direction Finding"
			aria-label="Direction Finding"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.3"/>
				<circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.3"/>
				<path d="M8 2v3M8 11v3M2 8h3M11 8h3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
			</svg>
		</button>

		<button
			class="rail-btn"
			class:active={panelMode === 'packets'}
			onclick={() => onToggle?.('packets')}
			title="Packet Inspector"
			aria-label="Packet Inspector"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<rect x="1" y="3" width="14" height="11" rx="1" stroke="currentColor" stroke-width="1.3"/>
				<path d="M3 6h2M3 8.5h4M3 11h3" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
				<path d="M1 5.5h14" stroke="currentColor" stroke-width="1.3"/>
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
		{#if $canAdmin}
			<button
				class="rail-btn"
				class:active={panelMode === 'settings'}
				onclick={() => onToggle?.('settings')}
				title="Settings"
				aria-label="Settings"
			>
				<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
					<path d="M6.8 1.5h2.4l.3 1.8.8.3 1.5-1 1.7 1.7-1 1.5.3.8 1.8.3v2.4l-1.8.3-.3.8 1 1.5-1.7 1.7-1.5-1-.8.3-.3 1.8H6.8l-.3-1.8-.8-.3-1.5 1-1.7-1.7 1-1.5-.3-.8-1.8-.3V6.8l1.8-.3.3-.8-1-1.5 1.7-1.7 1.5 1 .8-.3.3-1.8z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/>
					<circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.2"/>
				</svg>
				{#if $pendingRequests.length > 0}
					<span class="pending-badge">{$pendingRequests.length}</span>
				{/if}
			</button>
		{/if}
		<div class="help-container">
			<button
				class="rail-btn help-btn"
				class:active={helpOpen}
				onclick={toggleHelp}
				onblur={handleHelpBlur}
				title="Keyboard shortcuts"
				aria-label="Keyboard shortcuts"
			>
				<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
					<circle cx="8" cy="8" r="7" stroke="currentColor" stroke-width="1.4"/>
					<path d="M6 6a2 2 0 114 0c0 1-1.5 1.25-1.5 2.5M8 11.5h.01" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
				</svg>
			</button>
			{#if helpOpen}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div class="help-popover" onmousedown={(e) => e.preventDefault()}>
					<div class="help-title">Keyboard shortcuts</div>
					<div class="help-section">
						<span class="help-section-label">Global</span>
						<div class="help-row"><kbd>{isMac ? '⌘' : 'Ctrl'}+K</kbd><span>Command palette</span></div>
						<div class="help-row"><kbd>/</kbd><span>Command palette (when not typing)</span></div>
					</div>
					<div class="help-section">
						<span class="help-section-label">Command palette &mdash; search</span>
						<div class="help-row"><kbd>&uarr; &darr;</kbd><span>Navigate results</span></div>
						<div class="help-row"><kbd>Enter</kbd><span>Select station</span></div>
						<div class="help-row"><kbd>Esc</kbd><span>Close</span></div>
					</div>
					<div class="help-section">
						<span class="help-section-label">Command palette &mdash; station</span>
						<div class="help-row"><kbd>N</kbd><span>Focus note composer</span></div>
						<div class="help-row"><kbd>{isMac ? '⌘' : 'Ctrl'}+Enter</kbd><span>Save note</span></div>
						<div class="help-row"><kbd>S</kbd><span>Change status</span></div>
						<div class="help-row"><kbd>M</kbd><span>Assign mission</span></div>
						<div class="help-row"><kbd>F</kbd><span>Fly to on map</span></div>
						<div class="help-row"><kbd>Backspace</kbd><span>Back to search</span></div>
					</div>
				</div>
			{/if}
		</div>
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

	.rail-logo {
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-sm) 0;
		margin-bottom: var(--space-xs);
	}

	.rail-logo img {
		object-fit: contain;
		opacity: 0.9;
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

	.pending-badge {
		position: absolute;
		top: 4px;
		right: 4px;
		min-width: 14px;
		height: 14px;
		padding: 0 3px;
		background: var(--color-accent);
		color: white;
		border-radius: 7px;
		font-size: 0.6rem;
		font-weight: 700;
		display: flex;
		align-items: center;
		justify-content: center;
		pointer-events: none;
	}

	/* Command palette button */
	.cmd-palette-btn {
		position: relative;
	}

	.kbd-hint {
		position: absolute;
		bottom: 2px;
		right: 0;
		font-size: 0.5rem;
		font-weight: 600;
		color: var(--color-text-muted);
		line-height: 1;
		pointer-events: none;
		opacity: 0.7;
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

	.result-alias {
		color: var(--color-accent);
	}

	.result-secondary {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-weight: 400;
	}

	.result-comment {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Help button + popover */
	.help-container {
		position: relative;
	}

	.help-btn {
		width: 32px;
		height: 32px;
		opacity: 0.5;
		transition: opacity var(--duration-fast), color var(--duration-fast), background var(--duration-fast);
	}

	.help-btn:hover,
	.help-btn.active {
		opacity: 1;
	}

	.help-popover {
		position: absolute;
		bottom: -4px;
		right: calc(100% + 12px);
		width: 280px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg);
		padding: 12px 14px;
		z-index: 10;
	}

	.help-title {
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--color-text);
		margin-bottom: 10px;
	}

	.help-section {
		margin-bottom: 10px;
	}

	.help-section:last-child {
		margin-bottom: 0;
	}

	.help-section-label {
		display: block;
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
		margin-bottom: 4px;
	}

	.help-row {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 2px 0;
		font-size: 0.78rem;
		color: var(--color-text-muted);
	}

	.help-row kbd {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 22px;
		padding: 1px 5px;
		font-size: 0.68rem;
		font-family: inherit;
		background: var(--color-primary);
		border-radius: 4px;
		color: var(--color-text);
		white-space: nowrap;
	}

	.help-row span {
		color: var(--color-text);
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
