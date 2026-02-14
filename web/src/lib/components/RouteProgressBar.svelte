<script lang="ts">
	import type { CheckpointWithPassages } from '$lib/types';
	import { orderedCheckpoints, progressElements, hasCheckpoints } from '$lib/stores/netcontrol';
	import { statusColor } from '$lib/annotationMeta';
	import { timeAgo } from '$lib/utils';

	let {
		onCheckpointClick,
	}: {
		onCheckpointClick?: (cpId: string) => void;
	} = $props();

	let expandedCpId = $state<string | null>(null);

	// Element colors by label.
	const elementColors: Record<string, string> = {
		lead: '#22c55e',
		sweep: '#ef4444',
		tail: '#f59e0b',
		'main pack': '#3b82f6',
	};

	function getElementColor(label: string): string {
		return elementColors[label.toLowerCase()] || '#8b5cf6';
	}

	function dotSize(passageCount: number): number {
		return Math.min(20, Math.max(8, 8 + passageCount * 2));
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
			<div class="rp-bar">
				<!-- Connecting line -->
				<div class="rp-line"></div>

				<!-- Element markers (above the line) -->
				{#if $progressElements.length > 0}
					<div class="rp-elements">
						{#each $progressElements as elem (elem.label)}
							{@const cpIndex = $orderedCheckpoints.findIndex(c => c.meta.annotationId === elem.lastCheckpointId)}
							{@const totalCps = $orderedCheckpoints.length}
							{@const pct = totalCps > 1 ? (cpIndex / (totalCps - 1)) * 100 : 50}
							<div
								class="rp-element"
								style="left: {pct}%; --elem-color: {getElementColor(elem.label)}"
								title="{elem.label} at CP{elem.lastCheckpointSeq}"
							>
								<span class="rp-element-dot"></span>
								<span class="rp-element-label">{elem.label}</span>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Checkpoint dots -->
				<div class="rp-dots">
					{#each $orderedCheckpoints as cp, i (cp.meta.annotationId)}
						{@const totalCps = $orderedCheckpoints.length}
						{@const pct = totalCps > 1 ? (i / (totalCps - 1)) * 100 : 50}
						{@const size = dotSize(cp.passageCount)}
						{@const color = statusColor('checkpoint', cp.annotation.status)}
						<div
							class="rp-dot-wrapper"
							style="left: {pct}%"
						>
							<button
								class="rp-dot"
								style="width: {size}px; height: {size}px; background: {color}; border-color: {color}"
								onclick={() => toggleDetail(cp.meta.annotationId)}
								title="{cp.annotation.label} (#{cp.meta.sequenceNumber}) — {cp.passageCount} passages"
							>
								<span class="rp-dot-seq">{cp.meta.sequenceNumber}</span>
							</button>
							<span class="rp-dot-label">{cp.annotation.shortName || cp.annotation.label}</span>
							{#if cp.passageCount > 0}
								<span class="rp-dot-count">{cp.passageCount}</span>
							{/if}
						</div>
					{/each}
				</div>
			</div>
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
					{#if cp.passages.length > 0}
						<div class="rp-passages">
							{#each cp.passages.slice().reverse() as p (p.id)}
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

	.rp-bar {
		position: relative;
		min-height: 72px;
		min-width: 200px;
		padding: 24px 16px 0;
	}

	.rp-line {
		position: absolute;
		top: 36px;
		left: 16px;
		right: 16px;
		height: 2px;
		background: var(--color-primary);
		opacity: 0.6;
	}

	/* Element markers above the line */
	.rp-elements {
		position: absolute;
		top: 4px;
		left: 16px;
		right: 16px;
		height: 20px;
	}

	.rp-element {
		position: absolute;
		transform: translateX(-50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1px;
	}

	.rp-element-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--elem-color);
		box-shadow: 0 0 4px var(--elem-color);
	}

	.rp-element-label {
		font-size: 0.6rem;
		font-weight: 700;
		color: var(--elem-color);
		white-space: nowrap;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	/* Checkpoint dots */
	.rp-dots {
		position: absolute;
		top: 24px;
		left: 16px;
		right: 16px;
		height: 48px;
	}

	.rp-dot-wrapper {
		position: absolute;
		transform: translateX(-50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 3px;
	}

	.rp-dot {
		border: 2px solid;
		border-radius: 50%;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: transform var(--duration-fast), box-shadow var(--duration-fast);
		padding: 0;
		min-width: 0;
	}

	.rp-dot:hover {
		transform: scale(1.25);
		box-shadow: 0 0 8px rgba(255, 255, 255, 0.2);
	}

	.rp-dot-seq {
		font-size: 0.55rem;
		font-weight: 700;
		color: rgba(255, 255, 255, 0.9);
		line-height: 1;
	}

	.rp-dot-label {
		font-size: 0.6rem;
		color: var(--color-text-muted);
		white-space: nowrap;
		max-width: 60px;
		overflow: hidden;
		text-overflow: ellipsis;
		text-align: center;
	}

	.rp-dot-count {
		font-size: 0.55rem;
		color: var(--color-text-muted);
		opacity: 0.7;
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
