/**
 * Roster members placed along the course, for the course rail's roster lane
 * (docs/course-rail-spec.md §A4). The projection itself is the pure,
 * tested rosterCourse.projectRosterOnCourse; this module only feeds it live
 * stores and owns its continuity memory.
 *
 * Kept out of ride.ts deliberately: it depends on the station store and on
 * courseGeo, and nothing in ride.ts may depend on it — the rail's LEAD and
 * SWEEP come from reported sources only, never from here.
 */
import { derived } from 'svelte/store';
import { activeCheckIns, activeNetId } from './netcontrol';
import { stations } from './stations';
import { secondClock } from './clock';
import { courseState } from './ride';
import { courseRoute, courseStops } from './courseGeo';
import {
	projectRosterOnCourse,
	type RosterMemory,
	type RosterOnCourse,
	type RosterStopPoint
} from '$lib/rosterCourse';

/** Last unambiguous placement per check-in. Cleared when the net changes, so
 *  one event's positions can never steer the next event's projection. */
let memory: RosterMemory = new Map();
let memoryNetId = '';

export const rideRosterOnCourse = derived(
	[activeCheckIns, stations, courseRoute, courseStops, courseState, activeNetId, secondClock],
	([checkIns, stationMap, route, stops, course, netId, now]): RosterOnCourse[] => {
		if (netId !== memoryNetId) {
			memory = new Map();
			memoryNetId = netId;
		}
		const stopPoints: RosterStopPoint[] = [];
		for (const s of stops.values()) {
			try {
				const g = typeof s.annotation.geometry === 'string' ? JSON.parse(s.annotation.geometry) : s.annotation.geometry;
				const [lon, lat] = g?.coordinates ?? [];
				if (Number.isFinite(lat) && Number.isFinite(lon)) stopPoints.push({ id: s.id, seq: s.seq, lat, lon });
			} catch {
				// An unreadable stop point just can't fold anyone into it.
			}
		}
		return projectRosterOnCourse({
			checkIns: checkIns.map((ci) => ({
				id: ci.id,
				callsign: ci.callsign,
				tacticalCall: ci.tacticalCall,
				category: ci.category ?? 'general',
				lat: ci.lat,
				lon: ci.lon,
				lastHeard: ci.lastHeard,
				source: ci.source
			})),
			stationsByCallsign: stationMap,
			routeIndex: route.index,
			stops: stopPoints,
			nowMs: now,
			memory,
			// The unit that filed the latest sweep report. Marked, never promoted:
			// its GPS stays an ordinary roster pip (spec R0.3).
			sweepUnitCheckInId: course?.sweep?.latestReport?.checkInId || undefined
		});
	}
);

/** How many members are at each numbered stop — the stop's staffing count. */
export const rideStopStaffing = derived(rideRosterOnCourse, (list) => {
	const out = new Map<string, number>();
	for (const r of list) if (r.state === 'at-stop' && r.stopId) out.set(r.stopId, (out.get(r.stopId) ?? 0) + 1);
	return out;
});

/** Roster members the rail cannot show, and why — counted, never silently hidden. */
export const rideRosterUnplaced = derived(rideRosterOnCourse, (list) => ({
	offCourse: list.filter((r) => r.state === 'off-course').length,
	noPosition: list.filter((r) => r.state === 'no-position').length,
	noCourse: list.filter((r) => r.state === 'no-course').length
}));
