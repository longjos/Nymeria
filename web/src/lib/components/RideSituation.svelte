<script lang="ts">
	// The phone's whole ride surface, and the desktop strip's detail view.
	// Same $rideZones store as RideStrip, but each zone is a full-width row
	// (label · value · detail · chevron) — distinct markup from RideZone,
	// shared stores only (spec §0).
	import { rideZones, rideEmergency, ridePhase, handoffOpen, closeout, navigateZone, ackEmergency } from '$lib/stores/ride';
	import { wxIsNcs } from '$lib/stores/wxAlerts';
	import RideTierGlyph from './RideTierGlyph.svelte';
	import PhaseChip from './PhaseChip.svelte';
	import PhaseChangeDialog from './PhaseChangeDialog.svelte';
	import RidePendingList from './RidePendingList.svelte';
	import ShiftBriefing from './ShiftBriefing.svelte';

	let phaseDialogOpen = $state(false);
	let pendingOpen = $state(false);
	let briefingOpen = $state(false);
</script>

<div class="ride-situation">
	{#if $rideEmergency}
		<div class="rs-emergency">
			<RideTierGlyph tier={$rideEmergency.tier} size={20} title={$rideEmergency.tier.label} />
			<div class="rs-emergency-body">
				<strong>{$rideEmergency.kind.toUpperCase()} · {$rideEmergency.tier.label.toUpperCase()}</strong>
				<span>{$rideEmergency.summary}</span>
				<span class="rs-emergency-where">{$rideEmergency.where}</span>
			</div>
			<button class="rs-ack" onclick={() => $rideEmergency && ackEmergency($rideEmergency.id)}>Acknowledge</button>
		</div>
	{/if}

	<div class="rs-header">
		<h3 class="rs-heading">Ride Status</h3>
		<PhaseChip state={$ridePhase} canSet={$wxIsNcs} onOpen={() => (phaseDialogOpen = true)} />
	</div>

	<div class="rs-rows">
		{#each $rideZones as z (z.id)}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="rs-row" role="button" tabindex="0" onclick={() => navigateZone(z.id)} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); navigateZone(z.id); } }}>
				<span class="rs-row-label">{z.label}</span>
				<span class="rs-row-value">{z.lines[0]?.text ?? ''}</span>
				<span class="rs-row-detail">{z.lines.slice(1).map((l) => l.text).filter(Boolean).join(' · ')}</span>
				<span class="rs-row-chevron" aria-hidden="true">›</span>
			</div>
		{/each}
	</div>

	<div class="rs-footer-row">
		<button class="rs-link" onclick={() => (pendingOpen = true)}>Pending ({$handoffOpen.length}) ›</button>
		<button class="rs-link" onclick={() => (briefingOpen = true)}>Shift briefing ›</button>
	</div>

	{#if $ridePhase?.phase === 'reconcile' && $closeout}
		<div class="rs-closeout">
			<h4 class="rs-closeout-heading">Close-out checklist</h4>
			{#each $closeout.items as item (item.key)}
				<div class="rs-closeout-item" class:done={item.done}>
					<span>{item.done ? '✓' : '○'} {item.label}</span>
					{#if item.total > 0}<span class="rs-closeout-count">{item.count}/{item.total}</span>{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

{#if phaseDialogOpen}
	<PhaseChangeDialog phaseState={$ridePhase} onClose={() => (phaseDialogOpen = false)} />
{/if}
{#if pendingOpen}
	<RidePendingList onClose={() => (pendingOpen = false)} />
{/if}
{#if briefingOpen}
	<ShiftBriefing onClose={() => (briefingOpen = false)} />
{/if}

<style>
	.ride-situation {
		display: flex;
		flex-direction: column;
		border-bottom: 1px solid var(--color-primary);
	}

	.rs-emergency {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		border-left: 4px solid var(--color-ride-emergency);
		background: var(--color-ride-emergency-soft);
	}

	.rs-emergency-body {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: var(--space-2xs);
		font-size: var(--ride-t-body);
		min-width: 0;
	}

	.rs-emergency-where {
		color: var(--color-text-muted);
		font-size: var(--ride-t-label);
	}

	.rs-ack {
		min-height: 44px;
		padding: 0 var(--space-md);
		background: var(--color-ride-emergency);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-on-accent);
		font-weight: 700;
		cursor: pointer;
		flex-shrink: 0;
	}

	.rs-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 var(--space-md);
		min-height: 44px;
	}

	.rs-heading {
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
		margin: 0;
	}

	.rs-rows {
		display: flex;
		flex-direction: column;
	}

	.rs-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		min-height: 44px;
		padding: 0 var(--space-md);
		border-top: 1px solid var(--color-hairline);
		cursor: pointer;
	}

	.rs-row:hover,
	.rs-row:focus-visible {
		background: var(--color-raised);
	}

	.rs-row-label {
		width: 88px;
		flex-shrink: 0;
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-text-muted);
		letter-spacing: var(--ride-label-tracking);
	}

	.rs-row-value {
		font-size: 0.95rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.rs-row-detail {
		flex: 1;
		min-width: 0;
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.rs-row-chevron {
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.rs-footer-row {
		display: flex;
		gap: var(--space-md);
		padding: var(--space-sm) var(--space-md);
	}

	.rs-link {
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: var(--ride-t-body);
		cursor: pointer;
		min-height: 32px;
	}

	.rs-closeout {
		padding: var(--space-sm) var(--space-md);
		border-top: 1px solid var(--color-primary);
		gap: var(--space-xs);
	}

	.rs-closeout-heading {
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-text-muted);
		letter-spacing: var(--ride-label-tracking);
	}

	.rs-closeout-item {
		display: flex;
		justify-content: space-between;
		font-size: var(--ride-t-body);
		padding: 0;
		color: var(--color-text-muted);
		min-height: 32px;
	}

	.rs-closeout-item.done {
		color: var(--color-success);
	}

	.rs-closeout-count {
		font-variant-numeric: tabular-nums;
	}
</style>
