// Course closure client state (WP7, part 2: shutoffs, rest-stop ladder,
// sweep reports, rider exceptions, reconciliation). One net at a time.
//
// The rolling accumulator itself (CourseState — stations, shutoffs, sweep
// position, config) already lives in stores/ride.ts as `courseState`,
// loaded/reset by initRideStore() over the same WebSocket connection this
// file reuses. This module does NOT refetch or re-derive that state; it
// only adds what ride.ts has no reason to carry:
//   - the rider-exception log and sweep-report history (lists, not part of
//     CourseState — governing fact 1: bibs live ONLY here, never on a
//     status tile);
//   - the write actions course.go's handlers expose (shutoff CRUD/fire/
//     cancel/reinstate, rider record/status, sweep report, station
//     riders-clear/reopen); markSweepPassed/closeStation are ride.ts's
//     (re-exported below) so there is exactly one implementation of the
//     sweep-gate refusal handling.
import { writable, derived, get } from 'svelte/store';
import { api, ApiError } from '$lib/api';
import { wsClient } from './stations';
import { activeNetId } from './netcontrol';
import {
	courseState, rideMode, closeout, accounting,
	closeStation as rideCloseStation, markSweepPassed as rideMarkSweepPassed
} from './ride';
import { showToast } from './toast';
import type { RiderException, SweepReport, ShutoffPoint, CourseConfig } from '$lib/types';

export const markSweepPassed = rideMarkSweepPassed;
export const closeStation = rideCloseStation;

// ---- derived views over ride.ts's courseState (never re-fetched here) ----

export const courseStations = derived(courseState, (s) => s?.stations ?? []);
export const shutoffsList = derived(courseState, (s) => s?.shutoffs ?? []);
export const nextShutoffD = derived(courseState, (s) => s?.nextShutoff ?? null);
export const sweepPositionD = derived(courseState, (s) => s?.sweep ?? null);
export const courseConfigD = derived(courseState, (s) => s?.config ?? null);
export const stationsAwaitingSweep = derived(courseStations, (list) =>
	list.filter((s) => s.closure.state === 'riders_clear')
);
export const plannedShutoffCount = derived(shutoffsList, (list) => list.filter((s) => s.status === 'planned').length);

// ---- lists course.go owns that CourseState does not carry ----

export const riderExceptions = writable<RiderException[]>([]);
export const sweepReports = writable<SweepReport[]>([]);
export type CourseListLoad = 'idle' | 'loading' | 'ready' | 'unavailable' | 'error';
export const courseListLoad = writable<CourseListLoad>('idle');

function upsertRider(r: RiderException): void {
	riderExceptions.update((list) => {
		const idx = list.findIndex((x) => x.id === r.id);
		return idx >= 0 ? list.map((x, i) => (i === idx ? r : x)) : [r, ...list];
	});
}

async function loadCourseLists(netId: string): Promise<void> {
	courseListLoad.set('loading');
	try {
		const [riders, sweep] = await Promise.all([api.riderExceptions(netId), api.sweep(netId, 20)]);
		riderExceptions.set(riders ?? []);
		sweepReports.set(sweep?.reports ?? []);
		courseListLoad.set('ready');
	} catch (e) {
		courseListLoad.set(e instanceof ApiError && e.status === 503 ? 'unavailable' : 'error');
	}
}

// ---- closeout/accounting refresh (debounced — several WS events can fire
// in a burst right before a net closes) ----

let closeoutTimer: ReturnType<typeof setTimeout> | null = null;

export function refreshCloseout(): void {
	const netId = get(activeNetId);
	if (!netId || !get(rideMode)) return;
	if (closeoutTimer) clearTimeout(closeoutTimer);
	closeoutTimer = setTimeout(async () => {
		closeoutTimer = null;
		try {
			closeout.set(await api.rideCloseout(netId));
			accounting.set(await api.rideAccounting(netId));
		} catch {
			// advisory close-out data — the tab degrades to its own error state
		}
	}, 400);
}

// ---- init: WS handlers + per-net list loading ----

let initialized = false;
let lastNetId: string | null = null;

export function initCourseStore(): void {
	if (initialized) return;
	initialized = true;

	activeNetId.subscribe((id) => {
		if (id === lastNetId) return;
		lastNetId = id;
		riderExceptions.set([]);
		sweepReports.set([]);
		courseListLoad.set('idle');
		if (id && get(rideMode)) loadCourseLists(id);
	});

	wsClient.on('course_rider_created', (msg) => upsertRider(msg.data as RiderException));
	wsClient.on('course_rider_updated', (msg) => upsertRider(msg.data as RiderException));
	wsClient.on('course_sweep_report', (msg) => {
		const r = msg.data as SweepReport;
		sweepReports.update((list) => [r, ...list.filter((x) => x.id !== r.id)].slice(0, 20));
	});

	wsClient.on('course_state', refreshCloseout);
	wsClient.on('checkin_updated', refreshCloseout);
	wsClient.on('sag_request_updated', refreshCloseout);
	wsClient.on('ride_handoff_item_updated', refreshCloseout);
	wsClient.on('ride_shift_summary_updated', refreshCloseout);
}

// ---- actions: rest-stop ladder ----

function stationLabel(cpId: string): string {
	return get(courseStations).find((s) => s.checkpointId === cpId)?.label ?? cpId;
}

export async function reportRidersClear(cpId: string): Promise<void> {
	const netId = get(activeNetId);
	if (!netId) return;
	try {
		await api.stationRidersClear(netId, cpId);
		showToast(`${stationLabel(cpId)}: riders clear`, 'success');
	} catch (e) {
		showToast(e instanceof ApiError ? e.message : 'Could not report riders clear', 'error');
		throw e;
	}
}

export type CloseStationResult =
	| { ok: true }
	| { ok: false; gate: 'sweep_not_passed'; stationLabel: string }
	| { ok: false; gate: 'ncs_required' }
	| { ok: false; gate: 'already_closed' }
	| { ok: false; gate: 'other'; message: string };

/**
 * Wraps ride.ts's closeStation with a typed result instead of a thrown
 * error, so StationRow can show the sweep-gate refusal as an inline
 * explanation rather than a toast-only failure. closeStation() toasts the
 * success and any UNEXPECTED failure; the gate refusals below are silent
 * there and belong to whoever called this.
 */
export async function closeStationChecked(cpId: string, opts?: { override: boolean; reason: string }): Promise<CloseStationResult> {
	try {
		await rideCloseStation(cpId, opts);
		return { ok: true };
	} catch (e) {
		if (e instanceof ApiError) {
			const code = (e.body as Record<string, unknown>)?.code;
			if (code === 'sweep_not_passed') {
				return { ok: false, gate: 'sweep_not_passed', stationLabel: (e.body as any).stationLabel ?? stationLabel(cpId) };
			}
			if (code === 'ncs_or_admin_required') return { ok: false, gate: 'ncs_required' };
			if (code === 'illegal_transition') return { ok: false, gate: 'already_closed' };
			return { ok: false, gate: 'other', message: e.message };
		}
		return { ok: false, gate: 'other', message: 'Station close failed' };
	}
}

export async function reopenStation(cpId: string, reason: string): Promise<void> {
	const netId = get(activeNetId);
	if (!netId) return;
	try {
		await api.stationReopen(netId, cpId, reason);
		showToast(`${stationLabel(cpId)} reopened`, 'success');
	} catch (e) {
		showToast(e instanceof ApiError ? e.message : 'Could not reopen', 'error');
		throw e;
	}
}

// ---- actions: shutoffs ----

export async function createShutoff(sp: Partial<ShutoffPoint>): Promise<ShutoffPoint> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	return api.createShutoff(netId, sp);
}

export async function updateShutoff(id: string, sp: Partial<ShutoffPoint>): Promise<ShutoffPoint> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	return api.updateShutoff(netId, id, sp);
}

export async function deleteShutoff(id: string): Promise<void> {
	const netId = get(activeNetId);
	if (!netId) return;
	await api.deleteShutoff(netId, id);
}

export async function fireShutoff(
	id: string,
	body: { note?: string; bibs?: string[]; rerouteCount?: number; bibWithheld?: boolean }
): Promise<{ shutoff: ShutoffPoint; riders: RiderException[] }> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	const result = await api.fireShutoff(netId, id, body);
	for (const r of result.riders) upsertRider(r);
	return result;
}

export async function cancelShutoff(id: string, reason: string): Promise<ShutoffPoint> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	return api.cancelShutoff(netId, id, reason);
}

export async function reinstateShutoff(id: string, reason = ''): Promise<ShutoffPoint> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	return api.reinstateShutoff(netId, id, reason);
}

// ---- actions: rider exceptions ----

export async function recordRider(input: Partial<RiderException>): Promise<RiderException> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	const created = await api.recordRider(netId, input);
	upsertRider(created);
	return created;
}

export async function setRiderSupport(id: string, supportStatus: 'supported' | 'unsupported', reason: string): Promise<RiderException> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	const updated = await api.setRiderStatus(netId, id, supportStatus, reason);
	upsertRider(updated);
	return updated;
}

// ---- actions: sweep ----

export async function reportSweep(sr: Partial<SweepReport>): Promise<SweepReport> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	const created = await api.reportSweep(netId, sr);
	sweepReports.update((list) => [created, ...list.filter((x) => x.id !== created.id)].slice(0, 20));
	return created;
}

// ---- actions: course config ----

export async function saveCourseConfig(cfg: Partial<CourseConfig>): Promise<CourseConfig> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	const saved = await api.updateCourseConfig(netId, cfg);
	courseState.update((s) => (s ? { ...s, config: saved } : s));
	return saved;
}

// ---- actions: ride closeout ----

export async function submitRideCloseout(opts: { force?: boolean; reason?: string }) {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	return api.postRideCloseout(netId, opts);
}
