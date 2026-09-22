<script lang="ts">
	// Sweep reporting: last supported rider bib, estimated speed, and the
	// position the ride status strip's largest tile is built from. History
	// below the form so a shift-change operator can see the last few calls.
	import { sweepPositionD, sweepReports, reportSweep } from '$lib/stores/course';
	import { rideConfig } from '$lib/stores/netProfile';
	import { activeCheckIns } from '$lib/stores/netcontrol';
	import { ageState, ageText } from '$lib/rideMeta';
	import { secondClock } from '$lib/stores/clock';
	import { clock } from '$lib/wxAlertTime';
	import { showToast } from '$lib/stores/toast';

	let { onFlyTo }: { onFlyTo?: (lat: number, lon: number, zoom?: number) => void } = $props();

	let position = $derived($sweepPositionD);
	let latest = $derived(position?.latestReport ?? null);
	let age = $derived(ageState(latest?.reportedAt ?? position?.lastPassageTime ?? null, $secondClock));
	let ageLabel = $derived(ageText(latest?.reportedAt ?? position?.lastPassageTime ?? null, $secondClock));

	let sweepUnits = $derived($activeCheckIns.filter((c) => c.category === 'sag' || c.category === 'mobile'));
	let routes = $derived($rideConfig?.routes ?? []);

	let fRoute = $state('');
	let fCheckInId = $state('');
	let fMile = $state('');
	let fBib = $state('');
	let fSpeed = $state('');
	let fNote = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	function pickUnit(id: string): void {
		fCheckInId = id;
		const ci = sweepUnits.find((c) => c.id === id);
		if (!fRoute && routes.length === 1) fRoute = routes[0].id;
		void ci;
	}

	async function submit(): Promise<void> {
		error = null;
		submitting = true;
		try {
			await reportSweep({
				routeLabel: fRoute,
				checkInId: fCheckInId || undefined,
				routeMile: fMile.trim() === '' ? undefined : Number(fMile),
				lastRiderBib: fBib.trim(),
				estimatedSpeedMph: fSpeed.trim() === '' ? undefined : Number(fSpeed),
				note: fNote.trim()
			});
			fBib = '';
			fSpeed = '';
			fNote = '';
			showToast('Sweep report logged', 'success');
		} catch (e: any) {
			error = e?.message ?? 'Could not log sweep report';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="sw">
	<div class="sw-position">
		<div class="sw-position-row">
			<span class="sw-label">Last passed</span>
			<span class="sw-value">{position?.lastCheckpointSeq ? `#${position.lastCheckpointSeq}` : '—'}</span>
			{#if position?.nextStationLabel}<span class="sw-next">Next: {position.nextStationLabel}</span>{/if}
		</div>
		<div class="sw-position-row">
			<span class="sw-label">Latest report</span>
			{#if latest}
				<span class="sw-value">
					{#if latest.lastRiderBib}bib {latest.lastRiderBib}{:else}no bib given{/if}
					{#if latest.estimatedSpeedMph != null}· {latest.estimatedSpeedMph.toFixed(1)} mph{/if}
				</span>
				<span class="sw-age" class:stale={age === 'stale'} class:aging={age === 'aging'}>
					{ageLabel} ago{age === 'stale' ? ' · stale' : ''}
				</span>
			{:else}
				<span class="sw-value muted">not reported</span>
			{/if}
		</div>
	</div>

	<div class="sw-form">
		{#if routes.length > 1}
			<div class="form-group">
				<label for="sw-route">Route</label>
				<select id="sw-route" bind:value={fRoute}>
					<option value="">&mdash;</option>
					{#each routes as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</div>
		{/if}
		<div class="form-group">
			<label for="sw-unit">Sweep unit</label>
			<select id="sw-unit" bind:value={fCheckInId} onchange={(e) => pickUnit((e.target as HTMLSelectElement).value)}>
				<option value="">&mdash; unassigned &mdash;</option>
				{#each sweepUnits as ci (ci.id)}<option value={ci.id}>{ci.tacticalCall || ci.callsign}</option>{/each}
			</select>
		</div>
		<div class="form-row">
			<div class="form-group">
				<label for="sw-mile">Route mile</label>
				<input id="sw-mile" type="number" inputmode="decimal" bind:value={fMile} placeholder="31.4" />
			</div>
			<div class="form-group">
				<label for="sw-bib">Last supported rider bib</label>
				<input id="sw-bib" type="text" autocomplete="off" bind:value={fBib} placeholder="340" />
			</div>
			<div class="form-group">
				<label for="sw-speed">Est. speed (mph)</label>
				<input id="sw-speed" type="number" min="0" max="60" inputmode="decimal" bind:value={fSpeed} placeholder="11" />
			</div>
		</div>
		<div class="form-group">
			<label for="sw-note">Note</label>
			<input id="sw-note" type="text" bind:value={fNote} />
		</div>
		{#if error}<div class="form-error" role="alert">{error}</div>{/if}
		<div class="form-actions">
			<button class="btn-primary" onclick={submit} disabled={submitting} aria-busy={submitting}>{submitting ? 'Logging…' : 'Log sweep report'}</button>
		</div>
	</div>

	<h3 class="sw-history-heading">Recent reports</h3>
	{#if $sweepReports.length === 0}
		<p class="sw-empty">No sweep reports yet.</p>
	{:else}
		<div class="sw-history">
			{#each $sweepReports as r (r.id)}
				<button
					class="sw-row"
					disabled={!onFlyTo || r.lat == null || r.lon == null}
					onclick={() => { if (onFlyTo && r.lat != null && r.lon != null) onFlyTo(r.lat, r.lon, 14); }}
				>
					{clock(r.reportedAt)}{r.routeMile != null ? ` · mi ${r.routeMile.toFixed(1)}` : ''}{r.lastRiderBib ? ` · bib ${r.lastRiderBib}` : ''}{r.estimatedSpeedMph != null ? ` · ${r.estimatedSpeedMph.toFixed(1)} mph` : ''}{r.reportedBy ? ` · ${r.reportedBy}` : ''}
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.sw { display: flex; flex-direction: column; gap: var(--space-md); padding: var(--space-md); }

	.sw-position {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.sw-position-row { display: flex; align-items: baseline; gap: var(--space-sm); flex-wrap: wrap; }
	.sw-label { font-size: 0.68rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--color-text-muted); width: 90px; flex-shrink: 0; }
	.sw-value { font-size: 0.95rem; font-weight: 700; }
	.sw-value.muted { color: var(--color-text-muted); font-weight: 400; }
	.sw-next { font-size: 0.75rem; color: var(--color-text-muted); margin-left: auto; }
	.sw-age { font-size: 0.75rem; color: var(--color-text-muted); }
	.sw-age.aging { color: var(--color-warning); }
	.sw-age.stale { color: var(--color-text-muted); font-weight: 700; }

	.sw-form { display: flex; flex-direction: column; gap: var(--space-sm); }
	.form-group { display: flex; flex-direction: column; gap: var(--space-xs); }
	.form-row { display: flex; gap: var(--space-sm); }
	.form-row > .form-group { flex: 1; min-width: 0; }
	label { font-size: 0.72rem; color: var(--color-text-muted); }
	input, select {
		min-height: 36px; background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); font: inherit; font-size: 0.82rem; padding: 0 var(--space-sm);
	}
	.form-error { color: var(--color-error-text); font-size: 0.78rem; }
	.form-actions { display: flex; justify-content: flex-end; }
	.btn-primary {
		min-height: 40px; padding: 0 var(--space-md); background: var(--color-accent);
		border: none; border-radius: var(--radius-sm); color: var(--color-on-accent); font-weight: 700; cursor: pointer;
	}
	.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

	.sw-history-heading { font-size: 0.68rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--color-text-muted); margin: 0; }
	.sw-empty { color: var(--color-text-muted); font-size: 0.82rem; }
	.sw-history { display: flex; flex-direction: column; gap: var(--space-2xs); }
	.sw-row {
		display: block; width: 100%; text-align: left; font-size: 0.78rem; color: var(--color-text);
		padding: 6px var(--space-sm); border-radius: var(--radius-sm); cursor: pointer; background: none; border: none;
		font-variant-numeric: tabular-nums;
	}
	.sw-row:hover:not(:disabled) { background: var(--color-bg); }
	.sw-row:disabled { cursor: default; }
</style>
