<script lang="ts">
	// Create/edit dialog for a SAG request. Create takes pickup, dropoff,
	// reason, priority and N rider slots; edit (an existing, non-terminal
	// request) covers everything except slots — those are managed from the
	// request's own dispatch card (add rider / resolve slot), never here.
	import { onMount } from 'svelte';
	import { api, ApiError } from '$lib/api';
	import type { SAGRequest, SAGLocation, SAGConfigResponse, Annotation } from '$lib/types';
	import { rideLadder, upsertSagRequest } from '$lib/stores/ride';
	import { netAnnotations, activeCheckIns } from '$lib/stores/netcontrol';
	import { currentUser } from '$lib/stores/session';
	import { tierById } from '$lib/rideMeta';
	import { showToast } from '$lib/stores/toast';
	import RideTierGlyph from '../RideTierGlyph.svelte';
	import SagLocationField from './SagLocationField.svelte';

	let {
		netId,
		editing = null,
		initialReason = '',
		onClose
	}: {
		netId: string;
		editing?: SAGRequest | null;
		initialReason?: string;
		onClose: () => void;
	} = $props();

	interface SlotRow {
		bib: string;
		riderName: string;
		note: string;
		hasBike: boolean;
	}

	let config = $state<SAGConfigResponse | null>(null);
	let configError = $state<string | null>(null);

	let pickup = $state<SAGLocation>(editing?.pickup ?? { kind: 'course' });
	let dropoff = $state<SAGLocation>(editing?.dropoff ?? { kind: 'next_reststop' });
	let reason = $state(editing?.reason ?? initialReason);
	let priority = $state(editing?.priority ?? '');
	let priorityTouched = $state(!!editing);
	let notes = $state(editing?.notes ?? '');
	let requestedBy = $state(editing?.requestedBy ?? $currentUser?.callsign ?? $currentUser?.name ?? '');
	let slots = $state<SlotRow[]>([{ bib: '', riderName: '', note: '', hasBike: true }]);

	let submitting = $state(false);
	let error = $state<string | null>(null);

	onMount(async () => {
		try {
			config = await api.sagConfig(netId);
			if (!editing) {
				pickup.kind = config.pickupKinds[0] ?? pickup.kind;
				dropoff.kind = config.dropoffKinds.includes('next_reststop') ? 'next_reststop' : (config.dropoffKinds[0] ?? dropoff.kind);
			}
		} catch (e) {
			configError = e instanceof ApiError ? e.message : 'Could not load SAG configuration';
		}
	});

	// Default priority follows pickup kind (course -> high, elsewhere ->
	// medium, per the net's own config) until the operator picks one by hand.
	$effect(() => {
		if (priorityTouched || !config) return;
		const def = pickup.kind === 'course' ? config.config.defaultCoursePriority : config.config.defaultStopPriority;
		if (def) priority = def;
	});

	function setPriority(id: string): void {
		priority = id;
		priorityTouched = true;
	}

	function addSlot(): void {
		slots = [...slots, { bib: '', riderName: '', note: '', hasBike: true }];
	}

	function removeSlot(i: number): void {
		if (slots.length <= 1) return;
		slots = slots.filter((_, idx) => idx !== i);
	}

	let canSubmit = $derived(!!pickup.kind && !!dropoff.kind && !!priority && !submitting);

	async function submit(): Promise<void> {
		if (!canSubmit) return;
		submitting = true;
		error = null;
		try {
			if (editing) {
				const updated = await api.updateSagRequest(netId, editing.id, {
					pickup, dropoff, reason, priority, notes, requestedBy
				});
				upsertSagRequest(updated);
				showToast(`SAG ${updated.sequence} updated`, 'success');
			} else {
				const created = await api.createSagRequest(netId, {
					pickup, dropoff, reason, priority, notes, requestedBy,
					slots: slots.map((s) => ({ bib: s.bib.trim(), riderName: s.riderName.trim(), note: s.note.trim(), hasBike: s.hasBike }))
				});
				upsertSagRequest(created);
				showToast(`SAG ${created.sequence} requested`, 'success');
			}
			onClose();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Failed to save the SAG request';
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

	let checkInSuggestions = $derived(
		Array.from(new Set($activeCheckIns.map((c) => c.tacticalCall || c.callsign).filter(Boolean)))
	);
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="src-backdrop" role="presentation" onclick={onClose}>
	<div
		class="src"
		bind:this={dialogEl}
		role="dialog"
		tabindex="-1"
		aria-modal="true"
		aria-labelledby="src-title"
		onclick={(e) => e.stopPropagation()}
	>
		<h2 id="src-title" class="src-title">{editing ? `Edit SAG ${editing.sequence}` : 'New SAG request'}</h2>

		{#if configError}
			<p class="src-error">{configError}</p>
		{:else if !config}
			<p class="src-loading">Loading…</p>
		{:else}
			<div class="src-body">
				<SagLocationField
					legend="Pickup"
					idPrefix="src-pickup"
					value={pickup}
					kinds={config.pickupKinds}
					annotations={$netAnnotations}
					onChange={(v) => (pickup = v)}
				/>
				<SagLocationField
					legend="Dropoff"
					idPrefix="src-dropoff"
					value={dropoff}
					kinds={config.dropoffKinds}
					annotations={$netAnnotations}
					onChange={(v) => (dropoff = v)}
				/>

				<label class="src-field" for="src-reason">
					<span class="src-label">Reason</span>
					<input id="src-reason" type="text" list="src-reasons" bind:value={reason} placeholder="flat, mechanical, fatigue…" />
					<datalist id="src-reasons">
						{#each config.config.reasons as r (r)}<option value={r}></option>{/each}
					</datalist>
				</label>

				<div class="src-field">
					<span class="src-label">Priority</span>
					<div class="src-tiers">
						{#each config.config.priorities as id (id)}
							{@const tier = tierById($rideLadder, id)}
							<button type="button" class="src-tier-chip" class:active={priority === id} onclick={() => setPriority(id)}>
								{#if tier}<RideTierGlyph {tier} size={14} />{/if}
								{tier?.label ?? id}
							</button>
						{/each}
					</div>
				</div>

				<label class="src-field" for="src-requestedby">
					<span class="src-label">Requested by</span>
					<input id="src-requestedby" type="text" list="src-checkins" bind:value={requestedBy} placeholder="tactical call / callsign" />
					<datalist id="src-checkins">
						{#each checkInSuggestions as c (c)}<option value={c}></option>{/each}
					</datalist>
				</label>

				<label class="src-field" for="src-notes">
					<span class="src-label">Notes</span>
					<textarea id="src-notes" rows="2" bind:value={notes} placeholder="anything else NCS should know"></textarea>
				</label>

				{#if !editing}
					<fieldset class="src-slots">
						<legend class="src-legend">Riders ({slots.length})</legend>
						{#each slots as slot, i (i)}
							<div class="src-slot-row">
								<input type="text" placeholder="bib (optional)" bind:value={slot.bib} aria-label="Bib for rider {i + 1}" />
								<input type="text" placeholder="rider name (optional)" bind:value={slot.riderName} aria-label="Name for rider {i + 1}" />
								<input type="text" placeholder="note" bind:value={slot.note} aria-label="Note for rider {i + 1}" />
								<label class="src-bike"><input type="checkbox" bind:checked={slot.hasBike} /> bike</label>
								<button type="button" class="src-slot-remove" onclick={() => removeSlot(i)} disabled={slots.length <= 1} aria-label="Remove rider {i + 1}">×</button>
							</div>
						{/each}
						<button type="button" class="src-add-slot" onclick={addSlot}>+ Add rider</button>
					</fieldset>
				{:else if editing}
					<p class="src-hint">Riders are managed from the request card ({editing.slots.length} on this request).</p>
				{/if}
			</div>
		{/if}

		{#if error}<p class="src-error">{error}</p>{/if}

		<div class="src-actions">
			<button class="src-btn src-cancel" onclick={onClose}>Cancel</button>
			<button class="src-btn src-submit" disabled={!canSubmit} onclick={submit}>
				{submitting ? 'Saving…' : editing ? 'Save changes' : 'Request SAG'}
			</button>
		</div>
	</div>
</div>

<style>
	.src-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.src {
		width: 100%;
		max-width: 480px;
		max-height: 92vh;
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

	.src-title {
		font-size: 1.05rem;
		font-weight: 700;
	}

	.src-loading {
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}

	.src-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.src-field {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.src-label {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
	}

	.src-field input,
	.src-field textarea {
		min-height: 40px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
	}

	.src-field textarea {
		padding: var(--space-sm);
		resize: vertical;
	}

	.src-tiers {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.src-tier-chip {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		color: var(--color-text);
		font-size: 0.78rem;
		font-weight: 600;
		cursor: pointer;
	}

	.src-tier-chip.active {
		border-color: var(--color-accent);
		background: var(--color-raised-strong);
	}

	.src-slots {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		margin: 0;
	}

	.src-legend {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
		padding: 0 4px;
	}

	.src-slot-row {
		display: grid;
		grid-template-columns: 70px 1fr 1fr auto auto;
		gap: 6px;
		align-items: center;
	}

	.src-slot-row input[type='text'] {
		min-height: 36px;
		padding: 0 8px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		min-width: 0;
	}

	.src-bike {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		font-size: 0.72rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.src-slot-remove {
		min-width: 32px;
		min-height: 32px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.src-slot-remove:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	.src-add-slot {
		align-self: flex-start;
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: 0.8rem;
		cursor: pointer;
		min-height: 32px;
	}

	.src-hint {
		font-size: 0.78rem;
		color: var(--color-text-muted);
	}

	.src-error {
		color: var(--color-error-text);
		font-size: 0.8rem;
	}

	.src-actions {
		display: flex;
		gap: var(--space-sm);
		justify-content: flex-end;
		margin-top: var(--space-xs);
	}

	.src-btn {
		min-height: 44px;
		padding: 0 var(--space-md);
		border-radius: var(--radius-sm);
		font-weight: 700;
		cursor: pointer;
	}

	.src-cancel {
		background: none;
		border: 1px solid var(--color-primary);
		color: var(--color-text);
	}

	.src-submit {
		background: var(--color-accent);
		border: none;
		color: var(--color-on-accent);
	}

	.src-submit:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
