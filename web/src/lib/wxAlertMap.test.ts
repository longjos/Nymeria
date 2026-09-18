import { describe, it, expect } from 'vitest';
import { ringsOf, nearestVertex, chipHtml, pointInAlert, drawOrder, fallbackLayers } from './wxAlertMap';
import type { Annotation, WxAlert } from './types';
import { wxAlertFixtureAlerts } from './data/wxAlertFixture';

function makeAlert(overrides: Partial<WxAlert>): WxAlert {
	const base: WxAlert = {
		...wxAlertFixtureAlerts[0],
		id: 'a',
		event: 'Test Event',
		tier: 'statement',
		severity: 'Minor',
		zones: [],
		geometry: undefined,
		affects: { summary: '', entireCourse: false, routeMiles: 0, checkpoints: [], locations: [], stations: [], checkpointSeqRange: [], routeSpans: [] }
	};
	return { ...base, ...overrides };
}

describe('ringsOf', () => {
	it('swaps lon/lat to lat/lon for a Polygon and closes an already-open ring', () => {
		const rings = ringsOf({
			type: 'Polygon',
			coordinates: [
				[
					[-85.75, 42.9],
					[-85.6, 42.9],
					[-85.6, 43.0]
				]
			]
		});
		expect(rings).toHaveLength(1);
		expect(rings[0][0]).toEqual([42.9, -85.75]);
		expect(rings[0][1]).toEqual([42.9, -85.6]);
		expect(rings[0][2]).toEqual([43.0, -85.6]);
		// Not closed in the input (3 points) -> the first vertex is appended.
		expect(rings[0][3]).toEqual([42.9, -85.75]);
	});

	it('does not duplicate the closing vertex when the ring is already closed', () => {
		const rings = ringsOf({
			type: 'Polygon',
			coordinates: [
				[
					[0, 0],
					[1, 0],
					[1, 1],
					[0, 0]
				]
			]
		});
		expect(rings[0]).toHaveLength(4);
	});

	it('flattens every ring of a MultiPolygon', () => {
		const rings = ringsOf({
			type: 'MultiPolygon',
			coordinates: [
				[[[0, 0], [1, 0], [1, 1], [0, 0]]],
				[[[10, 10], [11, 10], [11, 11], [10, 10]]]
			]
		});
		expect(rings).toHaveLength(2);
		expect(rings[1][0]).toEqual([10, 10]);
	});

	it('is empty for a missing/undefined geometry', () => {
		expect(ringsOf(undefined)).toEqual([]);
		expect(ringsOf(null)).toEqual([]);
	});
});

describe('nearestVertex', () => {
	it('picks the ring vertex closest to the given point', () => {
		const rings = ringsOf({
			type: 'Polygon',
			coordinates: [
				[
					[-85.75, 42.9],
					[-85.6, 42.9],
					[-85.6, 43.0],
					[-85.75, 43.0],
					[-85.75, 42.9]
				]
			]
		});
		// Closest to the northeast corner (43.0, -85.6).
		const v = nearestVertex(rings, { lat: 42.99, lon: -85.61 });
		expect(v).toEqual([43.0, -85.6]);
	});

	it('returns null for no rings', () => {
		expect(nearestVertex([], { lat: 0, lon: 0 })).toBeNull();
	});
});

describe('chipHtml', () => {
	it('marks a warning-tier alert with its glyph and countdown, no stale/zone markers when live and polygon-based', () => {
		const tor = wxAlertFixtureAlerts[0];
		const now = Date.parse(tor.endsAt) - 42 * 60_000;
		const html = chipHtml(tor, 'live', now);
		expect(html).toContain('wx-alert-chip-warning');
		expect(html).toContain('TOR WARN');
		expect(html).toContain('42m');
		expect(html).not.toContain('? ');
		expect(html).not.toContain('ZONE');
	});

	it('prefixes "? " when the link is stale or down', () => {
		const tor = wxAlertFixtureAlerts[0];
		const now = Date.parse(tor.endsAt) - 60_000;
		expect(chipHtml(tor, 'stale', now)).toContain('? ');
		expect(chipHtml(tor, 'down', now)).toContain('? ');
		expect(chipHtml(tor, 'live', now)).not.toContain('? ');
	});

	it('appends " · ZONE" for a zone-sourced alert', () => {
		const flood = wxAlertFixtureAlerts[1];
		const now = Date.parse(flood.sent);
		expect(chipHtml(flood, 'live', now)).toContain('· ZONE');
	});
});

describe('pointInAlert', () => {
	const square: WxAlert = makeAlert({
		geometry: {
			type: 'Polygon',
			coordinates: [
				[
					[-1, -1],
					[1, -1],
					[1, 1],
					[-1, 1],
					[-1, -1]
				]
			]
		}
	});

	it('reports a point inside the polygon as inside', () => {
		expect(pointInAlert(square, 0, 0)).toBe(true);
	});

	it('reports a point outside the polygon as outside', () => {
		expect(pointInAlert(square, 5, 5)).toBe(false);
	});

	it('is false for a zone-only alert with no published geometry', () => {
		const zoneOnly = makeAlert({ geometry: undefined });
		expect(pointInAlert(zoneOnly, 0, 0)).toBe(false);
	});
});

describe('drawOrder', () => {
	it('paints statements, then advisories, then watches, then warnings', () => {
		const warning = makeAlert({ id: 'w', tier: 'warning', severity: 'Moderate' });
		const watch = makeAlert({ id: 'wa', tier: 'watch', severity: 'Moderate' });
		const advisory = makeAlert({ id: 'ad', tier: 'advisory', severity: 'Moderate' });
		const statement = makeAlert({ id: 's', tier: 'statement', severity: 'Moderate' });

		expect(drawOrder([warning, watch, advisory, statement]).map((a) => a.id)).toEqual(['s', 'ad', 'wa', 'w']);
	});

	it('within a tier paints Minor before Extreme (the important edge paints last)', () => {
		const minor = makeAlert({ id: 'minor', tier: 'warning', severity: 'Minor' });
		const extreme = makeAlert({ id: 'extreme', tier: 'warning', severity: 'Extreme' });

		expect(drawOrder([extreme, minor]).map((a) => a.id)).toEqual(['minor', 'extreme']);
	});
});

describe('fallbackLayers', () => {
	function fakeLeaflet() {
		const polylineCalls: unknown[][] = [];
		const circleCalls: unknown[][] = [];
		return {
			polylineCalls,
			circleCalls,
			leaflet: {
				polyline: (...args: unknown[]) => {
					polylineCalls.push(args);
					return { kind: 'polyline' } as never;
				},
				circleMarker: (...args: unknown[]) => {
					circleCalls.push(args);
					return { kind: 'circleMarker' } as never;
				}
			}
		};
	}

	it('draws a ribbon over the touched route span and halos every item without a cached zone', () => {
		const routeAnn: Annotation = {
			id: 'ann-route-1',
			type: 'line',
			label: 'Route',
			geometry: JSON.stringify({
				type: 'LineString',
				coordinates: [
					[-85.7, 42.9],
					[-85.68, 42.92],
					[-85.66, 42.94],
					[-85.64, 42.96]
				]
			}),
			createdAt: '',
			updatedAt: '',
			category: 'route',
			status: 'active',
			priority: 'normal',
			missionIds: []
		} as unknown as Annotation;

		const alert = wxAlertFixtureAlerts[1]; // Flood Watch: one cached zone, one missing
		const { leaflet, polylineCalls, circleCalls } = fakeLeaflet();

		const layers = fallbackLayers(alert, [routeAnn], leaflet);

		expect(polylineCalls.length).toBeGreaterThan(0);
		expect(layers.length).toBe(polylineCalls.length + circleCalls.length);
	});

	it('draws nothing extra when every zone is cached', () => {
		const tor = wxAlertFixtureAlerts[0]; // polygon alert with a fully cached zone, no route spans
		const { leaflet, polylineCalls, circleCalls } = fakeLeaflet();
		const layers = fallbackLayers(tor, [], leaflet);
		expect(polylineCalls).toHaveLength(0);
		expect(circleCalls).toHaveLength(0);
		expect(layers).toHaveLength(0);
	});
});
