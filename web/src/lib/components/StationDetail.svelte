<script lang="ts">
	import { onMount, tick } from 'svelte';
	import APRSIcon from './APRSIcon.svelte';
	import MessageBubble from './MessageBubble.svelte';
	import MessageCompose from './MessageCompose.svelte';
	import { api } from '$lib/api';
	import { stations } from '$lib/stores/stations';
	import { conversations, loadMessages } from '$lib/stores/messages';
	import { canOperate } from '$lib/stores/session';
	import { symbolInfo } from '$lib/symbols';
	import { timeAgo, formatCoord, formatSpeed, formatAltitude, formatCourse, stationDisplayName } from '$lib/utils';
	import type { Station } from '$lib/types';
	import type { DetailTab } from '$lib/stores/ui';

	let {
		stationKey,
		activeTab = 'info',
		onTabChange,
		onClose,
		onFlyTo
	}: {
		stationKey: string;
		activeTab?: DetailTab;
		onTabChange?: (tab: DetailTab) => void;
		onClose?: () => void;
		onFlyTo?: (lat: number, lon: number) => void;
	} = $props();

	let station = $state<Station | null>(null);
	let loaded = $state(false);
	let scrollEl: HTMLDivElement;

	let convo = $derived($conversations.get(stationKey));
	let messages = $derived(convo?.messages ?? []);

	// Whether this is an unknown station (not heard yet)
	let unknownStation = $derived(loaded && !station);

	$effect(() => {
		// Try store first, fall back to API
		const fromStore = $stations.get(stationKey);
		if (fromStore) {
			station = fromStore;
		}
	});

	onMount(async () => {
		const fromStore = $stations.get(stationKey) ?? null;
		if (fromStore) {
			station = fromStore;
			loaded = true;
		} else {
			try {
				station = await api.station(stationKey);
			} catch {
				// Station not heard yet — that's fine, we can still message them
			}
			loaded = true;
		}
	});

	$effect(() => {
		if (activeTab === 'messages' && stationKey) {
			loadMessages(stationKey);
		}
	});

	$effect(() => {
		if (messages.length && activeTab === 'messages') {
			tick().then(() => {
				scrollEl?.scrollTo({ top: scrollEl.scrollHeight });
			});
		}
	});

	let displayName = $derived(station ? stationDisplayName(station.callsign, station.ssid) : stationKey);
	let info = $derived(station ? symbolInfo(station.symbol) : null);

	function handleFlyTo() {
		if (station?.position) {
			onFlyTo?.(station.position.lat, station.position.lon);
		}
	}

	function handleMessage() {
		onTabChange?.('messages');
	}

	const tabs: { key: DetailTab; label: string }[] = [
		{ key: 'info', label: 'Info' },
		{ key: 'messages', label: 'Messages' },
		{ key: 'track', label: 'Track' }
	];
</script>

<div class="station-detail">
	{#if !loaded}
		<div class="loading">Loading...</div>
	{:else if unknownStation}
		<!-- Unknown station: show header + messages only -->
		<div class="detail-header">
			<div class="unknown-icon">?</div>
			<div class="header-text">
				<span class="callsign">{stationKey}</span>
				<span class="symbol-label">Not yet heard</span>
			</div>
		</div>

		<div class="tab-content messages-tab">
			<div class="message-area" bind:this={scrollEl}>
				{#if messages.length === 0}
					<p class="empty">No messages yet. Send the first one below.</p>
				{:else}
					{#each messages as msg (msg.id)}
						<MessageBubble message={msg} />
					{/each}
				{/if}
			</div>
			{#if $canOperate}
				<MessageCompose to={stationKey} onSent={() => {}} />
			{:else}
				<p class="role-hint">Operator role required to send messages</p>
			{/if}
		</div>
	{:else if station}
		<div class="detail-header">
			<APRSIcon symbol={station.symbol} size={36} />
			<div class="header-text">
				<span class="callsign">{displayName}</span>
				{#if info}
					<span class="symbol-label">{info.label}</span>
				{/if}
			</div>
			<span class="last-heard">{timeAgo(station.lastHeard)}</span>
		</div>

		<div class="tab-bar" role="tablist">
			{#each tabs as tab}
				<button
					role="tab"
					class="tab"
					class:active={activeTab === tab.key}
					aria-selected={activeTab === tab.key}
					onclick={() => onTabChange?.(tab.key)}
				>
					{tab.label}
				</button>
			{/each}
		</div>

		{#if activeTab === 'info'}
			<div class="tab-content">
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

				<div class="actions">
					{#if station.position}
						<button class="btn" onclick={handleFlyTo}>Fly To</button>
					{/if}
					<button class="btn btn-secondary" onclick={handleMessage}>Message</button>
				</div>
			</div>
		{:else if activeTab === 'messages'}
			<div class="tab-content messages-tab">
				<div class="message-area" bind:this={scrollEl}>
					{#if messages.length === 0}
						<p class="empty">No messages yet. Send the first one below.</p>
					{:else}
						{#each messages as msg (msg.id)}
							<MessageBubble message={msg} />
						{/each}
					{/if}
				</div>
				{#if $canOperate}
					<MessageCompose to={stationKey} onSent={() => {}} />
				{:else}
					<p class="role-hint">Operator role required to send messages</p>
				{/if}
			</div>
		{:else if activeTab === 'track'}
			<div class="tab-content">
				{#if station.track && station.track.length > 0}
					<div class="track-list">
						{#each station.track.slice().reverse().slice(0, 50) as tp}
							<div class="track-row">
								<span class="coords">{formatCoord(tp.lat, tp.lon)}</span>
								<span class="time">{timeAgo(tp.time)}</span>
							</div>
						{/each}
					</div>
				{:else}
					<p class="empty">No track history available.</p>
				{/if}
			</div>
		{/if}
	{/if}
</div>

<style>
	.station-detail {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.detail-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.header-text {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.callsign {
		font-weight: 600;
		font-family: monospace;
		font-size: 1.1rem;
	}

	.symbol-label {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.last-heard {
		margin-left: auto;
		font-size: 0.8rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.tab-bar {
		display: flex;
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.tab {
		flex: 1;
		padding: 0.6rem 0;
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--color-text-muted);
		font-size: 0.85rem;
		font-weight: 500;
		cursor: pointer;
		transition: color var(--duration-fast), border-color var(--duration-fast);
	}

	.tab:hover {
		color: var(--color-text);
	}

	.tab.active {
		color: var(--color-accent);
		border-bottom-color: var(--color-accent);
	}

	.tab-content {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-md);
	}

	.messages-tab {
		display: flex;
		flex-direction: column;
		padding: 0;
	}

	.message-area {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
	}

	.info-card {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		padding: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.info-card h3 {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-bottom: 0.5rem;
	}

	.info-row {
		display: flex;
		justify-content: space-between;
		padding: 0.3rem 0;
		font-size: 0.85rem;
	}

	.info-row + .info-row {
		border-top: 1px solid var(--color-primary);
	}

	.info-row .label {
		color: var(--color-text-muted);
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.btn {
		padding: 0.45rem 1rem;
		border-radius: var(--radius-sm);
		font-size: 0.85rem;
		font-weight: 600;
		background: var(--color-accent);
		color: white;
		border: none;
		cursor: pointer;
	}

	.btn-secondary {
		background: var(--color-primary);
		color: var(--color-text);
	}

	.track-list {
		max-height: 100%;
	}

	.track-row {
		display: flex;
		justify-content: space-between;
		padding: 0.25rem 0;
		font-size: 0.8rem;
		font-family: monospace;
	}

	.track-row + .track-row {
		border-top: 1px solid var(--color-primary);
	}

	.track-row .time {
		color: var(--color-text-muted);
	}

	.unknown-icon {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		border-radius: 50%;
		background: var(--color-primary);
		color: var(--color-text-muted);
		font-weight: 700;
		font-family: monospace;
		font-size: 1rem;
		flex-shrink: 0;
	}

	.loading, .empty {
		padding: 2rem;
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}

	.role-hint {
		padding: 0.75rem var(--space-md);
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		font-style: italic;
		border-top: 1px solid var(--color-primary);
	}
</style>
