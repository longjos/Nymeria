<script lang="ts">
	// The ONLY place an individual bib belongs in this UI (governing fact 1).
	// SAG'd / injured / DNF / unsupported records, newest first.
	import { riderExceptions, recordRider, setRiderSupport } from '$lib/stores/course';
	import { rideConfig } from '$lib/stores/netProfile';
	import { canOperate } from '$lib/stores/session';
	import { riderKindMeta, type RiderExceptionKind } from '$lib/courseMeta';
	import { clock } from '$lib/wxAlertTime';
	import { showToast } from '$lib/stores/toast';

	const KINDS = Object.keys(riderKindMeta) as RiderExceptionKind[];

	let kindFilter = $state<RiderExceptionKind | null>(null);
	let statusFilter = $state<'supported' | 'unsupported' | null>(null);

	let sorted = $derived([...$riderExceptions].sort((a, b) => Date.parse(b.recordedAt) - Date.parse(a.recordedAt)));
	let filtered = $derived(
		sorted.filter((r) => (kindFilter == null || r.kind === kindFilter) && (statusFilter == null || r.supportStatus === statusFilter))
	);

	// --- composer ---
	let composerOpen = $state(false);
	let fKind = $state<RiderExceptionKind>('sag');
	let fBib = $state('');
	let fBibWithheld = $state(false);
	let fRouteLabel = $state('');
	let fRouteMile = $state('');
	let fReason = $state('');
	let fNote = $state('');
	let fSaving = $state(false);
	let fError = $state<string | null>(null);

	let bibRequired = $derived(!fBibWithheld && fKind !== 'dnf' && fKind !== 'shutoff_reroute');

	function resetComposer(): void {
		fKind = 'sag';
		fBib = '';
		fBibWithheld = false;
		fRouteLabel = '';
		fRouteMile = '';
		fReason = '';
		fNote = '';
		fError = null;
	}

	async function submitComposer(): Promise<void> {
		fError = null;
		if (bibRequired && fBib.trim() === '') {
			fError = 'Bib is required for this kind, unless withheld.';
			return;
		}
		fSaving = true;
		try {
			await recordRider({
				kind: fKind,
				bib: fBibWithheld ? '' : fBib.trim(),
				bibWithheld: fBibWithheld,
				routeLabel: fRouteLabel.trim(),
				routeMile: fRouteMile.trim() === '' ? undefined : Number(fRouteMile),
				reason: fReason.trim(),
				note: fNote.trim()
				// supportStatus intentionally omitted: RecordRider defaults it from
				// Kind (dnf/declined_sag/shutoff_reroute -> unsupported, everything
				// else -> supported) — never override that here.
			});
			composerOpen = false;
			resetComposer();
			showToast('Rider exception recorded', 'success');
		} catch (e: any) {
			fError = e?.message ?? 'Could not record exception';
		} finally {
			fSaving = false;
		}
	}

	// --- status change ---
	let statusOpenId = $state<string | null>(null);
	let statusReason = $state('');
	let statusPending = $state(false);

	function openStatusChange(id: string): void {
		statusOpenId = id;
		statusReason = '';
	}

	async function submitStatusChange(id: string, to: 'supported' | 'unsupported'): Promise<void> {
		statusPending = true;
		try {
			await setRiderSupport(id, to, statusReason.trim());
			statusOpenId = null;
			statusReason = '';
		} catch (e: any) {
			showToast(e?.message ?? 'Could not update status', 'error');
		} finally {
			statusPending = false;
		}
	}
</script>

<div class="rel">
	<div class="rel-filters">
		<div class="chip-group" role="group" aria-label="Filter by kind">
			<button class="chip" class:active={kindFilter == null} aria-pressed={kindFilter == null} onclick={() => (kindFilter = null)}>All</button>
			{#each KINDS as k (k)}
				<button class="chip" class:active={kindFilter === k} aria-pressed={kindFilter === k} onclick={() => (kindFilter = kindFilter === k ? null : k)}>{riderKindMeta[k].short}</button>
			{/each}
		</div>
		<div class="chip-group" role="group" aria-label="Filter by support status">
			<button class="chip" class:active={statusFilter === 'supported'} aria-pressed={statusFilter === 'supported'} onclick={() => (statusFilter = statusFilter === 'supported' ? null : 'supported')}>Supported</button>
			<button class="chip" class:active={statusFilter === 'unsupported'} aria-pressed={statusFilter === 'unsupported'} onclick={() => (statusFilter = statusFilter === 'unsupported' ? null : 'unsupported')}>Unsupported</button>
		</div>
	</div>

	{#if $canOperate}
		<div class="rel-composer-toggle">
			<button class="btn-secondary" onclick={() => { composerOpen = !composerOpen; if (composerOpen) resetComposer(); }}>{composerOpen ? 'Cancel' : '+ Record exception'}</button>
		</div>
	{/if}

	{#if composerOpen}
		<div class="rel-composer">
			<div class="form-row">
				<div class="form-group">
					<label for="re-kind">Kind</label>
					<select id="re-kind" bind:value={fKind}>
						{#each KINDS as k (k)}<option value={k}>{riderKindMeta[k].label}</option>{/each}
					</select>
				</div>
				<div class="form-group">
					<label for="re-bib">Bib{bibRequired ? ' (required)' : ''}</label>
					<input id="re-bib" type="text" autocomplete="off" bind:value={fBib} disabled={fBibWithheld} />
				</div>
			</div>
			<label class="withhold-check"><input type="checkbox" bind:checked={fBibWithheld} /> Withhold bib</label>
			<div class="form-row">
				<div class="form-group">
					<label for="re-route">Route</label>
					<input id="re-route" type="text" bind:value={fRouteLabel} list="ride-routes-rel" placeholder="100-mile" />
					{#if $rideConfig}
						<datalist id="ride-routes-rel">{#each $rideConfig.routes as r (r.id)}<option value={r.name}></option>{/each}</datalist>
					{/if}
				</div>
				<div class="form-group">
					<label for="re-mile">Route mile</label>
					<input id="re-mile" type="number" inputmode="decimal" bind:value={fRouteMile} />
				</div>
			</div>
			<div class="form-group">
				<label for="re-reason">Reason</label>
				<input id="re-reason" type="text" bind:value={fReason} />
			</div>
			<div class="form-group">
				<label for="re-note">Note</label>
				<input id="re-note" type="text" bind:value={fNote} />
			</div>
			{#if fError}<div class="form-error" role="alert">{fError}</div>{/if}
			<div class="form-actions">
				<button class="btn-primary" onclick={submitComposer} disabled={fSaving} aria-busy={fSaving}>{fSaving ? 'Saving…' : 'Record'}</button>
			</div>
		</div>
	{/if}

	{#if filtered.length === 0}
		<p class="rel-empty">No rider exceptions{kindFilter || statusFilter ? ' match this filter' : ''}.</p>
	{:else}
		<div class="rel-list">
			{#each filtered as r (r.id)}
				<div class="rel-row">
					<div class="rel-row-head">
						<span class="rel-kind">{riderKindMeta[r.kind as RiderExceptionKind]?.label ?? r.kind}</span>
						{#if r.bibWithheld}
							<span class="rel-bib withheld" aria-label="bib withheld">&#128274; bib withheld</span>
						{:else if r.bib}
							<span class="rel-bib">Bib {r.bib}</span>
						{/if}
						<span class="rel-status" class:unsupported={r.supportStatus === 'unsupported'}>{r.supportStatus}</span>
						<span class="rel-when">{clock(r.recordedAt)}</span>
					</div>
					{#if r.reason}<p class="rel-reason">{r.reason}</p>{/if}
					{#if r.routeLabel || r.routeMile != null}
						<p class="rel-route">{r.routeLabel}{r.routeMile != null ? ` · mi ${r.routeMile.toFixed(1)}` : ''}</p>
					{/if}
					{#if r.note}<p class="rel-note">{r.note}</p>{/if}

					{#if $canOperate}
						{#if statusOpenId === r.id}
							<div class="rel-status-form">
								<input type="text" placeholder="Reason" bind:value={statusReason} />
								<button class="rel-btn" onclick={() => submitStatusChange(r.id, r.supportStatus === 'supported' ? 'unsupported' : 'supported')} disabled={statusPending}>Confirm</button>
								<button class="rel-btn" onclick={() => (statusOpenId = null)}>Cancel</button>
							</div>
						{:else}
							<button class="rel-btn" onclick={() => openStatusChange(r.id)}>
								{r.supportStatus === 'supported' ? 'Mark unsupported…' : 'Re-support…'}
							</button>
						{/if}
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.rel { display: flex; flex-direction: column; gap: var(--space-sm); padding: var(--space-md); }
	.rel-filters { display: flex; flex-direction: column; gap: 6px; }
	.chip-group { display: flex; flex-wrap: wrap; gap: var(--space-xs); }
	.chip {
		min-height: 28px; padding: 0 10px; border-radius: var(--radius-full);
		background: var(--color-bg); border: 1px solid var(--color-primary); color: var(--color-text-muted);
		font-size: 0.72rem; font-weight: 600; cursor: pointer;
	}
	.chip.active { color: var(--color-text); border-color: var(--color-accent); background: var(--color-primary); }

	.btn-secondary {
		min-height: 40px; padding: 0 var(--space-md); background: var(--color-bg);
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); color: var(--color-text);
		font-weight: 600; cursor: pointer;
	}
	.btn-primary {
		min-height: 40px; padding: 0 var(--space-md); background: var(--color-accent);
		border: none; border-radius: var(--radius-sm); color: var(--color-on-accent); font-weight: 700; cursor: pointer;
	}
	.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

	.rel-composer {
		display: flex; flex-direction: column; gap: var(--space-sm);
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); padding: var(--space-sm);
		background: var(--color-bg);
	}
	.withhold-check { display: flex; align-items: center; gap: 6px; font-size: 0.78rem; color: var(--color-text-muted); }
	.form-row { display: flex; gap: var(--space-sm); }
	.form-row > .form-group { flex: 1; min-width: 0; }
	.form-group { display: flex; flex-direction: column; gap: var(--space-xs); }
	label { font-size: 0.72rem; color: var(--color-text-muted); }
	input, select {
		min-height: 36px; background: var(--color-surface); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); font: inherit; font-size: 0.82rem; padding: 0 var(--space-sm);
	}
	.form-error { color: var(--color-error-text); font-size: 0.78rem; }
	.form-actions { display: flex; justify-content: flex-end; }

	.rel-empty { color: var(--color-text-muted); font-size: 0.82rem; }
	.rel-list { display: flex; flex-direction: column; gap: 6px; }
	.rel-row {
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); padding: var(--space-sm);
		display: flex; flex-direction: column; gap: var(--space-xs);
	}
	.rel-row-head { display: flex; align-items: center; gap: var(--space-sm); flex-wrap: wrap; font-size: 0.8rem; }
	.rel-kind { font-weight: 700; }
	.rel-bib { font-variant-numeric: tabular-nums; color: var(--color-text-muted); }
	.rel-bib.withheld { color: var(--color-warning); }
	.rel-status { font-size: 0.68rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--color-success); }
	.rel-status.unsupported { color: var(--color-text-muted); }
	.rel-when { margin-left: auto; font-size: 0.72rem; color: var(--color-text-muted); }
	.rel-reason, .rel-route, .rel-note { font-size: 0.78rem; color: var(--color-text-muted); }
	.rel-status-form { display: flex; gap: 6px; align-items: center; }
	.rel-status-form input {
		flex: 1; min-height: 32px; background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); font-size: 0.78rem; padding: 0 var(--space-sm);
	}
	.rel-btn {
		align-self: flex-start; min-height: 32px; padding: 0 var(--space-sm); background: var(--color-bg);
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); color: var(--color-text);
		font-size: 0.75rem; font-weight: 600; cursor: pointer;
	}
	.rel-btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
