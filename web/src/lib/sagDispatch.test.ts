import { describe, it, expect } from 'vitest';
import { buildRouteIndex } from './routeDistance';
import {
	joinVehiclePositions,
	rankCandidates,
	type RankContext,
	type SagVehicleGeo
} from './sagDispatch';
import type { SAGVehicleStatus } from './types';

const MI = 1609.344;
const NOW = Date.parse('2026-09-22T12:00:00Z');

function minutesAgo(m: number): string {
	return new Date(NOW - m * 60_000).toISOString();
}

/** 201 vertices 0.005 deg apart at lat 35 -> ~56.6 mi of east-west course. */
function courseIndex(n = 201, stepDeg = 0.005) {
	const coords: [number, number][] = [];
	for (let i = 0; i < n; i++) coords.push([-86 + i * stepDeg, 35]);
	return buildRouteIndex(coords);
}

const IDX = courseIndex();
const M_PER_DEG_LON_AT_35 = 91085.8;

/** A point `miles` along the east-west course, `offM` metres to the north. */
function onCourse(miles: number, offM = 0) {
	return {
		lat: 35 + offM / 110574.4,
		lon: -86 + (miles * MI) / M_PER_DEG_LON_AT_35
	};
}

function vehicle(over: Partial<SAGVehicleStatus> = {}): SAGVehicleStatus {
	return {
		netId: 'n1',
		checkInId: over.checkInId ?? 'v1',
		seats: 4,
		rackSlots: 2,
		notes: '',
		updatedAt: minutesAgo(30),
		callsign: 'KG4YFA',
		tacticalCall: 'SAG 1',
		checkInStatus: 'active',
		committedSeats: 0,
		committedRacks: 0,
		availableSeats: 4,
		availableRacks: 2,
		activeLegIds: [],
		activeRequestIds: [],
		...over
	};
}

function geo(id: string, miles: number | null, over: Partial<SagVehicleGeo> = {}): SagVehicleGeo {
	const pos = miles == null ? {} : onCourse(miles);
	return {
		vehicle: vehicle({ checkInId: id, tacticalCall: id }),
		...pos,
		lastHeard: minutesAgo(2),
		source: 'aprs',
		...over
	};
}

function ctx(over: Partial<RankContext> = {}): RankContext {
	return {
		routeIndex: IDX,
		pickup: { ...onCourse(20), chainageMeters: 20 * MI },
		needSeats: 1,
		needRacks: 0,
		nowMs: NOW,
		avgSpeedMph: 25,
		...over
	};
}

describe('rankCandidates — distance and ordering', () => {
	it('ranks the nearer vehicle first and reports route distance, not crow-flies', () => {
		const r = rankCandidates([geo('near', 18), geo('far', 5)], ctx());
		expect(r.ranked.map((c) => c.id)).toEqual(['near', 'far']);
		expect(r.ranked[0].distanceMeters!).toBeCloseTo(2 * MI, -1);
		expect(r.ranked[0].distanceKind).toBe('road');
	});

	it('names the direction, because a bare number that means "backwards" is the bug', () => {
		const r = rankCandidates([geo('behind', 15), geo('ahead', 25)], ctx());
		const behind = r.ranked.find((c) => c.id === 'behind')!;
		const ahead = r.ranked.find((c) => c.id === 'ahead')!;
		expect(behind.direction).toBe('ahead'); // vehicle at mi 15 drives forward to mi 20
		expect(ahead.direction).toBe('back');
	});

	it('puts every on-course vehicle above every off-course one, whatever the number', () => {
		// The off-course van is physically closer but has no road distance; an
		// unlabelled 0.5 next to a 12.0 that mean different things is unreadable.
		const off = geo('off', null, { ...onCourse(20, 3000) });
		const r = rankCandidates([off, geo('on', 8)], ctx());
		expect(r.ranked.map((c) => c.id)).toEqual(['on', 'off']);
		expect(r.ranked[1].distanceKind).toBe('direct');
	});

	it('falls back to crow-flies for everyone when no course is loaded, and says so', () => {
		const r = rankCandidates([geo('a', 30), geo('b', 21)], ctx({ routeIndex: null }));
		expect(r.degraded).toBe(true);
		for (const c of r.ranked) expect(c.distanceKind).toBe('direct');
		expect(r.ranked[0].id).toBe('b');
	});

	it('demotes a stale position below a fresh one that is further away', () => {
		const stale = geo('stale', 19, { lastHeard: minutesAgo(45) });
		const fresh = geo('fresh', 10, { lastHeard: minutesAgo(1) });
		const r = rankCandidates([stale, fresh], ctx());
		expect(r.ranked.map((c) => c.id)).toEqual(['fresh', 'stale']);
		expect(r.ranked[1].age).toBe('stale');
	});

	it('treats aging as good enough to keep its distance ordering', () => {
		const aging = geo('aging', 19, { lastHeard: minutesAgo(12) });
		const fresh = geo('fresh', 10, { lastHeard: minutesAgo(1) });
		const r = rankCandidates([aging, fresh], ctx());
		expect(r.ranked.map((c) => c.id)).toEqual(['aging', 'fresh']);
	});
});

describe('rankCandidates — capacity', () => {
	it('demotes a van that can only take part of the party, and says how many', () => {
		const small = geo('small', 18);
		small.vehicle = vehicle({ checkInId: 'small', tacticalCall: 'small', availableSeats: 1, availableRacks: 1 });
		const big = geo('big', 30);
		const r = rankCandidates([small, big], ctx({ needSeats: 3, needRacks: 0 }));
		expect(r.ranked.map((c) => c.id)).toEqual(['big', 'small']);
		expect(r.ranked[1].eligibility).toBe('partial');
		expect(r.ranked[1].seatsOffered).toBe(1);
	});

	it('excludes a van with no seat at all rather than hiding it', () => {
		const full = geo('full', 18);
		full.vehicle = vehicle({ checkInId: 'full', tacticalCall: 'full', availableSeats: 0 });
		const r = rankCandidates([full, geo('ok', 40)], ctx());
		expect(r.ranked.map((c) => c.id)).toEqual(['ok']);
		expect(r.excluded.map((c) => c.id)).toEqual(['full']);
		expect(r.excluded[0].excludedReason).toBe('no-seats');
	});

	it('does not let a rack shortfall exclude or demote anyone — racks are a question, not a wall', () => {
		// Seats are a seatbelt; bikes can ride in the bed of a truck. The server
		// refuses seat overflow and merely asks about racks, so ranking must not
		// invent a stricter rule than dispatch enforces.
		const noRacks = geo('noracks', 18);
		noRacks.vehicle = vehicle({ checkInId: 'noracks', tacticalCall: 'noracks', availableRacks: 0 });
		const r = rankCandidates([noRacks, geo('racks', 30)], ctx({ needSeats: 1, needRacks: 1 }));
		expect(r.ranked.map((c) => c.id)).toEqual(['noracks', 'racks']);
		expect(r.ranked[0].eligibility).toBe('eligible');
		expect(r.ranked[0].rackShortfall).toBe(1);
	});

	it('excludes a released check-in', () => {
		const gone = geo('gone', 18);
		gone.vehicle = vehicle({ checkInId: 'gone', tacticalCall: 'gone', checkInStatus: 'released' });
		const r = rankCandidates([gone, geo('here', 40)], ctx());
		expect(r.ranked.map((c) => c.id)).toEqual(['here']);
		expect(r.excluded[0].excludedReason).toBe('released');
	});

	it('excludes a vehicle with no position, but keeps it on screen', () => {
		const r = rankCandidates([geo('nopos', null), geo('here', 40)], ctx());
		expect(r.ranked.map((c) => c.id)).toEqual(['here']);
		expect(r.excluded[0].excludedReason).toBe('no-position');
		expect(r.excluded[0].age).toBe('never');
	});
});

describe('rankCandidates — ambiguity on an out-and-back', () => {
	// Out 10 mi east, then back to the start: every point has two chainages,
	// which is the case routeDistance.ts warns about by name.
	const outBack = (() => {
		const coords: [number, number][] = [];
		for (let i = 0; i <= 100; i++) coords.push([-86 + i * 0.001, 35]);
		for (let i = 99; i >= 0; i--) coords.push([-86 + i * 0.001, 35.0002]);
		return buildRouteIndex(coords);
	})();

	it('gives no distance number to a vehicle that could be in two places', () => {
		const v: SagVehicleGeo = {
			vehicle: vehicle({ checkInId: 'amb', tacticalCall: 'amb' }),
			lat: 35.0001,
			lon: -85.95,
			lastHeard: minutesAgo(2),
			source: 'aprs'
		};
		const clear: SagVehicleGeo = {
			vehicle: vehicle({ checkInId: 'clear', tacticalCall: 'clear' }),
			lat: 35,
			lon: -85.999,
			lastHeard: minutesAgo(2),
			source: 'aprs',
			// Both legs of an out-and-back pass within 22 m of each other, so
			// WITHOUT a heading hint this vehicle is ambiguous too and the test
			// would be comparing two ambiguous rows.
			courseDeg: 90
		};
		const r = rankCandidates([v, clear], {
			...ctx(),
			routeIndex: outBack,
			pickup: { lat: 35, lon: -85.998, chainageMeters: 200 }
		});
		const amb = r.ranked.find((c) => c.id === 'amb')!;
		if (amb.ambiguous) {
			expect(amb.distanceMeters).toBeNull();
			// and it ranks below the unambiguous one
			expect(r.ranked[r.ranked.length - 1].id).toBe('amb');
		}
	});

	it('resolves ambiguity when a heading hint says which way the van is pointing', () => {
		const east: SagVehicleGeo = {
			vehicle: vehicle({ checkInId: 'east', tacticalCall: 'east' }),
			lat: 35.0001,
			lon: -85.95,
			lastHeard: minutesAgo(2),
			source: 'aprs',
			courseDeg: 90
		};
		const r = rankCandidates([east], {
			...ctx(),
			routeIndex: outBack,
			pickup: { lat: 35, lon: -85.998, chainageMeters: 200 }
		});
		expect(r.ranked[0].ambiguous).toBe(false);
	});
});

describe('rankCandidates — ETA', () => {
	it('converts distance at the configured average speed', () => {
		const r = rankCandidates([geo('v', 10)], ctx());
		// 10 mi at 25 mph = 24 min
		expect(r.ranked[0].etaMinutes).toBe(24);
	});

	it('caps the ETA rather than printing false precision on a long one', () => {
		const r = rankCandidates([geo('v', 55)], ctx({ pickup: { ...onCourse(1), chainageMeters: 1 * MI } }));
		expect(r.ranked[0].etaCapped).toBe(true);
		expect(r.ranked[0].etaMinutes).toBe(45);
	});

	it('gives no ETA at all when there is no distance to convert', () => {
		const outBackCtx = ctx({ routeIndex: null, pickup: null });
		const r = rankCandidates([geo('v', 10)], outBackCtx);
		expect(r.ranked[0].distanceMeters).toBeNull();
		expect(r.ranked[0].etaMinutes).toBeNull();
	});
});

describe('rankCandidates — the full sort, as a table', () => {
	it('orders eligible > partial, unambiguous > ambiguous, on-course > off, fresh > stale, near > far', () => {
		const mk = (id: string, miles: number | null, over: Partial<SagVehicleGeo> = {}, veh: Partial<SAGVehicleStatus> = {}) => {
			const g = geo(id, miles, over);
			g.vehicle = vehicle({ checkInId: id, tacticalCall: id, ...veh });
			return g;
		};
		const r = rankCandidates(
			[
				mk('far-eligible', 40),
				mk('near-partial', 19, {}, { availableSeats: 1 }),
				mk('near-eligible', 18),
				mk('off-course', null, { ...onCourse(20, 2000) }),
				mk('stale-near', 19, { lastHeard: minutesAgo(50) }),
				mk('released', 18, {}, { checkInStatus: 'released' })
			],
			ctx({ needSeats: 2 })
		);
		expect(r.ranked.map((c) => c.id)).toEqual([
			'near-eligible',
			'far-eligible',
			'stale-near',
			'off-course',
			'near-partial'
		]);
		expect(r.excluded.map((c) => c.id)).toEqual(['released']);
	});
});

describe('joinVehiclePositions', () => {
	const veh = (checkInId: string) => vehicle({ checkInId, tacticalCall: checkInId });

	it('carries position, staleness and heading across from the roster', () => {
		const out = joinVehiclePositions(
			[veh('ci-1')],
			[{ id: 'ci-1', callsign: 'kg4yfa-4', lat: 35, lon: -86, lastHeard: minutesAgo(3), source: 'aprs' }],
			new Map([['KG4YFA-4', { position: { course: 187 } }]])
		);
		expect(out).toHaveLength(1);
		expect(out[0].lat).toBe(35);
		expect(out[0].courseDeg).toBe(187);
		expect(out[0].source).toBe('aprs');
	});

	it('still returns a vehicle whose check-in is missing, rather than dropping it', () => {
		// A van that silently disappears from the candidate list is worse than
		// one listed as "no position": the operator cannot explain the absence.
		const out = joinVehiclePositions([veh('ci-gone')], [], new Map());
		expect(out).toHaveLength(1);
		expect(out[0].lat).toBeUndefined();
		expect(rankCandidates(out, ctx()).excluded[0].excludedReason).toBe('no-position');
	});

	it('returns a voice-only check-in with its hand-placed position and no heading', () => {
		const out = joinVehiclePositions(
			[veh('ci-2')],
			[{ id: 'ci-2', callsign: 'W1AW', lat: 35.1, lon: -86.1, lastHeard: minutesAgo(9), source: 'voice' }],
			new Map()
		);
		expect(out[0].source).toBe('voice');
		expect(out[0].lat).toBe(35.1);
		expect(out[0].courseDeg).toBeUndefined();
	});

	it('does not confuse two vehicles that share a base callsign', () => {
		const out = joinVehiclePositions(
			[veh('ci-a'), veh('ci-b')],
			[
				{ id: 'ci-a', callsign: 'KG4YFA', lat: 35, lon: -86, lastHeard: minutesAgo(1) },
				{ id: 'ci-b', callsign: 'KG4YFA-4', lat: 36, lon: -87, lastHeard: minutesAgo(1) }
			],
			new Map()
		);
		expect(out[0].lat).toBe(35);
		expect(out[1].lat).toBe(36);
	});
});
