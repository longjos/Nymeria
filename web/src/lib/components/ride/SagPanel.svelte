<script lang="ts">
	// Tab root for the "SAG" surface (WP7, part 1): SAG board / composers,
	// plus the supply and medical traffic composers that share the same
	// backend package and the same net-scoped stores. One segmented sub-nav,
	// three self-contained board components — mirrors how NetControlPanel
	// itself hands each top-level tab to a single dedicated component
	// (LocationManager, SituationBoard).
	import { sagComposerSeed, rideSagSummary, medicalOpen, supplyAll } from '$lib/stores/ride';
	import SagBoard from './SagBoard.svelte';
	import SupplyBoard from './SupplyBoard.svelte';
	import MedicalBoard from './MedicalBoard.svelte';

	type SubTab = 'sag' | 'supply' | 'medical';
	let subTab = $state<SubTab>('sag');

	// The command palette's `s`/"sag " accelerator hands off here — jump to
	// the SAG sub-tab so SagBoard's own subscription to sagComposerSeed can
	// open the composer. SagBoard consumes and clears the seed itself.
	$effect(() => {
		if ($sagComposerSeed) subTab = 'sag';
	});

	const terminalSupply = new Set(['delivered', 'cancelled', 'merged']);
	let supplyOpenCount = $derived($supplyAll.filter((s) => !terminalSupply.has(s.status)).length);
</script>

<div class="sag-panel">
	<div class="sp-subnav" role="tablist" aria-label="SAG, supply and medical">
		<button class="sp-tab" role="tab" aria-selected={subTab === 'sag'} class:active={subTab === 'sag'} onclick={() => (subTab = 'sag')}>
			SAG
			<!-- Two different numbers, because they mean two different things:
			     how much work is live, and how much of it is blocked on NCS. -->
			{#if $rideSagSummary.active > 0}<span class="sp-count">{$rideSagSummary.active}</span>{/if}
			{#if $rideSagSummary.open > 0}
				<span class="sp-count sp-count-wait" title="{$rideSagSummary.ridersWaiting} rider(s) waiting for a vehicle">
					⬒ {$rideSagSummary.open}
				</span>
			{/if}
		</button>
		<button class="sp-tab" role="tab" aria-selected={subTab === 'supply'} class:active={subTab === 'supply'} onclick={() => (subTab = 'supply')}>
			Supply
			{#if supplyOpenCount > 0}<span class="sp-count">{supplyOpenCount}</span>{/if}
		</button>
		<button class="sp-tab" role="tab" aria-selected={subTab === 'medical'} class:active={subTab === 'medical'} onclick={() => (subTab = 'medical')}>
			Medical
			{#if $medicalOpen.length > 0}<span class="sp-count sp-count-alert">{$medicalOpen.length}</span>{/if}
		</button>
	</div>

	<div class="sp-body">
		{#if subTab === 'sag'}
			<SagBoard />
		{:else if subTab === 'supply'}
			<SupplyBoard />
		{:else}
			<MedicalBoard />
		{/if}
	</div>
</div>

<style>
	.sag-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
	}

	.sp-subnav {
		display: flex;
		gap: var(--space-2xs);
		padding: var(--space-sm) var(--space-md) 0;
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.sp-tab {
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
	}

	.sp-tab.active {
		color: var(--color-text);
		border-bottom-color: var(--color-accent);
	}

	.sp-count {
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

	.sp-count-wait {
		background: var(--color-ride-priority-soft);
		color: var(--color-warning);
	}

	.sp-count-alert {
		background: var(--color-ride-emergency-soft);
		color: var(--color-error-text);
	}

	.sp-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
	}
</style>
