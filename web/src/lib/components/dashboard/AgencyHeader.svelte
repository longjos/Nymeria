<script lang="ts">
	import type { Net, NetCheckIn } from '$lib/types';
	import { humanName } from '$lib/agencyTranslations';

	let {
		net,
		ncsCheckIn,
		lastUpdated,
		connected,
		presentationMode,
		onTogglePresentation,
	}: {
		net: Net;
		ncsCheckIn?: NetCheckIn;
		lastUpdated: Date;
		connected: boolean;
		presentationMode: boolean;
		onTogglePresentation: () => void;
	} = $props();

	let now = $state(Date.now());

	$effect(() => {
		const interval = setInterval(() => { now = Date.now(); }, 10_000);
		return () => clearInterval(interval);
	});

	let duration = $derived.by(() => {
		if (!net.openedAt) return '';
		const ms = now - new Date(net.openedAt).getTime();
		const h = Math.floor(ms / 3_600_000);
		const m = Math.floor((ms % 3_600_000) / 60_000);
		if (h > 0) return `Running for ${h}h ${m}m`;
		return `Running for ${m}m`;
	});

	let updatedAgo = $derived.by(() => {
		const ms = now - lastUpdated.getTime();
		const m = Math.floor(ms / 60_000);
		if (m < 1) return 'Just now';
		return `${m}m ago`;
	});

	let ncsName = $derived(ncsCheckIn ? humanName(ncsCheckIn) : net.ncsCallsign || 'Unknown');
</script>

<header class="agency-header" class:presentation={presentationMode}>
	<div class="header-left">
		<h1 class="net-name">{net.name}</h1>
		{#if duration}
			<span class="net-duration">{duration}</span>
		{/if}
	</div>

	<div class="header-center">
		<div class="header-info">
			<span class="info-label">Operations Lead:</span>
			<span class="info-value">{ncsName}</span>
		</div>
		{#if net.frequency}
			<div class="header-info">
				<span class="info-label">Channel:</span>
				<span class="info-value">{net.frequency}</span>
			</div>
		{/if}
	</div>

	<div class="header-right">
		<div class="connection-indicator" class:connected class:disconnected={!connected}>
			<span class="conn-dot"></span>
			<span class="conn-text">Updated {updatedAgo}</span>
		</div>
		<button
			class="presentation-btn"
			class:active={presentationMode}
			onclick={onTogglePresentation}
			title="Presentation mode"
		>
			<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
				<line x1="8" y1="21" x2="16" y2="21"/>
				<line x1="12" y1="17" x2="12" y2="21"/>
			</svg>
		</button>
	</div>
</header>

<style>
	.agency-header {
		display: flex;
		align-items: center;
		gap: var(--space-lg);
		padding: var(--space-md) var(--space-lg);
		background: var(--color-surface);
		border-bottom: 1px solid var(--color-primary);
		flex-wrap: wrap;
	}

	.agency-header.presentation {
		padding: var(--space-lg) var(--space-xl);
	}

	.header-left {
		display: flex;
		align-items: baseline;
		gap: var(--space-sm);
		min-width: 0;
	}

	.net-name {
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--color-text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.presentation .net-name {
		font-size: 1.6rem;
	}

	.net-duration {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.header-center {
		display: flex;
		gap: var(--space-lg);
		flex: 1;
		justify-content: center;
	}

	.header-info {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.85rem;
	}

	.presentation .header-info {
		font-size: 1rem;
	}

	.info-label {
		color: var(--color-text-muted);
	}

	.info-value {
		color: var(--color-text);
		font-weight: 600;
	}

	.header-right {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		margin-left: auto;
	}

	.connection-indicator {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.conn-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--color-warning);
		transition: background var(--duration-fast);
	}

	.connected .conn-dot {
		background: var(--color-success);
	}

	.disconnected .conn-dot {
		background: var(--color-error);
	}

	.presentation-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		border-radius: var(--radius-md);
		background: transparent;
		border: 1px solid var(--color-primary);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.presentation-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.presentation-btn.active {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: white;
	}

	@media (max-width: 768px) {
		.agency-header {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-sm);
			padding: var(--space-sm) var(--space-md);
		}

		.header-center {
			flex-direction: column;
			gap: var(--space-xs);
		}

		.header-right {
			width: 100%;
			justify-content: space-between;
		}
	}
</style>
