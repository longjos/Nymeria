<script lang="ts">
	import { activeNet, activeCheckIns } from '$lib/stores/netcontrol';
	import { wxUnackedCount, wxActiveWarningIn } from '$lib/stores/wxAlerts';
	import { canAdmin } from '$lib/stores/session';
	import { visiblePanelModes } from '$lib/stores/netProfile';
	import { panelModeVisible } from '$lib/profilePanels';
	import type { PanelMode } from '$lib/stores/ui';

	let {
		unreadCount = 0,
		activeMode = 'closed' as PanelMode,
		onSearchOpen,
		onMessagesOpen,
		onBulletinsOpen,
		onTransportsOpen,
		onAnnotationsOpen,
		onNetControlOpen,
		onWeatherOpen,
		onDFOpen,
		onPacketsOpen,
		onSettingsOpen,
		onCommandPalette
	}: {
		unreadCount?: number;
		activeMode?: PanelMode;
		onSearchOpen?: () => void;
		onMessagesOpen?: () => void;
		onBulletinsOpen?: () => void;
		onTransportsOpen?: () => void;
		onAnnotationsOpen?: () => void;
		onNetControlOpen?: () => void;
		onWeatherOpen?: () => void;
		onDFOpen?: () => void;
		onPacketsOpen?: () => void;
		onSettingsOpen?: () => void;
		onCommandPalette?: () => void;
	} = $props();

	let netActive = $derived($activeNet?.status === 'open');
	let netOpCount = $derived($activeCheckIns.length);

	/** Same gating as ActivityRail.svelte's desktop rail — see profilePanels.ts. */
	let visible = $derived((m: PanelMode) => panelModeVisible($visiblePanelModes, m));
</script>

<!-- Mobile navigation rail: lives in the bottom sheet's peek area only (P0-2 —
     +page.svelte renders this snippet exclusively at sheetState 'peek'; above
     peek the user has already chosen a destination and the sheet shows a
     context bar instead), so no target can ever be occluded by the sheet
     painting over it. -->
<div
	class="mobile-toolbar mobile-only"
	role="navigation"
	aria-label="Panels"
>
	<button class="fab search-fab" onclick={onSearchOpen} title="Search" aria-label="Search stations">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<circle cx="6.5" cy="6.5" r="5.5" stroke="currentColor" stroke-width="1.5"/>
			<path d="M10.5 10.5L15 15" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
		</svg>
		<span class="fab-label">Search</span>
	</button>
	<button class="fab cmd-fab" onclick={onCommandPalette} title="Command Palette" aria-label="Command palette">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<path d="M4 1v4H1M12 1v4h3M4 15v-4H1M12 15v-4h3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>
			<rect x="5" y="5" width="6" height="6" rx="1" stroke="currentColor" stroke-width="1.3"/>
		</svg>
		<span class="fab-label">Command</span>
	</button>
	<button
		class="fab"
		class:net-active={netActive}
		class:active={activeMode === 'netcontrol'}
		aria-current={(activeMode === 'netcontrol') ? 'page' : undefined}
		onclick={onNetControlOpen}
		title="Net Control"
		aria-label="Net control"
	>
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<path d="M8 1v4M4.5 3L6 6M11.5 3L10 6M8 6v5M5 11h6M3 14h10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
		</svg>
		<span class="fab-label">Net</span>
		{#if netActive}
			<span class="fab-badge">{netOpCount}</span>
		{/if}
	</button>
	<button
		class="fab"
		class:active={activeMode === 'messages' || activeMode === 'convo'}
		aria-current={(activeMode === 'messages' || activeMode === 'convo') ? 'page' : undefined}
		onclick={onMessagesOpen}
		title="Messages"
		aria-label="Messages"
	>
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<path d="M2 3h12v8H4l-2 2V3z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
		</svg>
		<span class="fab-label">Messages</span>
		{#if unreadCount > 0}
			<span class="fab-badge">{unreadCount}</span>
		{/if}
	</button>
	<button
		class="fab"
		class:active={activeMode === 'annotations'}
		aria-current={(activeMode === 'annotations') ? 'page' : undefined}
		onclick={onAnnotationsOpen}
		title="Annotations"
		aria-label="Annotations"
	>
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<path d="M8 1a5 5 0 00-5 5c0 4 5 9 5 9s5-5 5-9a5 5 0 00-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z" stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
		</svg>
		<span class="fab-label">Annotations</span>
	</button>
	{#if visible('weather')}
		<button
			class="fab"
			class:active={activeMode === 'weather'}
			aria-current={(activeMode === 'weather') ? 'page' : undefined}
			onclick={onWeatherOpen}
			title="Weather{$wxUnackedCount > 0 ? ` — ${$wxUnackedCount} unacknowledged NWS alerts` : ''}"
			aria-label="Weather{$wxUnackedCount > 0 ? `, ${$wxUnackedCount} unacknowledged NWS alerts` : ''}"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
				<circle cx="8" cy="8" r="3" stroke="currentColor" stroke-width="1.5"/>
				<path d="M8 1v2M8 13v2M1 8h2M13 8h2M3 3l1.5 1.5M11.5 11.5L13 13M13 3l-1.5 1.5M4.5 11.5L3 13" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
			</svg>
			<span class="fab-label">Weather</span>
			{#if $wxUnackedCount > 0}
				<span class="fab-badge wx" class:wx-warning={!!$wxActiveWarningIn} title="{$wxUnackedCount} unacknowledged NWS alerts">{$wxUnackedCount}</span>
			{/if}
		</button>
	{/if}
	<button
		class="fab"
		class:active={activeMode === 'transports'}
		aria-current={(activeMode === 'transports') ? 'page' : undefined}
		onclick={onTransportsOpen}
		title="Transports"
		aria-label="Transports"
	>
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<path d="M8 1v4M8 11v4M1 8h4M11 8h4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
			<circle cx="8" cy="8" r="3" stroke="currentColor" stroke-width="1.5"/>
		</svg>
		<span class="fab-label">Transports</span>
	</button>
	<button
		class="fab"
		class:active={activeMode === 'bulletins'}
		aria-current={(activeMode === 'bulletins') ? 'page' : undefined}
		onclick={onBulletinsOpen}
		title="Bulletin Board"
		aria-label="Bulletin board"
	>
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<path d="M2 2h12v11H2z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/>
			<path d="M5 5.5h6M5 8h4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
		</svg>
		<span class="fab-label">Bulletins</span>
	</button>
	<button
		class="fab"
		class:active={activeMode === 'df'}
		aria-current={(activeMode === 'df') ? 'page' : undefined}
		onclick={onDFOpen}
		title="Direction Finding"
		aria-label="Direction finding"
	>
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.3"/>
			<circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.3"/>
			<path d="M8 2v3M8 11v3M2 8h3M11 8h3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
		</svg>
		<span class="fab-label">Direction</span>
	</button>
	<button
		class="fab"
		class:active={activeMode === 'packets'}
		aria-current={(activeMode === 'packets') ? 'page' : undefined}
		onclick={onPacketsOpen}
		title="Packet Inspector"
		aria-label="Packet inspector"
	>
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
			<rect x="1" y="3" width="14" height="11" rx="1" stroke="currentColor" stroke-width="1.3"/>
			<path d="M3 6h2M3 8.5h4M3 11h3" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
			<path d="M1 5.5h14" stroke="currentColor" stroke-width="1.3"/>
		</svg>
		<span class="fab-label">Packets</span>
	</button>
	{#if $canAdmin}
		<button
			class="fab"
			class:active={activeMode === 'settings'}
		aria-current={(activeMode === 'settings') ? 'page' : undefined}
			onclick={onSettingsOpen}
			title="Settings"
			aria-label="Settings"
		>
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
				<path d="M6.8 1.5h2.4l.3 1.8.8.3 1.5-1 1.7 1.7-1 1.5.3.8 1.8.3v2.4l-1.8.3-.3.8 1 1.5-1.7 1.7-1.5-1-.8.3-.3 1.8H6.8l-.3-1.8-.8-.3-1.5 1-1.7-1.7 1-1.5-.3-.8-1.8-.3V6.8l1.8-.3.3-.8-1-1.5 1.7-1.7 1.5 1 .8-.3.3-1.8z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/>
				<circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.2"/>
			</svg>
			<span class="fab-label">Settings</span>
		</button>
	{/if}
</div>

<style>
	.mobile-toolbar {
		display: flex;
		flex-direction: row;
		align-items: stretch;
		gap: var(--space-sm);
		overflow-x: auto;
		overscroll-behavior-x: contain;
		touch-action: pan-x;
		scroll-snap-type: x proximity;
		flex-wrap: nowrap;
		scrollbar-width: none;
		-webkit-overflow-scrolling: touch;
		padding-bottom: var(--space-sm);
		pointer-events: auto;
		/* Fade only the trailing edge. A leading fade would also wash out the
		   sticky Search button pinned at left: 0, making the one always-visible
		   control look disabled. */
		mask-image: linear-gradient(to right, #000 calc(100% - 12px), transparent 100%);
		-webkit-mask-image: linear-gradient(to right, #000 calc(100% - 12px), transparent 100%);
	}

	.mobile-toolbar::-webkit-scrollbar { display: none; }

	.fab {
		flex: 0 0 auto;
		scroll-snap-align: start;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 2px;
		width: 76px;
		height: 58px;
		padding: 0 4px;
		box-sizing: border-box;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		color: var(--color-text);
		cursor: pointer;
		box-shadow: none;
		position: relative;
	}

	/* Search is the escape hatch: pinned so it never scrolls out of reach. */
	.fab.search-fab {
		position: sticky;
		left: 0;
		z-index: 1;
		background: var(--color-bg);
	}

	.fab-label {
		/* 12px, not 10px: this label is the only thing naming the control on a
		   hoverless device, read at arm's length in sunlight. Letter-spacing dropped
		   to buy back the width the larger size costs. */
		font-size: 0.75rem;
		line-height: 1;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text-muted);
	}

	.fab.active {
		border-color: var(--color-accent);
	}

	.fab.active .fab-label { color: var(--color-text); }

	.fab.cmd-fab { border-color: var(--color-accent); color: var(--color-accent); }
	.fab.net-active { border-color: var(--color-success); color: var(--color-success); }

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

	.fab-badge.wx {
		background: var(--color-wx-watch);
		color: #000;
	}

	.fab-badge.wx.wx-warning {
		background: var(--color-wx-warning);
		color: #fff;
	}
</style>
