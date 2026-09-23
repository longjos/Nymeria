/**
 * Along-course distance ("chainage") engine.
 *
 * Pure math, no Leaflet, no network, no Svelte. Everything here runs in the
 * browser against the route LineString that is already in the annotations
 * store, because the premise of the feature is "no cell service": a server
 * round-trip per GPS fix is exactly the dependency we cannot have.
 *
 * UNITS — the shared contract with the UI layer:
 *   - every distance is METRES
 *   - every bearing is DEGREES TRUE (0 = north, increasing clockwise)
 *   - GeoJSON input is [lon, lat]; the public API speaks {lat, lon}
 */

import { haversineMeters } from './geo';
import type { Annotation } from './types';

// --- Tuning constants (exported so the UI uses the same numbers, never re-derives them) ---

/** Beyond this off-track distance we refuse to quote road miles at all. Measured
 *  on the real course: 2 km off the line can put the projection 2.2 mi wrong AND
 *  on the wrong side of the next stop, so a quiet snap would be confidently wrong. */
export const TOL_OFF_COURSE_M = 400;
/** Within this, we don't even mention being off the line — 50 m of GPS error
 *  costs <= 20 m of along-route error, invisible at 0.1 mi display precision. */
export const ON_COURSE_M = 75;
/** How far a stop annotation may sit from the line and still be "on" the course
 *  (parking-lot offsets on the real file are 0-24 m). */
export const STOP_SNAP_M = 150;
/** Two projections closer than this along the course are the same place. */
export const CANDIDATE_SEP_M = 500;
/** Survivors spread less than this are close enough that picking the nearest is safe. */
export const AMBIGUITY_M = 1600;
/** A stop counts as behind you only once you are this far past it — stops the
 *  readout flickering as you roll through an aid station. */
export const HYSTERESIS_M = 50;
/** Minimum baseline, in metres, for measuring a segment's direction of travel.
 *  Short enough to follow a real road bend, long enough that 1 m coordinate
 *  rounding cannot dominate the angle. */
export const BEARING_SPAN_M = 30;

/** A "segment" longer than this is the void between two disjoint imports, not road. */
export const GAP_SEG_M = 2000;
/** First and last vertex within this distance => the course is a loop. */
export const CLOSED_LOOP_M = 25;

/** Metres per degree of latitude, used for the local equirectangular frame. */
const M_PER_DEG = 111319.49;
/** Grid cell height in degrees of latitude (~555 m — comfortably larger than
 *  TOL_OFF_COURSE_M, so a 3x3 neighbourhood always covers the search radius). */
const CELL_DEG_LAT = 0.005;

// --- Types ---

/** Below this speed a GPS course is noise, not a direction of travel. km/h,
 *  the unit internal/aprs/position.go stores. Above walking pace, below any
 *  vehicle that is actually driving. */
export const MOVING_MIN_KMH = 5;

/**
 * A heading worth handing to resolveCandidate, or undefined.
 *
 * A parked GPS still reports a course: the live SAG van's last five beacons
 * were all 0 km/h with courses 205, 205, 216, 210 and 142. Heading is what
 * picks the leg of a shared road, so a parked heading picks it confidently and
 * at random — worse than no hint, because "ambiguous" becomes a wrong answer
 * delivered calmly. Unknown speed is treated as parked for the same reason.
 * The one gate for every caller: SAG dispatch ranking and the course rail's
 * roster projection must never disagree about whether a van is moving.
 */
export function headingHint(
	pos: { speed?: number; course?: number } | null | undefined
): number | undefined {
	if (!pos) return undefined;
	const { speed, course } = pos;
	if (typeof speed !== 'number' || !Number.isFinite(speed) || speed < MOVING_MIN_KMH) return undefined;
	if (typeof course !== 'number' || !Number.isFinite(course)) return undefined;
	return ((course % 360) + 360) % 360;
}

export type DistanceKind = 'road' | 'direct';

export interface RouteIndex {
	lats: Float64Array;
	lons: Float64Array;
	/** cum[i] = metres from the start of the line to vertex i. */
	cum: Float64Array;
	/** bearings[i] = forward bearing, degrees true, of the segment i -> i+1. */
	bearings: Float64Array;
	totalMeters: number;
	closed: boolean;
	/** Coarse spatial hash: "ix,iy" -> segment indices whose bbox touches the cell. */
	grid: Map<string, number[]>;
	cellLat: number;
	cellLon: number;
}

export interface Candidate {
	chainageMeters: number;
	offTrackMeters: number;
	segIndex: number;
	segBearing: number;
}

export interface ResolveHint {
	bearingDeg?: number;
	lastChainage?: number;
	elapsedSeconds?: number;
	speedMps?: number;
}

export interface Resolution {
	candidate: Candidate | null;
	ambiguous: boolean;
	candidates: Candidate[];
}

export interface Stop {
	id: string;
	label: string;
	shortName?: string;
	chainageMeters: number;
	offTrackMeters: number;
}

/** The categories that may be a destination. `general` is deliberately absent:
 *  GPX turn cues import as `general`, so making them structurally ineligible
 *  means the worst possible output is "no stops marked" rather than the
 *  confident lie "next stop: Left, 0.3 mi". */
export const STOP_CATEGORIES: readonly string[] = [
	'aid', 'checkpoint', 'resource', 'start', 'finish', 'shelter', 'staging'
];

/** Minimum shape projectStops needs; a full Annotation satisfies it. */
export type StopSource = Pick<Annotation, 'id' | 'label' | 'geometry'> & {
	category: string;
	shortName?: string;
};

// --- Geometry helpers ---

/** Tolerantly pull [lon, lat] pairs out of an annotation geometry field, which
 *  may be a JSON string or an already-parsed object. LineString only. */
export function parseLineString(geometry: string): [number, number][] | null {
	let geo: { type?: string; coordinates?: unknown };
	try {
		geo = typeof geometry === 'string' ? JSON.parse(geometry) : (geometry as unknown as { type?: string });
	} catch {
		return null;
	}
	if (geo?.type !== 'LineString' || !Array.isArray(geo.coordinates)) return null;
	const out: [number, number][] = [];
	for (const c of geo.coordinates as unknown[]) {
		if (!Array.isArray(c) || typeof c[0] !== 'number' || typeof c[1] !== 'number') return null;
		out.push([c[0], c[1]]);
	}
	return out;
}

function parsePoint(geometry: string): { lat: number; lon: number } | null {
	let geo: { type?: string; coordinates?: unknown };
	try {
		geo = typeof geometry === 'string' ? JSON.parse(geometry) : (geometry as unknown as { type?: string });
	} catch {
		return null;
	}
	if (geo?.type !== 'Point' || !Array.isArray(geo.coordinates)) return null;
	const [lon, lat] = geo.coordinates as [number, number];
	if (typeof lat !== 'number' || typeof lon !== 'number') return null;
	return { lat, lon };
}

/** Forward (initial) great-circle bearing a -> b, degrees true in [0, 360). */
function bearingDegrees(lat1: number, lon1: number, lat2: number, lon2: number): number {
	const toRad = Math.PI / 180;
	const p1 = lat1 * toRad;
	const p2 = lat2 * toRad;
	const dl = (lon2 - lon1) * toRad;
	const y = Math.sin(dl) * Math.cos(p2);
	const x = Math.cos(p1) * Math.sin(p2) - Math.sin(p1) * Math.cos(p2) * Math.cos(dl);
	return (Math.atan2(y, x) * 180 / Math.PI + 360) % 360;
}

/** Smallest absolute angle between two bearings, 0..180 degrees. */
export function angularDiff(a: number, b: number): number {
	return Math.abs(((a - b) % 360 + 540) % 360 - 180);
}

// --- Index ---

/** Precompute cumulative chainage, per-segment bearings and a coarse spatial
 *  hash for one route LineString. Costs ~3-5 ms for 3,800 vertices and is never
 *  rebuilt per query. Throws on fewer than 2 coordinates. */
export function buildRouteIndex(coords: [number, number][]): RouteIndex {
	if (!Array.isArray(coords) || coords.length < 2) {
		throw new Error('buildRouteIndex: need at least 2 coordinates');
	}
	const n = coords.length;
	const lats = new Float64Array(n);
	const lons = new Float64Array(n);
	for (let i = 0; i < n; i++) {
		lons[i] = coords[i][0];
		lats[i] = coords[i][1];
	}

	const cum = new Float64Array(n);
	const bearings = new Float64Array(n - 1);
	for (let i = 1; i < n; i++) {
		cum[i] = cum[i - 1] + haversineMeters(lats[i - 1], lons[i - 1], lats[i], lons[i]);
		bearings[i - 1] = bearingDegrees(lats[i - 1], lons[i - 1], lats[i], lons[i]);
	}

	// A single segment is far too short a baseline to take a direction from.
	// Exported GPX writes 5 decimal places (~1 m), so a 1 m step can differ in
	// one coordinate only and yield a bearing of exactly 90 or 180 degrees —
	// rounding noise, not a heading. On the real 47-mile course 6.8% of
	// segments are <= 2 m and 255 have an exactly axis-aligned bearing.
	//
	// Downstream, one such value is enough to trip the 120-degree checks that
	// decide which way the rider is facing, and announce the aid station BEHIND
	// them with full confidence. So measure each segment's direction across at
	// least BEARING_SPAN_M of route centred on it. Both pointers only ever move
	// forward, so this stays O(n) and inside the one-off index build.
	let a = 0;
	let b = 1;
	for (let i = 0; i < n - 1; i++) {
		if (a > i) a = i;
		while (a < i && cum[i] - cum[a] > BEARING_SPAN_M / 2) a++;
		if (b < i + 1) b = i + 1;
		while (b < n - 1 && cum[b] - cum[i + 1] < BEARING_SPAN_M / 2) b++;
		// Identical endpoints carry no direction; keep the raw segment value.
		if (lats[a] !== lats[b] || lons[a] !== lons[b]) {
			bearings[i] = bearingDegrees(lats[a], lons[a], lats[b], lons[b]);
		}
	}
	const totalMeters = cum[n - 1];
	const closed = haversineMeters(lats[0], lons[0], lats[n - 1], lons[n - 1]) <= CLOSED_LOOP_M;

	// Longitude cells are widened by 1/cos(lat) so a cell is roughly square in
	// metres; otherwise cells shrink towards the poles and the 3x3 neighbourhood
	// would stop covering the search radius.
	let minLat = lats[0], maxLat = lats[0];
	for (let i = 1; i < n; i++) {
		if (lats[i] < minLat) minLat = lats[i];
		if (lats[i] > maxLat) maxLat = lats[i];
	}
	const midLat = (minLat + maxLat) / 2;
	const cosMid = Math.max(0.01, Math.cos(midLat * Math.PI / 180));
	const cellLat = CELL_DEG_LAT;
	const cellLon = CELL_DEG_LAT / cosMid;

	const grid = new Map<string, number[]>();
	for (let i = 0; i < n - 1; i++) {
		const ix0 = Math.floor(Math.min(lons[i], lons[i + 1]) / cellLon);
		const ix1 = Math.floor(Math.max(lons[i], lons[i + 1]) / cellLon);
		const iy0 = Math.floor(Math.min(lats[i], lats[i + 1]) / cellLat);
		const iy1 = Math.floor(Math.max(lats[i], lats[i + 1]) / cellLat);
		for (let ix = ix0; ix <= ix1; ix++) {
			for (let iy = iy0; iy <= iy1; iy++) {
				const key = `${ix},${iy}`;
				const bucket = grid.get(key);
				if (bucket) bucket.push(i);
				else grid.set(key, [i]);
			}
		}
	}

	return { lats, lons, cum, bearings, totalMeters, closed, grid, cellLat, cellLon };
}

const indexCache = new Map<string, RouteIndex>();

/** Memoised buildRouteIndex, keyed by annotation id + updatedAt so an edited
 *  route rebuilds but a re-render does not.
 *
 *  `coords` may be a thunk, and callers on the per-fix path MUST pass one: the
 *  route LineString is 79 KB of JSON on the real course, and parsing it once per
 *  GPS tick costs more than the index build this memo was added to avoid. With a
 *  thunk the parse happens once per edit, like the build. */
export function getRouteIndex(
	annotationId: string,
	updatedAt: string,
	coords: [number, number][] | (() => [number, number][] | null)
): RouteIndex {
	const key = `${annotationId}:${updatedAt}`;
	const hit = indexCache.get(key);
	if (hit) return hit;
	const resolved = typeof coords === 'function' ? coords() : coords;
	if (!resolved) throw new Error('getRouteIndex: geometry is not a LineString');
	const idx = buildRouteIndex(resolved);
	// Small cache: a handful of routes at most, but don't grow without bound.
	if (indexCache.size > 8) indexCache.clear();
	indexCache.set(key, idx);
	return idx;
}

/** Test/debug hook — drops the memo table. */
export function clearRouteIndexCache(): void {
	indexCache.clear();
}

// --- Projection ---

/** Signed forward difference b - a along the course, honouring wrap on a loop. */
function chainageGap(a: number, b: number, idx: RouteIndex): number {
	const raw = Math.abs(b - a);
	if (!idx.closed || idx.totalMeters === 0) return raw;
	return Math.min(raw, idx.totalMeters - raw);
}

/**
 * Project (lat, lon) onto the route and return every distinct place it could be.
 *
 * Returns MORE THAN ONE candidate on an out-and-back or a loop: 12% of the
 * user's real 100-miler has a twin within 25 m that is 60 miles away along the
 * course, and there is no geometric way to choose — the caller must disambiguate
 * with resolveCandidate() or ask the operator.
 *
 * Empty array means "not on this route".
 */
export function projectOnRoute(
	idx: RouteIndex,
	lat: number,
	lon: number,
	tolMeters = TOL_OFF_COURSE_M
): Candidate[] {
	const n = idx.lats.length;
	if (n < 2) return [];

	// Local equirectangular frame centred on the query point: metres, flat, and
	// accurate to far better than a metre over the few hundred metres we search.
	const lat0 = lat;
	const lon0 = lon;
	const kx = Math.cos(lat0 * Math.PI / 180) * M_PER_DEG;
	const ky = M_PER_DEG;

	const ix = Math.floor(lon / idx.cellLon);
	const iy = Math.floor(lat / idx.cellLat);
	const seen = new Set<number>();
	for (let dx = -1; dx <= 1; dx++) {
		for (let dy = -1; dy <= 1; dy++) {
			const bucket = idx.grid.get(`${ix + dx},${iy + dy}`);
			if (bucket) for (const s of bucket) seen.add(s);
		}
	}
	// Nothing nearby in the hash: either genuinely off-course or a pathological
	// route (one enormous segment spanning many cells). Fall back to a full scan;
	// 10k segments is ~0.3 ms, which is affordable for the rare case.
	const segs: Iterable<number> = seen.size > 0 ? seen : range(n - 1);

	const raw: Candidate[] = [];
	for (const i of segs) {
		const segLen = idx.cum[i + 1] - idx.cum[i];
		// A >2 km hop between consecutive vertices is the void between two
		// disjoint imports, not road. Chainage still accumulates across it, but
		// you cannot be standing "on" it.
		if (segLen > GAP_SEG_M) continue;

		const ax = (idx.lons[i] - lon0) * kx;
		const ay = (idx.lats[i] - lat0) * ky;
		const bx = (idx.lons[i + 1] - lon0) * kx;
		const by = (idx.lats[i + 1] - lat0) * ky;
		const dx = bx - ax;
		const dy = by - ay;
		const l2 = dx * dx + dy * dy;
		// MANDATORY guard: the real GPX has 72 and the real KML 315 duplicated
		// consecutive vertices. Dividing by l2 here would yield NaN.
		if (l2 === 0) continue;

		// P is the origin of the frame, so (P - A) = (-ax, -ay).
		let t = (-ax * dx + -ay * dy) / l2;
		if (t < 0) t = 0;
		else if (t > 1) t = 1;
		const fx = ax + t * dx;
		const fy = ay + t * dy;
		const offTrackMeters = Math.hypot(fx, fy);
		if (offTrackMeters > tolMeters) continue;

		raw.push({
			chainageMeters: idx.cum[i] + t * segLen,
			offTrackMeters,
			segIndex: i,
			segBearing: idx.bearings[i]
		});
	}

	// Nearest first, then greedily collapse the run of adjacent segments around
	// each match into a single candidate — keeping the genuinely distinct
	// out-and-back / loop matches, which are far apart in chainage.
	raw.sort((a, b) => a.offTrackMeters - b.offTrackMeters);
	const out: Candidate[] = [];
	for (const c of raw) {
		let distinct = true;
		for (const k of out) {
			if (chainageGap(k.chainageMeters, c.chainageMeters, idx) <= CANDIDATE_SEP_M) {
				distinct = false;
				break;
			}
		}
		if (distinct) out.push(c);
	}
	return out;
}

function* range(n: number): Generator<number> {
	for (let i = 0; i < n; i++) yield i;
}

/**
 * Pick between projection candidates.
 *
 * Bearing is tried FIRST and continuity second, deliberately: the outbound and
 * return legs of an out-and-back are near-antiparallel, so heading resolves them
 * on a cold start where there is no previous fix to be continuous with.
 */
export function resolveCandidate(
	cands: Candidate[],
	hint: ResolveHint = {},
	idx?: RouteIndex
): Resolution {
	if (!cands || cands.length === 0) return { candidate: null, ambiguous: false, candidates: [] };
	if (cands.length === 1) return { candidate: cands[0], ambiguous: false, candidates: cands };

	let survivors = cands;

	// 1. Heading. >120 deg from the segment means you are travelling the other leg.
	if (typeof hint.bearingDeg === 'number' && Number.isFinite(hint.bearingDeg)) {
		const kept = survivors.filter((c) => angularDiff(c.segBearing, hint.bearingDeg as number) <= 120);
		if (kept.length === 1) return { candidate: kept[0], ambiguous: false, candidates: kept };
		if (kept.length > 0) survivors = kept;
	}

	// 2. Continuity. You cannot have travelled further than 2.5x your plausible
	//    distance since the last fix (the 2.5 is slack for a dropped fix or a
	//    stale timestamp; the 300 m floor covers a standing start).
	if (typeof hint.lastChainage === 'number' && Number.isFinite(hint.lastChainage)) {
		const maxTravel = Math.max(300, (hint.speedMps ?? 15) * (hint.elapsedSeconds ?? 0) * 2.5);
		// Wrap-aware on a loop: crossing the start/finish takes you from
		// (total - 50) m to 50 m, which is 100 m of travel and 99.9 km of raw
		// subtraction. Comparing raw would drop every candidate at exactly the
		// moment the rider starts their next lap.
		const gap = (c: Candidate) =>
			idx ? chainageGap(hint.lastChainage as number, c.chainageMeters, idx)
				: Math.abs(c.chainageMeters - (hint.lastChainage as number));
		const kept = survivors.filter((c) => gap(c) <= maxTravel);
		if (kept.length === 1) return { candidate: kept[0], ambiguous: false, candidates: kept };
		if (kept.length > 0) survivors = kept;
	}

	// 3. Still several, but all within ~1 mile of each other: picking the nearest
	//    cannot produce a materially wrong answer.
	let lo = survivors[0].chainageMeters;
	let hi = lo;
	for (const c of survivors) {
		if (c.chainageMeters < lo) lo = c.chainageMeters;
		if (c.chainageMeters > hi) hi = c.chainageMeters;
	}
	if (hi - lo <= AMBIGUITY_M) {
		const best = survivors.reduce((a, b) => (b.offTrackMeters < a.offTrackMeters ? b : a));
		return { candidate: best, ambiguous: false, candidates: survivors };
	}

	// 4. Genuinely ambiguous — the caller must ask. Showing a number here would
	//    be a confident lie by up to the length of the loop.
	return { candidate: null, ambiguous: true, candidates: survivors.slice(0, 2) };
}

// --- Stops ---

/** Project every allowlisted point annotation onto the route, keeping only those
 *  that actually sit on it, ordered from the start of the course. */
export function projectStops(
	idx: RouteIndex,
	anns: StopSource[],
	maxOffTrackMeters = STOP_SNAP_M
): Stop[] {
	const out: Stop[] = [];
	for (const a of anns) {
		if (!STOP_CATEGORIES.includes(a.category)) continue;
		const pt = parsePoint(a.geometry);
		if (!pt) continue;
		const cands = projectOnRoute(idx, pt.lat, pt.lon, maxOffTrackMeters);
		if (cands.length === 0) continue;
		const best = cands[0]; // already sorted by off-track ascending
		out.push({
			id: a.id,
			label: a.label,
			shortName: a.shortName,
			chainageMeters: best.chainageMeters,
			offTrackMeters: best.offTrackMeters
		});
	}
	out.sort((a, b) => a.chainageMeters - b.chainageMeters);
	return out;
}

/** A numbered stop, as projectStopsBySequence needs it. */
export interface SequencedStop {
	id: string;
	/** Course-order number (CheckpointMeta.sequenceNumber). */
	seq: number;
	lat: number;
	lon: number;
}

export interface PlacedStop {
	id: string;
	chainageMeters: number;
	offTrackMeters: number;
	/** No placement consistent with the course order exists for this stop's
	 *  number: it is drawn where it physically is, and flagged, never hidden. */
	outOfSequence: boolean;
}

/** Chainage may tie (two stops at one place; start/finish of a loop). */
const SEQUENCE_TIE_M = 1;

/**
 * Place numbered stops on the course, choosing each stop's leg from its
 * sequence number rather than from which leg is a few metres closer.
 *
 * A stop beside a road the course uses twice has two candidate chainages, and
 * projectStops takes the nearest — so on the 100-miler's shared 28-33 / 91-96
 * mile road, "Stop 3" could be placed at mile 93, between stops 9 and 10.
 * Stops are numbered in course order, so chainage must not decrease along the
 * sequence. This chooses one candidate per stop minimising, in order:
 *   1. the number of stops that go backwards (so a genuinely mis-numbered
 *      stop costs one violation and does not drag its neighbours with it),
 *   2. total off-track distance (so nearest still wins when order allows).
 * A small exact dynamic programme: stops x candidates is tiny (tens x 1-3).
 *
 * Stops that project nowhere within `maxOffTrackMeters` are left out.
 */
export function projectStopsBySequence(
	idx: RouteIndex,
	stops: SequencedStop[],
	maxOffTrackMeters = STOP_SNAP_M
): Map<string, PlacedStop> {
	const ordered = stops
		.map((s) => ({ s, cands: projectOnRoute(idx, s.lat, s.lon, maxOffTrackMeters) }))
		.filter((x) => x.cands.length > 0)
		.sort((a, b) => a.s.seq - b.s.seq);

	const out = new Map<string, PlacedStop>();
	if (ordered.length === 0) return out;

	type Cell = { viol: number; off: number; from: number };
	const better = (a: Cell, b: Cell) => a.viol < b.viol || (a.viol === b.viol && a.off < b.off);

	const table: Cell[][] = ordered.map(() => []);
	ordered[0].cands.forEach((c, k) => {
		table[0][k] = { viol: 0, off: c.offTrackMeters, from: -1 };
	});
	for (let i = 1; i < ordered.length; i++) {
		const prev = ordered[i - 1].cands;
		ordered[i].cands.forEach((c, k) => {
			let best: Cell | null = null;
			prev.forEach((p, j) => {
				const back = c.chainageMeters < p.chainageMeters - SEQUENCE_TIE_M ? 1 : 0;
				const cell: Cell = {
					viol: table[i - 1][j].viol + back,
					off: table[i - 1][j].off + c.offTrackMeters,
					from: j
				};
				if (!best || better(cell, best)) best = cell;
			});
			table[i][k] = best as unknown as Cell;
		});
	}

	// Backtrack from the best final cell.
	const last = ordered.length - 1;
	let k = 0;
	table[last].forEach((cell, kk) => {
		if (better(cell, table[last][k])) k = kk;
	});
	const chosen: number[] = new Array(ordered.length);
	for (let i = last; i >= 0; i--) {
		chosen[i] = k;
		k = table[i][k].from;
	}

	let prevChain = -Infinity;
	ordered.forEach(({ s, cands }, i) => {
		const c = cands[chosen[i]];
		const backwards = c.chainageMeters < prevChain - SEQUENCE_TIE_M;
		out.set(s.id, {
			id: s.id,
			chainageMeters: c.chainageMeters,
			offTrackMeters: c.offTrackMeters,
			outOfSequence: backwards
		});
		// A backwards stop does not become the new floor: one mis-numbered stop
		// must not make every later, correctly numbered stop look wrong.
		if (!backwards) prevChain = c.chainageMeters;
	});
	return out;
}

/**
 * The next `count` stops ahead of `originChainage`, nearest first.
 *
 * A stop is "behind" only once you are more than HYSTERESIS_M past it, so the
 * readout does not flicker while you roll through an aid station. On a closed
 * loop "ahead" wraps; the hysteresis is applied by shifting the wrap window
 * rather than by a raw `> 0` test, so a stop 10 m behind you on a loop reads as
 * -10 m (still the nearest) instead of one-lap-minus-10-m.
 *
 * `reverse` measures the other way for a rider heading back up the course.
 */
export function nextStopsAhead(
	stops: Stop[],
	originChainage: number,
	idx: RouteIndex,
	count = 3,
	reverse = false
): Stop[] {
	const total = idx.totalMeters;
	const scored: { stop: Stop; delta: number }[] = [];
	for (const s of stops) {
		let delta = reverse ? originChainage - s.chainageMeters : s.chainageMeters - originChainage;
		if (idx.closed && total > 0) {
			delta = ((delta + HYSTERESIS_M) % total + total) % total - HYSTERESIS_M;
		}
		if (delta <= -HYSTERESIS_M) continue;
		scored.push({ stop: s, delta });
	}
	scored.sort((a, b) => a.delta - b.delta);
	return scored.slice(0, count).map((s) => s.stop);
}

/** Forward distance along the course from `origin` to `target`, wrapping on a
 *  loop. This is the number the operator reads on the radio. */
export function courseDistance(origin: number, target: number, idx: RouteIndex): number {
	if (idx.closed && idx.totalMeters > 0) {
		const d = ((target - origin) % idx.totalMeters + idx.totalMeters) % idx.totalMeters;
		return d;
	}
	return Math.abs(target - origin);
}

/** Slop tolerated at both ends before pointAtChainage refuses, in metres.
 *  `milesRemaining` arithmetic (totalMeters - miles * 1609.344) lands a few
 *  microns outside [0, total] routinely, and refusing there would make the
 *  finish line unplaceable on any course whose length is not a round number of
 *  miles. A millimetre is far below the precision of anything upstream. */
const CHAINAGE_EPS_M = 0.001;

/** The point `meters` along the course from its start — the exact inverse of
 *  Candidate.chainageMeters, and the function that turns "mile 34" into a pin.
 *
 *  Clamps to the endpoints on an open course and refuses (null) beyond
 *  CHAINAGE_EPS_M outside them, which is how a SAG location's `off-route` state
 *  is detected. Wraps on a closed loop, where "mile 34" of a 30-mile loop is a
 *  real place rather than an error.
 *
 *  Note the asymmetry with projectOnRoute: THIS direction is unambiguous. A
 *  chainage is a single scalar and names exactly one vertex pair, whereas a
 *  coordinate on an out-and-back can sit 25 m from two places 60 miles apart.
 *  Mileage-derived pins are therefore the most trustworthy points on the map. */
export function pointAtChainage(idx: RouteIndex, meters: number): { lat: number; lon: number } | null {
	const total = idx.totalMeters;
	const n = idx.cum.length;
	if (n < 2 || !Number.isFinite(meters)) return null;

	let m = meters;
	if (idx.closed && total > 0) {
		m = ((m % total) + total) % total;
	} else {
		if (m < -CHAINAGE_EPS_M || m > total + CHAINAGE_EPS_M) return null;
		m = Math.min(Math.max(m, 0), total);
	}

	// Last vertex i with cum[i] <= m.
	let lo = 0;
	let hi = n - 1;
	while (lo < hi) {
		const mid = (lo + hi + 1) >> 1;
		if (idx.cum[mid] <= m) lo = mid;
		else hi = mid - 1;
	}
	if (lo >= n - 1) return { lat: idx.lats[n - 1], lon: idx.lons[n - 1] };

	// Duplicate vertices make zero-length segments; walk forward off them so the
	// interpolation below never divides by zero.
	let i = lo;
	while (i < n - 2 && idx.cum[i + 1] === idx.cum[i]) i++;

	const segLen = idx.cum[i + 1] - idx.cum[i];
	const t = segLen > 0 ? (m - idx.cum[i]) / segLen : 0;
	return {
		lat: idx.lats[i] + (idx.lats[i + 1] - idx.lats[i]) * t,
		lon: idx.lons[i] + (idx.lons[i + 1] - idx.lons[i]) * t
	};
}
