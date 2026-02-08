<script lang="ts">
	import { transports } from '$lib/stores/transports';
	import { timeAgo } from '$lib/utils';

	let list = $derived($transports);
	let connectedCount = $derived(list.filter((t) => t.connected).length);

	function transportLabel(type: string): string {
		switch (type) {
			case 'aprsis': return 'APRS-IS';
			case 'kisstcp': return 'KISS TCP';
			case 'serial': return 'Serial';
			default: return type.toUpperCase();
		}
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
							<span class="transport-name">{t.id}</span>
							<span class="transport-type">{transportLabel(t.type)}</span>
						</div>
						<span class="state-label" class:connected={t.connected}>
							{t.connected ? 'Connected' : t.error ? 'Error' : 'Disconnected'}
						</span>
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

	.state-label.connected {
		background: color-mix(in srgb, var(--color-connected) 15%, transparent);
		color: var(--color-connected);
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
</style>
