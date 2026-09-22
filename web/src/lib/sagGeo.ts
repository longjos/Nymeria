/**
 * Turning a SAG location into a map point — docs/sag-map-spec.md §2.
 *
 * This is the highest-risk code in the SAG map feature, which is why it is
 * pure, tested first and lives outside any component. A wrong pin is worse
 * than no pin: net control will believe it and send a van to it.
 *
 * The governing fact is that `SAGLocation.lat/lon` are OPTIONAL and usually
 * absent. A pickup arrives over the radio as "mile 34" — route-relative, not a
 * coordinate — so nearly every pin on the map is derived. Every derivation
 * records HOW it was derived in `via`, because the map renders confidence
 * differently per source, and every failure records WHY in `reason`, because
 * an overlay that silently drops what it cannot place under-reports the board
 * while looking authoritative.
 */
import { pointAtChainage, type RouteIndex, type Stop } from './routeDistance';
import type { SAGLocation } from './types';

const METERS_PER_MILE = 1609.344;

/** Advertised vs. measured course length may differ by this fraction before
 *  the dock says so. Real GPX imports routinely run 1-3% off the number on the
 *  cue sheet. */
const COURSE_MISMATCH_TOLERANCE = 0.02;

export type PlacementVia = 'coordinate' | 'annotation' | 'mileage' | 'named-stop';

export type UnplaceableReason =
	/** Mileage given, but no route annotation is loaded at all. */
	| 'no-course'
	/** Free text or a bare kind — "just past the red barn". */
	| 'no-location'
	/** `loc.route` names a route whose geometry we do not have, or several
	 *  routes are loaded and nothing says which one the mileage refers to. */
	| 'route-unknown'
	/** Mileage resolves outside [0, totalMeters] on an open course. */
	| 'off-route';

export type SagPlacement =
	| {
			placed: true;
			lat: number;
			lon: number;
			via: PlacementVia;
			/** Distance along the course, where the placement came from or can be
			 *  projected onto it. Null for a raw coordinate, which we deliberately
			 *  do NOT project: see resolveSagPoint. */
			chainageMeters: number | null;
			label: string;
	  }
	| { placed: false; reason: UnplaceableReason; label: string };

export interface AnnotationPoint {
	lat: number;
	lon: number;
	label: string;
}

export interface SagGeoContext {
	/** The route this net is running, or null if no course is loaded. */
	routeIndex: RouteIndex | null;
	/** How many route annotations exist in the net. Used only to tell
	 *  `no-course` (zero) from `route-unknown` (several, none named). */
	routeCount: number;
	/** The name of `routeIndex`'s route, matched against `SAGLocation.route`. */
	routeName: string;
	/** Point annotations by id, for rule 2. */
	annotationPoints: Map<string, AnnotationPoint>;
	/** Course stops in chainage order, for rule 5. */
	stops: Stop[];
}

/** The kinds that name one particular place on the course. `course`, `other`
 *  and `hospital` are deliberately absent: they identify no single point, and
 *  guessing one would be the confident lie this module exists to prevent. */
const NAMED_STOP_KINDS = new Set(['next_reststop', 'start', 'finish']);

function normalizeRoute(s: string | undefined): string {
	return (s ?? '').trim().toLowerCase();
}

/** Does `loc` refer to the route we hold geometry for? */
function routeMatches(loc: SAGLocation, ctx: SagGeoContext): boolean {
	const want = normalizeRoute(loc.route);
	if (!want) {
		// No route named. Unambiguous only when exactly one route is loaded —
		// silently picking the first of three would mis-place a pin by tens of
		// miles, which is the failure this whole module is built to avoid.
		return ctx.routeCount <= 1;
	}
	return want === normalizeRoute(ctx.routeName);
}

/** Why mileage could not be used, given that mileage was supplied. */
function mileageFailure(loc: SAGLocation, ctx: SagGeoContext): UnplaceableReason {
	if (ctx.routeCount === 0) return 'no-course';
	return 'route-unknown';
}

function mileageLabel(loc: SAGLocation): string {
	if (loc.mileMarker != null) return `mi ${round1(loc.mileMarker)}`;
	if (loc.milesRemaining != null) return `${round1(loc.milesRemaining)} mi to go`;
	return '';
}

function round1(n: number): number {
	return Math.round(n * 10) / 10;
}

/** The stop rule 5 should use for this kind. `start` is the first stop on the
 *  course and `finish` the last; `next_reststop` means the next one a rider
 *  will reach, which without a rider position is the first stop we know of. */
function namedStop(kind: string, stops: Stop[]): Stop | null {
	if (stops.length === 0) return null;
	if (kind === 'finish') return stops[stops.length - 1];
	return stops[0];
}

function fallbackLabel(loc: SAGLocation): string {
	return loc.description?.trim() || mileageLabel(loc) || loc.kind || 'location';
}

/**
 * Resolve one SAG location to a map point, first rule wins:
 *
 *   1. explicit coordinates          -> `coordinate`
 *   2. a point annotation reference  -> `annotation`
 *   3. `mileMarker` on the route     -> `mileage`
 *   4. `milesRemaining` on the route -> `mileage`
 *   5. a kind naming one stop        -> `named-stop`
 *   6. otherwise                     -> unplaceable, with a reason
 *
 * Note that rules 3 and 4 are the TRUSTWORTHY ones, which is the opposite of a
 * mapping engineer's intuition. `projectOnRoute` (coordinate -> chainage) is
 * ambiguous on an out-and-back, where two points 60 miles apart along the
 * course can be 25 m apart on the ground. The inverse has no such problem: a
 * chainage is a single scalar naming exactly one vertex pair.
 */
export function resolveSagPoint(loc: SAGLocation, ctx: SagGeoContext): SagPlacement {
	// 1. Coordinates are a fact rather than a derivation, so they are taken
	//    verbatim and never range-checked against the course — a caller 40
	//    miles off route is where they actually are, and refusing that would
	//    hide a known position. chainageMeters stays null rather than guessing
	//    through the ambiguous projection.
	if (loc.lat != null && loc.lon != null) {
		return {
			placed: true,
			lat: loc.lat,
			lon: loc.lon,
			via: 'coordinate',
			chainageMeters: null,
			label: loc.description?.trim() || 'dropped pin'
		};
	}

	// 2. An annotation reference. A dangling id falls through rather than
	//    black-holing a request that also carries a good mile marker.
	if (loc.annotationId) {
		const ann = ctx.annotationPoints.get(loc.annotationId);
		if (ann) {
			return {
				placed: true,
				lat: ann.lat,
				lon: ann.lon,
				via: 'annotation',
				chainageMeters: null,
				label: ann.label
			};
		}
	}

	// 3 & 4. Mileage, either direction.
	const hasMileage = loc.mileMarker != null || loc.milesRemaining != null;
	if (hasMileage) {
		if (!ctx.routeIndex || !routeMatches(loc, ctx)) {
			const reason = mileageFailure(loc, ctx);
			return {
				placed: false,
				reason,
				label: reason === 'route-unknown' && loc.route ? `route "${loc.route}"` : mileageLabel(loc)
			};
		}
		const chainage =
			loc.mileMarker != null
				? loc.mileMarker * METERS_PER_MILE
				: ctx.routeIndex.totalMeters - (loc.milesRemaining as number) * METERS_PER_MILE;
		const pt = pointAtChainage(ctx.routeIndex, chainage);
		if (!pt) {
			return { placed: false, reason: 'off-route', label: mileageLabel(loc) };
		}
		return {
			placed: true,
			lat: pt.lat,
			lon: pt.lon,
			via: 'mileage',
			chainageMeters: chainage,
			label: mileageLabel(loc)
		};
	}

	// 5. A kind that names one particular stop.
	if (NAMED_STOP_KINDS.has(loc.kind)) {
		const stop = namedStop(loc.kind, ctx.stops);
		if (stop && ctx.routeIndex) {
			const pt = pointAtChainage(ctx.routeIndex, stop.chainageMeters);
			if (pt) {
				return {
					placed: true,
					lat: pt.lat,
					lon: pt.lon,
					via: 'named-stop',
					chainageMeters: stop.chainageMeters,
					label: stop.label
				};
			}
		}
	}

	// 6. Nothing to go on.
	return { placed: false, reason: 'no-location', label: fallbackLabel(loc) };
}

/**
 * The dock's one-time notice when the imported course does not measure what the
 * event advertises. Returns null when they agree, or when there is nothing to
 * compare against.
 *
 * Deliberately NOT auto-corrected: we cannot know whether a rest stop calling
 * in "mile 34" is reading a painted mile marker (advertised) or a bike computer
 * (measured), so scaling would be a guess applied to every pin on the map.
 * Saying so and letting the operator judge is the honest option.
 */
export function courseMismatchNotice(measuredMeters: number, advertisedMiles: number | null): string | null {
	if (!advertisedMiles || advertisedMiles <= 0 || measuredMeters <= 0) return null;
	const measuredMiles = measuredMeters / METERS_PER_MILE;
	const drift = Math.abs(measuredMiles - advertisedMiles) / advertisedMiles;
	if (drift <= COURSE_MISMATCH_TOLERANCE) return null;
	return (
		`Course measures ${measuredMiles.toFixed(1)} mi; config says ${round1(advertisedMiles)} mi — ` +
		`"miles remaining" pins use the measured length.`
	);
}
