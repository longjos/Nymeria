import { describe, it, expect } from 'vitest';
import { buildRouteIndex, type RouteIndex, type Stop } from './routeDistance';
import { resolveSagPoint, courseMismatchNotice, type SagGeoContext } from './sagGeo';
import type { SAGLocation } from './types';

const MI = 1609.344;

/** An east-west line at lat 35 starting at lon -86; 101 vertices 0.01 deg apart
 *  is roughly a 56.6 mi course, long enough for mile-34 fixtures to be real. */
function line(n = 101, stepDeg = 0.01): [number, number][] {
	const out: [number, number][] = [];
	for (let i = 0; i < n; i++) out.push([-86 + i * stepDeg, 35]);
	return out;
}

function ctx(over: Partial<SagGeoContext> = {}): SagGeoContext {
	return {
		routeIndex: buildRouteIndex(line()),
		routeCount: 1,
		routeName: '48',
		annotationPoints: new Map(),
		stops: [],
		...over
	};
}

function loc(over: Partial<SAGLocation> = {}): SAGLocation {
	return { kind: 'course', ...over };
}

describe('resolveSagPoint — resolution order (spec §2)', () => {
	it('rule 1: explicit coordinates win over everything else', () => {
		const p = resolveSagPoint(
			loc({ lat: 35.5, lon: -86.5, annotationId: 'a1', mileMarker: 10 }),
			ctx({ annotationPoints: new Map([['a1', { lat: 1, lon: 1, label: 'A1' }]]) })
		);
		expect(p.placed).toBe(true);
		if (!p.placed) return;
		expect(p.via).toBe('coordinate');
		expect(p.lat).toBe(35.5);
		expect(p.lon).toBe(-86.5);
	});

	it('rule 2: an annotation reference beats mileage', () => {
		const p = resolveSagPoint(
			loc({ annotationId: 'a1', mileMarker: 10 }),
			ctx({ annotationPoints: new Map([['a1', { lat: 35.2, lon: -85.9, label: 'Maxwell' }]]) })
		);
		expect(p.placed && p.via).toBe('annotation');
		expect(p.placed && p.lat).toBe(35.2);
	});

	it('rule 2: a pickup AT a course stop carries that stop\'s chainage', () => {
		// The stop's chainage is already placed by its sequence number
		// (courseGeo), so it is exact. Leaving it null made dispatch rank by
		// straight line and announce "No course loaded" with a course on the map.
		const p = resolveSagPoint(
			loc({ annotationId: 's2' }),
			ctx({
				annotationPoints: new Map([['s2', { lat: 35, lon: -85.8, label: 'Eakin' }]]),
				stops: [{ id: 's2', label: 'Eakin', chainageMeters: 20 * MI, offTrackMeters: 5 }]
			})
		);
		expect(p.placed && p.via).toBe('annotation');
		expect(p.placed && p.chainageMeters).toBe(20 * MI);
	});

	it('rule 2: any other annotation still gets no chainage (the projection is ambiguous)', () => {
		const p = resolveSagPoint(
			loc({ annotationId: 'a1' }),
			ctx({
				annotationPoints: new Map([['a1', { lat: 35, lon: -85.8, label: 'Parking' }]]),
				stops: [{ id: 's2', label: 'Eakin', chainageMeters: 20 * MI, offTrackMeters: 5 }]
			})
		);
		expect(p.placed && p.chainageMeters).toBe(null);
	});

	it('falls through to mileage when the annotation id is dangling', () => {
		// A deleted annotation must not black-hole a request that also carries a
		// perfectly good mile marker.
		const p = resolveSagPoint(loc({ annotationId: 'gone', mileMarker: 10 }), ctx());
		expect(p.placed && p.via).toBe('mileage');
	});

	it('rule 3: mileMarker projects onto the route', () => {
		const c = ctx();
		const p = resolveSagPoint(loc({ mileMarker: 34 }), c);
		expect(p.placed).toBe(true);
		if (!p.placed) return;
		expect(p.via).toBe('mileage');
		expect(p.chainageMeters).toBeCloseTo(34 * MI, 3);
	});

	it('rule 4: milesRemaining is measured back from the finish', () => {
		const c = ctx();
		const total = c.routeIndex!.totalMeters;
		const p = resolveSagPoint(loc({ milesRemaining: 10 }), c);
		expect(p.placed && p.via).toBe('mileage');
		expect(p.placed && p.chainageMeters).toBeCloseTo(total - 10 * MI, 3);
	});

	it('rule 4: milesRemaining = 0 places the finish, not an off-route refusal', () => {
		// The float slop this exercises is exactly why pointAtChainage tolerates
		// a millimetre at the ends.
		const p = resolveSagPoint(loc({ milesRemaining: 0 }), ctx());
		expect(p.placed).toBe(true);
	});

	it('rule 5: a named stop kind resolves through the stop list', () => {
		const c = ctx({
			stops: [{ id: 's1', label: 'Flat Creek', chainageMeters: 20 * MI, offTrackMeters: 5 }]
		});
		const p = resolveSagPoint(loc({ kind: 'next_reststop' }), c);
		expect(p.placed && p.via).toBe('named-stop');
		expect(p.placed && p.chainageMeters).toBeCloseTo(20 * MI, 3);
	});

	it('rule 5 does not fire for a kind that names no particular place', () => {
		const p = resolveSagPoint(loc({ kind: 'other' }), ctx({ stops: [] }));
		expect(p.placed).toBe(false);
	});
});

describe('resolveSagPoint — the unplaceable taxonomy', () => {
	it('no-location: bare kind, nothing else', () => {
		const p = resolveSagPoint(loc({ description: 'just past the red barn' }), ctx());
		expect(p.placed).toBe(false);
		if (p.placed) return;
		expect(p.reason).toBe('no-location');
		// The caller's own words survive, so the dock can quote them.
		expect(p.label).toContain('red barn');
	});

	it('no-course: mileage given, but no route is loaded', () => {
		const p = resolveSagPoint(loc({ mileMarker: 34 }), ctx({ routeIndex: null, routeCount: 0 }));
		expect(p.placed).toBe(false);
		if (p.placed) return;
		expect(p.reason).toBe('no-course');
	});

	it('route-unknown: several routes loaded and the request does not say which', () => {
		const p = resolveSagPoint(loc({ mileMarker: 34 }), ctx({ routeIndex: null, routeCount: 3 }));
		expect(p.placed).toBe(false);
		if (p.placed) return;
		expect(p.reason).toBe('route-unknown');
	});

	it('route-unknown: the named route is not the one we have geometry for', () => {
		const p = resolveSagPoint(
			loc({ mileMarker: 34, route: '100' }),
			ctx({ routeName: '48', routeCount: 2 })
		);
		expect(p.placed).toBe(false);
		if (p.placed) return;
		expect(p.reason).toBe('route-unknown');
		expect(p.label).toContain('100');
	});

	it('accepts a matching route name, case- and space-insensitively', () => {
		const p = resolveSagPoint(loc({ mileMarker: 5, route: ' 48 ' }), ctx({ routeName: '48', routeCount: 2 }));
		expect(p.placed && p.via).toBe('mileage');
	});

	it('off-route: mileage past the end of the course', () => {
		const p = resolveSagPoint(loc({ mileMarker: 900 }), ctx());
		expect(p.placed).toBe(false);
		if (p.placed) return;
		expect(p.reason).toBe('off-route');
	});

	it('off-route: negative mileage', () => {
		const p = resolveSagPoint(loc({ mileMarker: -3 }), ctx());
		expect(p.placed).toBe(false);
		if (p.placed) return;
		expect(p.reason).toBe('off-route');
	});

	it('an out-of-range coordinate is still placed — it is a fact, not a derivation', () => {
		// A GPS fix 40 miles off the course is where the caller actually is.
		// Refusing it would hide a real, known position.
		const p = resolveSagPoint(loc({ lat: 36.9, lon: -88.2 }), ctx());
		expect(p.placed).toBe(true);
		if (!p.placed) return;
		expect(p.chainageMeters).toBeNull();
	});
});

describe('resolveSagPoint — labels', () => {
	it('names a mileage pin by its mile marker', () => {
		const p = resolveSagPoint(loc({ mileMarker: 34 }), ctx());
		expect(p.label).toMatch(/34/);
	});

	it('names an annotation pin by the annotation label', () => {
		const p = resolveSagPoint(
			loc({ annotationId: 'a1' }),
			ctx({ annotationPoints: new Map([['a1', { lat: 35.2, lon: -85.9, label: 'Maxwell Chapel' }]]) })
		);
		expect(p.label).toBe('Maxwell Chapel');
	});
});

describe('courseMismatchNotice', () => {
	it('is silent when measured and advertised agree within 2%', () => {
		expect(courseMismatchNotice(76046, 47.5)).toBeNull();
		expect(courseMismatchNotice(76046, 48)).toBeNull();
	});

	it('speaks when they disagree by more than 2%', () => {
		// 155 km measures 96.3 mi against an advertised 100 — 3.7% off, which on
		// this course is nearly four miles of pin error.
		const n = courseMismatchNotice(155_000, 100);
		expect(n).not.toBeNull();
		expect(n).toContain('96.3');
		expect(n).toContain('100');
	});

	it('is silent when there is no advertised distance to compare against', () => {
		expect(courseMismatchNotice(76046, null)).toBeNull();
		expect(courseMismatchNotice(76046, 0)).toBeNull();
	});
});
