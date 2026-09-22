<script lang="ts">
	// Shutoff management: create/edit, cancel, reinstate, and the FIRE
	// control (a real confirm dialog — firing reroutes every rider behind
	// the point and is meant to feel consequential, see ShutoffFireDialog).
	import type { ShutoffPoint } from '$lib/types';
	import { createShutoff, updateShutoff, deleteShutoff, cancelShutoff, reinstateShutoff } from '$lib/stores/course';
	import { isNetNcs, rideConfig } from '$lib/stores/netProfile';
	import { canOperate } from '$lib/stores/session';
	import { activeCheckIns, netAnnotations } from '$lib/stores/netcontrol';
	import { shutoffStatusMeta } from '$lib/courseMeta';
	import { clock, countdown, countdownTone } from '$lib/wxAlertTime';
	import { secondClock } from '$lib/stores/clock';
	import { showToast } from '$lib/stores/toast';
	import { annotationCentroid } from '$lib/geo';
	import ShutoffFireDialog from './ShutoffFireDialog.svelte';

	let { shutoffs, onFlyTo }: { shutoffs: ShutoffPoint[]; onFlyTo?: (lat: number, lon: number, zoom?: number) => void } = $props();

	interface ShutoffGroup {
		heading: string;
		items: ShutoffPoint[];
	}

	let groups = $derived.by((): ShutoffGroup[] => {
		const planned = shutoffs.filter((s) => s.status === 'planned').sort((a, b) => Date.parse(a.scheduledAt) - Date.parse(b.scheduledAt));
		const fired = shutoffs.filter((s) => s.status === 'fired').sort((a, b) => Date.parse(b.firedAt ?? b.scheduledAt) - Date.parse(a.firedAt ?? a.scheduledAt));
		const cancelled = shutoffs.filter((s) => s.status === 'cancelled');
		return [
			{ heading: 'Planned', items: planned },
			{ heading: 'Fired', items: fired },
			{ heading: 'Cancelled', items: cancelled }
		];
	});

	function checkInLabel(id: string): string {
		const ci = $activeCheckIns.find((c) => c.id === id);
		return ci ? ci.tacticalCall || ci.callsign : '';
	}

	let pointAnnotations = $derived($netAnnotations.filter((a) => a.type === 'point'));

	// --- composer (create / edit) ---
	let formOpen = $state(false);
	let editingId = $state<string | null>(null);
	let fName = $state('');
	let fLocationMode = $state<'annotation' | 'coords'>('coords');
	let fAnnotationId = $state('');
	let fLat = $state('');
	let fLon = $state('');
	let fRouteMile = $state('');
	let fScheduledAt = $state('');
	let fDirection = $state('');
	let fDestination = $state('');
	let fInstructions = $state('');
	let fStaffedBy = $state('');
	let fError = $state<string | null>(null);
	let fSaving = $state(false);

	function resetForm(): void {
		fName = '';
		fLocationMode = 'coords';
		fAnnotationId = '';
		fLat = '';
		fLon = '';
		fRouteMile = '';
		fScheduledAt = '';
		fDirection = '';
		fDestination = '';
		fInstructions = '';
		fStaffedBy = '';
		fError = null;
		editingId = null;
	}

	function openCreate(): void {
		resetForm();
		formOpen = true;
	}

	function openEdit(sp: ShutoffPoint): void {
		editingId = sp.id;
		fName = sp.name;
		fLocationMode = sp.annotationId ? 'annotation' : 'coords';
		fAnnotationId = sp.annotationId ?? '';
		fLat = String(sp.lat);
		fLon = String(sp.lon);
		fRouteMile = sp.routeMile != null ? String(sp.routeMile) : '';
		fScheduledAt = toLocalInput(sp.scheduledAt);
		fDirection = sp.rerouteDirection;
		fDestination = sp.rerouteDestination;
		fInstructions = sp.rerouteInstructions;
		fStaffedBy = sp.staffedByCheckInId;
		fError = null;
		formOpen = true;
	}

	function toLocalInput(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '';
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	}

	function onAnnotationPick(id: string): void {
		fAnnotationId = id;
		const ann = pointAnnotations.find((a) => a.id === id);
		const centroid = ann ? annotationCentroid([ann]) : null;
		if (centroid) {
			fLat = String(centroid.lat);
			fLon = String(centroid.lon);
		}
	}

	async function submitForm(): Promise<void> {
		fError = null;
		const lat = Number(fLat);
		const lon = Number(fLon);
		if (fName.trim() === '') { fError = 'Name is required.'; return; }
		if (!fScheduledAt) { fError = 'Closes-at time is required.'; return; }
		if (Number.isNaN(lat) || lat < -90 || lat > 90 || Number.isNaN(lon) || lon < -180 || lon > 180) {
			fError = 'Latitude/longitude must be valid coordinates.';
			return;
		}
		const scheduledIso = new Date(fScheduledAt).toISOString();
		const body: Partial<ShutoffPoint> = {
			name: fName.trim(),
			lat,
			lon,
			routeMile: fRouteMile.trim() === '' ? undefined : Number(fRouteMile),
			annotationId: fLocationMode === 'annotation' ? fAnnotationId || undefined : undefined,
			scheduledAt: scheduledIso,
			rerouteDirection: fDirection.trim(),
			rerouteDestination: fDestination.trim(),
			rerouteInstructions: fInstructions.trim(),
			staffedByCheckInId: fStaffedBy
		};
		fSaving = true;
		try {
			if (editingId) {
				await updateShutoff(editingId, body);
				showToast(`${fName.trim()} updated`, 'success');
			} else {
				await createShutoff(body);
				showToast(`${fName.trim()} shutoff planned`, 'success');
			}
			formOpen = false;
			resetForm();
		} catch (e: any) {
			fError = e?.message ?? 'Could not save shutoff';
		} finally {
			fSaving = false;
		}
	}

	// --- cancel / reinstate / delete ---
	let cancelOpenId = $state<string | null>(null);
	let cancelReason = $state('');
	let unfireOpenId = $state<string | null>(null);
	let unfireReason = $state('');
	let deleteConfirmId = $state<string | null>(null);
	let fireDialogFor = $state<ShutoffPoint | null>(null);

	async function submitCancel(id: string): Promise<void> {
		try {
			await cancelShutoff(id, cancelReason.trim());
			cancelOpenId = null;
			cancelReason = '';
		} catch (e: any) {
			showToast(e?.message ?? 'Could not cancel shutoff', 'error');
		}
	}

	async function doReinstate(sp: ShutoffPoint): Promise<void> {
		try {
			await reinstateShutoff(sp.id);
			showToast(`${sp.name} reinstated`, 'success');
		} catch (e: any) {
			showToast(e?.message ?? 'Could not reinstate shutoff', 'error');
		}
	}

	async function submitUnfire(sp: ShutoffPoint): Promise<void> {
		if (unfireReason.trim() === '') return;
		try {
			await reinstateShutoff(sp.id, unfireReason.trim());
			unfireOpenId = null;
			unfireReason = '';
			showToast(`${sp.name} un-fired`, 'success');
		} catch (e: any) {
			showToast(e?.message ?? 'Could not un-fire shutoff', 'error');
		}
	}

	async function doDelete(id: string): Promise<void> {
		try {
			await deleteShutoff(id);
			deleteConfirmId = null;
		} catch (e: any) {
			showToast(e?.message ?? 'Could not delete shutoff', 'error');
		}
	}
</script>

<div class="sb">
	<div class="sb-toolbar">
		{#if $canOperate}
			<button class="sb-new" onclick={openCreate}>+ Shutoff</button>
		{/if}
	</div>

	{#if formOpen}
		<div class="sb-form">
			<div class="form-group">
				<label for="so-name">Name</label>
				<input id="so-name" type="text" bind:value={fName} placeholder="Benson Shutoff" />
			</div>

			<div class="form-group">
				<span class="form-label">Location</span>
				<div class="loc-mode">
					<label><input type="radio" name="loc-mode" value="coords" checked={fLocationMode === 'coords'} onchange={() => (fLocationMode = 'coords')} /> Coordinates</label>
					{#if pointAnnotations.length > 0}
						<label><input type="radio" name="loc-mode" value="annotation" checked={fLocationMode === 'annotation'} onchange={() => (fLocationMode = 'annotation')} /> Use a location</label>
					{/if}
				</div>
				{#if fLocationMode === 'annotation' && pointAnnotations.length > 0}
					<select bind:value={fAnnotationId} onchange={(e) => onAnnotationPick((e.target as HTMLSelectElement).value)}>
						<option value="">Choose a location&hellip;</option>
						{#each pointAnnotations as a (a.id)}
							<option value={a.id}>{a.label}</option>
						{/each}
					</select>
				{:else}
					<div class="form-row">
						<input type="text" inputmode="decimal" bind:value={fLat} placeholder="Latitude" aria-label="Latitude" />
						<input type="text" inputmode="decimal" bind:value={fLon} placeholder="Longitude" aria-label="Longitude" />
					</div>
				{/if}
			</div>

			<div class="form-row">
				<div class="form-group">
					<label for="so-mile">Route mile (optional)</label>
					<input id="so-mile" type="number" inputmode="decimal" bind:value={fRouteMile} placeholder="44.1" />
				</div>
				<div class="form-group">
					<label for="so-time">Closes at</label>
					<input id="so-time" type="datetime-local" bind:value={fScheduledAt} />
				</div>
			</div>

			<div class="form-row">
				<div class="form-group">
					<label for="so-dir">Reroute direction</label>
					<input id="so-dir" type="text" bind:value={fDirection} placeholder="West" />
				</div>
				<div class="form-group">
					<label for="so-dest">Reroute destination</label>
					<input id="so-dest" type="text" bind:value={fDestination} placeholder="55-mile route" list="ride-routes-list" />
					{#if $rideConfig}
						<datalist id="ride-routes-list">
							{#each $rideConfig.routes as r (r.id)}<option value={r.name}></option>{/each}
						</datalist>
					{/if}
				</div>
			</div>

			<div class="form-group">
				<label for="so-instr">On-air instructions</label>
				<textarea id="so-instr" rows="2" bind:value={fInstructions} placeholder="Riders not past Benson: turn West onto the 55-mile route."></textarea>
			</div>

			<div class="form-group">
				<label for="so-staff">Staffed by</label>
				<select id="so-staff" bind:value={fStaffedBy}>
					<option value="">&mdash; unassigned &mdash;</option>
					{#each $activeCheckIns as ci (ci.id)}
						<option value={ci.id}>{ci.tacticalCall || ci.callsign}{ci.category ? ` (${ci.category})` : ''}</option>
					{/each}
				</select>
			</div>

			{#if fError}<div class="form-error" role="alert">{fError}</div>{/if}

			<div class="form-actions">
				<button class="btn-secondary" onclick={() => { formOpen = false; resetForm(); }} disabled={fSaving}>Cancel</button>
				<button class="btn-primary" onclick={submitForm} disabled={fSaving} aria-busy={fSaving}>{fSaving ? 'Saving…' : editingId ? 'Save changes' : 'Create shutoff'}</button>
			</div>
		</div>
	{/if}

	{#if shutoffs.length === 0 && !formOpen}
		<p class="sb-empty">No shutoffs configured.</p>
	{/if}

	{#each groups as group (group.heading)}
		{#if group.items.length > 0}
			<h3 class="sb-group-heading">{group.heading}</h3>
			<div class="sb-list">
				{#each group.items as sp (sp.id)}
					{@const meta = shutoffStatusMeta[sp.status]}
					{@const msLeft = Date.parse(sp.scheduledAt) - $secondClock}
					<div class="shutoff-card" class:fired={sp.status === 'fired'} class:cancelled={sp.status === 'cancelled'} role="group">
						<div class="sc-head">
							<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
								<title>{meta.label}</title>
								<g stroke="currentColor" stroke-width="1.5" stroke-dasharray={meta.dashed ? '3 3' : undefined} style="color: var({meta.colorVar})">
									<path d={meta.glyph} />
								</g>
							</svg>
							{#if onFlyTo}
								<button class="sc-name" onclick={() => onFlyTo(sp.lat, sp.lon, 14)}>{sp.name}</button>
							{:else}
								<span class="sc-name">{sp.name}</span>
							{/if}
							{#if sp.routeMile != null}<span class="sc-mile">mi {sp.routeMile.toFixed(1)}</span>{/if}
						</div>
						<p class="sc-sub">
							{#if sp.status === 'planned'}
								Scheduled {clock(sp.scheduledAt)}
								{#if msLeft < 90 * 60000}
									<span class="countdown" class:soon={countdownTone(sp.scheduledAt, $secondClock) === 'soon'} class:urgent={countdownTone(sp.scheduledAt, $secondClock) === 'urgent' || countdownTone(sp.scheduledAt, $secondClock) === 'expired'}>
										&minus;{countdown(sp.scheduledAt, $secondClock).text}
									</span>
								{/if}
							{:else if sp.status === 'fired'}
								<output>FIRED {clock(sp.firedAt)}{sp.firedBy ? ` · ${sp.firedBy}` : ''}{sp.rerouteCount ? ` · ${sp.rerouteCount} rerouted` : ''}</output>
							{:else}
								<span class="strike">Scheduled {clock(sp.scheduledAt)}</span>
							{/if}
						</p>
						{#if sp.rerouteDirection || sp.rerouteDestination}
							<p class="sc-reroute">&rarr; {sp.rerouteDirection} onto {sp.rerouteDestination}</p>
						{/if}
						{#if sp.staffedByCheckInId}<p class="sc-staff">Staffed by {checkInLabel(sp.staffedByCheckInId) || sp.staffedByCheckInId}</p>{/if}

						<div class="sc-actions">
							{#if sp.status === 'planned'}
								{#if $canOperate}
									<button class="sc-btn sc-btn-danger" onclick={() => (fireDialogFor = sp)}>FIRE</button>
									<button class="sc-btn" onclick={() => openEdit(sp)}>Edit</button>
									{#if cancelOpenId === sp.id}
										<input class="sc-input" type="text" placeholder="Why? (optional)" bind:value={cancelReason} />
										<button class="sc-btn" onclick={() => submitCancel(sp.id)}>Confirm cancel</button>
										<button class="sc-btn" onclick={() => (cancelOpenId = null)}>Keep</button>
									{:else}
										<button class="sc-btn" onclick={() => { cancelOpenId = sp.id; cancelReason = ''; }}>Cancel&hellip;</button>
									{/if}
									{#if deleteConfirmId === sp.id}
										<span class="sc-confirm-text">Delete {sp.name}?</span>
										<button class="sc-btn sc-btn-danger" onclick={() => doDelete(sp.id)}>Delete</button>
										<button class="sc-btn" onclick={() => (deleteConfirmId = null)}>Keep</button>
									{:else}
										<button class="sc-btn" onclick={() => (deleteConfirmId = sp.id)}>Delete</button>
									{/if}
								{/if}
							{:else if sp.status === 'cancelled'}
								{#if $canOperate}<button class="sc-btn" onclick={() => doReinstate(sp)}>Reinstate</button>{/if}
							{:else if sp.status === 'fired' && $isNetNcs}
								{#if unfireOpenId === sp.id}
									<input class="sc-input" type="text" placeholder="Reason (required)" bind:value={unfireReason} />
									<button class="sc-btn" onclick={() => submitUnfire(sp)} disabled={unfireReason.trim() === ''}>Confirm un-fire</button>
									<button class="sc-btn" onclick={() => (unfireOpenId = null)}>Cancel</button>
								{:else}
									<button class="sc-btn" onclick={() => { unfireOpenId = sp.id; unfireReason = ''; }}>Un-fire&hellip;</button>
								{/if}
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{/each}
</div>

{#if fireDialogFor}
	<ShutoffFireDialog shutoff={fireDialogFor} onClose={() => (fireDialogFor = null)} />
{/if}

<style>
	.sb { display: flex; flex-direction: column; gap: var(--space-sm); padding: var(--space-md); }
	.sb-toolbar { display: flex; }
	.sb-new {
		min-height: 40px; padding: 0 var(--space-md);
		background: var(--color-accent); border: none; border-radius: var(--radius-sm);
		color: var(--color-on-accent); font-weight: 700; cursor: pointer;
	}
	.sb-empty { color: var(--color-text-muted); font-size: 0.82rem; }
	.sb-group-heading {
		font-size: 0.68rem; text-transform: uppercase; letter-spacing: 0.06em;
		color: var(--color-text-muted); margin: var(--space-sm) 0 0;
	}
	.sb-list { display: flex; flex-direction: column; gap: 6px; }

	.sb-form {
		display: flex; flex-direction: column; gap: var(--space-sm);
		padding: var(--space-sm); border: 1px solid var(--color-primary); border-radius: var(--radius-sm);
		background: var(--color-bg);
	}
	.form-group { display: flex; flex-direction: column; gap: var(--space-xs); }
	.form-label { font-size: 0.75rem; color: var(--color-text-muted); }
	.form-row { display: flex; gap: var(--space-sm); }
	.form-row > .form-group { flex: 1; min-width: 0; }
	.loc-mode { display: flex; gap: var(--space-md); font-size: 0.78rem; }
	label { font-size: 0.75rem; color: var(--color-text-muted); }
	input, select, textarea {
		min-height: 36px; background: var(--color-surface); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); font: inherit; font-size: 0.82rem;
		padding: 0 var(--space-sm);
	}
	textarea { padding: var(--space-sm); resize: vertical; min-height: unset; }
	.form-error { color: var(--color-error-text); font-size: 0.78rem; }
	.form-actions { display: flex; justify-content: flex-end; gap: var(--space-sm); }
	.btn-secondary {
		min-height: 40px; padding: 0 var(--space-md); background: var(--color-bg);
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); color: var(--color-text);
		font-weight: 600; cursor: pointer;
	}
	.btn-primary {
		min-height: 40px; padding: 0 var(--space-md); background: var(--color-accent);
		border: none; border-radius: var(--radius-sm); color: var(--color-on-accent); font-weight: 700; cursor: pointer;
	}
	.btn-primary:disabled, .btn-secondary:disabled { opacity: 0.5; cursor: not-allowed; }

	.shutoff-card {
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm);
		padding: var(--space-sm); display: flex; flex-direction: column; gap: var(--space-xs);
	}
	.shutoff-card.fired { border-left: 3px solid var(--color-error); background: color-mix(in srgb, var(--color-error) 10%, transparent); }
	.shutoff-card.cancelled { opacity: 0.75; }
	.sc-head { display: flex; align-items: center; gap: var(--space-sm); }
	.sc-name { font-weight: 700; font-size: 0.85rem; background: none; border: none; color: var(--color-text); padding: 0; cursor: pointer; text-align: left; }
	span.sc-name { cursor: default; }
	.shutoff-card.cancelled .sc-name { text-decoration: line-through; color: var(--color-text-muted); }
	.sc-mile { font-size: 0.72rem; color: var(--color-text-muted); margin-left: auto; font-variant-numeric: tabular-nums; }
	.sc-sub { font-size: 0.78rem; color: var(--color-text-muted); }
	.sc-sub output { font-weight: 600; color: var(--color-text); }
	.strike { text-decoration: line-through; }
	.countdown { margin-left: 6px; font-weight: 700; }
	.countdown.soon { color: var(--color-warning); }
	.countdown.urgent { color: var(--color-error-text); }
	.sc-reroute { font-size: 0.8rem; }
	.sc-staff { font-size: 0.75rem; color: var(--color-text-muted); }
	.sc-actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; align-items: center; }
	.sc-btn {
		min-height: 34px; padding: 0 var(--space-sm); background: var(--color-bg);
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); color: var(--color-text);
		font-size: 0.75rem; font-weight: 600; cursor: pointer;
	}
	.sc-btn:disabled { opacity: 0.5; cursor: not-allowed; }
	.sc-btn-danger { color: var(--color-on-accent); background: var(--color-error); border-color: var(--color-error); }
	.sc-input {
		min-height: 34px; padding: 0 var(--space-sm); background: var(--color-surface);
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); color: var(--color-text);
		font-size: 0.75rem; width: 160px;
	}
	.sc-confirm-text { font-size: 0.78rem; color: var(--color-text-muted); }
</style>
