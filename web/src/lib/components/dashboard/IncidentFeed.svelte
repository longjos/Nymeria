<script lang="ts">
	import type { NetMission, NetCheckIn } from '$lib/types';
	import { priorityLabels, missionStatusLabels, humanName } from '$lib/agencyTranslations';

	let {
		missions,
		checkIns,
		presentationMode = false,
	}: {
		missions: NetMission[];
		checkIns: NetCheckIn[];
		presentationMode?: boolean;
	} = $props();

	let now = $state(Date.now());

	$effect(() => {
		const interval = setInterval(() => { now = Date.now(); }, 30_000);
		return () => clearInterval(interval);
	});

	let activeMissions = $derived.by(() => {
		return missions
			.filter(m => m.status !== 'complete')
			.sort((a, b) => {
				const prio: Record<string, number> = { emergency: 0, priority: 1, welfare: 2, routine: 3 };
				const pa = prio[a.priority] ?? 3;
				const pb = prio[b.priority] ?? 3;
				if (pa !== pb) return pa - pb;
				return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
			});
	});

	function elapsedStr(createdAt: string): string {
		const ms = now - new Date(createdAt).getTime();
		const m = Math.floor(ms / 60_000);
		if (m < 60) return `Open for ${m}m`;
		const h = Math.floor(m / 60);
		return `Open for ${h}h ${m % 60}m`;
	}

	function priorityColor(priority: string): string {
		switch (priority) {
			case 'emergency': return '#ef4444';
			case 'priority': return '#f59e0b';
			case 'welfare': return '#3b82f6';
			default: return '#6b7280';
		}
	}

	function responders(mission: NetMission): string[] {
		return checkIns
			.filter(ci => ci.missionIds?.includes(mission.id) && ci.status !== 'released')
			.map(ci => humanName(ci));
	}
</script>

<section class="incident-feed" class:presentation={presentationMode}>
	<h2 class="section-title">Active Incidents</h2>

	{#if activeMissions.length === 0}
		<p class="empty">No active incidents</p>
	{:else}
		<div class="mission-list">
			{#each activeMissions as mission (mission.id)}
				{@const names = responders(mission)}
				<div class="mission-card" class:critical={mission.priority === 'emergency'}>
					<div class="mission-header">
						<span class="priority-dot" style="background: {priorityColor(mission.priority)}"></span>
						<span class="priority-label" style="color: {priorityColor(mission.priority)}">
							{priorityLabels[mission.priority] || mission.priority}
						</span>
						<span class="mission-elapsed">{elapsedStr(mission.createdAt)}</span>
					</div>

					<h3 class="mission-title">{mission.title}</h3>

					{#if mission.location}
						<p class="mission-location">{mission.location}</p>
					{/if}

					<div class="mission-footer">
						<span class="mission-status">
							{missionStatusLabels[mission.status] || mission.status}
						</span>
						{#if names.length > 0}
							<span class="mission-responders">
								{names.length} responder{names.length !== 1 ? 's' : ''}: {names.join(', ')}
							</span>
						{:else}
							<span class="mission-responders no-responders">No responders assigned</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</section>

<style>
	.incident-feed {
		padding: var(--space-md);
	}

	.section-title {
		font-size: 0.75rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
		margin-bottom: var(--space-md);
	}

	.presentation .section-title {
		font-size: 0.9rem;
	}

	.empty {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}

	.mission-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.mission-card {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		padding: var(--space-sm) var(--space-md);
	}

	.mission-card.critical {
		border-left: 3px solid #ef4444;
	}

	.mission-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-bottom: 4px;
	}

	.priority-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.priority-label {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.presentation .priority-label {
		font-size: 0.8rem;
	}

	.mission-elapsed {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		margin-left: auto;
	}

	.mission-title {
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--color-text);
		margin-bottom: 2px;
	}

	.presentation .mission-title {
		font-size: 1.05rem;
	}

	.mission-location {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		margin-bottom: 6px;
	}

	.mission-footer {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		flex-wrap: wrap;
	}

	.mission-status {
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.mission-responders {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.presentation .mission-responders {
		font-size: 0.85rem;
	}

	.no-responders {
		font-style: italic;
		opacity: 0.6;
	}
</style>
