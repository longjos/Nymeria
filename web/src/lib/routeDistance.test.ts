import { describe, it, expect } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { haversineMeters } from './geo';
import {
	buildRouteIndex,
	getRouteIndex,
	projectOnRoute,
	resolveCandidate,
	projectStops,
	nextStopsAhead,
	courseDistance,
	pointAtChainage,
	parseLineString,
	clearRouteIndexCache,
	angularDiff,
	STOP_CATEGORIES,
	type Candidate,
	type Stop,
	type StopSource
} from './routeDistance';
import { formatDistance, formatDistanceValue, formatDistanceSpoken } from './units';

// --- Fixture builders -------------------------------------------------------
// All fixtures live at latitude 35 (the user's real course), where one degree of
// longitude is 91,085.8 m and one degree of latitude is 110,574.4 m on the
// R = 6371000 sphere geo.ts uses.
const LAT0 = 35;
const M_PER_DEG_LON_AT_35 = 91085.8;
const M_PER_DEG_LAT = 110574.4;

/** An east-west line at lat 35 starting at lon -86, `n` vertices, `stepDeg` apart. */
function eastWestLine(n: number, stepDeg = 0.01, lat = LAT0, lon0 = -86): [number, number][] {
	const out: [number, number][] = [];
	for (let i = 0; i < n; i++) out.push([lon0 + i * stepDeg, lat]);
	return out;
}

function makeStop(id: string, chainageMeters: number): Stop {
	return { id, label: id, chainageMeters, offTrackMeters: 0 };
}

function pointAnnotation(id: string, category: string, lat: number, lon: number): StopSource {
	return {
		id,
		label: id,
		category,
		geometry: JSON.stringify({ type: 'Point', coordinates: [lon, lat] })
	};
}

// --- 1. haversine sanity ----------------------------------------------------

describe('haversineMeters (the one geo.ts already exports)', () => {
	it('measures one degree of latitude at the equator as ~111,195 m', () => {
		expect(haversineMeters(0, 0, 1, 0)).toBeCloseTo(111195, -2); // +/- 50 m
	});

	it('returns zero for identical points', () => {
		expect(haversineMeters(35.5, -86.5, 35.5, -86.5)).toBe(0);
	});
});

describe('angularDiff', () => {
	it('is the smallest angle between two bearings, wrapping through north', () => {
		expect(angularDiff(90, 90)).toBe(0);
		expect(angularDiff(90, 270)).toBe(180);
		expect(angularDiff(350, 10)).toBe(20);
		expect(angularDiff(10, 350)).toBe(20);
		expect(angularDiff(0, 91)).toBe(91);
	});
});

// --- 2. buildRouteIndex -----------------------------------------------------

describe('buildRouteIndex', () => {
	it('accumulates strictly increasing chainage and a hand-checkable total', () => {
		const idx = buildRouteIndex(eastWestLine(4, 0.01));
		expect(idx.cum[0]).toBe(0);
		for (let i = 1; i < idx.cum.length; i++) {
			expect(idx.cum[i]).toBeGreaterThan(idx.cum[i - 1]);
		}
		// 3 segments x 0.01 deg of longitude at lat 35 = 3 x 910.858 m
		expect(idx.totalMeters).toBeCloseTo(3 * 910.858, 0);
		expect(idx.closed).toBe(false);
	});

	it('records a due-east bearing for an eastbound segment', () => {
		const idx = buildRouteIndex(eastWestLine(3, 0.01));
		expect(idx.bearings[0]).toBeCloseTo(90, 1);
	});

	it('flags a loop as closed when the ends meet', () => {
		const idx = buildRouteIndex([
			[-86, 35],
			[-85.95, 35],
			[-85.95, 35.04],
			[-86, 35.04],
			[-86, 35]
		]);
		expect(idx.closed).toBe(true);
	});

	it('rejects a line with fewer than two coordinates', () => {
		expect(() => buildRouteIndex([[-86, 35]])).toThrow();
	});

	it('memoises by annotation id + updatedAt', () => {
		clearRouteIndexCache();
		const coords = eastWestLine(4, 0.01);
		const a = getRouteIndex('ann-1', 't1', coords);
		const b = getRouteIndex('ann-1', 't1', coords);
		const c = getRouteIndex('ann-1', 't2', coords);
		expect(b).toBe(a);
		expect(c).not.toBe(a);
	});

	// The per-fix caller passes a thunk so the 79 KB LineString is JSON.parsed
	// once per edit rather than once per GPS tick.
	it('does not re-resolve coordinates on a memo hit', () => {
		clearRouteIndexCache();
		const coords = eastWestLine(4, 0.01);
		let calls = 0;
		const thunk = () => {
			calls++;
			return coords;
		};
		const a = getRouteIndex('ann-lazy', 't1', thunk);
		const b = getRouteIndex('ann-lazy', 't1', thunk);
		expect(b).toBe(a);
		expect(calls).toBe(1);
	});

	it('throws when a lazy provider yields no line', () => {
		clearRouteIndexCache();
		expect(() => getRouteIndex('ann-bad', 't1', () => null)).toThrow();
	});
});

// --- 3. Zero-length segments (72 in the real GPX, 315 in the real KML) -------

describe('duplicate consecutive vertices', () => {
	it('does not change the total and produces no NaN', () => {
		const clean = eastWestLine(4, 0.01);
		const dupe: [number, number][] = [clean[0], clean[1], clean[1], clean[2], clean[3]];
		const a = buildRouteIndex(clean);
		const b = buildRouteIndex(dupe);
		expect(b.totalMeters).toBeCloseTo(a.totalMeters, 6);
		for (const v of b.cum) expect(Number.isNaN(v)).toBe(false);
		for (const v of b.bearings) expect(Number.isNaN(v)).toBe(false);
	});

	it('still returns a finite candidate when projecting next to the duplicate', () => {
		const clean = eastWestLine(4, 0.01);
		const dupe: [number, number][] = [clean[0], clean[1], clean[1], clean[2], clean[3]];
		const idx = buildRouteIndex(dupe);
		// 30 m north of the duplicated vertex
		const cands = projectOnRoute(idx, LAT0 + 30 / M_PER_DEG_LAT, -85.99);
		expect(cands.length).toBeGreaterThan(0);
		expect(Number.isFinite(cands[0].chainageMeters)).toBe(true);
		expect(Number.isFinite(cands[0].offTrackMeters)).toBe(true);
		expect(cands[0].offTrackMeters).toBeCloseTo(30, 0);
	});
});

// --- 4 & 5. Projection exactness and the 50 m error bound -------------------

describe('projectOnRoute', () => {
	const idx = buildRouteIndex(eastWestLine(6, 0.01));

	it('is exact on a vertex', () => {
		const cands = projectOnRoute(idx, LAT0, -85.97); // vertex index 3
		expect(cands).toHaveLength(1);
		expect(cands[0].offTrackMeters).toBeCloseTo(0, 2);
		expect(cands[0].chainageMeters).toBeCloseTo(idx.cum[3], 1); // +/- 0.05 m
	});

	it('lands halfway along a segment for a midpoint query', () => {
		const cands = projectOnRoute(idx, LAT0, -85.985); // midway between vertices 1 and 2
		const segLen = idx.cum[2] - idx.cum[1];
		expect(cands[0].chainageMeters).toBeCloseTo(idx.cum[1] + segLen / 2, 0);
	});

	it('keeps a 50 m perpendicular offset within 2 m off-track and 20 m of chainage', () => {
		const trueFootChainage = idx.cum[2] + (0.005 * M_PER_DEG_LON_AT_35);
		const cands = projectOnRoute(idx, LAT0 + 50 / M_PER_DEG_LAT, -85.975);
		expect(cands).toHaveLength(1);
		expect(Math.abs(cands[0].offTrackMeters - 50)).toBeLessThanOrEqual(2);
		expect(Math.abs(cands[0].chainageMeters - trueFootChainage)).toBeLessThanOrEqual(20);
	});

	// 6. The gate that stops a 2 km-off point being quietly snapped: on the real
	// course that produces a 2.2 mi error, sometimes past the very stop the rider
	// is approaching.
	it('returns nothing for a point 2 km off the course at the default tolerance', () => {
		const cands = projectOnRoute(idx, LAT0 + 2000 / M_PER_DEG_LAT, -85.975);
		expect(cands).toEqual([]);
	});

	it('rejects a candidate whose segment is a >2 km gap between disjoint imports', () => {
		// Two 0.01 deg lines 0.5 deg (~45 km) apart, joined by one huge segment.
		const idxGap = buildRouteIndex([
			[-86, 35],
			[-85.99, 35],
			[-85.49, 35],
			[-85.48, 35]
		]);
		// Right in the middle of the void — on the "segment", but not on the course.
		expect(projectOnRoute(idxGap, 35, -85.74)).toEqual([]);
		// The real segments still project fine.
		expect(projectOnRoute(idxGap, 35, -85.995).length).toBe(1);
	});
});

// --- 7 & 8. Out-and-back ambiguity ------------------------------------------

/** Out east along lat 35, turn around, come back 20 m north of the outbound leg. */
function outAndBack(): [number, number][] {
	const coords: [number, number][] = [];
	for (let lon = -86; lon <= -85.9499; lon += 0.005) coords.push([+lon.toFixed(5), LAT0]);
	const backLat = LAT0 + 20 / M_PER_DEG_LAT;
	for (let lon = -85.95; lon >= -85.9951; lon -= 0.005) coords.push([+lon.toFixed(5), backLat]);
	return coords;
}

describe('out-and-back candidate separation', () => {
	const idx = buildRouteIndex(outAndBack());
	// 10 m north of the outbound leg, i.e. exactly between the two legs.
	const qLat = LAT0 + 10 / M_PER_DEG_LAT;
	const qLon = -85.975;

	it('is not treated as a closed loop', () => {
		expect(idx.closed).toBe(false);
	});

	it('returns exactly two candidates, more than 500 m apart along the course', () => {
		const cands = projectOnRoute(idx, qLat, qLon);
		expect(cands).toHaveLength(2);
		expect(Math.abs(cands[0].chainageMeters - cands[1].chainageMeters)).toBeGreaterThan(500);
	});

	it('resolves to the outbound leg from an eastbound heading', () => {
		const cands = projectOnRoute(idx, qLat, qLon);
		const r = resolveCandidate(cands, { bearingDeg: 90 });
		expect(r.ambiguous).toBe(false);
		expect(r.candidate?.segBearing).toBeCloseTo(90, 0);
	});

	it('resolves to the return leg from a westbound heading', () => {
		const cands = projectOnRoute(idx, qLat, qLon);
		const r = resolveCandidate(cands, { bearingDeg: 270 });
		expect(r.ambiguous).toBe(false);
		expect(r.candidate?.segBearing).toBeCloseTo(270, 0);
	});

	it('refuses to guess with no hints at all', () => {
		const cands = projectOnRoute(idx, qLat, qLon);
		const r = resolveCandidate(cands);
		expect(r.ambiguous).toBe(true);
		expect(r.candidate).toBeNull();
		expect(r.candidates).toHaveLength(2);
	});
});

// --- 9. Continuity ----------------------------------------------------------

describe('resolveCandidate continuity', () => {
	const far: Candidate[] = [
		{ chainageMeters: 30000, offTrackMeters: 12, segIndex: 10, segBearing: 90 },
		{ chainageMeters: 90000, offTrackMeters: 8, segIndex: 400, segBearing: 95 }
	];

	it('picks the candidate near the last known chainage', () => {
		const r = resolveCandidate(far, { lastChainage: 30100, elapsedSeconds: 10, speedMps: 10 });
		expect(r.ambiguous).toBe(false);
		expect(r.candidate?.chainageMeters).toBe(30000);
	});

	it('is still ambiguous without a hint, even though one is closer to the line', () => {
		expect(resolveCandidate(far).ambiguous).toBe(true);
	});

	it('picks the nearest when all survivors are within the ambiguity window', () => {
		const close: Candidate[] = [
			{ chainageMeters: 1000, offTrackMeters: 30, segIndex: 1, segBearing: 90 },
			{ chainageMeters: 2000, offTrackMeters: 5, segIndex: 2, segBearing: 90 }
		];
		const r = resolveCandidate(close);
		expect(r.ambiguous).toBe(false);
		expect(r.candidate?.chainageMeters).toBe(2000);
	});

	// Crossing the start/finish of a loop takes you from (total - 50) m to 50 m:
	// 100 m of travel but a whole lap of raw subtraction. A non-wrapping filter
	// drops every candidate at exactly the moment the next lap begins.
	it('measures continuity the short way round on a closed course', () => {
		const loop = buildRouteIndex([
			[-86, 35],
			[-85.95, 35],
			[-85.95, 35.04],
			[-86, 35.04],
			[-86, 35]
		]);
		const total = loop.totalMeters;
		const cands: Candidate[] = [
			{ chainageMeters: 30, offTrackMeters: 10, segIndex: 0, segBearing: 90 },
			{ chainageMeters: total * 0.5, offTrackMeters: 6, segIndex: 2, segBearing: 270 }
		];
		const hint = { lastChainage: total - 50, elapsedSeconds: 1, speedMps: 10 };
		expect(resolveCandidate(cands, hint, loop).candidate?.chainageMeters).toBe(30);
		// Without the index the same hint cannot wrap, and both candidates fail.
		expect(resolveCandidate(cands, hint).ambiguous).toBe(true);
	});

	it('returns nothing for an empty candidate list', () => {
		const r = resolveCandidate([]);
		expect(r.candidate).toBeNull();
		expect(r.ambiguous).toBe(false);
	});
});

// --- 10. nextStopsAhead + the turn-cue safety guarantee ----------------------

describe('nextStopsAhead', () => {
	const idx = buildRouteIndex(eastWestLine(6, 0.01)); // ~4554 m, open
	const stops = [makeStop('a', 1000), makeStop('b', 2000), makeStop('c', 3000)];

	it('names the next stop ahead', () => {
		expect(nextStopsAhead(stops, 1500, idx, 1)[0].id).toBe('b');
	});

	it('holds the stop you are 10 m short of', () => {
		expect(nextStopsAhead(stops, 1990, idx, 1)[0].id).toBe('b');
	});

	it('holds the stop you are 40 m past (inside the 50 m hysteresis)', () => {
		expect(nextStopsAhead(stops, 2040, idx, 1)[0].id).toBe('b');
	});

	it('advances once you are more than 50 m past the stop', () => {
		expect(nextStopsAhead(stops, 2060, idx, 1)[0].id).toBe('c');
	});

	it('lists the next three in order', () => {
		expect(nextStopsAhead(stops, 500, idx, 3).map((s) => s.id)).toEqual(['a', 'b', 'c']);
	});

	// 12
	it('returns an empty list when there are no stops at all', () => {
		expect(nextStopsAhead([], 500, idx)).toEqual([]);
	});

	it('returns an empty list once every stop is behind you', () => {
		expect(nextStopsAhead(stops, 4000, idx)).toEqual([]);
	});

	it('measures the other way when the rider is heading back up the course', () => {
		expect(nextStopsAhead(stops, 2500, idx, 1, true)[0].id).toBe('b');
	});
});

describe('projectStops category allowlist (the turn-cue guarantee)', () => {
	const idx = buildRouteIndex(eastWestLine(6, 0.01));
	const lonAt = (m: number) => -86 + m / M_PER_DEG_LON_AT_35;

	it('never returns a general-category annotation, however close to the line', () => {
		const anns: StopSource[] = [
			pointAnnotation('cue-left', 'general', LAT0, lonAt(1600)), // sits ON the line
			pointAnnotation('rest-stop', 'aid', LAT0 + 20 / M_PER_DEG_LAT, lonAt(2000))
		];
		const stops = projectStops(idx, anns);
		expect(stops.map((s) => s.id)).toEqual(['rest-stop']);
		// And therefore it can never be quoted on the radio.
		expect(nextStopsAhead(stops, 1500, idx, 3).map((s) => s.id)).toEqual(['rest-stop']);
	});

	it('accepts every allowlisted category and rejects general', () => {
		expect(STOP_CATEGORIES).toEqual(['aid', 'checkpoint', 'resource', 'start', 'finish', 'shelter', 'staging']);
		expect(STOP_CATEGORIES).not.toContain('general');
		const anns = STOP_CATEGORIES.map((c, i) => pointAnnotation(c, c, LAT0, lonAt(500 + i * 400)));
		expect(projectStops(idx, anns)).toHaveLength(STOP_CATEGORIES.length);
	});

	it('drops an allowlisted stop that is not actually on this course', () => {
		const anns = [pointAnnotation('far', 'aid', LAT0 + 5000 / M_PER_DEG_LAT, lonAt(2000))];
		expect(projectStops(idx, anns)).toEqual([]);
	});

	it('orders stops from the start of the course, not from import order', () => {
		const anns = [
			pointAnnotation('third', 'aid', LAT0, lonAt(3000)),
			pointAnnotation('first', 'aid', LAT0, lonAt(500)),
			pointAnnotation('second', 'checkpoint', LAT0, lonAt(1500))
		];
		expect(projectStops(idx, anns).map((s) => s.id)).toEqual(['first', 'second', 'third']);
	});
});

// --- 11. Closed-route wrap --------------------------------------------------

describe('closed course', () => {
	const idx = buildRouteIndex([
		[-86, 35],
		[-85.95, 35],
		[-85.95, 35.04],
		[-86, 35.04],
		[-86, 35]
	]);

	it('wraps "ahead" past the start/finish rather than measuring the long way round', () => {
		expect(idx.closed).toBe(true);
		const total = idx.totalMeters;
		const stops = [makeStop('near-start', total * 0.02), makeStop('far', total * 0.5)];
		const ahead = nextStopsAhead(stops, total * 0.95, idx, 2);
		expect(ahead[0].id).toBe('near-start');
		const d = courseDistance(total * 0.95, ahead[0].chainageMeters, idx);
		expect(d).toBeCloseTo(total * 0.07, 0);
		expect(d).toBeLessThan(total * 0.1);
	});

	it('still holds a stop you have only just rolled through', () => {
		const total = idx.totalMeters;
		const stops = [makeStop('here', total * 0.5)];
		// 10 m past it — inside the hysteresis, so it must NOT read as a full lap away.
		expect(nextStopsAhead(stops, total * 0.5 + 10, idx, 1).map((s) => s.id)).toEqual(['here']);
	});
});

// --- parseLineString --------------------------------------------------------

describe('parseLineString', () => {
	it('accepts a JSON string and an already-parsed object', () => {
		const geo = { type: 'LineString', coordinates: [[-86, 35], [-85.99, 35]] };
		expect(parseLineString(JSON.stringify(geo))).toEqual([[-86, 35], [-85.99, 35]]);
		expect(parseLineString(geo as unknown as string)).toEqual([[-86, 35], [-85.99, 35]]);
	});

	it('returns null for a Point, for junk, and for malformed coordinates', () => {
		expect(parseLineString(JSON.stringify({ type: 'Point', coordinates: [-86, 35] }))).toBeNull();
		expect(parseLineString('not json')).toBeNull();
		expect(parseLineString(JSON.stringify({ type: 'LineString', coordinates: [['a', 1]] }))).toBeNull();
	});
});

// --- 13. formatDistance -----------------------------------------------------

describe('formatDistance', () => {
	it.each([
		[120, 'imperial', 'road', '390 ft by road'],
		[5150, 'imperial', 'road', '3.2 mi by road'],
		[40000, 'imperial', 'direct', '25 mi direct'],
		[320, 'metric', 'road', '320 m by road'],
		[5150, 'metric', 'road', '5.2 km by road'],
		[40000, 'metric', 'direct', '40 km direct']
	] as const)('formats %s m (%s, %s) as "%s"', (m, units, kind, expected) => {
		expect(formatDistance(m, units, kind)).toBe(expected);
	});

	it('uses the house "--" sentinel for an unknown distance', () => {
		expect(formatDistance(undefined, 'imperial', 'road')).toBe('--');
		expect(formatDistanceValue(undefined, 'metric', 'direct')).toBe('--');
		expect(formatDistanceSpoken(undefined, 'metric', 'direct', 'Eakin')).toBe('--');
	});

	it('drops the label for the bare value form', () => {
		expect(formatDistanceValue(5150, 'imperial', 'road')).toBe('3.2 mi');
		expect(formatDistanceValue(5150, 'imperial', 'direct')).toBe('3.2 mi');
	});

	it('speaks the sentence the operator reads into the microphone', () => {
		expect(formatDistanceSpoken(5150, 'imperial', 'road', 'Eakin Elementary'))
			.toBe('3.2 miles by road to Eakin Elementary');
		expect(formatDistanceSpoken(4500, 'imperial', 'direct', 'Eakin Elementary'))
			.toBe('2.8 miles direct to Eakin Elementary — road distance will be more');
	});
});

// --- 14. The user's real course (skipped when the file is absent) -----------

const gpxPath = fileURLToPath(new URL('../../../Day_1_48M_Jack_and_Back.gpx', import.meta.url));
const haveGpx = existsSync(gpxPath);

describe('real course: Day 1 48M Jack and Back', () => {
	it.skipIf(!haveGpx)('reproduces the measured total and the four rest-stop chainages', () => {
		const xml = readFileSync(gpxPath, 'utf8');
		const coords: [number, number][] = [];
		const re = /<trkpt lat="([-\d.]+)" lon="([-\d.]+)"/g;
		let m: RegExpExecArray | null;
		while ((m = re.exec(xml)) !== null) coords.push([parseFloat(m[2]), parseFloat(m[1])]);
		expect(coords.length).toBe(3801);

		const idx = buildRouteIndex(coords);
		expect(Math.abs(idx.totalMeters - 76046)).toBeLessThanOrEqual(50);
		expect(idx.closed).toBe(false);

		const stops: [string, number, number, number][] = [
			['Maxwell Chapel', 35.61365266280593, -86.54982271163941, 21254],
			['Eakin Elementary', 35.500702780314725, -86.44572066879583, 38785],
			['Flat Creek', 35.39051937562392, -86.40947431325912, 56971],
			['Finish', 35.28489503441051, -86.37205728115387, 76046]
		];
		for (const [name, lat, lon, expectedM] of stops) {
			const cands = projectOnRoute(idx, lat, lon);
			expect(cands.length, `${name}: no projection`).toBeGreaterThan(0);
			// This point-to-point course has zero self-overlap, so there is exactly
			// one place each stop can be.
			expect(cands.length, `${name}: ambiguous`).toBe(1);
			expect(Math.abs(cands[0].chainageMeters - expectedM), `${name} chainage`)
				.toBeLessThanOrEqual(30);
			expect(cands[0].offTrackMeters, `${name} off-track`).toBeLessThanOrEqual(30);
		}
	});
});

describe('segment bearings are robust to near-duplicate vertices', () => {
	// Real coordinates lifted from Day_1_48M_Jack_and_Back.gpx around chainage
	// 30,770 m. The file is written to 5 decimal places (~1 m), so consecutive
	// points can differ in ONE coordinate only, making that segment's raw
	// bearing exactly 90/180 degrees — pure rounding noise, not a direction of
	// travel. 6.8% of this course's 3,800 segments are <= 2 m long and 255 of
	// them have an exactly axis-aligned bearing.
	//
	// This matters because a single garbage bearing is enough to flip the
	// pill's notion of "ahead" and announce the aid station BEHIND the rider.
	const realRun: [number, number][] = [
		[-86.49107, 35.54984],
		[-86.49086, 35.54947],
		[-86.49077, 35.54931],
		[-86.49076, 35.54931], // 1 m step, latitude identical -> raw bearing 90.0
		[-86.49067, 35.54915],
		[-86.49067, 35.54914], // 1 m step, longitude identical -> raw bearing 180.0
		[-86.49058, 35.54898],
		[-86.49015, 35.54821],
		[-86.48985, 35.54767]
	];

	it('never reports a local direction more than 30 degrees off the real one', () => {
		const idx = buildRouteIndex(realRun);
		// True heading of this stretch of road, measured end to end.
		const trueCourse = 155.4;
		for (let i = 0; i < idx.bearings.length; i++) {
			expect(
				angularDiff(idx.bearings[i], trueCourse),
				`segment ${i} bearing ${idx.bearings[i].toFixed(1)} deg`
			).toBeLessThanOrEqual(30);
		}
	});

	it('keeps a rider travelling the true course pointing forwards', () => {
		const idx = buildRouteIndex(realRun);
		// A rider actually on this road, heading 155 deg, must never have a
		// candidate rejected by the 120 deg bearing filter in resolveCandidate.
		for (const [lon, lat] of realRun) {
			const cands = projectOnRoute(idx, lat, lon);
			expect(cands.length).toBeGreaterThan(0);
			expect(angularDiff(cands[0].segBearing, 155.4)).toBeLessThanOrEqual(120);
		}
	});
});

// --- pointAtChainage --------------------------------------------------------
// The inverse of Candidate.chainageMeters, and the function every SAG pickup
// pin on the map depends on (docs/sag-map-spec.md §2). A wrong point here is
// worse than no point: net control will believe it and send a van there.

describe('pointAtChainage', () => {
	// 11 vertices, 0.01 deg apart at lat 35 -> ~910.858 m per segment.
	const idx = buildRouteIndex(eastWestLine(11));

	it('returns the start at 0 and the end at totalMeters', () => {
		expect(pointAtChainage(idx, 0)).toEqual({ lat: 35, lon: -86 });
		const end = pointAtChainage(idx, idx.totalMeters)!;
		expect(end.lat).toBeCloseTo(35, 9);
		expect(end.lon).toBeCloseTo(-85.9, 9);
	});

	it('lands exactly on an interior vertex at that vertex chainage', () => {
		// Driven off the index's own chainage, not off SEG: the metres-per-degree
		// constant at the top of this file is rounded, so SEG * 4 misses vertex 4
		// by about a centimetre and would be testing the fixture, not the code.
		const p = pointAtChainage(idx, idx.cum[4])!;
		expect(p.lon).toBeCloseTo(-86 + 0.04, 9);
		expect(p.lat).toBeCloseTo(35, 9);
	});

	it('interpolates linearly inside a segment', () => {
		const p = pointAtChainage(idx, (idx.cum[2] + idx.cum[3]) / 2)!;
		expect(p.lon).toBeCloseTo(-86 + 0.025, 9);
	});

	it('refuses out-of-range chainage on an open course', () => {
		expect(idx.closed).toBe(false);
		expect(pointAtChainage(idx, -1)).toBeNull();
		expect(pointAtChainage(idx, idx.totalMeters + 1)).toBeNull();
	});

	it('tolerates floating-point slop at the ends rather than refusing', () => {
		// milesRemaining arithmetic (total - miles*1609.344) lands a few microns
		// outside the range constantly; refusing there would make the finish
		// unplaceable on a course whose length is a non-terminating conversion.
		expect(pointAtChainage(idx, -0.0004)).toEqual({ lat: 35, lon: -86 });
		expect(pointAtChainage(idx, idx.totalMeters + 0.0004)).not.toBeNull();
	});

	it('round-trips against projectOnRoute', () => {
		for (const frac of [0.05, 0.2, 0.5, 0.77, 0.95]) {
			const m = idx.totalMeters * frac;
			const p = pointAtChainage(idx, m)!;
			const cands = projectOnRoute(idx, p.lat, p.lon);
			expect(cands.length, `frac ${frac}`).toBeGreaterThan(0);
			expect(Math.abs(cands[0].chainageMeters - m), `frac ${frac}`).toBeLessThanOrEqual(1);
		}
	});

	it('wraps on a closed loop instead of refusing', () => {
		const loop = buildRouteIndex([
			[-86, 35],
			[-85.95, 35],
			[-85.95, 35.04],
			[-86, 35.04],
			[-86, 35]
		]);
		expect(loop.closed).toBe(true);
		const total = loop.totalMeters;
		const wrapped = pointAtChainage(loop, total + total * 0.25)!;
		const plain = pointAtChainage(loop, total * 0.25)!;
		expect(wrapped.lat).toBeCloseTo(plain.lat, 9);
		expect(wrapped.lon).toBeCloseTo(plain.lon, 9);
		// And negative chainage wraps backwards off the start.
		const back = pointAtChainage(loop, -total * 0.25)!;
		const fwd = pointAtChainage(loop, total * 0.75)!;
		expect(back.lat).toBeCloseTo(fwd.lat, 9);
		expect(back.lon).toBeCloseTo(fwd.lon, 9);
	});

	it('steps over zero-length segments without dividing by zero', () => {
		const dup = buildRouteIndex([
			[-86, 35],
			[-85.99, 35],
			[-85.99, 35], // duplicate vertex -> zero-length segment
			[-85.98, 35]
		]);
		const p = pointAtChainage(dup, dup.totalMeters / 2)!;
		expect(Number.isFinite(p.lat)).toBe(true);
		expect(Number.isFinite(p.lon)).toBe(true);
		expect(p.lon).toBeCloseTo(-85.99, 6);
	});

	it.skipIf(!haveGpx)('places the real rest stops from their measured mileage', () => {
		const xml = readFileSync(gpxPath, 'utf8');
		const coords: [number, number][] = [];
		const re = /<trkpt lat="([-\d.]+)" lon="([-\d.]+)"/g;
		let m: RegExpExecArray | null;
		while ((m = re.exec(xml)) !== null) coords.push([parseFloat(m[2]), parseFloat(m[1])]);
		const idx48 = buildRouteIndex(coords);

		// Same four stops as the projection test above, driven the other way:
		// chainage in, coordinates out. Within 30 m of the surveyed point.
		const stops: [string, number, number, number][] = [
			['Maxwell Chapel', 35.61365266280593, -86.54982271163941, 21254],
			['Eakin Elementary', 35.500702780314725, -86.44572066879583, 38785],
			['Flat Creek', 35.39051937562392, -86.40947431325912, 56971],
			['Finish', 35.28489503441051, -86.37205728115387, 76046]
		];
		for (const [name, lat, lon, chainage] of stops) {
			const p = pointAtChainage(idx48, chainage)!;
			expect(p, `${name}: refused`).not.toBeNull();
			expect(haversineMeters(p.lat, p.lon, lat, lon), `${name}: off by too much`)
				.toBeLessThanOrEqual(30);
		}
	});
});
