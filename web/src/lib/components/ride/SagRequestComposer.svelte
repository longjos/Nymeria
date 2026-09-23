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
	import { tierById, SAG_BIKE_LABELS, SAG_BIKE_GLYPHS, SAG_REASON_LABELS, enumLabel, BIKE_ORDER } from '$lib/rideMeta';
	import { showToast } from '$lib/stores/toast';
	import RideTierGlyph from '../RideTierGlyph.svelte';
	import RideDialog from './RideDialog.svelte';
	import SagLocationField from './SagLocationField.svelte';

	let {
		netId,
		editing = null,
		initialReason = '',
		focusField = null,
		onClose
	}: {
		netId: string;
		editing?: SAGRequest | null;
		initialReason?: string;
		/** Put the caret straight in the pickup mile marker — the SAG map dock's
		 *  "Add mile" path, where the caller is still on the air saying a number. */
		focusField?: 'pickupMile' | null;
		onClose: () => void;
	} = $props();

	interface SlotRow {
		bib: string;
		riderName: string;
		note: string;
		bike: string;
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
	let slots = $state<SlotRow[]>([{ bib: '', riderName: '', note: '', bike: 'with_rider' }]);

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
		slots = [...slots, { bib: '', riderName: '', note: '', bike: 'with_rider' }];
		// Bibs arrive at pencil speed, one after another: "334, 335, and 512."
		// Typing the next one must never need the mouse.
		queueMicrotask(() => {
			const inputs = formEl?.querySelectorAll<HTMLInputElement>('.src-slot-row input[type="text"]');
			inputs?.[(slots.length - 1) * 3]?.focus();
		});
	}

	/** Enter in a bib field opens the next rider row — the on-air rhythm. */
	function bibKeydown(e: KeyboardEvent, i: number): void {
		if (e.key !== 'Enter') return;
		e.preventDefault();
		if (i === slots.length - 1) addSlot();
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
						slots: slots.map((s) => ({ bib: s.bib.trim(), riderName: s.riderName.trim(), note: s.note.trim(), bike: s.bike }))
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

	/** The form wrapper inside RideDialog's body — the scope for the focus
	 *  query below. The dialog element itself belongs to RideDialog. */
	let formEl = $state<HTMLElement | null>(null);
	/**
	 * Focus the FIRST BIB, once the body actually exists. The old version ran
	 * while `config` was still null — the only focusable thing in the dialog
	 * at that moment was the Cancel button, so every new request opened with
	 * focus on Cancel and the operator's first keystroke went nowhere.
	 */
	let focused = $state(false);
	$effect(() => {
		if (!formEl || !config || focused) return;
		const target =
			(focusField === 'pickupMile'
				? formEl.querySelector<HTMLElement>('#src-pickup-mile')
				: null) ??
			formEl.querySelector<HTMLElement>('.src-slot-row input[type="text"]') ??
			formEl.querySelector<HTMLElement>('input, select, textarea');
		target?.focus();
		// The dock opens this straight off a radio call, so the operator types a
		// number immediately — select what is there rather than making them
		// clear it first.
		(target as HTMLInputElement | null)?.select?.();
		focused = true;
	});

	// No Tab trap and no Escape handler here: RideDialog opens a native modal
	// with showModal(), which supplies both, plus inertness and the scroll lock.

	let checkInSuggestions = $derived(
		Array.from(new Set($activeCheckIns.map((c) => c.tacticalCall || c.callsign).filter(Boolean)))
	);
</script>

<RideDialog
	title={editing ? `Edit SAG ${editing.sequence}` : 'New SAG request'}
	titleId="src-title"
	{onClose}
	closeDisabled={submitting}
>
	{#snippet children()}
		<div class="src-form" bind:this={formEl}>
			{#if configError}
				<p class="src-error">{configError}</p>
			{:else if !config}
				<p class="src-loading">Loading…</p>
			{:else}
				{#if !editing}
					<!-- RIDERS FIRST. Bibs are passed at pencil speed, in the first
					     breath of the call ("SAG at Maxwell, bib 334, flat"); burying
					     them under six location fields made the operator scroll to
					     reach the one thing being dictated. Enter opens the next row. -->
					<fieldset class="src-slots">
						<legend class="src-legend">Riders ({slots.length})</legend>
						{#each slots as slot, i (i)}
							<div class="src-slot-row">
								<input type="text" placeholder="bib" bind:value={slot.bib} aria-label="Bib for rider {i + 1}" onkeydown={(e) => bibKeydown(e, i)} />
								<input type="text" placeholder="name (hospital/start only)" bind:value={slot.riderName} aria-label="Name for rider {i + 1}" />
								<input type="text" placeholder="note" bind:value={slot.note} aria-label="Note for rider {i + 1}" />
								<select class="src-bike-select" bind:value={slot.bike} aria-label="Bike for rider {i + 1}">
									{#each BIKE_ORDER as b (b)}
										<option value={b}>{SAG_BIKE_GLYPHS[b]} {SAG_BIKE_LABELS[b]}</option>
									{/each}
								</select>
								<button type="button" class="src-slot-remove" onclick={() => removeSlot(i)} disabled={slots.length <= 1} aria-label="Remove rider {i + 1}">×</button>
							</div>
						{/each}
						<button type="button" class="src-add-slot" onclick={addSlot}>+ Add rider</button>
						<p class="src-hint">The bike answer here is the caller's; the driver confirms it at Load.</p>
					</fieldset>
				{:else}
					<p class="src-hint">Riders are managed from the request card ({editing.slots.length} on this request).</p>
				{/if}

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

				<div class="src-field">
					<span class="src-label" id="src-reason-label">Reason</span>
					<!-- One click, not seven keystrokes, for the six things it almost
					     always is. All three of the operator's real requests were
					     logged with an EMPTY reason: a free-text box with a datalist
					     is a box you skip when somebody is talking. -->
					<div class="src-tiers" role="group" aria-labelledby="src-reason-label">
						{#each config.config.reasons as rsn (rsn)}
							<button type="button" class="src-tier-chip" class:active={reason === rsn} onclick={() => (reason = reason === rsn ? '' : rsn)}>
								{enumLabel(rsn, SAG_REASON_LABELS)}
							</button>
						{/each}
					</div>
					<input id="src-reason" type="text" bind:value={reason} placeholder="or type it" aria-label="Reason" />
				</div>

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
			{/if}
		</div>
	{/snippet}

	{#snippet footer()}
		<!-- The error lives in the footer, not at the end of the scrolling body:
		     a submit failure on a long form would otherwise land off-screen,
		     below the fold the operator is not looking at. -->
		{#if error}<p class="src-error src-error-inline">{error}</p>{/if}
		<button class="src-btn src-cancel" onclick={onClose} disabled={submitting}>Cancel</button>
		<button class="src-btn src-submit" disabled={!canSubmit} onclick={submit}>
			{submitting ? 'Saving…' : editing ? 'Save changes' : 'Request SAG'}
		</button>
	{/snippet}
</RideDialog>

<style>
	/* The box, the title, the backdrop and the scroll now belong to
	   RideDialog. What is left here is only this form's own content. */
	.src-loading {
		color: var(--color-text-muted);
		font-size: var(--ride-t-body);
	}

	.src-form {
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
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
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
		gap: var(--space-sm);
	}

	.src-tier-chip {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		color: var(--color-text);
		font-size: var(--ride-t-body);
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
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
		padding: 0 var(--space-xs);
	}

	.src-slot-row {
		display: grid;
		grid-template-columns: 70px 1fr 1fr minmax(0, 140px) auto;
		gap: var(--space-sm);
		align-items: center;
	}

	.src-bike-select {
		min-height: 36px;
		min-width: 0;
		padding: 0 var(--space-xs);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: var(--ride-t-label);
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
		opacity: 0.45;
		cursor: not-allowed;
	}

	.src-add-slot {
		align-self: flex-start;
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: var(--ride-t-body);
		cursor: pointer;
		min-height: 32px;
	}

	.src-hint {
		font-size: var(--ride-t-body);
		color: var(--color-text-muted);
	}

	.src-error {
		color: var(--color-error-text);
		font-size: var(--ride-t-body);
	}

	/* Pushes the buttons to the right edge and keeps the message on the left,
	   on the one line, where a failed submit is impossible to miss. */
	.src-error-inline {
		margin-right: auto;
		flex: 1 1 12ch;
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
		opacity: 0.45;
		cursor: not-allowed;
	}
</style>
