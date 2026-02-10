<script lang="ts">
	import { onMount } from 'svelte';
	import { activeNet, activeCheckIns } from '$lib/stores/netcontrol';
	import { canAdmin } from '$lib/stores/session';

	let isMac = $state(false);
	let helpOpen = $state(false);

	onMount(() => {
		isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0;
	});

	let {
		unreadCount = 0,
		onSearchOpen,
		onMessagesOpen,
		onBulletinsOpen,
		onTransportsOpen,
		onAnnotationsOpen,
		onNetControlOpen,
		onWeatherOpen,
		onDFOpen,
		onSettingsOpen,
		onCommandPalette
	}: {
		unreadCount?: number;
		onSearchOpen?: () => void;
		onMessagesOpen?: () => void;
		onBulletinsOpen?: () => void;
		onTransportsOpen?: () => void;
		onAnnotationsOpen?: () => void;
		onNetControlOpen?: () => void;
		onWeatherOpen?: () => void;
		onDFOpen?: () => void;
		onSettingsOpen?: () => void;
		onCommandPalette?: () => void;
	} = $props();

	let netActive = $derived($activeNet?.status === 'open');
	let netOpCount = $derived($activeCheckIns.length);
</script>

<!-- Mobile floating buttons -->
<div class="mobile-toolbar mobile-only">
	<button class="fab" onclick={onSearchOpen} title="Search">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<circle cx="6.5" cy="6.5" r="5.5" stroke="currentColor" stroke-width="1.5"/>
			<path d="M10.5 10.5L15 15" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
		</svg>
	</button>
	<button class="fab cmd-fab" onclick={onCommandPalette} title="Command Palette">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<path d="M4 1v4H1M12 1v4h3M4 15v-4H1M12 15v-4h3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>
			<rect x="5" y="5" width="6" height="6" rx="1" stroke="currentColor" stroke-width="1.3"/>
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
	<button class="fab" onclick={onBulletinsOpen} title="Bulletin Board">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<path d="M2 2h12v11H2z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/>
			<path d="M5 5.5h6M5 8h4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
		</svg>
	</button>
	<button class="fab" onclick={onWeatherOpen} title="Weather">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<circle cx="8" cy="8" r="3" stroke="currentColor" stroke-width="1.5"/>
			<path d="M8 1v2M8 13v2M1 8h2M13 8h2M3 3l1.5 1.5M11.5 11.5L13 13M13 3l-1.5 1.5M4.5 11.5L3 13" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
		</svg>
	</button>
	<button class="fab" onclick={onDFOpen} title="Direction Finding">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.3"/>
			<circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.3"/>
			<path d="M8 2v3M8 11v3M2 8h3M11 8h3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
		</svg>
	</button>
	<button class="fab" onclick={onTransportsOpen} title="Transports">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<path d="M8 1v4M8 11v4M1 8h4M11 8h4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
			<circle cx="8" cy="8" r="3" stroke="currentColor" stroke-width="1.5"/>
		</svg>
	</button>
	<button class="fab" onclick={onAnnotationsOpen} title="Annotations">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<path d="M8 1a5 5 0 00-5 5c0 4 5 9 5 9s5-5 5-9a5 5 0 00-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z" stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
		</svg>
	</button>
	<button class="fab" class:net-active={netActive} onclick={onNetControlOpen} title="Net Control">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<path d="M8 1v4M4.5 3L6 6M11.5 3L10 6M8 6v5M5 11h6M3 14h10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
		</svg>
		{#if netActive}
			<span class="fab-badge">{netOpCount}</span>
		{/if}
	</button>
	{#if $canAdmin}
		<button class="fab" onclick={onSettingsOpen} title="Settings">
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<path d="M6.8 1.5h2.4l.3 1.8.8.3 1.5-1 1.7 1.7-1 1.5.3.8 1.8.3v2.4l-1.8.3-.3.8 1 1.5-1.7 1.7-1.5-1-.8.3-.3 1.8H6.8l-.3-1.8-.8-.3-1.5 1-1.7-1.7 1-1.5-.3-.8-1.8-.3V6.8l1.8-.3.3-.8-1-1.5 1.7-1.7 1.5 1 .8-.3.3-1.8z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/>
				<circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.2"/>
			</svg>
		</button>
	{/if}
	<div class="help-container">
		<button class="fab help-fab" onclick={() => { helpOpen = !helpOpen; }} title="Keyboard shortcuts">
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
</div>

<style>
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

	.fab.cmd-fab {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.fab.net-active {
		border-color: #22c55e;
		color: #22c55e;
	}

	/* Help button + popover */
	.help-container {
		position: relative;
	}

	.fab.help-fab {
		width: 36px;
		height: 36px;
		opacity: 0.5;
		border-color: rgba(255, 255, 255, 0.1);
	}

	.fab.help-fab:hover {
		opacity: 1;
	}

	.help-popover {
		position: absolute;
		top: 0;
		right: calc(100% + 10px);
		width: 270px;
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
