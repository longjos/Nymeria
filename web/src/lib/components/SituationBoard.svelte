<script lang="ts">
	import { timeAgo } from '$lib/utils';
	import { api } from '$lib/api';
	import type { NetCheckIn, NetMission } from '$lib/types';
	import {
		activeNet, checkIns,
		attentionItems, activeMissions, recentSignificantEvents,
		pinnedNotes,
	} from '$lib/stores/netcontrol';
	import type { AttentionItem } from '$lib/stores/netcontrol';

	let {
		onNavigateTab,
		onFlyTo,
	}: {
		onNavigateTab: (tab: 'roster' | 'missions' | 'timeline', filter?: string) => void;
		onFlyTo?: (lat: number, lon: number) => void;
	} = $props();

	const priorityColors: Record<string, string> = {
		emergency: '#ef4444',
		priority: '#f59e0b',
		welfare: '#3b82f6',
		routine: '#22c55e',
	};

	const reasonIcons: Record<string, string> = {
		missing: '\u{1F534}',   // red circle
		emergency: '\u{1F534}', // red circle
		stale: '\u{26A0}\u{FE0F}',      // warning
		rollcall: '\u{26A0}\u{FE0F}',   // warning
	};

	function missionElapsed(m: NetMission): string {
		const start = new Date(m.createdAt).getTime();
		const end = m.completedAt ? new Date(m.completedAt).getTime() : Date.now();
		const ms = end - start;
		const h = Math.floor(ms / 3600000);
		const min = Math.floor((ms % 3600000) / 60000);
		if (h > 0) return `${h}h ${min}m`;
		return `${min}m`;
	}

	function operatorsForMission(missionId: string): NetCheckIn[] {
		return $checkIns.filter(ci => ci.missionIds?.includes(missionId) && ci.status !== 'released');
	}

	async function handleAttentionAction(item: AttentionItem) {
		if (!$activeNet) return;
		const ci = item.checkIn;
		switch (item.reason) {
			case 'missing':
				if (ci.lat != null && ci.lon != null) {
					onFlyTo?.(ci.lat, ci.lon);
				}
				onNavigateTab('roster', 'missing');
				break;
			case 'emergency':
				onNavigateTab('roster', 'missing');
				break;
			case 'stale':
				onNavigateTab('roster', 'stale');
				break;
			case 'rollcall':
				try {
					await api.recordRollCallResponse($activeNet.id, ci.id);
				} catch (e) {
					console.error('Roll call response failed:', e);
				}
				break;
		}
	}

	function handleMissionClick(m: NetMission) {
		onNavigateTab('missions');
	}

	function handleViewTimeline() {
		onNavigateTab('timeline');
	}

	const eventIcons: Record<string, string> = {
		net_opened: '\u{1F4E1}',
		net_closed: '\u{1F512}',
		checkin: '\u{1F4E5}',
		checkout: '\u{1F4E4}',
		status_change: '\u{1F504}',
		assignment: '\u{1F4CB}',
		mission_created: '\u{1F3AF}',
		mission_updated: '\u{2705}',
		rollcall: '\u{1F4E2}',
		note: '\u{1F4DD}',
		ncs_transfer: '\u{1F500}',
	};
</script>

<div class="sitboard">
	<!-- NEEDS ATTENTION -->
	{#if $attentionItems.length > 0}
		<section class="sb-section sb-attention">
			<h3 class="sb-heading sb-heading-alert">
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
					<path d="M12 2L2 20h20L12 2zM12 10v4M12 17h.01"/>
				</svg>
				Needs Attention
				<span class="sb-count">{$attentionItems.length}</span>
			</h3>
			<div class="sb-items">
				{#each $attentionItems as item (item.checkIn.id + item.reason)}
					<div class="attn-item" class:attn-critical={item.reason === 'missing' || item.reason === 'emergency'}>
						<span class="attn-icon">{reasonIcons[item.reason]}</span>
						<div class="attn-info">
							<span class="attn-callsign">{item.checkIn.callsign}</span>
							{#if item.checkIn.tacticalCall}
								<span class="attn-tactical">{item.checkIn.tacticalCall}</span>
							{/if}
							<span class="attn-detail">{item.detail}</span>
						</div>
						<button class="attn-action" onclick={() => handleAttentionAction(item)}>
							{item.action}
						</button>
					</div>
				{/each}
			</div>
		</section>
	{:else if $checkIns.filter(ci => ci.status !== 'released').length > 0}
		<section class="sb-section sb-all-clear">
			<div class="all-clear-indicator">
				<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#22c55e" stroke-width="2.5">
					<path d="M22 11.08V12a10 10 0 11-5.93-9.14"/>
					<polyline points="22 4 12 14.01 9 11.01"/>
				</svg>
				<span>All clear</span>
			</div>
		</section>
	{/if}

	<!-- ACTIVE MISSIONS -->
	{#if $activeMissions.length > 0}
		<section class="sb-section">
			<h3 class="sb-heading">
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="10"/>
					<path d="M12 2l3 7h7l-5.5 4 2 7L12 16l-6.5 4 2-7L2 9h7z"/>
				</svg>
				Active Missions
				<span class="sb-count">{$activeMissions.length}</span>
			</h3>
			<div class="sb-items">
				{#each $activeMissions as mission (mission.id)}
					{@const ops = operatorsForMission(mission.id)}
					<button class="mission-item" onclick={() => handleMissionClick(mission)}>
						<span class="mission-prio-dot" style="background: {priorityColors[mission.priority] ?? '#6b7280'}"></span>
						<div class="mission-info">
							<div class="mission-title-row">
								<span class="mission-title">{mission.title}</span>
								<span class="mission-meta">{ops.length} assigned &middot; {missionElapsed(mission)}</span>
							</div>
							{#if ops.length > 0}
								<span class="mission-ops">
									{ops.map(o => o.callsign).join(' \u00B7 ')}
								</span>
							{:else}
								<span class="mission-warning">No operators assigned</span>
							{/if}
						</div>
					</button>
				{/each}
			</div>
		</section>
	{/if}

	<!-- PINNED NOTES -->
	{#if $pinnedNotes.length > 0}
		<section class="sb-section">
			<h3 class="sb-heading">
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M12 2v14M5 10l7 7 7-7"/>
				</svg>
				Pinned Notes
				<span class="sb-count">{$pinnedNotes.length}</span>
			</h3>
			<div class="sb-items">
				{#each $pinnedNotes as note (note.id)}
					<div class="note-item">
						<span class="note-pin-icon">{'\u{1F4CC}'}</span>
						<div class="note-info">
							<span class="note-content">{note.content}</span>
							<span class="note-author">{note.authorName} &middot; {timeAgo(note.createdAt)}</span>
						</div>
					</div>
				{/each}
			</div>
		</section>
	{/if}

	<!-- RECENT ACTIVITY -->
	{#if $recentSignificantEvents.length > 0}
		<section class="sb-section">
			<h3 class="sb-heading">
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="10"/>
					<polyline points="12 6 12 12 16 14"/>
				</svg>
				Recent Activity
			</h3>
			<div class="sb-items">
				{#each $recentSignificantEvents as evt (evt.id)}
					<div class="activity-item">
						<span class="activity-icon">{eventIcons[evt.type] ?? '\u2022'}</span>
						<span class="activity-summary">{evt.summary}</span>
						<span class="activity-time">{timeAgo(evt.createdAt)}</span>
					</div>
				{/each}
			</div>
			<button class="view-full-link" onclick={handleViewTimeline}>
				View full log &rarr;
			</button>
		</section>
	{/if}

	<!-- Empty state when net is open but nothing is happening yet -->
	{#if $attentionItems.length === 0 && $activeMissions.length === 0 && $pinnedNotes.length === 0 && $recentSignificantEvents.length === 0 && $checkIns.filter(ci => ci.status !== 'released').length === 0}
		<div class="sb-empty">
			<p>No activity yet. Check in operators to get started.</p>
		</div>
	{/if}
</div>

<style>
	.sitboard {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.sb-section {
		border-bottom: 1px solid var(--color-primary);
	}

	.sb-heading {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
		padding: 10px var(--space-md) 6px;
		margin: 0;
	}

	.sb-heading-alert {
		color: #ef4444;
	}

	.sb-count {
		font-size: 0.65rem;
		background: var(--color-primary);
		padding: 1px 6px;
		border-radius: 8px;
		margin-left: 2px;
	}

	.sb-items {
		display: flex;
		flex-direction: column;
	}

	/* Needs Attention */
	.sb-attention {
		animation: pulse-attention 3s ease-in-out infinite;
	}

	@keyframes pulse-attention {
		0%, 100% { background: transparent; }
		50% { background: rgba(239, 68, 68, 0.04); }
	}

	.attn-item {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px var(--space-md);
		border-top: 1px solid rgba(255, 255, 255, 0.04);
		transition: background var(--duration-fast);
	}

	.attn-item:first-child {
		border-top: none;
	}

	.attn-item:hover {
		background: rgba(255, 255, 255, 0.03);
	}

	.attn-item.attn-critical {
		border-left: 3px solid #ef4444;
	}

	.attn-icon {
		font-size: 0.9rem;
		flex-shrink: 0;
		line-height: 1;
	}

	.attn-info {
		flex: 1;
		display: flex;
		align-items: baseline;
		gap: 6px;
		min-width: 0;
		flex-wrap: wrap;
	}

	.attn-callsign {
		font-family: monospace;
		font-weight: 700;
		font-size: 0.9rem;
	}

	.attn-tactical {
		font-size: 0.8rem;
		color: var(--color-accent);
	}

	.attn-detail {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.attn-action {
		flex-shrink: 0;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		padding: 5px 12px;
		background: rgba(239, 68, 68, 0.12);
		border: 1px solid rgba(239, 68, 68, 0.35);
		border-radius: var(--radius-sm);
		color: #ef4444;
		cursor: pointer;
		transition: all var(--duration-fast);
		min-height: 32px;
	}

	.attn-action:hover {
		background: rgba(239, 68, 68, 0.22);
		border-color: #ef4444;
	}

	/* All Clear */
	.sb-all-clear {
		border-bottom: 1px solid var(--color-primary);
	}

	.all-clear-indicator {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 12px var(--space-md);
		font-size: 0.85rem;
		color: #22c55e;
		font-weight: 600;
	}

	/* Active Missions */
	.mission-item {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		padding: 10px var(--space-md);
		background: none;
		border: none;
		border-top: 1px solid rgba(255, 255, 255, 0.04);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
		transition: background var(--duration-fast);
		width: 100%;
		min-height: 44px;
	}

	.mission-item:first-child {
		border-top: none;
	}

	.mission-item:hover {
		background: rgba(255, 255, 255, 0.03);
	}

	.mission-prio-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		flex-shrink: 0;
		margin-top: 3px;
	}

	.mission-info {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.mission-title-row {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 8px;
	}

	.mission-title {
		font-size: 0.85rem;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.mission-meta {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.mission-ops {
		font-size: 0.75rem;
		font-family: monospace;
		color: var(--color-text-muted);
	}

	.mission-warning {
		font-size: 0.75rem;
		color: #f59e0b;
		font-weight: 600;
	}

	/* Pinned Notes */
	.note-item {
		display: flex;
		align-items: flex-start;
		gap: 8px;
		padding: 8px var(--space-md);
		border-top: 1px solid rgba(255, 255, 255, 0.04);
	}

	.note-item:first-child {
		border-top: none;
	}

	.note-pin-icon {
		font-size: 0.85rem;
		flex-shrink: 0;
		line-height: 1.3;
	}

	.note-info {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.note-content {
		font-size: 0.82rem;
		line-height: 1.4;
	}

	.note-author {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	/* Recent Activity */
	.activity-item {
		display: flex;
		align-items: baseline;
		gap: 8px;
		padding: 6px var(--space-md);
		border-top: 1px solid rgba(255, 255, 255, 0.04);
		font-size: 0.8rem;
	}

	.activity-item:first-child {
		border-top: none;
	}

	.activity-icon {
		font-size: 0.75rem;
		flex-shrink: 0;
	}

	.activity-summary {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text-muted);
	}

	.activity-time {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		opacity: 0.7;
		white-space: nowrap;
		flex-shrink: 0;
	}

	.view-full-link {
		display: block;
		width: 100%;
		padding: 8px var(--space-md);
		background: none;
		border: none;
		border-top: 1px solid rgba(255, 255, 255, 0.04);
		color: var(--color-accent);
		font-size: 0.75rem;
		text-align: right;
		cursor: pointer;
		transition: color var(--duration-fast);
	}

	.view-full-link:hover {
		color: var(--color-text);
	}

	/* Empty state */
	.sb-empty {
		padding: var(--space-lg) var(--space-md);
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
</style>
