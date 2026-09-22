<script lang="ts">
	// The closeout checklist and rider accounting — should reach all-clear
	// before the net closes. "End net…" here posts POST /ride/closeout
	// (which returns the blocking items on a 409) rather than the general
	// close-net flow, since a ride net's close is gated on this checklist.
	import { onMount } from 'svelte';
	import { closeout, accounting } from '$lib/stores/ride';
	import { refreshCloseout, submitRideCloseout } from '$lib/stores/course';
	import { isNetNcs } from '$lib/stores/netProfile';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { api } from '$lib/api';
	import { riderKindLabel } from '$lib/courseMeta';
	import { clock } from '$lib/wxAlertTime';

	onMount(() => {
		refreshCloseout();
	});

	let netId = $derived($activeNetId);
	let items = $derived($closeout?.items ?? []);
	let ready = $derived($closeout?.ready ?? false);

	let forceOpen = $state(false);
	let forceReason = $state('');
	let ending = $state(false);
	let endError = $state<string | null>(null);
	let endedNotice = $state<string | null>(null);

	async function endNet(force: boolean): Promise<void> {
		ending = true;
		endError = null;
		try {
			const result = await submitRideCloseout({ force, reason: force ? forceReason.trim() : undefined });
			endedNotice = `Net closed${force ? ' (forced)' : ''}.`;
			forceOpen = false;
		} catch (e: any) {
			if (e?.body?.closeout) {
				closeout.set(e.body.closeout);
			}
			if (e?.status === 403) {
				endError = 'Force-close is not permitted for this net.';
			} else {
				endError = e?.message ?? 'Could not close the net.';
			}
		} finally {
			ending = false;
		}
	}
</script>

<div class="rv">
	{#if !$closeout}
		<p class="rv-loading">Loading close-out status&hellip;</p>
	{:else}
		<ul class="rv-checklist">
			{#each items as item (item.key)}
				<li class="rv-item" class:done={item.done} class:advisory={!item.blocking}>
					<span class="rv-mark" aria-hidden="true">{item.done ? '✓' : '○'}</span>
					<div class="rv-item-body">
						<span class="rv-item-label">{item.label}{#if !item.blocking}<span class="rv-advisory-tag"> (advisory)</span>{/if}</span>
						{#if item.total > 0}<span class="rv-item-count">{item.count} of {item.total}</span>{/if}
						{#if item.detail}<p class="rv-item-detail">{item.detail}</p>{/if}
					</div>
				</li>
			{/each}
		</ul>

		{#if ready}
			<div class="rv-banner rv-banner-ready" role="status">Close-out complete &mdash; the net can end.</div>
		{/if}

		{#if $accounting}
			<div class="rv-accounting">
				<p class="rv-sweep-detail">{$accounting.sweepDetail}</p>
				<div class="rv-counts">
					<span class="rv-count-chip">{$accounting.counts.supported} supported</span>
					<span class="rv-count-chip">{$accounting.counts.unsupported} unsupported</span>
					{#each Object.entries($accounting.counts.byKind) as [kind, n] (kind)}
						{#if n > 0}<span class="rv-count-chip">{n} {riderKindLabel(kind)}</span>{/if}
					{/each}
				</div>
				{#if $accounting.openExceptions.length > 0}
					<p class="rv-open-heading">Open exceptions</p>
					<ul class="rv-open-list">
						{#each $accounting.openExceptions as ex (ex.id)}
							<li>{ex.bibWithheld ? 'bib withheld' : ex.bib ? `Bib ${ex.bib}` : 'no bib'} &middot; {riderKindLabel(ex.kind)}{ex.reason ? ` · ${ex.reason}` : ''}</li>
						{/each}
					</ul>
				{/if}
			</div>
		{/if}

		<div class="rv-exports">
			<a class="rv-export-link" href={api.ics211ExportUrl(netId)} target="_blank" rel="noopener">ICS 211 &#8599;</a>
			<a class="rv-export-link" href={api.ics214ExportUrl(netId)} target="_blank" rel="noopener">ICS 214 &#8599;</a>
		</div>

		{#if $isNetNcs}
			<div class="rv-end">
				{#if endedNotice}
					<p class="rv-ended">{endedNotice}</p>
				{:else if forceOpen}
					<label for="rv-force-reason">Force-close reason (required)</label>
					<input id="rv-force-reason" type="text" bind:value={forceReason} />
					{#if endError}<p class="rv-error" role="alert">{endError}</p>{/if}
					<div class="rv-end-actions">
						<button class="btn-secondary" onclick={() => (forceOpen = false)} disabled={ending}>Cancel</button>
						<button class="btn-danger" onclick={() => endNet(true)} disabled={ending || forceReason.trim() === ''} aria-busy={ending}>Force close</button>
					</div>
				{:else}
					{#if endError}<p class="rv-error" role="alert">{endError}</p>{/if}
					<div class="rv-end-actions">
						<button class="btn-primary" onclick={() => endNet(false)} disabled={ending} aria-busy={ending}>{ending ? 'Ending…' : 'End net…'}</button>
						{#if !ready}<button class="btn-secondary" onclick={() => (forceOpen = true)}>Force close…</button>{/if}
					</div>
				{/if}
			</div>
		{/if}
	{/if}
</div>

<style>
	.rv { display: flex; flex-direction: column; gap: var(--space-md); padding: var(--space-md); }
	.rv-loading { color: var(--color-text-muted); font-size: 0.82rem; }

	.rv-checklist { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
	.rv-item { display: flex; gap: var(--space-sm); align-items: flex-start; }
	.rv-mark { font-size: 0.9rem; color: var(--color-text-muted); flex-shrink: 0; }
	.rv-item.done .rv-mark { color: var(--color-success); }
	.rv-item-body { flex: 1; min-width: 0; }
	.rv-item-label { font-size: 0.85rem; font-weight: 600; }
	.rv-advisory-tag { font-weight: 400; color: var(--color-text-muted); font-size: 0.75rem; }
	.rv-item-count { margin-left: 6px; font-size: 0.75rem; color: var(--color-text-muted); font-variant-numeric: tabular-nums; }
	.rv-item-detail { margin: 2px 0 0; font-size: 0.75rem; color: var(--color-text-muted); }

	.rv-banner-ready {
		padding: var(--space-sm); border-radius: var(--radius-sm);
		background: color-mix(in srgb, var(--color-success) 12%, transparent);
		color: var(--color-success); font-weight: 700; font-size: 0.85rem;
	}

	.rv-accounting {
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); padding: var(--space-sm);
		display: flex; flex-direction: column; gap: 6px;
	}
	.rv-sweep-detail { font-size: 0.82rem; margin: 0; }
	.rv-counts { display: flex; flex-wrap: wrap; gap: 6px; }
	.rv-count-chip {
		font-size: 0.72rem; padding: 2px 8px; border-radius: var(--radius-full);
		background: var(--color-bg); color: var(--color-text-muted);
	}
	.rv-open-heading { font-size: 0.68rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--color-text-muted); margin: 4px 0 0; }
	.rv-open-list { margin: 0; padding-left: 18px; font-size: 0.78rem; color: var(--color-text); }

	.rv-exports { display: flex; gap: var(--space-md); }
	.rv-export-link { color: var(--color-accent); font-size: 0.82rem; font-weight: 600; }

	.rv-end { display: flex; flex-direction: column; gap: 6px; border-top: 1px solid var(--color-primary); padding-top: var(--space-sm); }
	.rv-end label { font-size: 0.72rem; color: var(--color-text-muted); }
	.rv-end input {
		min-height: 36px; background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); font: inherit; padding: 0 var(--space-sm);
	}
	.rv-end-actions { display: flex; gap: var(--space-sm); }
	.rv-error { color: var(--color-error-text); font-size: 0.78rem; margin: 0; }
	.rv-ended { color: var(--color-success); font-weight: 700; }

	.btn-primary {
		min-height: 40px; padding: 0 var(--space-md); background: var(--color-accent);
		border: none; border-radius: var(--radius-sm); color: var(--color-on-accent); font-weight: 700; cursor: pointer;
	}
	.btn-secondary {
		min-height: 40px; padding: 0 var(--space-md); background: var(--color-bg);
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); color: var(--color-text);
		font-weight: 600; cursor: pointer;
	}
	.btn-danger {
		min-height: 40px; padding: 0 var(--space-md); background: var(--color-error);
		border: none; border-radius: var(--radius-sm); color: var(--color-on-accent); font-weight: 700; cursor: pointer;
	}
	.btn-primary:disabled, .btn-secondary:disabled, .btn-danger:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
