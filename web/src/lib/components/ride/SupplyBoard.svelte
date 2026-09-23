<script lang="ts">
	// Supply request board: the ladder from draft -> confirmed -> relayed ->
	// en_route -> delivered, plus the elapsed-time readout the doctrine calls
	// for explicitly ("ice truck said 20 min — 34 min ago"). A lingering
	// draft (composer closed before read-back finished) is resumable here,
	// not stranded.
	import { api, ApiError } from '$lib/api';
	import type { SupplyRequest } from '$lib/types';
	import { supplyAll, rideLadder, upsertSupply } from '$lib/stores/ride';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { canOperate, currentUser } from '$lib/stores/session';
	import { secondClock } from '$lib/stores/clock';
	import { showToast } from '$lib/stores/toast';
	import { tierById, ageText, SUPPLY_STATUS_LABELS, SUPPLY_TERMINAL_STATUSES } from '$lib/rideMeta';
	import RideTierGlyph from '../RideTierGlyph.svelte';
	import SupplyComposer from './SupplyComposer.svelte';
	import ReasonDialog from '../ReasonDialog.svelte';

	let netId = $derived($activeNetId);
	let showAll = $state(false);
	let composerOpen = $state(false);

	let requests = $derived(
		$supplyAll
			.filter((r) => showAll || !SUPPLY_TERMINAL_STATUSES.has(r.status))
			.sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
	);

	function formatItem(it: { item: string; quantity: number; unit?: string }): string {
		if (it.quantity > 0) return `${it.quantity}${it.unit ? ` ${it.unit}` : ''} ${it.item}`;
		return it.item;
	}

	function itemsSummary(r: SupplyRequest): string {
		return r.items.map(formatItem).join(', ') || '—';
	}

	/** "ETA 20m given 14m ago — due in 6m" / "…— overdue 9m". Never renders a
	 * bare number without saying how long ago it was GIVEN — that gap is the
	 * whole point of keeping ETA history. */
	function etaLine(r: SupplyRequest, now: number): string | null {
		if (r.etas.length === 0) return null;
		const last = r.etas[r.etas.length - 1];
		const givenAgo = ageText(last.givenAt, now);
		const dueMs = Date.parse(last.dueAt) - now;
		const dueText = dueMs >= 0 ? `due in ${Math.round(dueMs / 60000)}m` : `overdue ${Math.round(-dueMs / 60000)}m`;
		return `ETA ${last.minutes}m given ${givenAgo} ago — ${dueText}`;
	}

	function etaOverdue(r: SupplyRequest, now: number): boolean {
		if (r.etas.length === 0) return false;
		return Date.parse(r.etas[r.etas.length - 1].dueAt) < now;
	}

	// --- inline actions ---

	let readBackBy = $derived($currentUser?.callsign ?? $currentUser?.name ?? '');
	let correctingId = $state<string | null>(null);
	let correctionText = $state('');

	async function confirmReadback(r: SupplyRequest): Promise<void> {
		try {
			upsertSupply(await api.readbackRideSupply(netId, r.id, { confirmed: true, readBackBy }));
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not confirm read-back', 'error');
		}
	}

	async function submitCorrection(r: SupplyRequest): Promise<void> {
		try {
			upsertSupply(await api.readbackRideSupply(netId, r.id, { confirmed: false, correction: correctionText }));
			correctingId = null;
			correctionText = '';
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not log correction', 'error');
		}
	}

	let relayOpenFor = $state<string | null>(null);
	let relayTo = $state('');

	async function submitRelay(r: SupplyRequest): Promise<void> {
		try {
			upsertSupply(await api.relayRideSupply(netId, r.id, { relayedTo: relayTo.trim() }));
			relayOpenFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not relay request', 'error');
		}
	}

	let etaOpenFor = $state<string | null>(null);
	let etaMinutes = $state('');
	let etaSource = $state('');

	async function submitEta(r: SupplyRequest): Promise<void> {
		const minutes = Number(etaMinutes);
		if (!minutes || minutes <= 0) return;
		try {
			upsertSupply(await api.etaRideSupply(netId, r.id, { minutes, source: etaSource.trim() || undefined }));
			etaOpenFor = null;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not record ETA', 'error');
		}
	}

	async function deliver(r: SupplyRequest): Promise<void> {
		try {
			upsertSupply(await api.deliverRideSupply(netId, r.id));
			showToast(`Supply delivered to ${r.requestedByCall}`, 'success');
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not record delivery', 'error');
		}
	}

	// Reason goes to the timeline + ICS-214; captured in a real dialog.
	let cancelTarget = $state<SupplyRequest | null>(null);

	async function submitCancel(reason: string): Promise<void> {
		const r = cancelTarget;
		if (!r) return;
		try {
			upsertSupply(await api.cancelRideSupply(netId, r.id, { reason }));
			cancelTarget = null;
			showToast('Supply request cancelled', 'success');
		} catch (e) {
			throw new Error(e instanceof ApiError ? e.message : 'Could not cancel request');
		}
	}
</script>

<div class="sup">
	<div class="sup-toolbar">
		<button class="sup-new" onclick={() => (composerOpen = true)} disabled={!$canOperate}>+ New supply request</button>
		<label class="sup-showall"><input type="checkbox" bind:checked={showAll} /> Show delivered/cancelled</label>
	</div>

	{#if requests.length === 0}
		<p class="sup-empty">No {showAll ? '' : 'open '}supply requests.</p>
	{/if}

	<div class="sup-list">
		{#each requests as r (r.id)}
			{@const tier = tierById($rideLadder, r.priority)}
			<div class="sup-card">
				<div class="sup-row">
					{#if tier}<RideTierGlyph {tier} size={16} title={tier.label} />{/if}
					<span class="sup-status" data-status={r.status}>{SUPPLY_STATUS_LABELS[r.status] ?? r.status}</span>
					<span class="sup-from">{r.requestedByCall}</span>
					<span class="sup-loc">{r.location}</span>
					<span class="sup-age" title={new Date(r.createdAt).toLocaleString()}>{ageText(r.createdAt, $secondClock)} old</span>
				</div>
				<p class="sup-items">{itemsSummary(r)}</p>
				{#if r.notes}<p class="sup-notes">{r.notes}</p>{/if}
				{#if etaLine(r, $secondClock)}
					<p class="sup-eta" class:overdue={etaOverdue(r, $secondClock)}>{etaLine(r, $secondClock)}</p>
				{/if}

				{#if $canOperate}
					<div class="sup-actions">
						{#if r.status === 'draft'}
							{#if correctingId === r.id}
								<textarea class="sup-correction" rows="2" bind:value={correctionText} placeholder="what did they correct?"></textarea>
								<button class="sup-btn" onclick={() => submitCorrection(r)}>Log correction</button>
								<button class="sup-btn" onclick={() => (correctingId = null)}>Cancel</button>
							{:else}
								<button class="sup-btn" onclick={() => confirmReadback(r)}>Confirm read-back</button>
								<button class="sup-btn" onclick={() => (correctingId = r.id)}>They corrected something</button>
							{/if}
						{:else if r.status === 'confirmed'}
							{#if relayOpenFor === r.id}
								<input class="sup-inline-input" type="text" placeholder="relayed to…" bind:value={relayTo} />
								<button class="sup-btn" onclick={() => submitRelay(r)} disabled={!relayTo.trim()}>Relay</button>
								<button class="sup-btn" onclick={() => (relayOpenFor = null)}>Cancel</button>
							{:else}
								<button class="sup-btn" onclick={() => { relayOpenFor = r.id; relayTo = ''; }}>Relay to…</button>
							{/if}
						{:else if r.status === 'relayed' || r.status === 'en_route'}
							{#if etaOpenFor === r.id}
								<input class="sup-inline-input sup-inline-narrow" type="number" min="1" placeholder="minutes" bind:value={etaMinutes} />
								<input class="sup-inline-input" type="text" placeholder="source (optional)" bind:value={etaSource} />
								<button class="sup-btn" onclick={() => submitEta(r)}>Record ETA</button>
								<button class="sup-btn" onclick={() => (etaOpenFor = null)}>Cancel</button>
							{:else}
								<button class="sup-btn" onclick={() => { etaOpenFor = r.id; etaMinutes = ''; etaSource = ''; }}>Record ETA</button>
							{/if}
							<button class="sup-btn" onclick={() => deliver(r)}>Delivered</button>
						{/if}
						{#if !SUPPLY_TERMINAL_STATUSES.has(r.status)}
							<button class="sup-btn sup-btn-danger" onclick={() => (cancelTarget = r)}>Cancel request</button>
						{/if}
					</div>
				{/if}
			</div>
		{/each}
	</div>
</div>

{#if composerOpen}
	<SupplyComposer {netId} onClose={() => (composerOpen = false)} />
{/if}

{#if cancelTarget}
	<ReasonDialog
		title="Cancel supply request"
		confirmLabel="Cancel request"
		danger
		onConfirm={submitCancel}
		onCancel={() => (cancelTarget = null)}
	/>
{/if}

<style>
	.sup {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		padding: var(--space-md);
	}

	.sup-toolbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
	}

	.sup-new {
		min-height: 44px;
		padding: 0 var(--space-md);
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-on-accent);
		font-weight: 700;
		cursor: pointer;
	}

	.sup-new:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.sup-showall {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		font-size: var(--ride-t-body);
		color: var(--color-text-muted);
	}

	.sup-empty {
		color: var(--color-text-muted);
		font-size: var(--ride-t-body);
		padding: var(--space-sm) 0;
	}

	.sup-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.sup-card {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.sup-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		font-size: var(--ride-t-body);
		min-height: 44px;
	}

	.sup-status {
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	.sup-status[data-status='delivered'] {
		color: var(--color-success);
	}

	.sup-status[data-status='cancelled'],
	.sup-status[data-status='merged'] {
		color: var(--color-text-muted);
		text-decoration: line-through;
	}

	.sup-from {
		font-weight: 700;
	}

	.sup-loc {
		flex: 1;
		min-width: 0;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sup-age {
		color: var(--color-text-muted);
		font-size: var(--ride-t-label);
		white-space: nowrap;
	}

	.sup-items {
		font-size: var(--ride-t-body);
	}

	.sup-notes {
		font-size: var(--ride-t-body);
		color: var(--color-text-muted);
		font-style: italic;
	}

	.sup-eta {
		font-size: var(--ride-t-body);
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
	}

	.sup-eta.overdue {
		color: var(--color-warning);
	}

	.sup-actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-sm);
	}

	.sup-btn {
		min-height: 44px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: var(--ride-t-label);
		font-weight: 600;
		cursor: pointer;
	}

	.sup-btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.sup-btn-danger {
		color: var(--color-error-text);
		border-color: var(--color-error);
	}

	.sup-inline-input {
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		font-size: var(--ride-t-body);
	}

	.sup-inline-narrow {
		width: 80px;
	}

	.sup-correction {
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
