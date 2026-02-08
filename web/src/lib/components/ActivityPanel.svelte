<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { timeAgo } from '$lib/utils';
	import type { ActivityEntry } from '$lib/types';
	import { wsClient } from '$lib/stores/stations';

	let entries = $state<ActivityEntry[]>([]);
	let loading = $state(true);

	const actionLabels: Record<string, string> = {
		message_sent: 'Sent message',
		message_claimed: 'Claimed conversation',
		object_created: 'Created object',
		object_killed: 'Killed object',
		annotation_created: 'Created annotation',
		annotation_deleted: 'Deleted annotation',
		config_changed: 'Changed config',
		session_started: 'Connected',
		session_ended: 'Disconnected',
		beacon_sent: 'Sent beacon',
		transport_connect: 'Transport connected',
		transport_disconnect: 'Transport disconnected'
	};

	onMount(async () => {
		try {
			const resp = await api.activity({ limit: '50' });
			entries = resp.entries ?? [];
		} catch {
			// silent
		} finally {
			loading = false;
		}

		// Live updates
		wsClient.on('activity_logged', (msg) => {
			const entry = msg.entry as ActivityEntry;
			if (entry) {
				entries = [entry, ...entries].slice(0, 100);
			}
		});
	});

	function handleExport() {
		window.open(api.activityExportUrl(), '_blank');
	}
</script>

<div class="activity-panel">
	<div class="panel-header">
		<span class="title">Activity Log</span>
		<button class="export-btn" onclick={handleExport} title="Export CSV">
			<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
				<path d="M2 10v3h12v-3M8 2v8M5 7l3 3 3-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
			</svg>
			CSV
		</button>
	</div>

	<div class="entries">
		{#if loading}
			<p class="empty">Loading...</p>
		{:else if entries.length === 0}
			<p class="empty">No activity recorded yet.</p>
		{:else}
			{#each entries as entry (entry.id)}
				<div class="entry">
					<div class="entry-main">
						<span class="action">{actionLabels[entry.action] ?? entry.action}</span>
						{#if entry.target}
							<span class="target">{entry.target}</span>
						{/if}
					</div>
					<div class="entry-meta">
						{#if entry.userName}
							<span class="user">{entry.userName}</span>
						{/if}
						<span class="time">{timeAgo(entry.timestamp)}</span>
					</div>
				</div>
			{/each}
		{/if}
	</div>
</div>

<style>
	.activity-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.panel-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.title {
		font-weight: 600;
		font-size: 0.95rem;
	}

	.export-btn {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 0.25rem 0.5rem;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.export-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.entries {
		flex: 1;
		overflow-y: auto;
	}

	.entry {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		padding: 0.5rem var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.entry-main {
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 0;
	}

	.action {
		font-size: 0.8rem;
	}

	.target {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		font-family: monospace;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.entry-meta {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 1px;
		flex-shrink: 0;
		margin-left: 0.5rem;
	}

	.user {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.time {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		opacity: 0.7;
	}

	.empty {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
</style>
