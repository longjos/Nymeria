<script lang="ts">
	// Medical notification composer, in the exact COURSE-plan scripted field
	// order (docs/ride-mode-plan.md WP4 / internal/ride/medical.go's doc
	// comment): bib / sex / age / exact location / chief complaint / readback.
	// ETA, on-scene and departed are separate follow-up actions on the board
	// (MedicalBoard.svelte) — they happen over time as EMS actually responds,
	// not at report time, and on-scene/departed must carry their own,
	// separately-logged timestamps.
	//
	// Medical data is notification-and-logistics only (governing fact 6):
	// there are no clinical fields here beyond chief complaint, and there
	// never should be.
	import { api, ApiError } from '$lib/api';
	import type { MedicalNotification } from '$lib/types';
	import { rideLadder, rideProfile, upsertMedical } from '$lib/stores/ride';
	import { activeCheckIns } from '$lib/stores/netcontrol';
	import { currentUser } from '$lib/stores/session';
	import RideTierGlyph from '../RideTierGlyph.svelte';
	import RideDialog from './RideDialog.svelte';

	let { netId, onClose }: { netId: string; onClose: () => void } = $props();

	type Step = 'fields' | 'readback';
	let step = $state<Step>('fields');

	const sexes = [
		{ id: 'U', label: 'Unknown' },
		{ id: 'M', label: 'M' },
		{ id: 'F', label: 'F' },
		{ id: 'X', label: 'X' }
	];
	const severities = [
		{ id: 'routine', label: 'Routine' },
		{ id: 'serious', label: 'Serious' },
		{ id: 'severe', label: 'Severe' }
	];

	let bib = $state('');
	let sex = $state('U');
	let age = $state('');
	let location = $state('');
	let milesRemaining = $state('');
	let chiefComplaint = $state('');
	let severity = $state('routine');
	let priority = $state($rideLadder.find((t) => t.rank === 2)?.id ?? $rideLadder[0]?.id ?? '');
	let notes = $state('');
	let reportedByCall = $state('');

	let withholdsSevereBib = $derived($rideProfile?.rideConfig?.withholdBibOnSevereInjury ?? false);

	let submitting = $state(false);
	let error = $state<string | null>(null);
	let created = $state<MedicalNotification | null>(null);
	let readBackBy = $state($currentUser?.callsign ?? $currentUser?.name ?? '');
	let correcting = $state(false);
	let correction = $state('');

	let canSubmit = $derived(!!location.trim() && !!chiefComplaint.trim() && !!priority && !submitting);

	async function submitCreate(): Promise<void> {
		if (!canSubmit) return;
		submitting = true;
		error = null;
		try {
			const n = await api.createRideMedical(netId, {
				reportedByCall: reportedByCall.trim(),
				bib: bib.trim() || undefined,
				sex,
				age: age.trim(),
				location: location.trim(),
				milesRemaining: milesRemaining.trim() ? Number(milesRemaining) : undefined,
				chiefComplaint: chiefComplaint.trim(),
				severity,
				priority,
				notes: notes.trim() || undefined
			});
			upsertMedical(n);
			created = n;
			step = 'readback';
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not create the medical notification';
		} finally {
			submitting = false;
		}
	}

	async function confirmReadback(): Promise<void> {
		if (!created) return;
		submitting = true;
		error = null;
		try {
			const updated = await api.medicalReadback(netId, created.id, { confirmed: true, readBackBy });
			upsertMedical(updated);
			onClose();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not confirm the read-back';
		} finally {
			submitting = false;
		}
	}

	async function submitCorrection(): Promise<void> {
		if (!created) return;
		submitting = true;
		error = null;
		try {
			const updated = await api.medicalReadback(netId, created.id, { confirmed: false, correction });
			upsertMedical(updated);
			correcting = false;
			correction = '';
			created = updated;
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not log the correction';
		} finally {
			submitting = false;
		}
	}

	/** The form wrapper inside RideDialog's body. The dialog element itself
	 *  belongs to RideDialog; Escape and the Tab trap come from showModal(). */
	let formEl = $state<HTMLElement | null>(null);
	$effect(() => {
		formEl?.querySelector<HTMLElement>('input, select, button, textarea')?.focus();
	});

	let checkInSuggestions = $derived(Array.from(new Set($activeCheckIns.map((c) => c.tacticalCall || c.callsign).filter(Boolean))));
</script>

<RideDialog title="Medical notification" titleId="mec-title" {onClose} closeDisabled={submitting}>
	{#snippet children()}
		<div class="mec-form" bind:this={formEl}>
			{#if step === 'fields'}
				<label class="mec-field" for="mec-bib">
					<span class="mec-label">1. Bib</span>
					<input id="mec-bib" type="text" bind:value={bib} placeholder="optional / unknown" />
				</label>

				<div class="mec-field">
					<span class="mec-label">2. Sex</span>
					<div class="mec-choices">
						{#each sexes as s (s.id)}
							<button type="button" class="mec-chip" class:active={sex === s.id} onclick={() => (sex = s.id)}>{s.label}</button>
						{/each}
					</div>
				</div>

				<label class="mec-field" for="mec-age">
					<span class="mec-label">3. Age</span>
					<input id="mec-age" type="text" bind:value={age} placeholder="34 / ~40s / unknown" />
				</label>

				<label class="mec-field" for="mec-location">
					<span class="mec-label">4. Exact location</span>
					<input id="mec-location" type="text" bind:value={location} placeholder="mile 44.1, base of the Skyline climb" />
				</label>
				<label class="mec-field" for="mec-miles">
					<span class="mec-label">Miles remaining (on-air, optional)</span>
					<input id="mec-miles" type="number" step="0.1" inputmode="decimal" bind:value={milesRemaining} />
				</label>

				<label class="mec-field" for="mec-complaint">
					<span class="mec-label">5. Chief complaint</span>
					<textarea id="mec-complaint" rows="2" bind:value={chiefComplaint} placeholder="mechanism + presenting complaint — no clinical detail beyond this"></textarea>
				</label>

				<div class="mec-field">
					<span class="mec-label">Severity</span>
					<div class="mec-choices">
						{#each severities as s (s.id)}
							<button type="button" class="mec-chip" class:active={severity === s.id} onclick={() => (severity = s.id)}>{s.label}</button>
						{/each}
					</div>
					{#if severity === 'severe' && withholdsSevereBib}
						<span class="mec-hint">This net withholds the bib on severe injuries — it will be recorded but redacted for observers.</span>
					{/if}
				</div>

				<div class="mec-field">
					<span class="mec-label">Priority</span>
					<div class="mec-choices">
						{#each $rideLadder as tier (tier.id)}
							<button type="button" class="mec-chip" class:active={priority === tier.id} onclick={() => (priority = tier.id)}>
								<RideTierGlyph {tier} size={14} />
								{tier.label}
							</button>
						{/each}
					</div>
				</div>

				<label class="mec-field" for="mec-reportedby">
					<span class="mec-label">Reported by</span>
					<input id="mec-reportedby" type="text" list="mec-checkins" bind:value={reportedByCall} placeholder="tactical call / callsign" />
					<datalist id="mec-checkins">
						{#each checkInSuggestions as c (c)}<option value={c}></option>{/each}
					</datalist>
				</label>

				<label class="mec-field" for="mec-notes">
					<span class="mec-label">Notes</span>
					<textarea id="mec-notes" rows="2" bind:value={notes}></textarea>
				</label>
			{:else if step === 'readback' && created}
				<p class="mec-readback-intro">6. Read back to <strong>{created.reportedByCall || 'the reporting station'}</strong>:</p>
				<p class="mec-readback-text">
					{created.bibWithheld ? 'bib withheld' : created.bib ? `bib ${created.bib}` : 'bib not given'},
					{created.sex}, {created.age || 'age unknown'}, at {created.location}{created.milesRemaining != null ? ` (mi ${created.milesRemaining.toFixed(1)} to go)` : ''}.
					Chief complaint: {created.chiefComplaint}. Priority {created.priority}.
				</p>

				{#if correcting}
					<label class="mec-field" for="mec-correction">
						<span class="mec-label">What did they correct?</span>
						<textarea id="mec-correction" rows="2" bind:value={correction}></textarea>
					</label>
				{/if}
			{/if}
		</div>
	{/snippet}

	{#snippet footer()}
		<!-- Pinned outside the scroller: the read-back is the step that must be
		     committed, and it is at the bottom of the longest form in ride mode. -->
		{#if error}<p class="mec-error mec-error-inline">{error}</p>{/if}
		{#if step === 'fields'}
			<button class="mec-btn mec-cancel" onclick={onClose}>Cancel</button>
			<button class="mec-btn mec-submit" disabled={!canSubmit} aria-busy={submitting} onclick={submitCreate}>{submitting ? 'Sending…' : 'Continue to read-back'}</button>
		{:else if step === 'readback' && created}
			{#if correcting}
				<button class="mec-btn mec-cancel" onclick={() => (correcting = false)}>Back</button>
				<button class="mec-btn mec-submit" disabled={submitting} aria-busy={submitting} onclick={submitCorrection}>{submitting ? 'Logging…' : 'Log correction'}</button>
			{:else}
				<button class="mec-btn mec-cancel" onclick={() => (correcting = true)}>They corrected something</button>
				<button class="mec-btn mec-submit" disabled={submitting} aria-busy={submitting} onclick={confirmReadback}>{submitting ? 'Transmitting…' : 'Confirmed — transmit'}</button>
			{/if}
		{/if}
	{/snippet}
</RideDialog>

<style>
	/* The box, the title, the backdrop and the scroll belong to RideDialog. */
	.mec-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.mec-field {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.mec-label {
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
	}

	.mec-field input,
	.mec-field textarea {
		min-height: 40px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
	}

	.mec-field textarea {
		padding: var(--space-sm);
		resize: vertical;
	}

	.mec-choices {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-sm);
	}

	.mec-chip {
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

	.mec-chip.active {
		border-color: var(--color-accent);
		background: var(--color-raised-strong);
	}

	.mec-hint {
		font-size: var(--ride-t-label);
		color: var(--color-warning);
	}

	.mec-readback-intro {
		font-size: var(--ride-t-body);
		color: var(--color-text-muted);
	}

	.mec-readback-text {
		font-size: 0.95rem;
		padding: var(--space-sm);
		background: var(--color-bg);
		border-radius: var(--radius-sm);
	}

	.mec-error {
		color: var(--color-error-text);
		font-size: var(--ride-t-body);
	}

	/* Keeps a failed submit on the same line as the buttons, at the left. */
	.mec-error-inline {
		margin-right: auto;
		flex: 1 1 12ch;
	}

	.mec-btn {
		min-height: 44px;
		padding: 0 var(--space-md);
		border-radius: var(--radius-sm);
		font-weight: 700;
		cursor: pointer;
	}

	.mec-cancel {
		background: none;
		border: 1px solid var(--color-primary);
		color: var(--color-text);
	}

	.mec-submit {
		background: var(--color-accent);
		border: none;
		color: var(--color-on-accent);
	}

	.mec-submit:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
</style>
