<script lang="ts">
	// Tab root for the "Course" surface (WP7, part 2): rest-stop ladder,
	// shutoffs, sweep, rider exceptions, reconciliation. One segmented
	// sub-nav, five self-contained boards — mirrors SagPanel.svelte, which
	// mirrors how NetControlPanel itself hands each top-level tab to one
	// dedicated component.
	import { courseStations, shutoffsList, plannedShutoffCount, stationsAwaitingSweep, courseListLoad, initCourseStore } from '$lib/stores/course';
	import { courseState } from '$lib/stores/ride';
	import { isNetNcs } from '$lib/stores/netProfile';
	import { canOperate } from '$lib/stores/session';
	import { activeNet } from '$lib/stores/netcontrol';
	import { courseRequestedTab } from '$lib/stores/ui';
	import RestStopBoard from './RestStopBoard.svelte';
	import ShutoffBoard from './ShutoffBoard.svelte';
	import SweepTab from './SweepTab.svelte';
	import RiderExceptionLog from './RiderExceptionLog.svelte';
	import ReconcileView from './ReconcileView.svelte';

	let { onFlyTo, onNavigateTab }: {
		onFlyTo?: (lat: number, lon: number, zoom?: number) => void;
		onNavigateTab?: (tab: string) => void;
	} = $props();

	initCourseStore();

	type CourseSubTab = 'stops' | 'shutoffs' | 'sweep' | 'riders' | 'closeout';
	let subTab = $state<CourseSubTab>('stops');

	$effect(() => {
		const requested = $courseRequestedTab;
		if (requested) {
			subTab = requested;
			courseRequestedTab.set(null);
		}
	});

	const TABS: Array<{ key: CourseSubTab; label: string }> = [
		{ key: 'stops', label: 'Stops' },
		{ key: 'shutoffs', label: 'Shutoffs' },
		{ key: 'sweep', label: 'Sweep' },
		{ key: 'riders', label: 'Riders' },
		{ key: 'closeout', label: 'Closeout' }
	];

	function handleTabKeydown(e: KeyboardEvent): void {
		if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'Home' && e.key !== 'End') return;
		e.preventDefault();
		const idx = TABS.findIndex((t) => t.key === subTab);
		let next = idx;
		if (e.key === 'ArrowRight') next = (idx + 1) % TABS.length;
		else if (e.key === 'ArrowLeft') next = (idx - 1 + TABS.length) % TABS.length;
		else if (e.key === 'Home') next = 0;
		else if (e.key === 'End') next = TABS.length - 1;
		subTab = TABS[next].key;
		(document.getElementById(`course-tab-${TABS[next].key}`) as HTMLElement | null)?.focus();
	}
</script>

<div class="course-panel">
	<div class="cp-header">
		{#if $courseState}
			<p class="cp-clear-through" aria-live="polite">
				{#if $courseState.clearThroughSeq > 0}
					Clear through <strong>{$courseState.clearThroughLabel}</strong>
					<span class="muted">· {$courseState.stationsClosed} of {$courseStations.length} closed</span>
				{:else}
					<span class="muted">Nothing cleared yet · {$courseStations.length} stop{$courseStations.length === 1 ? '' : 's'}</span>
				{/if}
			</p>
		{/if}
	</div>

	<div class="cp-tabs" role="tablist" aria-label="Course sections" tabindex="-1" onkeydown={handleTabKeydown}>
		{#each TABS as t (t.key)}
			<button
				class="cp-tab tab"
				role="tab"
				id="course-tab-{t.key}"
				aria-selected={subTab === t.key}
				aria-controls="course-pane-{t.key}"
				tabindex={subTab === t.key ? 0 : -1}
				class:active={subTab === t.key}
				onclick={() => (subTab = t.key)}
			>
				<span class="tab-label">{t.label}</span>
				{#if t.key === 'stops' && $stationsAwaitingSweep.length > 0}
					<span class="tab-count tab-count-alert" aria-label="{$stationsAwaitingSweep.length} stops awaiting sweep">{$stationsAwaitingSweep.length}</span>
				{:else if t.key === 'shutoffs' && $plannedShutoffCount > 0}
					<span class="tab-count">{$plannedShutoffCount}</span>
				{/if}
			</button>
		{/each}
	</div>

	<div class="cp-content" id="course-pane-{subTab}" role="tabpanel" aria-labelledby="course-tab-{subTab}" tabindex="0">
		{#if $courseListLoad === 'unavailable'}
			<div class="empty-state">
				<p>Course closure is not enabled on this server.</p>
			</div>
		{:else if subTab === 'stops'}
			{#if $courseStations.length === 0}
				<div class="empty-state">
					<p>No sequenced stops yet.</p>
					<p class="muted">Give your rest-stop locations sequence numbers in Net Control &rarr; Locations.</p>
					<button class="btn-secondary" onclick={() => onNavigateTab?.('locations')}>Open Locations</button>
				</div>
			{:else}
				<RestStopBoard
					stations={$courseStations}
					config={$courseState?.config ?? null}
					clearThroughSeq={$courseState?.clearThroughSeq ?? 0}
					canOperate={$canOperate}
					isNcs={$isNetNcs}
					ncsCallsign={$activeNet?.ncsCallsign ?? ''}
					{onFlyTo}
				/>
			{/if}
		{:else if subTab === 'shutoffs'}
			<ShutoffBoard shutoffs={$shutoffsList} {onFlyTo} />
		{:else if subTab === 'sweep'}
			<SweepTab {onFlyTo} />
		{:else if subTab === 'riders'}
			<RiderExceptionLog />
		{:else}
			<ReconcileView />
		{/if}
	</div>
</div>

<style>
	.course-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
	}

	.cp-header {
		padding: var(--space-sm) var(--space-md) 0;
		flex-shrink: 0;
	}

	.cp-clear-through {
		font-size: 0.8rem;
		color: var(--color-text);
	}

	.muted {
		color: var(--color-text-muted);
	}

	.cp-tabs {
		display: flex;
		gap: var(--space-2xs);
		padding: var(--space-sm) var(--space-md) 0;
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
		overflow-x: auto;
	}

	.cp-tab {
		display: flex;
		align-items: center;
		gap: 6px;
		min-height: 40px;
		padding: 0 var(--space-sm);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		font-weight: 700;
		cursor: pointer;
		white-space: nowrap;
	}

	.cp-tab.active {
		color: var(--color-text);
		border-bottom-color: var(--color-accent);
	}

	.tab-count {
		min-width: 18px;
		height: 18px;
		padding: 0 5px;
		border-radius: var(--radius-full);
		background: var(--color-bg);
		color: var(--color-text-muted);
		font-size: 0.68rem;
		font-weight: 700;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.tab-count-alert {
		background: var(--color-ride-priority-soft);
		color: var(--color-warning);
	}

	.cp-content {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
	}

	.empty-state {
		padding: var(--space-lg) var(--space-md);
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		align-items: center;
	}

	.btn-secondary {
		min-height: 40px;
		padding: 0 var(--space-md);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-weight: 600;
		cursor: pointer;
	}
</style>
