<script lang="ts">
	import type { CheckpointWithPassages, ProgressElement, WxAlert } from '$lib/types';
	import { statusColor } from '$lib/annotationMeta';
	import WxTierGlyph from '../WxTierGlyph.svelte';

	let {
		checkpoints,
		elements,
		presentationMode = false,
		wxAlerts = [],
	}: {
		checkpoints: CheckpointWithPassages[];
		elements: ProgressElement[];
		presentationMode?: boolean;
		/** Active IN alerts — the dashboard keeps its own snapshot (no shared session with the main app). */
		wxAlerts?: WxAlert[];
	} = $props();

	// Tier brackets over the affected checkpoint span — mirrors RouteProgressBar.svelte §8.6.
	let wxBrackets = $derived(
		wxAlerts
			.filter((a) => a.tier !== 'statement' && a.affects.checkpointSeqRange.length === 2)
			.map((a) => {
				const [loSeq, hiSeq] = a.affects.checkpointSeqRange;
				const total = checkpoints.length;
				const loIdx = checkpoints.findIndex((c) => c.meta.sequenceNumber === loSeq);
				const hiIdx = checkpoints.findIndex((c) => c.meta.sequenceNumber === hiSeq);
				if (loIdx < 0 || hiIdx < 0) return null;
				const pctStart = total > 1 ? (loIdx / (total - 1)) * 100 : 0;
				const pctEnd = total > 1 ? (hiIdx / (total - 1)) * 100 : 100;
				return { alert: a, pctStart, pctEnd, loSeq, hiSeq };
			})
			.filter((b): b is NonNullable<typeof b> => b !== null)
	);

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
		return Math.min(24, Math.max(10, 10 + passageCount * 2));
	}

	function timeAgoShort(dateStr: string): string {
		const ms = Date.now() - new Date(dateStr).getTime();
		const m = Math.floor(ms / 60_000);
		if (m < 60) return `${m}m ago`;
		const h = Math.floor(m / 60);
		return `${h}h ${m % 60}m ago`;
	}
</script>

{#if checkpoints.length > 0}
	<section class="event-progress" class:presentation={presentationMode}>
		<h2 class="section-title">Event Progress</h2>

		<div class="progress-container">
			<div class="progress-bar" class:has-wx={wxBrackets.length > 0}>
				{#if wxBrackets.length > 0}
					<div class="wx-layer">
						{#each wxBrackets as b, i (b.alert.id)}
							<div
								class="wx-bracket"
								style="left: {b.pctStart}%; width: {b.pctEnd - b.pctStart}%; top: {2 + i * 14}px; --wx: var(--color-wx-{b.alert.tier})"
								title="{b.alert.event}: CP {b.loSeq}–CP {b.hiSeq}"
							>
								<span class="wx-bracket-label"><WxTierGlyph tier={b.alert.tier} size={presentationMode ? 13 : 10} /> {b.alert.shortCode}</span>
							</div>
						{/each}
					</div>
				{/if}
				<div class="progress-line"></div>

				<!-- Element position markers above the line -->
				{#if elements.length > 0}
					<div class="element-markers">
						{#each elements as elem (elem.label)}
							{@const cpIndex = checkpoints.findIndex(c => c.meta.annotationId === elem.lastCheckpointId)}
							{@const totalCps = checkpoints.length}
							{@const pct = totalCps > 1 ? (cpIndex / (totalCps - 1)) * 100 : 50}
							<div
								class="element-marker"
								style="left: {pct}%; --elem-color: {getElementColor(elem.label)}"
								title="{elem.label} — last seen {timeAgoShort(elem.lastPassageTime)}"
							>
								<span class="elem-dot"></span>
								<span class="elem-label">{elem.label}</span>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Checkpoint dots on the line -->
				<div class="checkpoint-dots">
					{#each checkpoints as cp, i (cp.meta.annotationId)}
						{@const totalCps = checkpoints.length}
						{@const pct = totalCps > 1 ? (i / (totalCps - 1)) * 100 : 50}
						{@const size = dotSize(cp.passageCount)}
						{@const color = statusColor('checkpoint', cp.annotation.status)}
						<div class="cp-wrapper" style="left: {pct}%">
							<div
								class="cp-dot"
								style="width: {size}px; height: {size}px; background: {color}; border-color: {color}"
								title="{cp.annotation.label} — {cp.passageCount} passages"
							>
								<span class="cp-seq">{cp.meta.sequenceNumber}</span>
							</div>
							<span class="cp-label">{cp.annotation.shortName || cp.annotation.label}</span>
							{#if cp.passageCount > 0}
								<span class="cp-count">{cp.passageCount} through</span>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		</div>
	</section>
{/if}

<style>
	.event-progress {
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

	.progress-container {
		overflow-x: auto;
		padding-bottom: var(--space-sm);
	}

	.progress-bar {
		position: relative;
		min-height: 90px;
		min-width: 250px;
		padding: 30px 20px 0;
	}

	.presentation .progress-bar {
		min-height: 110px;
		padding: 36px 24px 0;
	}

	.progress-bar.has-wx {
		padding-top: 46px;
	}
	.presentation .progress-bar.has-wx {
		padding-top: 54px;
	}

	.wx-layer {
		position: absolute;
		top: 0;
		left: 20px;
		right: 20px;
		height: 46px;
	}

	.wx-bracket {
		position: absolute;
		height: 8px;
		border: 2px solid var(--wx);
		border-bottom: none;
		border-radius: 3px 3px 0 0;
	}

	.wx-bracket-label {
		position: absolute;
		top: -13px;
		left: 0;
		display: inline-flex;
		align-items: center;
		gap: 2px;
		font-size: 0.6rem;
		font-weight: 700;
		color: var(--wx);
		white-space: nowrap;
	}
	.presentation .wx-bracket-label {
		font-size: 0.75rem;
	}

	.progress-line {
		position: absolute;
		top: 44px;
		left: 20px;
		right: 20px;
		height: 3px;
		background: var(--color-primary);
		opacity: 0.6;
		border-radius: 2px;
	}

	.presentation .progress-line {
		top: 52px;
		height: 4px;
	}

	.element-markers {
		position: absolute;
		top: 6px;
		left: 20px;
		right: 20px;
		height: 28px;
	}

	.element-marker {
		position: absolute;
		transform: translateX(-50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
	}

	.elem-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--elem-color);
		box-shadow: 0 0 6px var(--elem-color);
	}

	.presentation .elem-dot {
		width: 10px;
		height: 10px;
	}

	.elem-label {
		font-size: 0.65rem;
		font-weight: 700;
		color: var(--elem-color);
		white-space: nowrap;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.presentation .elem-label {
		font-size: 0.8rem;
	}

	.checkpoint-dots {
		position: absolute;
		top: 30px;
		left: 20px;
		right: 20px;
		height: 60px;
	}

	.presentation .checkpoint-dots {
		top: 36px;
		height: 70px;
	}

	.cp-wrapper {
		position: absolute;
		transform: translateX(-50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 3px;
	}

	.cp-dot {
		border: 2px solid;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.cp-seq {
		font-size: 0.6rem;
		font-weight: 700;
		color: rgba(255, 255, 255, 0.9);
		line-height: 1;
	}

	.presentation .cp-seq {
		font-size: 0.75rem;
	}

	.cp-label {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		white-space: nowrap;
		max-width: 70px;
		overflow: hidden;
		text-overflow: ellipsis;
		text-align: center;
	}

	.presentation .cp-label {
		font-size: 0.8rem;
		max-width: 90px;
	}

	.cp-count {
		font-size: 0.55rem;
		color: var(--color-text-muted);
		opacity: 0.7;
	}

	.presentation .cp-count {
		font-size: 0.7rem;
	}
</style>
