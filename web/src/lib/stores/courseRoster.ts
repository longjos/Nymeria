/**
 * The course rail's model for the MAIN app — the ride strip and the
 * SituationBoard panel — built from live stores by the pure
 * railModel.buildRailModel. The agency dashboard builds the same model from
 * its own data, so all three rails show one course one way.
 *
 * Kept out of ride.ts deliberately: nothing in ride.ts may depend on the
 * roster projection — the rail's LEAD and SWEEP come from reported sources
 * only ("We'd always take the reported sweep over the GPS").
 */
import { derived } from 'svelte/store';
import { activeCheckIns, activeNetId, orderedCheckpoints } from './netcontrol';
import { annotations } from './annotations';
import { stations } from './stations';
import { secondClock } from './clock';
import { courseState, ridePhase } from './ride';
import { courseRoute } from './courseGeo';
import { railIncidentPins } from './sagMap';
import { wxInAreaAlerts } from './wxAlerts';
import { buildRailModel, type RailModel } from '$lib/railModel';
import type { RosterMemory } from '$lib/rosterCourse';

/** Last unambiguous placement per check-in. Cleared when the net changes, so
 *  one event's positions can never steer the next event's projection. */
let memory: RosterMemory = new Map();
let memoryNetId = '';

export const rideRailModel = derived(
	[
		courseRoute,
		orderedCheckpoints,
		annotations,
		courseState,
		activeCheckIns,
		stations,
		railIncidentPins,
		wxInAreaAlerts,
		ridePhase,
		activeNetId,
		secondClock
	],
	([route, cps, live, course, checkIns, stationMap, incidents, wx, phase, netId, now]): RailModel => {
		if (netId !== memoryNetId) {
			memory = new Map();
			memoryNetId = netId;
		}
		return buildRailModel({
			routeIndex: route.index,
			checkpoints: cps,
			liveAnnotations: live,
			courseState: course,
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
			rosterMemory: memory,
			incidents,
			wxAlerts: wx,
			phase: phase?.phase ?? null,
			nowMs: now
		});
	}
);
