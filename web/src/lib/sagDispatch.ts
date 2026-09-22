/**
 * Ranking the vehicles that could take a SAG request — docs/sag-map-spec.md §6.
 *
 * This is the product: the whole feature exists so that "which van do I send?"
 * is answered by the app instead of by the operator holding a map in their head
 * while someone reads a bib number over the radio.
 *
 * Pure and synchronous. No Leaflet, no stores, no Svelte — everything here is
 * table-testable, because the ordering rules ARE the design and a silent change
 * to them changes which vehicle gets dispatched to a person lying by the road.
 */
import {
	courseDistance,
	projectOnRoute,
	resolveCandidate,
	type DistanceKind,
	type RouteIndex
} from './routeDistance';
import { haversineMeters } from './geo';
import { ageState, type AgeState } from './rideMeta';
import type { SAGVehicleStatus } from './types';

const METERS_PER_MILE = 1609.344;

/** Beyond this the ETA stops claiming precision it does not have and renders
 *  "~45+ min" instead. [REF: NNGroup on false precision in time estimates] */
export const ETA_CAP_MINUTES = 45;

/** Default road speed for ETA, mph. One config number, deliberately NOT derived
 *  from APRS speed: a van parked at a rest stop reports 0 mph, which would
 *  produce an infinite ETA for the vehicle most able to respond. */
export const DEFAULT_AVG_SPEED_MPH = 25;

/** A straight line counts as a shortcut worth mentioning when it is under this
 *  fraction of the road distance AND saves more than SHORTCUT_MIN_SAVING_M.
 *  Both guards matter: without the ratio every number differs slightly, and
 *  without the floor a 300 m saving on a 500 m trip would shout. */
const SHORTCUT_RATIO = 0.6;
const SHORTCUT_MIN_SAVING_M = 0.5 * METERS_PER_MILE;

/** The straight line only DISPLACES road miles when it wins by a margin that
 *  is real. On a straight stretch of course the two differ by centimetres
 *  (a chord against a sum of segments), and letting that flip the label would
 *  throw away the direction word over floating-point noise. */
const DIRECT_WINS_RATIO = 0.95;
const DIRECT_WINS_MIN_SAVING_M = 100;

export type Eligibility = 'eligible' | 'partial';
export type ExcludedReason = 'released' | 'no-seats' | 'no-position';
export type Direction = 'ahead' | 'back';

export interface SagVehicleGeo {
	vehicle: SAGVehicleStatus;
	lat?: number;
	lon?: number;
	/** ISO timestamp of the position, for staleness. */
	lastHeard?: string;
	source?: 'aprs' | 'voice';
	/** APRS course over ground, degrees true — the hint that resolves an
	 *  out-and-back on a cold start. */
	courseDeg?: number;
	/** This vehicle's previously resolved chainage, for continuity. */
	lastChainage?: number | null;
	/** Seconds since that previous chainage was taken. */
	elapsedSeconds?: number;
}

export interface RankContext {
	routeIndex: RouteIndex | null;
	pickup: { lat: number; lon: number; chainageMeters: number | null } | null;
	/** Riders still waiting and unassigned on this request. */
	needSeats: number;
	/** Bikes among them that travel with their rider. */
	needRacks: number;
	nowMs: number;
	avgSpeedMph: number;
}

export interface SagCandidate {
	id: string;
	vehicle: SAGVehicleStatus;
	eligibility: Eligibility;
	excludedReason?: ExcludedReason;
	/** How many of the waiting riders this vehicle could actually take. */
	seatsOffered: number;
	/** Bikes that would have no rack. Never blocks or demotes — the server
	 *  asks about racks rather than refusing, so ranking must not invent a
	 *  stricter rule than dispatch enforces. */
	rackShortfall: number;
	/** The number to act on: the SHORTER of the road and straight-line
	 *  distances, because the roads are two-way and drivers use side roads. */
	distanceMeters: number | null;
	distanceKind: DistanceKind | null;
	direction: Direction | null;
	/** Distance along the course, when the vehicle is on it. */
	routeMeters: number | null;
	/** Straight-line distance to the pickup. */
	directMeters: number | null;
	/** The straight line is materially shorter than following the course —
	 *  usually the opposite leg of an out-and-back, i.e. the same road. Worth
	 *  saying out loud, because whether a cut-through exists is local
	 *  knowledge the app does not have. */
	shortcut: boolean;
	ambiguous: boolean;
	/** The two places an ambiguous vehicle might be, in miles along the course. */
	ambiguityMiles: [number, number] | null;
	age: AgeState;
	etaMinutes: number | null;
	etaCapped: boolean;
	chainageMeters: number | null;
}

export interface RankResult {
	ranked: SagCandidate[];
	/** Rendered below a rule, greyed, with a reason — never hidden. An absence
	 *  the operator cannot explain is worse than a greyed row. */
	excluded: SagCandidate[];
	/** True when no course is loaded and every number is crow-flies. The panel
	 *  says so above the list, before any distance. */
	degraded: boolean;
}

function base(g: SagVehicleGeo, age: AgeState): SagCandidate {
	return {
		id: g.vehicle.checkInId,
		vehicle: g.vehicle,
		eligibility: 'eligible',
		seatsOffered: 0,
		rackShortfall: 0,
		distanceMeters: null,
		distanceKind: null,
		direction: null,
		routeMeters: null,
		directMeters: null,
		shortcut: false,
		ambiguous: false,
		ambiguityMiles: null,
		age,
		etaMinutes: null,
		etaCapped: false,
		chainageMeters: null
	};
}

function etaFor(meters: number | null, avgSpeedMph: number): { minutes: number | null; capped: boolean } {
	if (meters == null || !(avgSpeedMph > 0)) return { minutes: null, capped: false };
	const miles = meters / METERS_PER_MILE;
	const raw = Math.round((miles / avgSpeedMph) * 60);
	if (raw > ETA_CAP_MINUTES) return { minutes: ETA_CAP_MINUTES, capped: true };
	return { minutes: raw, capped: false };
}

/** Rank order key 4: only `stale` is a demotion. `aging` is still a position
 *  worth acting on, and treating it as suspect would demote most of the fleet
 *  most of the time on a net whose vans beacon every ten minutes. */
function isStale(age: AgeState): boolean {
	return age === 'stale' || age === 'never';
}

/**
 * Rank the vehicles that could take `request`'s waiting riders.
 *
 * Total sort, first difference wins:
 *   1. eligible (can seat the whole party) before partial capacity
 *   2. unambiguous position                before ambiguous
 *   3. fresh/aging position                before stale
 *   4. shorter effective distance          before longer
 *
 * Nothing is preselected. Ranking encodes distance, seats and staleness; it
 * cannot encode "SAG 1's driver just radioed that he's stuck behind the
 * parade", so the operator picks. [REF: automation bias]
 */
export function rankCandidates(vehicles: SagVehicleGeo[], ctx: RankContext): RankResult {
	const idx = ctx.routeIndex;
	const pickupChainage = ctx.pickup?.chainageMeters ?? null;
	const canUseRoute = idx != null && pickupChainage != null;
	const ranked: SagCandidate[] = [];
	const excluded: SagCandidate[] = [];

	for (const g of vehicles) {
		const age = ageState(g.lastHeard, ctx.nowMs);
		const c = base(g, age);
		const v = g.vehicle;

		// --- exclusions: rendered, never hidden ---
		if (v.checkInStatus === 'released') {
			excluded.push({ ...c, excludedReason: 'released' });
			continue;
		}
		if (v.availableSeats <= 0) {
			excluded.push({ ...c, excludedReason: 'no-seats' });
			continue;
		}
		if (g.lat == null || g.lon == null) {
			excluded.push({ ...c, excludedReason: 'no-position', age: 'never' });
			continue;
		}

		// --- capacity: seats are a wall, racks are a question ---
		c.seatsOffered = Math.min(v.availableSeats, Math.max(1, ctx.needSeats));
		c.eligibility = v.availableSeats >= ctx.needSeats ? 'eligible' : 'partial';
		c.rackShortfall = Math.max(0, ctx.needRacks - v.availableRacks);

		// --- position along the course ---
		if (canUseRoute) {
			const cands = projectOnRoute(idx as RouteIndex, g.lat, g.lon);
			const res = resolveCandidate(
				cands,
				{
					bearingDeg: g.courseDeg,
					lastChainage: g.lastChainage ?? undefined,
					elapsedSeconds: g.elapsedSeconds
				},
				idx as RouteIndex
			);
			if (res.ambiguous) {
				// No number. Showing one here would be a confident lie by up to
				// the length of the loop — routeDistance.ts warns about exactly
				// this case on the user's own 100-miler.
				c.ambiguous = true;
				c.ambiguityMiles = [
					res.candidates[0].chainageMeters / METERS_PER_MILE,
					res.candidates[1].chainageMeters / METERS_PER_MILE
				];
			} else if (res.candidate) {
				const vc = res.candidate.chainageMeters;
				c.chainageMeters = vc;
				const forward = courseDistance(vc, pickupChainage as number, idx as RouteIndex);
				const reverse = courseDistance(pickupChainage as number, vc, idx as RouteIndex);
				let meters: number;
				let dir: Direction;
				if ((idx as RouteIndex).closed) {
					// On a loop, driving forward is the normal case; going back
					// is only worth it when it is dramatically shorter.
					if (reverse < forward / 3) {
						meters = reverse;
						dir = 'back';
					} else {
						meters = forward;
						dir = 'ahead';
					}
				} else {
					// On an open course courseDistance is symmetric, so the
					// direction has to come from the sign — otherwise a van PAST
					// the pickup reads "2.1 mi ahead" when it has to turn round,
					// which is precisely the error the direction word exists to
					// prevent.
					meters = forward;
					dir = (pickupChainage as number) >= vc ? 'ahead' : 'back';
				}
				c.routeMeters = meters;
				c.direction = dir;
			}
		}

		// --- straight-line distance, always computed ---
		if (ctx.pickup) {
			c.directMeters = haversineMeters(g.lat, g.lon, ctx.pickup.lat, ctx.pickup.lon);
		}

		// The effective distance is the SHORTER of the two.
		//
		// This is not a shortcut in the code; it is how these events actually
		// run. The roads are two-way and drivers take side roads, so a van on
		// the return leg of an out-and-back is frequently on the SAME PHYSICAL
		// ROAD as the pickup and need only turn around — while `courseDistance`
		// insists it is thirty miles away, because that is how far a RIDER
		// would have to travel. Ranking by road miles alone would bury the
		// nearest vehicle on the user's own Jack-and-Back course.
		//
		// The straight line is never passed off as road miles: whether a
		// connecting road exists is local knowledge the app does not have, so
		// the number is labelled `direct` and the operator judges it.
		if (c.routeMeters != null && c.directMeters != null) {
			const directWins =
				c.directMeters < c.routeMeters * DIRECT_WINS_RATIO &&
				c.routeMeters - c.directMeters > DIRECT_WINS_MIN_SAVING_M;
			if (directWins) {
				c.distanceMeters = c.directMeters;
				c.distanceKind = 'direct';
				c.direction = null;
			} else {
				c.distanceMeters = c.routeMeters;
				c.distanceKind = 'road';
			}
			c.shortcut =
				c.directMeters < c.routeMeters * SHORTCUT_RATIO &&
				c.routeMeters - c.directMeters > SHORTCUT_MIN_SAVING_M;
		} else if (c.routeMeters != null) {
			c.distanceMeters = c.routeMeters;
			c.distanceKind = 'road';
		} else if (c.directMeters != null) {
			c.distanceMeters = c.directMeters;
			c.distanceKind = 'direct';
			c.direction = null;
		}
		// An ambiguous vehicle keeps its "two possible positions" badge and gets
		// no distance at all: a crow-flies number next to the ambiguity warning
		// would just be read as the answer.
		if (c.ambiguous) {
			c.distanceMeters = null;
			c.distanceKind = null;
			c.direction = null;
			c.routeMeters = null;
			c.shortcut = false;
		}

		const eta = etaFor(c.distanceMeters, ctx.avgSpeedMph);
		c.etaMinutes = eta.minutes;
		c.etaCapped = eta.capped;
		ranked.push(c);
	}

	ranked.sort((a, b) => {
		// 1. whole party before part of it
		const partial = (c: SagCandidate) => (c.eligibility === 'partial' ? 1 : 0);
		if (partial(a) !== partial(b)) return partial(a) - partial(b);
		// 2. a real position before "could be one of two places"
		if (a.ambiguous !== b.ambiguous) return a.ambiguous ? 1 : -1;
		// 3. a position we believe before one we do not
		if (isStale(a.age) !== isStale(b.age)) return isStale(a.age) ? 1 : -1;
		// 4. nearer — on the EFFECTIVE distance, each row labelled with which
		//    kind of number it is, so the units are never mixed silently
		const da = a.distanceMeters ?? Number.POSITIVE_INFINITY;
		const db = b.distanceMeters ?? Number.POSITIVE_INFINITY;
		if (da !== db) return da - db;
		return a.vehicle.tacticalCall.localeCompare(b.vehicle.tacticalCall);
	});

	return { ranked, excluded, degraded: !canUseRoute };
}

/** Minimum roster shape the join needs. */
export interface CheckInPosition {
	id: string;
	callsign: string;
	lat?: number;
	lon?: number;
	lastHeard?: string;
	source?: 'aprs' | 'voice';
}

/** Minimum station shape the join needs, for the heading hint. */
export interface StationHeading {
	position?: { course?: number };
}

/**
 * Join SAG vehicles to their live positions.
 *
 * A vehicle IS a net check-in, so its position comes from the roster and its
 * heading — the hint that resolves an out-and-back — from the APRS station
 * behind it. Every vehicle is returned whether or not it has a position: a van
 * that silently vanishes from the candidate list because its check-in could
 * not be found is the failure this function is factored out to make testable.
 */
export function joinVehiclePositions(
	vehicles: SAGVehicleStatus[],
	checkIns: CheckInPosition[],
	stationsByCallsign: Map<string, StationHeading>
): SagVehicleGeo[] {
	const byId = new Map(checkIns.map((ci) => [ci.id, ci]));
	return vehicles.map((v) => {
		const ci = byId.get(v.checkInId);
		const st = ci ? stationsByCallsign.get(ci.callsign.toUpperCase()) : undefined;
		return {
			vehicle: v,
			lat: ci?.lat,
			lon: ci?.lon,
			lastHeard: ci?.lastHeard,
			source: ci?.source,
			courseDeg: st?.position?.course
		};
	});
}
