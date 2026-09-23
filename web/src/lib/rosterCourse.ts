/**
 * Put roster members who are near the course ON the course, as a mile —
 * the course rail's roster lane (docs/course-rail-spec.md §A4).
 *
 * This answers "who do I have near mile X?", which nothing on the rail could
 * answer before: every marker came from logged passages. It reuses the SAG
 * engine's rules verbatim (projectOnRoute -> resolveCandidate, headingHint,
 * TOL_OFF_COURSE_M, STOP_SNAP_M, ageState) so the rail and the SAG dock can
 * never disagree about whether the same van is near the course, or which leg
 * it is on.
 *
 * WHAT THIS MUST NEVER DO: move LEAD or SWEEP. The user's rule is settled —
 * "We'd always take the reported sweep over the GPS." A sweep van's fix is
 * where the VAN is, not where the sweep line is. RosterOnCourse is its own
 * type and no function that changes course state (sweep report, passage,
 * closure, shutoff) accepts it (spec R0.5).
 */
import { haversineMeters } from './geo';
import {
	headingHint,
	projectOnRoute,
	resolveCandidate,
	STOP_SNAP_M,
	TOL_OFF_COURSE_M,
	type RouteIndex
} from './routeDistance';
import { ageState, type AgeState } from './rideMeta';

const METERS_PER_MILE = 1609.344;

/** The roster fields the projection needs; a NetCheckIn satisfies it. */
export interface RosterCheckIn {
	id: string;
	callsign: string;
	tacticalCall?: string;
	category: string;
	lat?: number;
	lon?: number;
	lastHeard?: string;
	source?: 'aprs' | 'voice' | string;
}

/** The APRS station behind a check-in: heading and true position age. */
export interface RosterStation {
	position?: { speed?: number; course?: number };
	track?: { time: string }[];
}

/** A numbered stop's point, for the at-stop fold. */
export interface RosterStopPoint {
	id: string;
	seq: number;
	lat: number;
	lon: number;
}

/** Last UNAMBIGUOUS placement per check-in — the continuity hint (R8). The
 *  caller owns it and keeps it across calls; cleared on net change. */
export type RosterMemory = Map<string, { chainageMeters: number; atMs: number }>;

export type RosterCourseState =
	/** Placed at one mile on the course. */
	| 'on-course'
	/** Within STOP_SNAP_M of a numbered stop: counted on the stop, no pip. */
	| 'at-stop'
	/** On a road the course uses twice and we cannot tell which leg. */
	| 'ambiguous'
	/** More than TOL_OFF_COURSE_M from the line: counted, not placed. */
	| 'off-course'
	/** There is no course line to measure against. */
	| 'no-course'
	/** No usable position at all. */
	| 'no-position';

export interface RosterOnCourse {
	checkInId: string;
	callsign: string;
	tacticalCall?: string;
	category: string;
	state: RosterCourseState;
	/** Only for 'on-course'. Never set for 'ambiguous' — there is no one mile. */
	chainageMeters?: number;
	/** Only for 'ambiguous': both possible miles, ascending. */
	candidatesMiles?: number[];
	/** Only for 'at-stop'. */
	stopId?: string;
	/** ISO time of the POSITION (newest track point), not of the last packet. */
	positionAt: string | null;
	age: AgeState;
	/** A voice check-in's position was set by hand. */
	handPlaced: boolean;
	/** The latest sweep report's unit. Still an ordinary member (R0.3). */
	isSweepUnit: boolean;
}

export interface RosterInput {
	checkIns: RosterCheckIn[];
	/** Keyed by UPPER-CASE callsign, as the station store is. */
	stationsByCallsign: Map<string, RosterStation>;
	routeIndex: RouteIndex | null;
	stops: RosterStopPoint[];
	nowMs: number;
	memory: RosterMemory;
	sweepUnitCheckInId?: string;
}

function isNullIsland(lat: number, lon: number): boolean {
	return Math.abs(lat) < 1e-6 && Math.abs(lon) < 1e-6;
}

/**
 * The newest track point is when the position was last TRUE. `lastHeard` is
 * the last packet of any kind (telemetry and status packets refresh it too),
 * so it is only the fallback (spec B19).
 */
function positionTime(ci: RosterCheckIn, st: RosterStation | undefined): string | null {
	const track = st?.track;
	if (track && track.length > 0) {
		let best = track[0].time;
		for (const p of track) if (Date.parse(p.time) > Date.parse(best)) best = p.time;
		return best;
	}
	return ci.lastHeard ?? null;
}

/**
 * Project every roster member onto the course. `memory` is UPDATED in place
 * with each unambiguous placement, so pass the same Map on the next call.
 */
export function projectRosterOnCourse(input: RosterInput): RosterOnCourse[] {
	const { routeIndex: idx, nowMs, memory } = input;
	return input.checkIns.map((ci): RosterOnCourse => {
		const st = input.stationsByCallsign.get(ci.callsign.toUpperCase());
		const handPlaced = ci.source === 'voice';
		const positionAt = positionTime(ci, st);
		let age = ageState(positionAt, nowMs);
		// R12: a hand-placed position's timestamp is refreshed by any NCS
		// interaction, not by the position being checked, so it can never be
		// vouched for as fresh.
		if (handPlaced && age === 'fresh') age = 'aging';

		const base = {
			checkInId: ci.id,
			callsign: ci.callsign,
			tacticalCall: ci.tacticalCall,
			category: ci.category,
			positionAt,
			age,
			handPlaced,
			isSweepUnit: !!input.sweepUnitCheckInId && ci.id === input.sweepUnitCheckInId
		};

		const { lat, lon } = ci;
		if (typeof lat !== 'number' || typeof lon !== 'number' || !Number.isFinite(lat) || !Number.isFinite(lon) || isNullIsland(lat, lon)) {
			return { ...base, state: 'no-position' };
		}

		// R2: at a numbered stop. Straight-line to the stop's own point, so it
		// never needs to know which leg of a shared road the stop is on.
		let nearestStop: { id: string; d: number } | null = null;
		for (const s of input.stops) {
			const d = haversineMeters(lat, lon, s.lat, s.lon);
			if (d <= STOP_SNAP_M && (!nearestStop || d < nearestStop.d)) nearestStop = { id: s.id, d };
		}
		if (nearestStop) return { ...base, state: 'at-stop', stopId: nearestStop.id };

		if (!idx) return { ...base, state: 'no-course' };

		const cands = projectOnRoute(idx, lat, lon, TOL_OFF_COURSE_M);
		if (cands.length === 0) return { ...base, state: 'off-course' };

		const mem = memory.get(ci.id);
		const res = resolveCandidate(
			cands,
			{
				bearingDeg: headingHint(st?.position),
				lastChainage: mem?.chainageMeters,
				elapsedSeconds: mem ? Math.max(0, (nowMs - mem.atMs) / 1000) : undefined
				// speedMps deliberately left to resolveCandidate's vehicle default,
				// exactly as SAG ranking does: a parked van's 0 km/h would shrink
				// the continuity window to its 300 m floor and throw away a fix
				// that is plainly the same van a few hundred metres on.
			},
			idx
		);

		if (res.ambiguous || !res.candidate) {
			const miles = res.candidates.map((c) => c.chainageMeters / METERS_PER_MILE).sort((a, b) => a - b);
			return { ...base, state: 'ambiguous', candidatesMiles: miles };
		}

		const chainageMeters = res.candidate.chainageMeters;
		const at = positionAt ? Date.parse(positionAt) : nowMs;
		memory.set(ci.id, { chainageMeters, atMs: Number.isNaN(at) ? nowMs : at });
		return { ...base, state: 'on-course', chainageMeters };
	});
}
