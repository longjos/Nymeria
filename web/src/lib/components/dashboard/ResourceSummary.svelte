<script lang="ts">
	import type { NetCheckIn, StationCategory } from '$lib/types';
	import { stationCategoryMeta } from '$lib/stationCategoryMeta';
	import { statusLabels, humanName } from '$lib/agencyTranslations';

	let {
		checkIns,
		presentationMode = false,
	}: {
		checkIns: NetCheckIn[];
		presentationMode?: boolean;
	} = $props();

	const STALE_MS = 20 * 60 * 1000;

	let expandedCategory = $state<StationCategory | null>(null);

	interface CategoryGroup {
		category: StationCategory;
		label: string;
		color: string;
		operators: NetCheckIn[];
		available: number;
		responding: number;
		onScene: number;
		stale: number;
		missing: number;
	}

	let groups = $derived.by((): CategoryGroup[] => {
		const active = checkIns.filter(ci => ci.status !== 'released');
		const byCategory = new Map<StationCategory, NetCheckIn[]>();

		for (const ci of active) {
			const cat = (ci.category || 'general') as StationCategory;
			const list = byCategory.get(cat) || [];
			list.push(ci);
			byCategory.set(cat, list);
		}

		const now = Date.now();
		const result: CategoryGroup[] = [];

		for (const [cat, ops] of byCategory) {
			const meta = stationCategoryMeta[cat] || stationCategoryMeta.general;
			let available = 0, responding = 0, onScene = 0, stale = 0, missing = 0;

			for (const op of ops) {
				if (op.status === 'missing') { missing++; continue; }
				if (op.status === 'available') available++;
				else if (op.status === 'enroute') responding++;
				else if (op.status === 'onscene') onScene++;

				const elapsed = now - new Date(op.lastHeard).getTime();
				if (elapsed > STALE_MS && op.status !== 'missing') stale++;
			}

			result.push({
				category: cat,
				label: meta.label,
				color: meta.color,
				operators: ops,
				available,
				responding,
				onScene,
				stale,
				missing,
			});
		}

		// Sort: command first, then by operator count desc
		const catOrder: Record<string, number> = { command: 0, medical: 1, sag: 2, marshal: 3, tactical: 4, fixed: 5, mobile: 6, general: 7 };
		result.sort((a, b) => (catOrder[a.category] ?? 99) - (catOrder[b.category] ?? 99));
		return result;
	});

	function toggleExpand(cat: StationCategory) {
		expandedCategory = expandedCategory === cat ? null : cat;
	}

	function currentAssignment(ci: NetCheckIn): string {
		return ci.assignment || statusLabels[ci.status] || ci.status;
	}
</script>

<section class="resource-summary" class:presentation={presentationMode}>
	<h2 class="section-title">Resources</h2>

	{#if groups.length === 0}
		<p class="empty">No active operators</p>
	{:else}
		<div class="category-grid">
			{#each groups as group (group.category)}
				<div
					class="category-card"
					role="button"
					tabindex="0"
					onclick={() => toggleExpand(group.category)}
					onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggleExpand(group.category); }}
				>
					<div class="card-bar" style="background: {group.color}"></div>
					<div class="card-body">
						<div class="card-header">
							<span class="card-title">{group.label}</span>
							<span class="card-count">{group.operators.length}</span>
						</div>
						<div class="card-stats">
							{#if group.available > 0}
								<span class="stat">{group.available} Available</span>
							{/if}
							{#if group.responding > 0}
								<span class="stat">{group.responding} Responding</span>
							{/if}
							{#if group.onScene > 0}
								<span class="stat">{group.onScene} On Scene</span>
							{/if}
							{#if group.missing > 0}
								<span class="stat stat-danger">{group.missing} UNREACHABLE</span>
							{/if}
						</div>
						{#if group.stale > 0}
							<div class="stale-warning">
								<svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
									<path d="M8 1.5l6.5 13H1.5L8 1.5zM8 6v3M8 11h.01"/>
								</svg>
								{group.stale} with no recent contact
							</div>
						{/if}
					</div>

					{#if expandedCategory === group.category}
						<div class="card-detail">
							{#each group.operators as op (op.id)}
								<div class="op-row" class:op-missing={op.status === 'missing'}>
									<span class="op-name">{humanName(op)}</span>
									<span class="op-status">{currentAssignment(op)}</span>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</section>

<style>
	.resource-summary {
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

	.category-grid {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.category-card {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		overflow: hidden;
		cursor: pointer;
		transition: background var(--duration-fast);
	}

	.category-card:hover {
		background: color-mix(in srgb, var(--color-surface) 85%, white);
	}

	.card-bar {
		height: 3px;
	}

	.card-body {
		padding: var(--space-sm) var(--space-md);
	}

	.card-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 4px;
	}

	.card-title {
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.presentation .card-title {
		font-size: 1.05rem;
	}

	.card-count {
		font-size: 1.1rem;
		font-weight: 700;
		color: var(--color-text);
	}

	.presentation .card-count {
		font-size: 1.3rem;
	}

	.card-stats {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-sm);
	}

	.stat {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.presentation .stat {
		font-size: 0.85rem;
	}

	.stat-danger {
		color: var(--color-error);
		font-weight: 700;
	}

	.stale-warning {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 0.7rem;
		color: var(--color-warning);
		margin-top: 4px;
	}

	.presentation .stale-warning {
		font-size: 0.8rem;
	}

	.card-detail {
		border-top: 1px solid var(--color-primary);
		padding: var(--space-sm) var(--space-md);
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.op-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		font-size: 0.8rem;
		padding: 2px 0;
	}

	.presentation .op-row {
		font-size: 0.9rem;
	}

	.op-name {
		font-weight: 500;
		color: var(--color-text);
	}

	.op-status {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.op-missing .op-name {
		color: var(--color-error);
	}

	.op-missing .op-status {
		color: var(--color-error);
		font-weight: 700;
	}
</style>
