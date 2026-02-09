<script lang="ts">
	import { activeNet, activeCheckIns } from '$lib/stores/netcontrol';

	let {
		unreadCount = 0,
		onSearchOpen,
		onMessagesOpen,
		onBulletinsOpen,
		onTransportsOpen,
		onAnnotationsOpen,
		onNetControlOpen
	}: {
		unreadCount?: number;
		onSearchOpen?: () => void;
		onMessagesOpen?: () => void;
		onBulletinsOpen?: () => void;
		onTransportsOpen?: () => void;
		onAnnotationsOpen?: () => void;
		onNetControlOpen?: () => void;
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

	.fab.net-active {
		border-color: #22c55e;
		color: #22c55e;
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
