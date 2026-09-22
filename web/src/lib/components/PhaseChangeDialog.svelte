<script lang="ts">
	// Opened by PhaseChip (spec §8). Forward one step is pre-selected when the
	// system has a live suggestion; any other forward step or a backward move
	// is available too, with illegal targets disabled and explained. A
	// backward move requires a reason; a forward move does not (the trigger
	// that earned it is reason enough).
	import type { RidePhaseStatus, RidePhaseID } from '$lib/types';
	import { setPhase } from '$lib/stores/ride';
	import { phaseLabel, RIDE_PHASE_ORDER } from '$lib/rideMeta';
	import { showToast } from '$lib/stores/toast';

	let { phaseState, onClose }: { phaseState: RidePhaseStatus | null; onClose: () => void } = $props();

	let fromIdx = $derived(RIDE_PHASE_ORDER.indexOf(phaseState?.phase ?? 'pre-start'));
	let suggested = $derived(phaseState?.suggestion.phase || null);

	let selected = $state<RidePhaseID | null>(null);
	$effect(() => {
		if (selected === null) selected = (suggested as RidePhaseID) ?? RIDE_PHASE_ORDER[Math.min(fromIdx + 1, RIDE_PHASE_ORDER.length - 1)];
	});

	let reason = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	function legal(to: RidePhaseID): boolean {
		const ti = RIDE_PHASE_ORDER.indexOf(to);
		if (ti === fromIdx + 1) return true;
		return ti < fromIdx;
	}

	function why(to: RidePhaseID): string {
		const ti = RIDE_PHASE_ORDER.indexOf(to);
		if (ti === fromIdx) return 'same phase';
		if (ti > fromIdx + 1) return 'forward one step only';
		return '';
	}

	let isBackward = $derived(selected != null && RIDE_PHASE_ORDER.indexOf(selected) < fromIdx);
	let canSubmit = $derived(selected != null && legal(selected) && (!isBackward || reason.trim().length > 0) && !submitting);

	async function submit(): Promise<void> {
		if (!selected || !canSubmit) return;
		submitting = true;
		error = null;
		try {
			await setPhase(selected, reason.trim() || undefined);
			showToast(`Ride phase ${phaseLabel(selected)}`, 'success');
			onClose();
		} catch (e: any) {
			error = e?.error ?? (e instanceof Error ? e.message : 'Phase change failed');
		} finally {
			submitting = false;
		}
	}

	let dialogEl = $state<HTMLElement | null>(null);
	$effect(() => {
		dialogEl?.querySelector<HTMLElement>('input, select, button, textarea')?.focus();
	});

	function trapTab(e: KeyboardEvent): void {
		if (e.key !== 'Tab' || !dialogEl) return;
		const focusable = Array.from(dialogEl.querySelectorAll<HTMLElement>('input, select, textarea, button:not([disabled])'));
		if (focusable.length === 0) return;
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	function handleKeydown(e: KeyboardEvent): void {
		if (e.key === 'Escape') onClose();
		else trapTab(e);
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="pcd-backdrop" role="presentation" onclick={onClose}>
	<div
		class="pcd"
		bind:this={dialogEl}
		role="dialog"
		tabindex="-1"
		aria-modal="true"
		aria-labelledby="pcd-title"
		data-blocks-escape="true"
		onclick={(e) => e.stopPropagation()}
	>
		<h2 id="pcd-title" class="pcd-title">Change ride phase</h2>
		<p class="pcd-current">Currently <strong>{phaseLabel(phaseState?.phase ?? 'pre-start')}</strong>{#if suggested}<span class="pcd-suggest"> · suggested: {phaseLabel(suggested)}</span>{/if}</p>

		<fieldset class="pcd-fieldset">
			<legend class="sr-only">Target phase</legend>
			{#each RIDE_PHASE_ORDER as p (p)}
				{@const ok = legal(p)}
				<label class="pcd-option" class:disabled={!ok}>
					<input type="radio" name="phase" value={p} checked={selected === p} disabled={!ok} onchange={() => (selected = p)} />
					<span class="pcd-option-label">{phaseLabel(p)}</span>
					{#if !ok && why(p)}<span class="pcd-option-why">({why(p)})</span>{/if}
				</label>
			{/each}
		</fieldset>

		{#if isBackward}
			<label class="pcd-reason-label" for="pcd-reason">Reason (required for a backward move)</label>
			<textarea id="pcd-reason" class="pcd-reason" bind:value={reason} rows="2" placeholder="Why are we moving back?"></textarea>
		{/if}

		{#if error}<p class="pcd-error">{error}</p>{/if}

		<div class="pcd-actions">
			<button class="pcd-btn pcd-cancel" onclick={onClose}>Cancel</button>
			<button class="pcd-btn pcd-submit" disabled={!canSubmit} onclick={submit}>{submitting ? 'Saving…' : 'Set phase'}</button>
		</div>
	</div>
</div>

<style>
	.pcd-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.pcd {
		width: 100%;
		max-width: 380px;
		max-height: 90vh;
		overflow-y: auto;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.pcd-title {
		font-size: 1.05rem;
		font-weight: 700;
	}

	.pcd-current {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}

	.pcd-suggest {
		color: var(--color-warning);
	}

	.pcd-fieldset {
		border: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2xs);
	}

	.pcd-option {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		min-height: 40px;
		padding: 4px 6px;
		border-radius: var(--radius-sm);
		cursor: pointer;
	}

	.pcd-option:hover:not(.disabled) {
		background: var(--color-bg);
	}

	.pcd-option.disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.pcd-option-label {
		font-weight: 700;
		font-size: 0.85rem;
	}

	.pcd-option-why {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.pcd-reason-label {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.pcd-reason {
		width: 100%;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-sm);
		font: inherit;
		resize: vertical;
	}

	.pcd-error {
		color: var(--color-error-text);
		font-size: 0.8rem;
	}

	.pcd-actions {
		display: flex;
		gap: var(--space-sm);
		justify-content: flex-end;
		margin-top: var(--space-xs);
	}

	.pcd-btn {
		min-height: 44px;
		padding: 0 var(--space-md);
		border-radius: var(--radius-sm);
		font-weight: 700;
		cursor: pointer;
	}

	.pcd-cancel {
		background: none;
		border: 1px solid var(--color-primary);
		color: var(--color-text);
	}

	.pcd-submit {
		background: var(--color-accent);
		border: none;
		color: var(--color-on-accent);
	}

	.pcd-submit:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}
</style>
