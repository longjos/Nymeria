<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { transports } from '$lib/stores/transports';
	import { wsClient } from '$lib/stores/stations';
	import { timeAgo } from '$lib/utils';
	import { api } from '$lib/api';
	import type { TileCacheStatus } from '$lib/types';
	import { kissLinkState, kissLinkLabel, kissLinkHint } from '$lib/serialPorts';

	let list = $derived($transports);
	let connectedCount = $derived(list.filter((t) => t.connected).length);

	// Tile cache state
	let tileStatus = $state<TileCacheStatus | null>(null);
	let tileExpanded = $state(false);
	let preloadZoomMin = $state(8);
	let preloadZoomMax = $state(14);
	let preloadEstimate = $state<number | null>(null);
	let preloading = $state(false);
	let preloadProgress = $state<{ done: number; total: number } | null>(null);

	onMount(() => {
		loadTileStatus();

		wsClient.on('tile_preload_progress', (msg) => {
			const data = msg.data as { done: number; total: number; skipped: number };
			if (data) preloadProgress = { done: data.done, total: data.total };
		});
		wsClient.on('tile_preload_complete', () => {
			preloading = false;
			loadTileStatus();
		});
	});

	async function loadTileStatus() {
		try {
			tileStatus = await api.tileCacheStatus();
		} catch {
			// tile cache may not be available
		}
	}

	async function handleEstimate() {
		try {
			// Use approximate current map viewport (whole earth as fallback)
			const bounds = getStoredBounds();
			const result = await api.estimateTiles(
				bounds.south, bounds.west, bounds.north, bounds.east,
				preloadZoomMin, preloadZoomMax
			);
			preloadEstimate = result.tileCount;
		} catch (e) {
			console.error('Estimate failed:', e);
		}
	}

	async function handlePreload() {
		const bounds = getStoredBounds();
		preloading = true;
		preloadProgress = null;
		try {
			const result = await api.preloadTiles(
				bounds.south, bounds.west, bounds.north, bounds.east,
				preloadZoomMin, preloadZoomMax
			);
			preloadProgress = { done: 0, total: result.tileCount };
		} catch (e) {
			console.error('Preload failed:', e);
			preloading = false;
		}
	}

	function getStoredBounds() {
		// Try to read viewport from a stored value, fallback to CONUS
		return { south: 24, west: -125, north: 50, east: -66 };
	}

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(1024));
		return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
	}

	function transportLabel(type: string): string {
		switch (type) {
			case 'aprsis': return 'APRS-IS';
			case 'kisstcp': return 'KISS TCP';
			case 'serial': return 'Serial';
			default: return type.toUpperCase();
		}
	}

	function isKissPipe(type: string): boolean {
		return type === 'serial' || type === 'kisstcp';
	}

	function formatPackets(n: number): string {
		if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
		if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
		return n.toString();
	}
</script>

<div class="transport-panel">
	<div class="panel-header">
		<h2>Transports</h2>
		<span class="summary">
			{connectedCount}/{list.length} connected
		</span>
	</div>

	{#if list.length === 0}
		<div class="empty-state">
			<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
				<path d="M12 2L2 7l10 5 10-5-10-5z"/>
				<path d="M2 17l10 5 10-5"/>
				<path d="M2 12l10 5 10-5"/>
			</svg>
			<p>No transports configured</p>
			<p class="hint">Add transports in your config file</p>
		</div>
	{:else}
		<div class="transport-list">
			{#each list as t (t.id)}
				<div class="transport-card" class:error={!!t.error}>
					<div class="card-header">
						<span
							class="status-dot"
							class:connected={t.connected}
							class:disconnected={!t.connected && !t.error}
							class:errored={!!t.error}
						></span>
						<div class="card-title">
							<span class="transport-name">{t.name || t.id}</span>
							<span class="transport-type">{transportLabel(t.type)}</span>
						</div>
						{#if isKissPipe(t.type)}
							{@const link = kissLinkState(t)}
							<span class="state-label {link}" title={kissLinkHint(link)}>{kissLinkLabel(link)}</span>
						{:else}
							<span class="state-label" class:connected={t.connected}>
								{t.connected ? 'Connected' : t.error ? 'Error' : 'Disconnected'}
							</span>
						{/if}
					</div>

					<div class="card-stats">
						<div class="stat">
							<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
								<path d="M8 12V4M8 4L4 8M8 4l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
							</svg>
							<span class="stat-value">{formatPackets(t.packetsRx)}</span>
							<span class="stat-label">RX</span>
						</div>
						<div class="stat">
							<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
								<path d="M8 4v8M8 12l4-4M8 12L4 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
							</svg>
							<span class="stat-value">{formatPackets(t.packetsTx)}</span>
							<span class="stat-label">TX</span>
						</div>
						{#if t.lastActivity}
							<div class="stat last-activity">
								<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
									<circle cx="8" cy="8" r="6.5" stroke="currentColor" stroke-width="1.5"/>
									<path d="M8 4.5V8l2.5 2.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
								</svg>
								<span class="stat-value">{timeAgo(t.lastActivity)}</span>
							</div>
						{/if}
					</div>

					{#if t.error}
						<div class="error-msg">{t.error}</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	<!-- Offline Tiles -->
	{#if tileStatus?.enabled}
		<div class="tile-section">
			<button class="tile-toggle" onclick={() => (tileExpanded = !tileExpanded)}>
				<svg width="12" height="12" viewBox="0 0 16 16" fill="none" class="chevron" class:expanded={tileExpanded}>
					<path d="M6 4l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
				</svg>
				<span>Offline Tiles</span>
				<span class="tile-count">{tileStatus.tileCount.toLocaleString()} cached</span>
			</button>

			{#if tileExpanded}
				<div class="tile-content">
					<div class="tile-stats">
						<div class="tile-stat">
							<span class="tile-stat-label">Tiles</span>
							<span class="tile-stat-value">{tileStatus.tileCount.toLocaleString()}</span>
						</div>
						<div class="tile-stat">
							<span class="tile-stat-label">Disk</span>
							<span class="tile-stat-value">{formatBytes(tileStatus.diskUsage)}</span>
						</div>
						{#if tileStatus.maxZoom}
							<div class="tile-stat">
								<span class="tile-stat-label">Max Zoom</span>
								<span class="tile-stat-value">{tileStatus.maxZoom}</span>
							</div>
						{/if}
					</div>

					<div class="preload-controls">
						<div class="zoom-range">
							<label>
								<span class="zoom-label">Zoom</span>
								<input type="number" bind:value={preloadZoomMin} min="1" max="18" class="zoom-input" />
								<span class="zoom-sep">–</span>
								<input type="number" bind:value={preloadZoomMax} min="1" max="18" class="zoom-input" />
							</label>
						</div>
						<div class="preload-actions">
							<button class="tile-btn" onclick={handleEstimate}>Estimate</button>
							<button class="tile-btn primary" onclick={handlePreload} disabled={preloading}>
								{preloading ? 'Caching...' : 'Cache View'}
							</button>
						</div>
					</div>

					{#if preloadEstimate !== null}
						<div class="estimate-result">
							~{preloadEstimate.toLocaleString()} tiles
						</div>
					{/if}

					{#if preloadProgress}
						<div class="preload-progress">
							<div class="progress-bar">
								<div class="progress-fill" style="width: {preloadProgress.total > 0 ? (preloadProgress.done / preloadProgress.total * 100) : 0}%"></div>
							</div>
							<span class="progress-text">{preloadProgress.done} / {preloadProgress.total}</span>
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.transport-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow-y: auto;
	}

	.panel-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		padding: var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.panel-header h2 {
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.summary {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-sm);
		padding: var(--space-2xl) var(--space-md);
		color: var(--color-text-muted);
		text-align: center;
	}

	.empty-state p {
		font-size: 0.9rem;
	}

	.empty-state .hint {
		font-size: 0.75rem;
		opacity: 0.7;
	}

	.transport-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		padding: var(--space-sm);
	}

	.transport-card {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		padding: var(--space-md);
		transition: border-color var(--duration-fast);
	}

	.transport-card:hover {
		border-color: color-mix(in srgb, var(--color-primary) 60%, var(--color-text) 40%);
	}

	.transport-card.error {
		border-color: color-mix(in srgb, var(--color-error) 40%, transparent);
	}

	.card-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.status-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.status-dot.connected {
		background: var(--color-connected);
		box-shadow: 0 0 6px var(--color-connected);
	}

	.status-dot.disconnected {
		background: var(--color-disconnected);
	}

	.status-dot.errored {
		background: var(--color-error);
		animation: pulse-dot 1.5s ease-in-out infinite;
	}

	@keyframes pulse-dot {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.4; }
	}

	.card-title {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
	}

	.transport-name {
		font-family: monospace;
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--color-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.transport-type {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.state-label {
		font-size: 0.7rem;
		padding: 2px 8px;
		border-radius: var(--radius-full);
		background: color-mix(in srgb, var(--color-disconnected) 15%, transparent);
		color: var(--color-disconnected);
		flex-shrink: 0;
	}

	.state-label.connected,
	.state-label.kiss {
		background: color-mix(in srgb, var(--color-connected) 15%, transparent);
		color: var(--color-connected);
	}

	.state-label.quiet {
		background: color-mix(in srgb, var(--color-warning) 15%, transparent);
		color: var(--color-warning);
	}

	.state-label.error {
		background: color-mix(in srgb, var(--color-error) 15%, transparent);
		color: var(--color-error);
	}

	.card-stats {
		display: flex;
		gap: var(--space-md);
		margin-top: var(--space-sm);
		padding-top: var(--space-sm);
		border-top: 1px solid color-mix(in srgb, var(--color-primary) 60%, transparent);
	}

	.stat {
		display: flex;
		align-items: center;
		gap: 4px;
		color: var(--color-text-muted);
		font-size: 0.75rem;
	}

	.stat-value {
		font-family: monospace;
		font-weight: 600;
		color: var(--color-text);
		font-size: 0.8rem;
	}

	.stat-label {
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		opacity: 0.7;
	}

	.last-activity {
		margin-left: auto;
	}

	.error-msg {
		margin-top: var(--space-sm);
		padding: var(--space-xs) var(--space-sm);
		background: color-mix(in srgb, var(--color-error) 10%, transparent);
		border-radius: var(--radius-sm);
		font-size: 0.75rem;
		color: var(--color-error);
		word-break: break-word;
	}

	/* Tile cache section */
	.tile-section {
		border-top: 1px solid var(--color-primary);
		margin-top: var(--space-sm);
	}

	.tile-toggle {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		padding: var(--space-sm) var(--space-md);
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
		text-align: left;
	}
	.tile-toggle:hover {
		background: color-mix(in srgb, var(--color-primary) 40%, transparent);
	}

	.chevron {
		transition: transform var(--duration-fast);
	}
	.chevron.expanded {
		transform: rotate(90deg);
	}

	.tile-count {
		margin-left: auto;
		font-size: 0.7rem;
		color: var(--color-text-muted);
		font-weight: 400;
	}

	.tile-content {
		padding: 0 var(--space-md) var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.tile-stats {
		display: flex;
		gap: var(--space-md);
	}

	.tile-stat {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}

	.tile-stat-label {
		font-size: 0.6rem;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		color: var(--color-text-muted);
	}

	.tile-stat-value {
		font-family: monospace;
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.preload-controls {
		display: flex;
		align-items: flex-end;
		gap: var(--space-sm);
	}

	.zoom-range {
		flex: 1;
	}

	.zoom-range label {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.zoom-label {
		margin-right: 4px;
	}

	.zoom-input {
		width: 44px;
		padding: 3px 6px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
		font-family: monospace;
		text-align: center;
	}
	.zoom-input:focus {
		outline: none;
		border-color: var(--color-accent);
	}

	.zoom-sep {
		color: var(--color-text-muted);
	}

	.preload-actions {
		display: flex;
		gap: var(--space-xs);
	}

	.tile-btn {
		padding: 4px 10px;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.7rem;
		font-weight: 600;
		cursor: pointer;
	}
	.tile-btn:hover {
		background: var(--color-primary);
	}
	.tile-btn.primary {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: white;
	}
	.tile-btn.primary:hover {
		background: color-mix(in srgb, var(--color-accent) 80%, white 20%);
	}
	.tile-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.estimate-result {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-family: monospace;
	}

	.preload-progress {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.progress-bar {
		flex: 1;
		height: 6px;
		background: var(--color-primary);
		border-radius: var(--radius-full);
		overflow: hidden;
	}

	.progress-fill {
		height: 100%;
		background: var(--color-accent);
		border-radius: var(--radius-full);
		transition: width var(--duration-fast);
	}

	.progress-text {
		font-size: 0.7rem;
		font-family: monospace;
		color: var(--color-text-muted);
		white-space: nowrap;
	}
</style>
