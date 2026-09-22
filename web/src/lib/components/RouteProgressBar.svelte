<script lang="ts">
	import { orderedCheckpoints, progressElements, hasCheckpoints } from '$lib/stores/netcontrol';
	import { statusColor } from '$lib/annotationMeta';
	import { timeAgo } from '$lib/utils';
	import { wxInAreaAlerts } from '$lib/stores/wxAlerts';
	import CourseRail from './CourseRail.svelte';

	let {
		onCheckpointClick,
	}: {
		onCheckpointClick?: (cpId: string) => void;
	} = $props();

	let expandedCpId = $state<string | null>(null);

	// Element colors by label — used only by the expanded passage detail below;
	// the rail itself (CourseRail) owns the canonical copy of this table.
	const elementColors: Record<string, string> = {
		lead: 'var(--color-ride-lead)',
		sweep: 'var(--color-ride-sweep)',
		tail: '#f59e0b',
		'main pack': '#3b82f6',
	};

	function getElementColor(label: string): string {
		return elementColors[label.toLowerCase()] || '#8b5cf6';
	}

	function toggleDetail(cpId: string) {
		expandedCpId = expandedCpId === cpId ? null : cpId;
	}
</script>

{#if $hasCheckpoints}
	<section class="route-progress">
		<h3 class="rp-heading">
			<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
				<path d="M4 2v12M4 3h7l-2 3 2 3H4"/>
			</svg>
			Route Progress
			<span class="rp-count">{$orderedCheckpoints.length} checkpoints</span>
		</h3>

		<!-- Progress bar visualization -->
		<div class="rp-bar-container">
			<CourseRail
				checkpoints={$orderedCheckpoints}
				elements={$progressElements}
				wxAlerts={$wxInAreaAlerts}
				onStopActivate={toggleDetail}
			/>
		</div>

		<!-- Expanded checkpoint detail -->
		{#if expandedCpId}
			{@const cp = $orderedCheckpoints.find(c => c.meta.annotationId === expandedCpId)}
			{#if cp}
				<div class="rp-detail">
					<div class="rp-detail-header">
						<strong>{cp.annotation.label}</strong>
						<span class="rp-detail-status" style="color: {statusColor('checkpoint', cp.annotation.status)}">
							{cp.annotation.status}
						</span>
						<button class="rp-detail-close" onclick={() => expandedCpId = null}>&times;</button>
					</div>
					{#if (cp.passages ?? []).length > 0}
						<div class="rp-passages">
							{#each (cp.passages ?? []).slice().reverse() as p (p.id)}
								<div class="rp-passage">
									<span class="rp-passage-label" style="color: {getElementColor(p.label)}">{p.label}</span>
									<span class="rp-passage-dir">{p.direction}</span>
									<span class="rp-passage-time">{timeAgo(p.passageTime)}</span>
								</div>
							{/each}
						</div>
					{:else}
						<p class="rp-no-passages">No passages recorded</p>
					{/if}
				</div>
			{/if}
		{/if}
	</section>
{/if}

<style>
	.route-progress {
		border-bottom: 1px solid var(--color-primary);
	}

	.rp-heading {
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

	.rp-count {
		font-size: 0.65rem;
		background: var(--color-primary);
		padding: 1px 6px;
		border-radius: 8px;
		margin-left: 2px;
		font-weight: 600;
	}

	.rp-bar-container {
		padding: 8px var(--space-md) 12px;
		overflow-x: auto;
	}

	/* Detail popover */
	.rp-detail {
		margin: 0 var(--space-md) 8px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 8px 10px;
	}

	.rp-detail-header {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.82rem;
		margin-bottom: 6px;
	}

	.rp-detail-status {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		font-weight: 600;
	}

	.rp-detail-close {
		margin-left: auto;
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.1rem;
		cursor: pointer;
		padding: 0 4px;
		line-height: 1;
	}

	.rp-passages {
		display: flex;
		flex-direction: column;
		gap: 3px;
	}

	.rp-passage {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.75rem;
	}

	.rp-passage-label {
		font-weight: 700;
		min-width: 60px;
	}

	.rp-passage-dir {
		color: var(--color-text-muted);
		font-size: 0.7rem;
	}

	.rp-passage-time {
		margin-left: auto;
		color: var(--color-text-muted);
		font-size: 0.65rem;
		opacity: 0.7;
	}

	.rp-no-passages {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		margin: 0;
	}
</style>
