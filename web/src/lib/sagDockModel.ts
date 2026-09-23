/**
 * The SAG dock's action and zoom logic (docs/sag-dock-design.md), kept out of
 * the component so it can be tested against the server's rules.
 *
 * The leg ladder mirrors internal/ride/status.go: dispatched -> enroute ->
 * onscene -> loaded -> delivered, forward only, forward skips allowed.
 * `enroute`/`onscene` are written by the advance verb; `loaded` ONLY by the
 * load verb and `delivered` ONLY by the deliver verb — the server refuses
 * either through advance.
 */
import type { SAGLeg, SAGRequest, SAGSlot, SAGVehicleStatus } from './types';
import { slotBike, SAG_TERMINAL_STATUSES } from './rideMeta';

export type LegVerb = 'advance' | 'load' | 'deliver';

export interface LegAction {
	verb: LegVerb;
	/** The leg status this action produces. */
	to: 'enroute' | 'onscene' | 'loaded' | 'delivered';
	/** The target state in on-air words, for "SAG 2 → On scene". */
	label: string;
	/** Load and deliver carry a payload (who, which bike, where), so they open
	 *  an inline confirm; en route / on scene commit on the press. */
	needsConfirm: boolean;
}

const ACT: Record<LegAction['to'], LegAction> = {
	enroute: { verb: 'advance', to: 'enroute', label: 'En route', needsConfirm: false },
	onscene: { verb: 'advance', to: 'onscene', label: 'On scene', needsConfirm: false },
	loaded: { verb: 'load', to: 'loaded', label: 'Loaded', needsConfirm: true },
	delivered: { verb: 'deliver', to: 'delivered', label: 'Delivered', needsConfirm: true }
};

/** The one primary action for a leg: the next rung of the ladder. */
export function nextLegAction(status: string): LegAction | null {
	switch (status) {
		case 'dispatched':
			return ACT.enroute;
		case 'enroute':
			return ACT.onscene;
		case 'onscene':
			return ACT.loaded;
		case 'loaded':
			return ACT.delivered;
		default:
			return null;
	}
}

/** Forward skips past the primary ("SAG 2 has both riders" from dispatched).
 *  Deliberately a separate list: a skip is never the default button. */
export function legSkips(status: string): LegAction[] {
	switch (status) {
		case 'dispatched':
			return [ACT.onscene, ACT.loaded];
		case 'enroute':
			return [ACT.loaded];
		default:
			return [];
	}
}

const ACTIVE_LEG = new Set(['dispatched', 'enroute', 'onscene', 'loaded']);

export function isActiveLeg(l: { status: string }): boolean {
	return ACTIVE_LEG.has(l.status);
}

/** A request's legs that still have a vehicle committed, in dispatch order. */
export function activeLegs(r: SAGRequest): SAGLeg[] {
	return r.legs
		.filter(isActiveLeg)
		.sort((a, b) => Date.parse(a.dispatchedAt) - Date.parse(b.dispatchedAt));
}

/** When the leg entered its current state. A skipped rung has no timestamp of
 *  its own, so the latest earlier one stands in. */
export function legStateSince(l: SAGLeg): string {
	switch (l.status) {
		case 'loaded':
			return l.loadedAt ?? l.onSceneAt ?? l.enrouteAt ?? l.dispatchedAt;
		case 'onscene':
			return l.onSceneAt ?? l.enrouteAt ?? l.dispatchedAt;
		case 'enroute':
			return l.enrouteAt ?? l.dispatchedAt;
		default:
			return l.dispatchedAt;
	}
}

export function slotName(s: Pick<SAGSlot, 'bib' | 'riderName'>): string {
	if (s.bib) return `Bib ${s.bib}`;
	return s.riderName || 'Rider';
}

export interface LegException {
	slotId: string;
	name: string;
	/** Slot dispositions the server's ResolveSlot accepts from here. */
	options: string[];
}

/**
 * Per-rider outcomes reachable from the dock. Mirrors slotResolutions in
 * status.go minus `cancelled` (a paperwork outcome that belongs on the board,
 * not in a moving vehicle). A waiting rider can self-resolve, decline or be
 * not found; a loaded rider can only be handed off to medical.
 */
export function legExceptions(r: SAGRequest, l: SAGLeg): LegException[] {
	const out: LegException[] = [];
	for (const s of r.slots) {
		if (s.legId !== l.id) continue;
		if (s.disposition === 'waiting') out.push({ slotId: s.id, name: slotName(s), options: ['self_resolved', 'declined', 'not_found'] });
		else if (s.disposition === 'loaded') out.push({ slotId: s.id, name: slotName(s), options: ['handed_off'] });
	}
	return out;
}

export interface VehicleJob {
	request: SAGRequest;
	leg: SAGLeg;
}

/** Vehicle check-in id -> its active jobs, oldest dispatch first. */
export function vehicleJobs(requests: SAGRequest[]): Map<string, VehicleJob[]> {
	const out = new Map<string, VehicleJob[]>();
	for (const r of requests) {
		if (SAG_TERMINAL_STATUSES.has(r.status)) continue;
		for (const l of r.legs) {
			if (!isActiveLeg(l)) continue;
			const list = out.get(l.vehicleCheckInId) ?? [];
			list.push({ request: r, leg: l });
			out.set(l.vehicleCheckInId, list);
		}
	}
	for (const list of out.values()) list.sort((a, b) => Date.parse(a.leg.dispatchedAt) - Date.parse(b.leg.dispatchedAt));
	return out;
}

export function unitLabel(v: Pick<SAGVehicleStatus, 'tacticalCall' | 'callsign'>): string {
	return v.tacticalCall || v.callsign;
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

/** Stable natural order by name. A list the operator searches by the name a
 *  driver just said must not reshuffle as states change. */
export function sortVehicles<T extends Pick<SAGVehicleStatus, 'tacticalCall' | 'callsign' | 'checkInId'>>(vs: T[]): T[] {
	return [...vs].sort((a, b) => collator.compare(unitLabel(a), unitLabel(b)) || a.checkInId.localeCompare(b.checkInId));
}

function norm(s: string): string {
	return s.toLowerCase().replace(/[\s\-_/]/g, '');
}

/**
 * The unit the operator is typing. Exact name first; a bare number means the
 * unit NUMBERED that ("2" is SAG 2, not KG4YFA-2's neighbour SAG 12); then
 * prefix, then contains — on the tactical call or the callsign.
 */
export function findUnit(query: string, vs: Pick<SAGVehicleStatus, 'tacticalCall' | 'callsign' | 'checkInId'>[]): string | null {
	const q = norm(query.trim());
	if (!q) return null;
	const names = (v: (typeof vs)[number]) => [v.tacticalCall, v.callsign].filter(Boolean).map(norm);
	const pick = (pred: (n: string) => boolean) => vs.find((v) => names(v).some(pred))?.checkInId ?? null;
	if (/^\d+$/.test(q)) {
		const hit = pick((n) => n.match(/(\d+)$/)?.[1] === q);
		if (hit) return hit;
	}
	return pick((n) => n === q) ?? pick((n) => n.startsWith(q)) ?? pick((n) => n.includes(q));
}

export interface LatLon {
	lat: number;
	lon: number;
}

type Placement = { placed: true; lat: number; lon: number } | { placed: false };
interface JobLike {
	request: SAGRequest;
	pickup: Placement | { placed: boolean; lat?: number; lon?: number };
	dropoff: Placement | { placed: boolean; lat?: number; lon?: number };
}
interface GeoLike {
	vehicle: Pick<SAGVehicleStatus, 'checkInId' | 'checkInStatus'>;
	lat?: number;
	lon?: number;
}

function pt(p: { placed: boolean; lat?: number; lon?: number }): LatLon | null {
	return p.placed && p.lat != null && p.lon != null ? { lat: p.lat, lon: p.lon } : null;
}

/**
 * A job's extent: pickup, dropoff and every active leg's vehicle that has a
 * fix. An unplaceable pickup moves the map nowhere (sag-map-spec §7) — a map
 * that jumps somewhere it cannot justify is worse than one that stays put.
 */
export function jobExtent(job: JobLike, geo: GeoLike[]): LatLon[] {
	const pickup = pt(job.pickup);
	if (!pickup) return [];
	const out: LatLon[] = [pickup];
	const drop = pt(job.dropoff);
	if (drop) out.push(drop);
	const ids = new Set(job.request.legs.filter(isActiveLeg).map((l) => l.vehicleCheckInId));
	for (const g of geo) {
		if (ids.has(g.vehicle.checkInId) && g.lat != null && g.lon != null) out.push({ lat: g.lat, lon: g.lon });
	}
	return out;
}

/** Every open, placed pickup and every checked-in SAG vehicle with a fix. */
export function allSagExtent(jobs: JobLike[], geo: GeoLike[]): LatLon[] {
	const out: LatLon[] = [];
	for (const j of jobs) {
		const p = pt(j.pickup);
		if (p) out.push(p);
	}
	for (const g of geo) {
		if (g.vehicle.checkInStatus === 'released' || g.lat == null || g.lon == null) continue;
		out.push({ lat: g.lat, lon: g.lon });
	}
	return out;
}

export function vehicleExtent(g: GeoLike): LatLon[] {
	return g.lat != null && g.lon != null ? [{ lat: g.lat, lon: g.lon }] : [];
}

/** A lone point becomes a ~1.3 km box, so fitting it lands near z14 rather
 *  than slamming to the fit's max zoom. */
export function padSinglePoint(pts: LatLon[], deg = 0.006): LatLon[] {
	if (pts.length !== 1) return pts;
	const { lat, lon } = pts[0];
	return [
		{ lat: lat - deg, lon: lon - deg },
		{ lat: lat + deg, lon: lon + deg }
	];
}

// ---- load / deliver drafts ----

export type LoadState = 'aboard' | 'stays' | 'declined';

export interface LoadLine {
	slotId: string;
	name: string;
	state: LoadState;
	bike: string;
}

/** Every rider still waiting on this leg, prefilled as aboard with the bike
 *  the request assumed — so the common answer is a single confirm. */
export function loadDraft(r: SAGRequest, l: SAGLeg): LoadLine[] {
	const out: LoadLine[] = [];
	for (const sid of l.slotIds) {
		const s = r.slots.find((x) => x.id === sid);
		if (!s || s.disposition !== 'waiting') continue;
		out.push({ slotId: s.id, name: slotName(s), state: 'aboard', bike: slotBike(s) });
	}
	return out;
}

function plural(n: number, one: string, many: string): string {
	return `${n} ${n === 1 ? one : many}`;
}

export function loadSummary(d: LoadLine[]): string {
	const aboard = d.filter((x) => x.state === 'aboard');
	const racks = aboard.filter((x) => x.bike === 'with_rider').length;
	const parts = [plural(aboard.length, 'rider', 'riders'), racks === 0 ? 'no bikes on the rack' : `${plural(racks, 'bike', 'bikes')} on the rack`];
	const stays = d.filter((x) => x.state === 'stays').length;
	const declined = d.filter((x) => x.state === 'declined').length;
	if (stays) parts.push(`${stays} stays`);
	if (declined) parts.push(`${declined} declined`);
	return parts.join(' · ');
}

/** The load call's body, plus riders to resolve as declined AFTER it — the
 *  server detaches an unloaded rider back to waiting, which is what makes
 *  that resolve legal. */
export function loadPlan(d: LoadLine[]): { slotIds: string[]; bike: Record<string, string>; decline: string[] } {
	const aboard = d.filter((x) => x.state === 'aboard');
	const bike: Record<string, string> = {};
	for (const x of aboard) bike[x.slotId] = x.bike;
	return { slotIds: aboard.map((x) => x.slotId), bike, decline: d.filter((x) => x.state === 'declined').map((x) => x.slotId) };
}

/** Riders on this leg who are aboard and can be delivered. */
export function deliverDraft(r: SAGRequest, l: SAGLeg): string[] {
	return l.slotIds.filter((sid) => r.slots.find((s) => s.id === sid)?.disposition === 'loaded');
}
