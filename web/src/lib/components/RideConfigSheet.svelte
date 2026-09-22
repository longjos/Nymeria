<script lang="ts">
	// Per-net ride configuration: event identity, routes, cutoff policy,
	// privacy, the priority ladder, and course closure policy. Replaces the
	// panel content the same way WxWatchAreaSheet.svelte does (back button,
	// not a floating dialog) — same mount/close convention as that sheet.
	import { api } from '$lib/api';
	import { showToast } from '$lib/stores/toast';
	import { saveRideConfig, effectiveTiers } from '$lib/stores/netProfile';
	import { saveCourseConfig } from '$lib/stores/course';
	import type { NetRideConfig, CourseConfig, RideRoute, PriorityTier } from '$lib/types';
	import PriorityLadderEditor from './PriorityLadderEditor.svelte';

	let { netId, onClose }: { netId: string; onClose: () => void } = $props();

	let loading = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);

	let rideCfg = $state<NetRideConfig | null>(null);
	let courseCfg = $state<CourseConfig | null>(null);

	// Editable local fields (kept flat so inputs bind directly).
	let agencyName = $state('');
	let eventName = $state('');
	let eventDate = $state('');
	let routes = $state<RideRoute[]>([]);
	let courseOpensAt = $state('');
	let courseClosesAt = $state('');
	let mandatorySag = $state(false);
	let declinedSagUnsupported = $state(true);
	let cutoffNotes = $state('');
	let withholdBib = $state(false);
	let priorityTiers = $state<PriorityTier[]>([]);
	let sweepLabel = $state('SWEEP');
	let leadLabel = $state('LEAD');
	let closeRequiresSweep = $state(true);
	let autoSweepFromPassage = $state(true);

	function toLocalInput(iso?: string): string {
		if (!iso) return '';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '';
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	}

	$effect(() => {
		let cancelled = false;
		(async () => {
			try {
				const [rc, cc] = await Promise.all([api.rideConfig(netId), api.courseConfig(netId)]);
				if (cancelled) return;
				rideCfg = rc;
				courseCfg = cc;
				agencyName = rc.agencyName;
				eventName = rc.eventName;
				eventDate = rc.eventDate;
				routes = rc.routes.map((r) => ({ ...r }));
				courseOpensAt = toLocalInput(rc.cutoff.courseOpensAt);
				courseClosesAt = toLocalInput(rc.cutoff.courseClosesAt);
				mandatorySag = rc.cutoff.mandatorySagAfterCutoff;
				declinedSagUnsupported = rc.cutoff.declinedSagIsUnsupported;
				cutoffNotes = rc.cutoff.notes;
				withholdBib = rc.withholdBibOnSevereInjury;
				priorityTiers = rc.priorityTiers.map((t) => ({ ...t, examples: [...t.examples] }));
				sweepLabel = cc.sweepLabel;
				leadLabel = cc.leadLabel;
				closeRequiresSweep = cc.closeRequiresSweep;
				autoSweepFromPassage = cc.autoSweepFromPassage;
			} catch {
				showToast('Could not load ride configuration.', 'error');
			} finally {
				if (!cancelled) loading = false;
			}
		})();
		return () => {
			cancelled = true;
		};
	});

	function addRoute(): void {
		routes = [...routes, { id: '', name: '', distanceMiles: 0 }];
	}

	function updateRoute(idx: number, patch: Partial<RideRoute>): void {
		routes = routes.map((r, i) => (i === idx ? { ...r, ...patch } : r));
	}

	function removeRoute(idx: number): void {
		routes = routes.filter((_, i) => i !== idx);
	}

	function localToIso(local: string): string | undefined {
		if (!local) return undefined;
		const d = new Date(local);
		return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
	}

	async function save(): Promise<void> {
		error = null;
		saving = true;
		try {
			const rideBody: NetRideConfig = {
				netId,
				agencyName: agencyName.trim(),
				eventName: eventName.trim(),
				eventDate: eventDate.trim(),
				routes: routes.map((r) => ({ ...r, id: r.id.trim().toLowerCase(), name: r.name.trim() })),
				cutoff: {
					courseOpensAt: localToIso(courseOpensAt),
					courseClosesAt: localToIso(courseClosesAt),
					mandatorySagAfterCutoff: mandatorySag,
					declinedSagIsUnsupported: declinedSagUnsupported,
					notes: cutoffNotes.trim()
				},
				withholdBibOnSevereInjury: withholdBib,
				priorityTiers,
				updatedAt: rideCfg?.updatedAt ?? ''
			};
			await saveRideConfig(netId, rideBody);

			await saveCourseConfig({
				sweepLabel: sweepLabel.trim() || 'SWEEP',
				leadLabel: leadLabel.trim() || 'LEAD',
				closeRequiresSweep,
				autoSweepFromPassage
			});

			showToast('Ride configuration saved', 'success');
			onClose();
		} catch (e: any) {
			error = e?.message ?? 'Could not save ride configuration';
		} finally {
			saving = false;
		}
	}
</script>

<div class="rcs-sheet">
	<div class="rcs-header">
		<button class="rcs-back" onclick={onClose}>&lsaquo; Net Control</button>
		<h2 class="rcs-h2">Ride settings</h2>
	</div>

	{#if loading}
		<p class="rcs-loading">Loading&hellip;</p>
	{:else}
		<div class="rcs-body">
			<section class="rcs-section">
				<h3 class="rcs-title">Event</h3>
				<div class="form-row">
					<div class="form-group">
						<label for="rcs-agency">Agency</label>
						<input id="rcs-agency" type="text" bind:value={agencyName} />
					</div>
					<div class="form-group">
						<label for="rcs-event">Event name</label>
						<input id="rcs-event" type="text" bind:value={eventName} />
					</div>
				</div>
				<div class="form-group">
					<label for="rcs-date">Date</label>
					<input id="rcs-date" type="date" bind:value={eventDate} />
				</div>
			</section>

			<section class="rcs-section">
				<h3 class="rcs-title">Routes</h3>
				{#each routes as r, idx (idx)}
					<div class="rcs-route-row">
						<input class="rcs-route-id" type="text" value={r.id} oninput={(e) => updateRoute(idx, { id: (e.target as HTMLInputElement).value })} placeholder="id" aria-label="Route id" />
						<input class="rcs-route-name" type="text" value={r.name} oninput={(e) => updateRoute(idx, { name: (e.target as HTMLInputElement).value })} placeholder="100 Mile Century" aria-label="Route name" />
						<input class="rcs-route-mi" type="number" min="0" step="0.1" value={r.distanceMiles} oninput={(e) => updateRoute(idx, { distanceMiles: Number((e.target as HTMLInputElement).value) })} aria-label="Distance miles" />
						<button type="button" class="rcs-route-remove" onclick={() => removeRoute(idx)} aria-label="Remove route {r.name || r.id}">&times;</button>
					</div>
				{/each}
				<button type="button" class="btn-secondary" onclick={addRoute}>+ Route</button>
			</section>

			<section class="rcs-section">
				<h3 class="rcs-title">Cut-off policy</h3>
				<div class="form-row">
					<div class="form-group">
						<label for="rcs-opens">Course opens</label>
						<input id="rcs-opens" type="datetime-local" bind:value={courseOpensAt} />
					</div>
					<div class="form-group">
						<label for="rcs-closes">Course closes</label>
						<input id="rcs-closes" type="datetime-local" bind:value={courseClosesAt} />
					</div>
				</div>
				<label class="rcs-check"><input type="checkbox" bind:checked={mandatorySag} /> Mandatory SAG after cutoff</label>
				<label class="rcs-check"><input type="checkbox" bind:checked={declinedSagUnsupported} /> Declined SAG counts as unsupported</label>
				<div class="form-group">
					<label for="rcs-notes">Notes</label>
					<textarea id="rcs-notes" rows="2" bind:value={cutoffNotes}></textarea>
				</div>
			</section>

			<section class="rcs-section">
				<h3 class="rcs-title">Privacy</h3>
				<label class="rcs-check"><input type="checkbox" bind:checked={withholdBib} /> Withhold bib on severe injury (default for new medical exceptions)</label>
			</section>

			<section class="rcs-section">
				<h3 class="rcs-title">Priority ladder</h3>
				<PriorityLadderEditor tiers={priorityTiers} shipped={$effectiveTiers} onChange={(t) => (priorityTiers = t)} />
			</section>

			<section class="rcs-section">
				<h3 class="rcs-title">Course closure policy</h3>
				<div class="form-row">
					<div class="form-group">
						<label for="rcs-sweep-label">Sweep label</label>
						<input id="rcs-sweep-label" type="text" bind:value={sweepLabel} />
					</div>
					<div class="form-group">
						<label for="rcs-lead-label">Lead label</label>
						<input id="rcs-lead-label" type="text" bind:value={leadLabel} />
					</div>
				</div>
				<label class="rcs-check"><input type="checkbox" bind:checked={closeRequiresSweep} /> Rest stops may not close until sweep passes</label>
				<label class="rcs-check"><input type="checkbox" bind:checked={autoSweepFromPassage} /> Auto-mark sweep-passed when the sweep label is logged at a stop</label>
			</section>

			{#if error}<div class="form-error" role="alert">{error}</div>{/if}
		</div>

		<div class="rcs-footer">
			<button class="btn-secondary" onclick={onClose} disabled={saving}>Cancel</button>
			<button class="btn-primary" onclick={save} disabled={saving} aria-busy={saving}>{saving ? 'Saving…' : 'Save'}</button>
		</div>
	{/if}
</div>

<style>
	.rcs-sheet { display: flex; flex-direction: column; height: 100%; min-height: 0; }
	.rcs-header { display: flex; align-items: center; gap: var(--space-sm); padding: var(--space-sm) var(--space-md); border-bottom: 1px solid var(--color-primary); }
	.rcs-back { background: none; border: none; color: var(--color-text-muted); font-size: 0.8rem; cursor: pointer; }
	.rcs-h2 { font-size: 0.9rem; font-weight: 700; }
	.rcs-loading { padding: var(--space-md); color: var(--color-text-muted); }
	.rcs-body { flex: 1; overflow-y: auto; padding: var(--space-sm) var(--space-md); display: flex; flex-direction: column; gap: var(--space-md); }
	.rcs-section { display: flex; flex-direction: column; gap: 6px; }
	.rcs-title { font-size: 0.7rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.03em; color: var(--color-text-muted); }

	.form-row { display: flex; gap: var(--space-sm); }
	.form-row > .form-group { flex: 1; min-width: 0; }
	.form-group { display: flex; flex-direction: column; gap: var(--space-xs); }
	label { font-size: 0.75rem; color: var(--color-text-muted); }
	input, textarea {
		min-height: 40px; background: var(--color-bg); border: 1px solid var(--color-primary); border-radius: var(--radius-sm);
		color: var(--color-text); font: inherit; font-size: 0.85rem; padding: 0 var(--space-sm);
	}
	textarea { padding: var(--space-sm); resize: vertical; min-height: unset; }
	.rcs-check { display: flex; align-items: center; gap: 6px; font-size: 0.82rem; color: var(--color-text); }

	.rcs-route-row { display: flex; gap: 6px; align-items: center; }
	.rcs-route-id { width: 90px; }
	.rcs-route-name { flex: 1; min-width: 0; }
	.rcs-route-mi { width: 90px; }
	.rcs-route-remove {
		width: 40px; height: 40px; background: var(--color-bg); border: 1px solid var(--color-error);
		border-radius: var(--radius-sm); color: var(--color-error-text); cursor: pointer;
	}

	.form-error { color: var(--color-error-text); font-size: 0.8rem; }

	.rcs-footer { display: flex; justify-content: flex-end; gap: var(--space-sm); padding: var(--space-sm) var(--space-md); border-top: 1px solid var(--color-primary); }
	.btn-secondary {
		min-height: 44px; padding: 0 var(--space-md); background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); font-weight: 600; cursor: pointer;
	}
	.btn-primary {
		min-height: 44px; padding: 0 var(--space-md); background: var(--color-accent); border: none;
		border-radius: var(--radius-sm); color: var(--color-on-accent); font-weight: 700; cursor: pointer;
	}
	.btn-primary:disabled, .btn-secondary:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
