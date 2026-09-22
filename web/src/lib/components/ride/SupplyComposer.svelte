<script lang="ts">
	// Supply request composer. Published doctrine (docs/ride-mode-plan.md WP4):
	// ask what ELSE they're running low on and combine into one request, then
	// hold a read-back confirmation before anything counts as transmitted.
	// CreateSupplyRequest always lands as a draft (internal/ride/supply.go) —
	// the read-back step is a required second call, never implicit.
	import { onMount } from 'svelte';
	import { api, ApiError } from '$lib/api';
	import type { SupplyCatalogEntry, SupplyItemInput, SupplyRequest } from '$lib/types';
	import { rideLadder, upsertSupply } from '$lib/stores/ride';
	import { activeCheckIns } from '$lib/stores/netcontrol';
	import { currentUser } from '$lib/stores/session';
	import { tierById } from '$lib/rideMeta';
	import { showToast } from '$lib/stores/toast';
	import RideTierGlyph from '../RideTierGlyph.svelte';

	let { netId, onClose }: { netId: string; onClose: () => void } = $props();

	interface ItemRow {
		item: string;
		quantity: string;
		unit: string;
		note: string;
	}

	// 'append' is the post-correction state: the original request already
	// exists as a draft, so further item changes go through AddSupplyItems
	// (append-only — there is no "replace items" endpoint), never a second
	// create.
	type Step = 'items' | 'append' | 'readback';
	let step = $state<Step>('items');

	let catalog = $state<SupplyCatalogEntry[]>([]);
	onMount(async () => {
		try {
			catalog = await api.supplyCatalog(netId);
		} catch {
			// suggestions only — a blank list still works, it just loses autofill
		}
	});

	let location = $state('');
	let milesRemaining = $state('');
	let requestedByCall = $state('');
	let priority = $state('');
	let notes = $state('');
	let items = $state<ItemRow[]>([{ item: '', quantity: '', unit: '', note: '' }]);
	let askedWhatElse = $state(false);

	let submitting = $state(false);
	let error = $state<string | null>(null);
	let created = $state<SupplyRequest | null>(null);

	let readBackBy = $state($currentUser?.callsign ?? $currentUser?.name ?? '');
	let correction = $state('');
	let correcting = $state(false);

	function addItem(fromPrompt: boolean): void {
		items = [...items, { item: '', quantity: '', unit: '', note: '' }];
		if (fromPrompt) askedWhatElse = true;
	}

	function removeItem(i: number): void {
		if (items.length <= 1) return;
		items = items.filter((_, idx) => idx !== i);
	}

	function applyCatalogEntry(i: number, name: string): void {
		const entry = catalog.find((c) => c.item === name);
		items[i].item = name;
		if (entry && !priority) priority = entry.defaultTier;
	}

	let itemsForSubmit = $derived<SupplyItemInput[]>(
		items.filter((r) => r.item.trim()).map((r) => ({ item: r.item.trim(), quantity: r.quantity ? Number(r.quantity) : 0, unit: r.unit.trim() || undefined, note: r.note.trim() || undefined }))
	);

	let canSubmitItems = $derived(itemsForSubmit.length > 0 && !!location.trim() && !!requestedByCall.trim() && !!priority && !submitting);

	async function submitCreate(): Promise<void> {
		if (!canSubmitItems) return;
		submitting = true;
		error = null;
		try {
			const req = await api.createRideSupply(netId, {
				requestedByCall: requestedByCall.trim(),
				location: location.trim(),
				milesRemaining: milesRemaining.trim() ? Number(milesRemaining) : undefined,
				items: itemsForSubmit,
				askedWhatElse,
				priority,
				notes: notes.trim() || undefined
			});
			upsertSupply(req);
			created = req;
			step = 'readback';
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not create the supply request';
		} finally {
			submitting = false;
		}
	}

	async function confirmReadback(): Promise<void> {
		if (!created) return;
		submitting = true;
		error = null;
		try {
			const updated = await api.readbackRideSupply(netId, created.id, { confirmed: true, readBackBy });
			upsertSupply(updated);
			showToast(`Supply request from ${updated.requestedByCall} confirmed`, 'success');
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
			const updated = await api.readbackRideSupply(netId, created.id, { confirmed: false, correction });
			upsertSupply(updated);
			correcting = false;
			correction = '';
			created = updated;
			// Fresh, EMPTY rows — AddSupplyItems only ever appends. The original
			// items stay exactly as recorded; this is for "oh, and also…", not
			// for re-typing what is already on the request.
			items = [{ item: '', quantity: '', unit: '', note: '' }];
			step = 'append';
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not log the correction';
		} finally {
			submitting = false;
		}
	}

	// Post-correction: append whatever new rows the operator filled in, then
	// go back to read-back. If they added nothing (the correction was fully
	// captured by the note itself), skip straight to another read-back.
	async function continueFromAppend(): Promise<void> {
		if (!created) return;
		const toAdd = itemsForSubmit;
		if (toAdd.length === 0) {
			step = 'readback';
			return;
		}
		submitting = true;
		error = null;
		try {
			const updated = await api.addRideSupplyItems(netId, created.id, { items: toAdd, askedWhatElse: true });
			upsertSupply(updated);
			created = updated;
			step = 'readback';
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not add items';
		} finally {
			submitting = false;
		}
	}

	let dialogEl = $state<HTMLElement | null>(null);
	$effect(() => {
		dialogEl?.querySelector<HTMLElement>('input, select, button, textarea')?.focus();
	});

	function handleKeydown(e: KeyboardEvent): void {
		if (e.key === 'Escape') onClose();
	}

	let checkInSuggestions = $derived(Array.from(new Set($activeCheckIns.map((c) => c.tacticalCall || c.callsign).filter(Boolean))));

	function formatItem(it: { item: string; quantity: number; unit?: string }): string {
		if (it.quantity > 0) return `${it.quantity}${it.unit ? ` ${it.unit}` : ''} ${it.item}`;
		return it.item;
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="suc-backdrop" role="presentation" onclick={onClose}>
	<div class="suc" bind:this={dialogEl} role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="suc-title" onclick={(e) => e.stopPropagation()}>
		<h2 id="suc-title" class="suc-title">Supply request</h2>

		{#if step === 'items'}
			<div class="suc-body">
				<label class="suc-field" for="suc-location">
					<span class="suc-label">Location</span>
					<input id="suc-location" type="text" bind:value={location} placeholder="Rest Stop 3 / Nicasio" />
				</label>
				<label class="suc-field" for="suc-miles">
					<span class="suc-label">Miles remaining (on-air)</span>
					<input id="suc-miles" type="number" step="0.1" inputmode="decimal" bind:value={milesRemaining} placeholder="optional" />
				</label>
				<label class="suc-field" for="suc-reqby">
					<span class="suc-label">Requested by</span>
					<input id="suc-reqby" type="text" list="suc-checkins" bind:value={requestedByCall} placeholder="RS3" />
					<datalist id="suc-checkins">
						{#each checkInSuggestions as c (c)}<option value={c}></option>{/each}
					</datalist>
				</label>

				<div class="suc-field">
					<span class="suc-label">Priority</span>
					<div class="suc-tiers">
						{#each $rideLadder as tier (tier.id)}
							<button type="button" class="suc-tier-chip" class:active={priority === tier.id} onclick={() => (priority = tier.id)}>
								<RideTierGlyph {tier} size={14} />
								{tier.label}
							</button>
						{/each}
					</div>
				</div>

				<fieldset class="suc-items">
					<legend class="suc-legend">Items</legend>
					{#each items as row, i (i)}
						<div class="suc-item-row">
							<input type="text" list="suc-catalog" placeholder="item" bind:value={row.item} onchange={(e) => applyCatalogEntry(i, (e.target as HTMLInputElement).value)} />
							<input type="text" inputmode="decimal" placeholder="qty" bind:value={row.quantity} />
							<input type="text" placeholder="unit" bind:value={row.unit} />
							<input type="text" placeholder="note" bind:value={row.note} />
							<button type="button" class="suc-item-remove" disabled={items.length <= 1} onclick={() => removeItem(i)} aria-label="Remove item {i + 1}">×</button>
						</div>
					{/each}
					<datalist id="suc-catalog">
						{#each catalog as c (c.item + (c.when ?? ''))}<option value={c.item}>{c.when ? `${c.item} — ${c.when}` : c.item}</option>{/each}
					</datalist>

					<div class="suc-what-else">
						<button type="button" class="suc-add-item" onclick={() => addItem(true)}>+ Anything else they're running low on?</button>
						<label class="suc-asked"><input type="checkbox" bind:checked={askedWhatElse} /> Asked "what else"</label>
					</div>
				</fieldset>

				<label class="suc-field" for="suc-notes">
					<span class="suc-label">Notes</span>
					<textarea id="suc-notes" rows="2" bind:value={notes}></textarea>
				</label>
			</div>

			{#if error}<p class="suc-error">{error}</p>{/if}
			<div class="suc-actions">
				<button class="suc-btn suc-cancel" onclick={onClose}>Cancel</button>
				<button class="suc-btn suc-submit" disabled={!canSubmitItems} aria-busy={submitting} onclick={submitCreate}>
					{submitting ? 'Sending…' : 'Continue to read-back'}
				</button>
			</div>
		{:else if step === 'append' && created}
			<div class="suc-body">
				<p class="suc-readback-intro">Already on this request: {created.items.map(formatItem).join(', ') || '—'}</p>
				<fieldset class="suc-items">
					<legend class="suc-legend">Anything else?</legend>
					{#each items as row, i (i)}
						<div class="suc-item-row">
							<input type="text" list="suc-catalog" placeholder="item" bind:value={row.item} onchange={(e) => applyCatalogEntry(i, (e.target as HTMLInputElement).value)} />
							<input type="text" inputmode="decimal" placeholder="qty" bind:value={row.quantity} />
							<input type="text" placeholder="unit" bind:value={row.unit} />
							<input type="text" placeholder="note" bind:value={row.note} />
							<button type="button" class="suc-item-remove" disabled={items.length <= 1} onclick={() => removeItem(i)} aria-label="Remove item {i + 1}">×</button>
						</div>
					{/each}
					<button type="button" class="suc-add-item" onclick={() => addItem(false)}>+ Another item</button>
				</fieldset>
			</div>
			{#if error}<p class="suc-error">{error}</p>{/if}
			<div class="suc-actions">
				<button class="suc-btn suc-cancel" onclick={onClose}>Close</button>
				<button class="suc-btn suc-submit" disabled={submitting} aria-busy={submitting} onclick={continueFromAppend}>
					{submitting ? 'Saving…' : 'Read back again'}
				</button>
			</div>
		{:else if step === 'readback' && created}
			<div class="suc-readback">
				<p class="suc-readback-intro">Read back to <strong>{created.requestedByCall}</strong>:</p>
				<p class="suc-readback-text">
					{created.items.map(formatItem).join(', ')} — {created.location}
					{created.milesRemaining != null ? ` (mi ${created.milesRemaining.toFixed(1)} to go)` : ''},
					priority {tierById($rideLadder, created.priority)?.label ?? created.priority}.
				</p>

				{#if correcting}
					<label class="suc-field" for="suc-correction">
						<span class="suc-label">What did they correct?</span>
						<textarea id="suc-correction" rows="2" bind:value={correction}></textarea>
					</label>
					<div class="suc-actions">
						<button class="suc-btn suc-cancel" onclick={() => (correcting = false)}>Back</button>
						<button class="suc-btn suc-submit" disabled={submitting} aria-busy={submitting} onclick={submitCorrection}>{submitting ? 'Logging…' : 'Log correction'}</button>
					</div>
				{:else}
					{#if error}<p class="suc-error">{error}</p>{/if}
					<div class="suc-actions">
						<button class="suc-btn suc-cancel" onclick={() => (correcting = true)}>They corrected something</button>
						<button class="suc-btn suc-submit" disabled={submitting} aria-busy={submitting} onclick={confirmReadback}>{submitting ? 'Transmitting…' : 'Confirmed — transmit'}</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

<style>
	.suc-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.suc {
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

	.suc-title {
		font-size: 1.05rem;
		font-weight: 700;
	}

	.suc-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.suc-field {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.suc-label {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
	}

	.suc-field input,
	.suc-field textarea {
		min-height: 40px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
	}

	.suc-field textarea {
		padding: var(--space-sm);
		resize: vertical;
	}

	.suc-tiers {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.suc-tier-chip {
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

	.suc-tier-chip.active {
		border-color: var(--color-accent);
		background: var(--color-raised-strong);
	}

	.suc-items {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		margin: 0;
	}

	.suc-legend {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
		padding: 0 4px;
	}

	.suc-item-row {
		display: grid;
		grid-template-columns: 2fr 1fr 1fr 2fr auto;
		gap: 6px;
	}

	.suc-item-row input {
		min-height: 36px;
		padding: 0 8px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		min-width: 0;
	}

	.suc-item-remove {
		min-width: 32px;
		min-height: 32px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.suc-item-remove:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	.suc-what-else {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: var(--space-sm);
		padding-top: 4px;
		border-top: 1px dashed var(--color-primary);
	}

	.suc-add-item {
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: 0.78rem;
		cursor: pointer;
		min-height: 32px;
	}

	.suc-asked {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		font-size: 0.72rem;
		color: var(--color-text-muted);
	}

	.suc-readback {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.suc-readback-intro {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}

	.suc-readback-text {
		font-size: 0.95rem;
		padding: var(--space-sm);
		background: var(--color-bg);
		border-radius: var(--radius-sm);
	}

	.suc-error {
		color: var(--color-error-text);
		font-size: 0.8rem;
	}

	.suc-actions {
		display: flex;
		gap: var(--space-sm);
		justify-content: flex-end;
		margin-top: var(--space-xs);
	}

	.suc-btn {
		min-height: 44px;
		padding: 0 var(--space-md);
		border-radius: var(--radius-sm);
		font-weight: 700;
		cursor: pointer;
	}

	.suc-cancel {
		background: none;
		border: 1px solid var(--color-primary);
		color: var(--color-text);
	}

	.suc-submit {
		background: var(--color-accent);
		border: none;
		color: var(--color-on-accent);
	}

	.suc-submit:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
