<script lang="ts">
	// Medical notification board: everything after the composer's read-back —
	// ETA, on-scene, departed (destination + patient count), release, cancel.
	// On-scene and departed are separate actions with separate timestamps,
	// never combined into one "EMS handled it" button.
	//
	// $medicalOpen already comes from the backend Redacted() (governing fact
	// 6 / GET ?status=open is always redacted) — PatientName is never present
	// and Bib may be blanked. Both are handled as the normal case, not an
	// error: a bib-less, name-less row renders exactly like any other.
	import { api, ApiError } from '$lib/api';
	import type { MedicalNotification } from '$lib/types';
	import { medicalOpen, rideLadder, upsertMedical } from '$lib/stores/ride';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { canOperate } from '$lib/stores/session';
	import { secondClock } from '$lib/stores/clock';
	import { showToast } from '$lib/stores/toast';
	import { tierById, ageText, MEDICAL_STATUS_LABELS, MEDICAL_DESTINATION_LABELS } from '$lib/rideMeta';
	import RideTierGlyph from '../RideTierGlyph.svelte';
	import MedicalComposer from './MedicalComposer.svelte';
	import ReasonDialog from '../ReasonDialog.svelte';

	let netId = $derived($activeNetId);
	let composerOpen = $state(false);

	let requests = $derived([...$medicalOpen].sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt)));

	function nameAllowedDest(dest: string): boolean {
		return dest === 'hospital' || dest === 'start';
	}

	// --- readback resumption (a "reported" notification whose composer
	// dialog was closed before confirming) ---

	let correctingId = $state<string | null>(null);
	let correctionText = $state('');

	async function confirmReadback(n: MedicalNotification): Promise<void> {
		try {
			upsertMedical(await api.medicalReadback(netId, n.id, { confirmed: true }));
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not confirm read-back', 'error');
		}
	}

	async function submitCorrection(n: MedicalNotification): Promise<void> {
		try {
			upsertMedical(await api.medicalReadback(netId, n.id, { confirmed: false, correction: correctionText }));
			correctingId = null;
			correctionText = '';
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not log correction', 'error');
		}
	}

	// --- ETA ---

	let etaOpenFor = $state<string | null>(null);
	let etaUnit = $state('');
	let etaMinutes = $state('');

	async function submitEta(n: MedicalNotification): Promise<void> {
		const minutes = Number(etaMinutes);
		if (!minutes || minutes <= 0) return;
		try {
			upsertMedical(await api.etaRideMedical(netId, n.id, { emsUnit: etaUnit.trim(), minutes }));
			etaOpenFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not record ETA', 'error');
		}
	}

	// --- on scene ---

	let onSceneOpenFor = $state<string | null>(null);
	let onSceneUnit = $state('');
	let onSceneMinutesAgo = $state('');

	function minutesAgoToIso(minutesAgo: string): string | undefined {
		const m = Number(minutesAgo);
		if (!m || m <= 0) return undefined;
		return new Date(Date.now() - m * 60000).toISOString();
	}

	async function submitOnScene(n: MedicalNotification): Promise<void> {
		try {
			upsertMedical(await api.onSceneRideMedical(netId, n.id, { emsUnit: onSceneUnit.trim() || undefined, at: minutesAgoToIso(onSceneMinutesAgo) }));
			onSceneOpenFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not record on-scene', 'error');
		}
	}

	// --- departed ---

	let departOpenFor = $state<string | null>(null);
	let departDest = $state('hospital');
	let departDestName = $state('');
	let departCount = $state('1');
	let departPatientName = $state('');
	let departMinutesAgo = $state('');

	function openDepart(id: string): void {
		departOpenFor = id;
		departDest = 'hospital';
		departDestName = '';
		departCount = '1';
		departPatientName = '';
		departMinutesAgo = '';
	}

	async function submitDepart(n: MedicalNotification): Promise<void> {
		const count = Number(departCount) || 1;
		try {
			const updated = await api.departRideMedical(netId, n.id, {
				destination: departDest,
				destinationName: departDestName.trim(),
				patientCount: count,
				patientName: nameAllowedDest(departDest) ? departPatientName.trim() || undefined : undefined,
				at: minutesAgoToIso(departMinutesAgo)
			});
			upsertMedical(updated);
			departOpenFor = null;
			showToast('Departure logged', 'success');
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not record departure', 'error');
		}
	}

	// --- release (treated on scene, no transport) ---

	let releaseOpenFor = $state<string | null>(null);
	let releaseReason = $state('');

	async function submitRelease(n: MedicalNotification): Promise<void> {
		try {
			upsertMedical(await api.releaseRideMedical(netId, n.id, { reason: releaseReason.trim() }));
			releaseOpenFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not release', 'error');
		}
	}

	// The cancel reason is written to the net timeline and the ICS-214
	// export, so it is captured in a real dialog with a pending state and an
	// inline error, not window.prompt().
	let cancelTarget = $state<MedicalNotification | null>(null);

	async function submitCancel(reason: string): Promise<void> {
		const n = cancelTarget;
		if (!n) return;
		try {
			upsertMedical(await api.cancelRideMedical(netId, n.id, { reason }));
			cancelTarget = null;
			showToast('Medical notification cancelled', 'success');
		} catch (e) {
			throw new Error(e instanceof ApiError ? e.message : 'Could not cancel');
		}
	}
</script>

<div class="med">
	<div class="med-toolbar">
		<button class="med-new" onclick={() => (composerOpen = true)} disabled={!$canOperate}>+ New medical notification</button>
	</div>

	{#if requests.length === 0}
		<p class="med-empty">No open medical notifications.</p>
	{/if}

	<div class="med-list">
		{#each requests as n (n.id)}
			{@const tier = tierById($rideLadder, n.priority)}
			<div class="med-card">
				<div class="med-row">
					{#if tier}<RideTierGlyph {tier} size={16} title={tier.label} />{/if}
					<span class="med-status" data-status={n.status}>{MEDICAL_STATUS_LABELS[n.status] ?? n.status}</span>
					<span class="med-who">{n.bibWithheld ? 'bib withheld' : n.bib ? `Bib ${n.bib}` : 'no bib'} · {n.sex} · {n.age || 'age ?'}</span>
					<span class="med-age" title={new Date(n.createdAt).toLocaleString()}>{ageText(n.createdAt, $secondClock)} old</span>
				</div>
				<p class="med-location">{n.location}{n.milesRemaining != null ? ` · mi ${n.milesRemaining.toFixed(1)} to go` : ''}</p>
				<p class="med-complaint">{n.chiefComplaint}</p>
				{#if n.emsUnit}<p class="med-ems">{n.emsUnit}{n.etaMinutes != null ? ` · ETA ${n.etaMinutes}m given ${ageText(n.etaGivenAt, $secondClock)} ago` : ''}{n.onSceneAt ? ` · on scene ${ageText(n.onSceneAt, $secondClock)} ago` : ''}</p>{/if}
				{#if n.notes}<p class="med-notes">{n.notes}</p>{/if}

				{#if $canOperate}
					<div class="med-actions">
						{#if n.status === 'reported'}
							{#if correctingId === n.id}
								<textarea class="med-correction" rows="2" bind:value={correctionText} placeholder="what did they correct?"></textarea>
								<button class="med-btn" onclick={() => submitCorrection(n)}>Log correction</button>
								<button class="med-btn" onclick={() => (correctingId = null)}>Cancel</button>
							{:else}
								<button class="med-btn" onclick={() => confirmReadback(n)}>Confirm read-back</button>
								<button class="med-btn" onclick={() => (correctingId = n.id)}>They corrected something</button>
							{/if}
						{:else if n.status === 'confirmed' || n.status === 'ems_enroute'}
							{#if etaOpenFor === n.id}
								<input class="med-input med-input-narrow" type="text" placeholder="unit" bind:value={etaUnit} />
								<input class="med-input med-input-narrow" type="number" min="1" placeholder="ETA min" bind:value={etaMinutes} />
								<button class="med-btn" onclick={() => submitEta(n)}>Record ETA</button>
								<button class="med-btn" onclick={() => (etaOpenFor = null)}>Cancel</button>
							{:else}
								<button class="med-btn" onclick={() => { etaOpenFor = n.id; etaUnit = n.emsUnit ?? ''; etaMinutes = ''; }}>Record ETA</button>
							{/if}
							{#if onSceneOpenFor === n.id}
								<input class="med-input med-input-narrow" type="text" placeholder="unit" bind:value={onSceneUnit} />
								<input class="med-input med-input-narrow" type="number" min="0" placeholder="min ago" bind:value={onSceneMinutesAgo} />
								<button class="med-btn" onclick={() => submitOnScene(n)}>Confirm on scene</button>
								<button class="med-btn" onclick={() => (onSceneOpenFor = null)}>Cancel</button>
							{:else}
								<button class="med-btn" onclick={() => { onSceneOpenFor = n.id; onSceneUnit = n.emsUnit ?? ''; onSceneMinutesAgo = ''; }}>On scene</button>
							{/if}
						{:else if n.status === 'on_scene'}
							{#if departOpenFor === n.id}
								<div class="med-inline-form">
									<select class="med-select" bind:value={departDest}>
										{#each Object.entries(MEDICAL_DESTINATION_LABELS) as [id, label] (id)}
											<option value={id}>{label}</option>
										{/each}
									</select>
									<input class="med-input" type="text" placeholder="destination name" bind:value={departDestName} />
									<input class="med-input med-input-narrow" type="number" min="1" placeholder="patients" bind:value={departCount} />
									{#if nameAllowedDest(departDest)}
										<input class="med-input" type="text" placeholder="patient name (hospital/start only)" bind:value={departPatientName} />
									{/if}
									<input class="med-input med-input-narrow" type="number" min="0" placeholder="min ago" bind:value={departMinutesAgo} />
									<div class="med-inline-actions">
										<button class="med-btn" onclick={() => submitDepart(n)}>Confirm departed</button>
										<button class="med-btn" onclick={() => (departOpenFor = null)}>Cancel</button>
									</div>
								</div>
							{:else}
								<button class="med-btn" onclick={() => openDepart(n.id)}>Departed…</button>
							{/if}
							{#if releaseOpenFor === n.id}
								<input class="med-input" type="text" placeholder="reason (treated and released)" bind:value={releaseReason} />
								<button class="med-btn" onclick={() => submitRelease(n)}>Confirm release</button>
								<button class="med-btn" onclick={() => (releaseOpenFor = null)}>Cancel</button>
							{:else}
								<button class="med-btn" onclick={() => { releaseOpenFor = n.id; releaseReason = ''; }}>Released on scene…</button>
							{/if}
						{/if}
						<button class="med-btn med-btn-danger" onclick={() => (cancelTarget = n)}>Cancel notification</button>
					</div>
				{/if}
			</div>
		{/each}
	</div>
</div>

{#if composerOpen}
	<MedicalComposer {netId} onClose={() => (composerOpen = false)} />
{/if}

{#if cancelTarget}
	<ReasonDialog
		title="Cancel medical notification"
		confirmLabel="Cancel notification"
		danger
		onConfirm={submitCancel}
		onCancel={() => (cancelTarget = null)}
	/>
{/if}

<style>
	.med {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		padding: var(--space-md);
	}

	.med-toolbar {
		display: flex;
		justify-content: flex-start;
	}

	.med-new {
		min-height: 40px;
		padding: 0 var(--space-md);
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-on-accent);
		font-weight: 700;
		cursor: pointer;
	}

	.med-new:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.med-empty {
		color: var(--color-text-muted);
		font-size: 0.82rem;
		padding: var(--space-sm) 0;
	}

	.med-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.med-card {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.med-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		font-size: 0.8rem;
	}

	.med-status {
		font-size: 0.68rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	.med-who {
		flex: 1;
		min-width: 0;
		font-weight: 700;
	}

	.med-age {
		color: var(--color-text-muted);
		font-size: 0.72rem;
		white-space: nowrap;
	}

	.med-location {
		font-size: 0.82rem;
	}

	.med-complaint {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}

	.med-ems {
		font-size: 0.78rem;
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
	}

	.med-notes {
		font-size: 0.78rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	.med-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin-top: 4px;
		align-items: center;
	}

	.med-btn {
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
	}

	.med-btn-danger {
		color: var(--color-error-text);
		border-color: var(--color-error);
	}

	.med-input {
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		font-size: 0.78rem;
	}

	.med-input-narrow {
		width: 80px;
	}

	.med-select {
		min-height: 36px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		font-size: 0.78rem;
	}

	.med-inline-form {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
		width: 100%;
	}

	.med-inline-actions {
		display: flex;
		gap: 6px;
	}

	.med-correction {
		width: 100%;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		padding: var(--space-sm);
		resize: vertical;
	}
</style>
