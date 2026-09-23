import { describe, it, expect } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { buildRouteIndex, pointAtChainage, projectOnRoute, type RouteIndex } from './routeDistance';
import {
	projectRosterOnCourse,
	type RosterCheckIn,
	type RosterStation,
	type RosterStopPoint,
	type RosterMemory
} from './rosterCourse';

const MI = 1609.344;
const NOW = Date.parse('2026-09-23T12:00:00Z');
const ago = (min: number) => new Date(NOW - min * 60_000).toISOString();

const LAT0 = 35;
const M_PER_DEG_LON_AT_35 = 91085.8;
const M_PER_DEG_LAT = 110574.4;

function ci(over: Partial<RosterCheckIn> & { id: string }): RosterCheckIn {
	return {
		callsign: over.id.toUpperCase(),
		category: 'sag',
		source: 'aprs',
		lastHeard: ago(1),
		...over
	};
}

function run(
	idx: RouteIndex | null,
	checkIns: RosterCheckIn[],
	opts: {
		stations?: Map<string, RosterStation>;
		stops?: RosterStopPoint[];
		memory?: RosterMemory;
		sweepUnitCheckInId?: string;
	} = {}
) {
	return projectRosterOnCourse({
		checkIns,
		stationsByCallsign: opts.stations ?? new Map(),
		routeIndex: idx,
		stops: opts.stops ?? [],
		nowMs: NOW,
		memory: opts.memory ?? new Map(),
		sweepUnitCheckInId: opts.sweepUnitCheckInId
	});
}

// --- a synthetic out-and-back that runs in CI ---------------------------------
// A 1.8 km lead-in that is used ONCE, then east ~9.1 km and back west 20 m
// north: one road, two chainages. The lead-in matters twice over: it gives
// the plain on-course tests a stretch that is genuinely unambiguous, and it
// keeps the two ends 1.8 km apart — without it they are 20 m apart, under
// CLOSED_LOOP_M, and the engine rightly treats the course as a LOOP (where
// continuing round really can reach the other leg).
const lead: [number, number][] = [[-86.02, LAT0], [-86.01, LAT0]];
const out: [number, number][] = Array.from({ length: 11 }, (_, i) => [-86 + i * 0.01, LAT0]);
const back = out.slice().reverse().map(([lon, lat]) => [lon, lat + 20 / M_PER_DEG_LAT] as [number, number]);
const oab = buildRouteIndex([...lead, ...out, ...back]);
const LEAD_M = 0.02 * M_PER_DEG_LON_AT_35;
const at2km = { lat: LAT0 + 10 / M_PER_DEG_LAT, lon: -86 + 2000 / M_PER_DEG_LON_AT_35 };
/** On the lead-in: used once, never ambiguous. ~455 m from the start. */
const onceUsed = { lat: LAT0, lon: -86.015 };

describe('projectRosterOnCourse — who qualifies (R1-R4)', () => {
	it('fixture check: the course is open, not a loop', () => {
		expect(oab.closed).toBe(false);
	});

	it('places an on-course member at a single mile', () => {
		const [r] = run(oab, [ci({ id: 'a', ...onceUsed })]);
		expect(r.state).toBe('on-course');
		expect(r.chainageMeters).toBeCloseTo(0.005 * M_PER_DEG_LON_AT_35, -1);
	});

	it('beyond 400 m of the line is off-course — counted, not placed', () => {
		const [r] = run(oab, [ci({ id: 'far', lat: LAT0 + 1000 / M_PER_DEG_LAT, lon: -85.95 })]);
		expect(r.state).toBe('off-course');
		expect(r.chainageMeters).toBeUndefined();
	});

	it('no position at all is its own state, never a guess', () => {
		const [r] = run(oab, [ci({ id: 'voice', lat: undefined, lon: undefined, source: 'voice' })]);
		expect(r.state).toBe('no-position');
	});

	it('a 0,0 check-in position is no position (a no-fix beacon), not Null Island', () => {
		const [r] = run(oab, [ci({ id: 'nofix', lat: 0, lon: 0 })]);
		expect(r.state).toBe('no-position');
	});

	it('every category qualifies — a filter would hide the unit nobody categorised', () => {
		const pt = onceUsed;
		const got = run(
			oab,
			['sag', 'medical', 'marshal', 'general', 'fixed'].map((category, i) => ci({ id: `u${i}`, category, ...pt }))
		);
		expect(got.every((r) => r.state === 'on-course')).toBe(true);
	});

	it('R2: within 150 m of a numbered stop folds into that stop', () => {
		const stop = { id: 'rs1', seq: 1, lat: LAT0, lon: -86 + 3000 / M_PER_DEG_LON_AT_35 };
		const [r] = run(oab, [ci({ id: 'captain', lat: LAT0 + 20 / M_PER_DEG_LAT, lon: stop.lon })], { stops: [stop] });
		expect(r.state).toBe('at-stop');
		expect(r.stopId).toBe('rs1');
	});

	it('R2 works even with no course line (stop staffing is straight-line)', () => {
		const stop = { id: 'rs1', seq: 1, lat: LAT0, lon: -86 };
		const [r] = run(null, [ci({ id: 'captain', lat: LAT0, lon: -86 })], { stops: [stop] });
		expect(r.state).toBe('at-stop');
	});

	it('no course line and not at a stop cannot be placed: no-course, not off-course', () => {
		// "Off course" would be a claim about distance we cannot make.
		const [r] = run(null, [ci({ id: 'x', lat: LAT0, lon: -86 })]);
		expect(r.state).toBe('no-course');
	});
});

describe('projectRosterOnCourse — shared road (R6-R9)', () => {
	it('R9: parked on the shared road is ambiguous, with BOTH miles and no single one', () => {
		const [r] = run(oab, [ci({ id: 'van', ...at2km })], {
			stations: new Map([['VAN', { position: { speed: 0, course: 205 } }]])
		});
		expect(r.state).toBe('ambiguous');
		expect(r.chainageMeters).toBeUndefined();
		expect(r.candidatesMiles).toHaveLength(2);
		expect(r.candidatesMiles![0]).toBeCloseTo((LEAD_M + 2000) / MI, 1);
		expect(r.candidatesMiles![1]).toBeCloseTo((oab.totalMeters - 2000) / MI, 1);
	});

	it('R7: a MOVING heading resolves the leg', () => {
		const [r] = run(oab, [ci({ id: 'van', ...at2km })], {
			stations: new Map([['VAN', { position: { speed: 40, course: 270 } }]]) // westbound = return leg
		});
		expect(r.state).toBe('on-course');
		expect(r.chainageMeters!).toBeCloseTo(oab.totalMeters - 2000, -2);
	});

	it('R7: a PARKED heading is ignored (it is GPS noise)', () => {
		const [r] = run(oab, [ci({ id: 'van', ...at2km })], {
			stations: new Map([['VAN', { position: { speed: 0, course: 270 } }]])
		});
		expect(r.state).toBe('ambiguous');
	});

	it('R8: continuity from the last unambiguous fix resolves a parked unit', () => {
		const memory: RosterMemory = new Map([['van', { chainageMeters: LEAD_M + 1900, atMs: NOW - 4 * 60_000 }]]);
		const [r] = run(oab, [ci({ id: 'van', ...at2km })], {
			stations: new Map([['VAN', { position: { speed: 0 } }]]),
			memory
		});
		expect(r.state).toBe('on-course');
		expect(r.chainageMeters!).toBeCloseTo(LEAD_M + 2000, -2);
	});

	it('R8: an unambiguous fix is remembered for next time; an ambiguous one is not', () => {
		const memory: RosterMemory = new Map();
		run(oab, [ci({ id: 'moving', ...at2km })], {
			stations: new Map([['MOVING', { position: { speed: 40, course: 90 } }]]),
			memory
		});
		expect(memory.get('moving')?.chainageMeters).toBeCloseTo(LEAD_M + 2000, -2);

		run(oab, [ci({ id: 'parked', ...at2km })], {
			stations: new Map([['PARKED', { position: { speed: 0 } }]]),
			memory
		});
		expect(memory.has('parked')).toBe(false);
	});
});

describe('projectRosterOnCourse — staleness and source (R10-R12)', () => {
	const pt = onceUsed;

	it('R10: reuses the fresh/aging/stale states', () => {
		const got = run(oab, [
			ci({ id: 'f', ...pt, lastHeard: ago(3) }),
			ci({ id: 'a', ...pt, lastHeard: ago(15) }),
			ci({ id: 's', ...pt, lastHeard: ago(25) })
		]);
		expect(got.map((r) => r.age)).toEqual(['fresh', 'aging', 'stale']);
	});

	it('R10: position age is the newest TRACK point, not the last packet of any kind', () => {
		// lastHeard updates on telemetry and status packets too; the position
		// may be much older than that (spec B19).
		const [r] = run(oab, [ci({ id: 'w', ...pt, lastHeard: ago(1) })], {
			stations: new Map([['W', { track: [{ time: ago(40) }, { time: ago(25) }] }]])
		});
		expect(r.age).toBe('stale');
		expect(r.positionAt).toBe(ago(25));
	});

	it('R12: a hand-placed voice position is never styled fresh', () => {
		// Its lastHeard is refreshed by NCS interaction, not by the position.
		const [r] = run(oab, [ci({ id: 'v', ...pt, source: 'voice', lastHeard: ago(1) })]);
		expect(r.handPlaced).toBe(true);
		expect(r.age).not.toBe('fresh');
	});

	it('R0.3: the sweep unit is marked as such, and is still an ordinary member', () => {
		const [r] = run(oab, [ci({ id: 'sw', ...pt })], { sweepUnitCheckInId: 'sw' });
		expect(r.isSweepUnit).toBe(true);
		expect(r.state).toBe('on-course');
	});
});

// --- real courses -------------------------------------------------------------

const gpxPath = fileURLToPath(new URL('../../../Day_1_48M_Jack_and_Back.gpx', import.meta.url));
const kmlPath = fileURLToPath(new URL('../../../docs/GR_2025_100_miler.kml', import.meta.url));

function gpxIndex(): RouteIndex {
	const xml = readFileSync(gpxPath, 'utf8');
	const coords: [number, number][] = [];
	const re = /<trkpt lat="([-\d.]+)" lon="([-\d.]+)"/g;
	let m: RegExpExecArray | null;
	while ((m = re.exec(xml)) !== null) coords.push([parseFloat(m[2]), parseFloat(m[1])]);
	return buildRouteIndex(coords);
}

function kmlIndex(): RouteIndex {
	const xml = readFileSync(kmlPath, 'utf8');
	const body = /<LineString>[\s\S]*?<coordinates>([\s\S]*?)<\/coordinates>/.exec(xml)![1];
	const coords = body
		.trim()
		.split(/\s+/)
		.map((t) => t.split(',').map(Number))
		.map(([lon, lat]) => [lon, lat] as [number, number]);
	return buildRouteIndex(coords);
}

describe('real course: Day 1 (acceptance 1)', () => {
	it.skipIf(!existsSync(gpxPath))('the live SAG van, parked 108 km away at home, is off-course', () => {
		const [r] = run(gpxIndex(), [ci({ id: 'KG4YFA-4', lat: 36.54479, lon: -87.32679 })]);
		expect(r.state).toBe('off-course');
	});
});

describe('real course: the 100-miler shared road (acceptance 3-5)', () => {
	const have = existsSync(kmlPath);

	it.skipIf(!have)('parked at mile 30 on the shared road: ambiguous, never one mile', () => {
		const idx = kmlIndex();
		const p = pointAtChainage(idx, 30 * MI)!;
		const [r] = run(idx, [ci({ id: 'van', ...p })], {
			stations: new Map([['VAN', { position: { speed: 0, course: 205 } }]])
		});
		expect(r.state).toBe('ambiguous');
		expect(r.chainageMeters).toBeUndefined();
		const [lo, hi] = r.candidatesMiles!;
		expect(lo).toBeGreaterThan(29.5);
		expect(lo).toBeLessThan(30.5);
		expect(hi).toBeGreaterThan(91);
		expect(hi).toBeLessThan(97);
	});

	it.skipIf(!have)('moving along the outbound leg: resolved to mile 30', () => {
		const idx = kmlIndex();
		const p = pointAtChainage(idx, 30 * MI)!;
		const outbound = projectOnRoute(idx, p.lat, p.lon).find((c) => Math.abs(c.chainageMeters - 30 * MI) < 200)!;
		const [r] = run(idx, [ci({ id: 'van', ...p })], {
			stations: new Map([['VAN', { position: { speed: 25, course: outbound.segBearing } }]])
		});
		expect(r.state).toBe('on-course');
		expect(r.chainageMeters! / MI).toBeCloseTo(30, 0);
	});

	it.skipIf(!have)('parked, but seen at mile 29.8 four minutes ago: resolved by continuity', () => {
		const idx = kmlIndex();
		const p = pointAtChainage(idx, 30 * MI)!;
		const memory: RosterMemory = new Map([['van', { chainageMeters: 29.8 * MI, atMs: NOW - 4 * 60_000 }]]);
		const [r] = run(idx, [ci({ id: 'van', ...p })], {
			stations: new Map([['VAN', { position: { speed: 0 } }]]),
			memory
		});
		expect(r.state).toBe('on-course');
		expect(r.chainageMeters! / MI).toBeCloseTo(30, 0);
	});
});
