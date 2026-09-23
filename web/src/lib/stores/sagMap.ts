/**
 * SAG on the map — derived client state (docs/sag-map-spec.md §2, §6, §7).
 *
 * Everything here is a join of things the app already has: SAG requests and
 * vehicles from `sagBoard`, positions from the net roster, and the course from
 * the net's route annotation. No new backend surface exists for this feature,
 * and none is needed.
 *
 * The rules this file must not break:
 *   - A request we cannot place goes in `sagUnplaced`, never nowhere. An
 *     overlay that silently drops what it cannot draw under-reports the board
 *     while looking authoritative.
 *   - `sagFocus` is written only on an explicit operator gesture, never from
 *     inside an `$effect` that also reads it. That is the infinite-flyTo loop.
 */
import { derived, get, writable } from 'svelte/store';
import { sagBoard, rideMode, rideProfile } from './ride';
import { activeCheckIns, netAnnotations } from './netcontrol';
import { stations } from './stations';
import { secondClock } from './clock';
import { mapSettings } from './mapSettings';
import type { Stop } from '$lib/routeDistance';
import { courseRoute, courseStops } from './courseGeo';
import {
	resolveSagPoint,
	courseMismatchNotice,
	type AnnotationPoint,
	type SagGeoContext,
	type SagPlacement
} from '$lib/sagGeo';
import {
	joinVehiclePositions,
	rankCandidates,
	DEFAULT_AVG_SPEED_MPH,
	type RankResult,
	type SagVehicleGeo
} from '$lib/sagDispatch';
import { unassignedRiders } from '$lib/rideMeta';
import type { SAGRequest } from '$lib/types';

/** The course this net is running, plus enough context to tell "no course
 *  loaded" from "several courses and the request didn't say which". One
 *  definition, in courseGeo, shared with the course rail. */
export const sagRoute = courseRoute;

/** Point annotations by id, for the resolver's rule 2. */
const annotationPoints = derived(netAnnotations, (anns) => {
	const out = new Map<string, AnnotationPoint>();
	for (const a of anns) {
		try {
			const geo = typeof a.geometry === 'string' ? JSON.parse(a.geometry) : a.geometry;
			if (geo?.type !== 'Point' || !Array.isArray(geo.coordinates)) continue;
			out.set(a.id, { lat: geo.coordinates[1], lon: geo.coordinates[0], label: a.label || a.id });
		} catch {
			// A malformed geometry is one unplaceable pin, not a broken overlay.
		}
	}
	return out;
});

/** Numbered stops in course order, for the "next rest stop" rule. Placed by
 *  courseGeo: live annotations (a moved stop moves here too), and each on the
 *  leg its sequence number implies rather than the nearer of a shared road. */
const sagStops = derived(courseStops, (stops) =>
	Array.from(stops.values(), (s): Stop => ({
		id: s.id,
		label: s.annotation.label,
		shortName: s.annotation.shortName,
		chainageMeters: s.chainageMeters,
		offTrackMeters: s.offTrackMeters
	})).sort((a, b) => a.chainageMeters - b.chainageMeters)
);

const geoContext = derived(
	[sagRoute, annotationPoints, sagStops],
	([route, points, stops]): SagGeoContext => ({
		routeIndex: route.index,
		routeCount: route.count,
		routeName: route.name,
		annotationPoints: points,
		stops
	})
);

export interface PlacedRequest {
	request: SAGRequest;
	pickup: SagPlacement;
	dropoff: SagPlacement;
	/** Riders still standing at the roadside with no vehicle assigned. */
	waiting: number;
}

/** Every open request with both ends resolved. Complete and cancelled requests
 *  are not drawn: the map shows work in progress, not history. */
export const sagPlaced = derived([sagBoard, geoContext], ([board, ctx]) => {
	const out: PlacedRequest[] = [];
	for (const r of board?.requests ?? []) {
		if (r.status === 'complete' || r.status === 'cancelled') continue;
		out.push({
			request: r,
			pickup: resolveSagPoint(r.pickup, ctx),
			dropoff: resolveSagPoint(r.dropoff, ctx),
			waiting: unassignedRiders(r)
		});
	}
	return out;
});

/** Open requests whose PICKUP could not be placed. These are invisible
 *  everywhere else on the map, so the dock is their only home and it lists
 *  them above everything, whatever their tier. */
export const sagUnplaced = derived(sagPlaced, (placed) => placed.filter((p) => !p.pickup.placed));

/** The one-time notice when the imported course does not measure what the
 *  event advertises. */
export const sagCourseNotice = derived([sagRoute, rideProfile], ([route, profile]) => {
	if (!route.index) return null;
	// The advertised length comes from the net's ride config; the measured one
	// from the imported GPX. Match by name where the config names its routes,
	// so a two-route event does not compare the 48 against the 100.
	const routes = profile?.rideConfig?.routes ?? [];
	const want = route.name.trim().toLowerCase();
	const cfg = routes.find((r) => (r.name ?? '').trim().toLowerCase() === want) ?? routes[0];
	return courseMismatchNotice(route.index.totalMeters, cfg?.distanceMiles ?? null);
});

/**
 * SAG vehicles joined to their live positions.
 *
 * A vehicle is a net check-in, so the position comes from the roster, and the
 * heading hint (which resolves an out-and-back) comes from the APRS station
 * behind it. A vehicle with no position is still returned, with no coordinates:
 * the candidate panel lists it, greyed, with a reason.
 */
export const sagVehicleGeo = derived(
	[sagBoard, activeCheckIns, stations],
	([board, checkIns, stationMap]): SagVehicleGeo[] =>
		joinVehiclePositions(board?.vehicles ?? [], checkIns, stationMap)
);

/** Vehicles that have a position, for the map layer. */
export const sagVehiclesOnMap = derived(sagVehicleGeo, (all) =>
	all.filter((g) => g.lat != null && g.lon != null && g.vehicle.checkInStatus !== 'released')
);

// ---- focus ----

export type SagFocusMode = 'select' | 'dispatch';

export interface SagFocus {
	/** The request being looked at, or null when the operator has selected a
	 *  vehicle rather than a request. */
	requestId: string | null;
	/** The vehicle being looked at. Set by a chit click, and the thing that
	 *  makes `sagLegLines: 'selected'` mean "this van's legs" as well as "this
	 *  request's legs" (spec §5.2, §7). */
	vehicleCheckInId: string | null;
	mode: SagFocusMode;
}

/**
 * What the operator is looking at. Written ONLY from an explicit gesture —
 * a marker click, a board row click, a keyboard shortcut. Never from an
 * `$effect` that also reads it, which is the classic runaway-flyTo bug.
 *
 * Dispatch focus is deliberately not persisted: it is a transient mode, not a
 * setting.
 */
export const sagFocus = writable<SagFocus | null>(null);

export function focusSagRequest(requestId: string, mode: SagFocusMode = 'select'): void {
	sagFocus.set({ requestId, vehicleCheckInId: null, mode });
}

/**
 * Select a vehicle (spec §7, row 2). A chit click is not a request selection:
 * the operator is asking "what is THIS van doing", and the answer is the
 * selection ring plus that van's own leg lines — which may belong to a request
 * that is not, and should not become, the focused one.
 */
export function focusSagVehicle(vehicleCheckInId: string): void {
	sagFocus.set({ requestId: null, vehicleCheckInId, mode: 'select' });
}

export function clearSagFocus(): void {
	sagFocus.set(null);
}

/** Enter dispatch focus for whatever is currently selected, or for `id`. */
export function startDispatchFocus(id?: string): void {
	const current = get(sagFocus);
	const requestId = id ?? current?.requestId;
	if (!requestId) return;
	sagFocus.set({ requestId, vehicleCheckInId: null, mode: 'dispatch' });
}

export const sagFocusedRequest = derived([sagFocus, sagPlaced], ([focus, placed]) =>
	focus?.requestId ? (placed.find((p) => p.request.id === focus.requestId) ?? null) : null
);

/** The selected vehicle's check-in id, or null. Read by the map for the chit's
 *  selection ring and for "the legs of the focused vehicle". */
export const sagFocusedVehicleId = derived(sagFocus, (f) => f?.vehicleCheckInId ?? null);

/** Average road speed used for every ETA on screen. One number, not per
 *  vehicle, and never derived from APRS speed — a van parked at a rest stop
 *  reports 0 mph, which would give the most available vehicle an infinite ETA. */
export const sagAvgSpeedMph = writable<number>(DEFAULT_AVG_SPEED_MPH);

/** The ranked candidate list for the focused request. Recomputed on the shared
 *  second clock so staleness ages live rather than at whatever moment the last
 *  websocket frame happened to arrive. */
export const sagCandidates = derived(
	[sagFocusedRequest, sagVehicleGeo, sagRoute, sagAvgSpeedMph, secondClock],
	([focused, vehicles, route, avgSpeedMph, now]): RankResult | null => {
		if (!focused || !focused.pickup.placed) return null;
		let needRacks = 0;
		for (const s of focused.request.slots) {
			if (s.disposition === 'waiting' && !s.legId && s.bike === 'with_rider') needRacks++;
		}
		return rankCandidates(vehicles, {
			routeIndex: route.index,
			pickup: {
				lat: focused.pickup.lat,
				lon: focused.pickup.lon,
				chainageMeters: focused.pickup.chainageMeters
			},
			needSeats: Math.max(1, focused.waiting),
			needRacks,
			nowMs: now,
			avgSpeedMph
		});
	}
);

// ---- layer toggle ----

/** The SAG layer's persisted settings live in the existing mapSettings blob,
 *  beside every other map toggle — one place the operator looks, and one
 *  indicator dot that can warn them SAG is hidden. */
export const showSagOverlay = derived(mapSettings, (s) => s.showSagOverlay);
export const sagLegLines = derived(mapSettings, (s) => s.sagLegLines);

/** The overlay draws only in ride mode: SAG is meaningless in a general APRS
 *  net and would be one more thing to turn off. */
export const sagOverlayActive = derived(
	[rideMode, mapSettings],
	([isRide, s]) => isRide && s.showSagOverlay
);

/** The dock is a separate toggle from the markers — an operator who wants the
 *  full map can hide the column without losing the pins. */
export const sagDockActive = derived(
	[rideMode, mapSettings],
	([isRide, s]) => isRide && s.showSagOverlay && s.showSagDock
);
