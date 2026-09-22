// Zone-set resolution tests (finding 12, docs/ride-strip-spec.md §8/§9): the
// zone SET rendered by RideStrip.svelte is a pure function of (phase,
// viewport) — PHASE_ZONE_TEMPLATE + tabletizeZones — independent of the
// live-data derivations the rest of ride.ts builds on top of it. Testing at
// this layer avoids mocking the whole netcontrol/wxAlerts/weather store
// graph `rideZones` ultimately depends on.
import { describe, it, expect } from 'vitest';
import {
	PHASE_ZONE_TEMPLATE, RIDE_GRID_COLUMNS, zoneSpanSum, tabletizeZones,
	type RideZoneDescriptor, type RideZoneId
} from './ride';
import { RIDE_PHASE_ORDER } from '$lib/rideMeta';

function zone(id: RideZoneId, opts: Partial<RideZoneDescriptor> = {}): RideZoneDescriptor {
	return {
		id,
		label: id.toUpperCase(),
		lines: [{ text: `${id}-value`, size: 'value' }],
		aria: `${id} aria`,
		...opts
	};
}

/** The full, all-optional-zones-present candidate set for a phase, with
 * realistic spans applied — the exact worst case PHASE_ZONE_TEMPLATE
 * describes. Any real (live-data) resolution only ever OMITS zones from
 * this set, never adds to it, so asserting against this superset covers
 * every real combination of optional-zone presence for that phase. */
function fullZonesForPhase(phaseId: (typeof RIDE_PHASE_ORDER)[number]): RideZoneDescriptor[] {
	return PHASE_ZONE_TEMPLATE[phaseId].map(({ id, span }) => zone(id, span ? { span } : {}));
}

/** Simulates the EMERGENCY absorption step `rideZones` applies before the
 * tablet transform: SAG + WX dropped, TRAFFIC expands to span 3. */
function withEmergency(zones: RideZoneDescriptor[]): RideZoneDescriptor[] {
	return zones
		.filter((z) => z.id !== 'sag' && z.id !== 'wx')
		.map((z) => (z.id === 'traffic' ? { ...z, span: 3 as const, borderVar: '--color-ride-emergency' } : z));
}

describe('PHASE_ZONE_TEMPLATE', () => {
	it('defines a template for every phase in RIDE_PHASE_ORDER', () => {
		for (const phase of RIDE_PHASE_ORDER) {
			expect(PHASE_ZONE_TEMPLATE[phase], phase).toBeDefined();
			expect(PHASE_ZONE_TEMPLATE[phase].length, phase).toBeGreaterThan(0);
		}
	});

	it('never lists the same zone id twice within one phase', () => {
		for (const phase of RIDE_PHASE_ORDER) {
			const ids = PHASE_ZONE_TEMPLATE[phase].map((z) => z.id);
			expect(new Set(ids).size, phase).toBe(ids.length);
		}
	});
});

describe('desktop span-sum invariant', () => {
	// The desktop grid (`grid-auto-flow: column`) cannot wrap into a second
	// row regardless of column count, but RIDE_GRID_COLUMNS.desktop documents
	// the template's own computed worst case so a future zone-set change that
	// silently grows it gets caught here rather than discovered by an
	// operator staring at nine 90px tiles.
	for (const phase of RIDE_PHASE_ORDER) {
		it(`${phase}: full zone set fits the desktop ceiling`, () => {
			const sum = zoneSpanSum(fullZonesForPhase(phase));
			expect(sum).toBeLessThanOrEqual(RIDE_GRID_COLUMNS.desktop);
		});

		it(`${phase}: EMERGENCY-absorbed zone set fits the desktop ceiling`, () => {
			const sum = zoneSpanSum(withEmergency(fullZonesForPhase(phase)));
			expect(sum).toBeLessThanOrEqual(RIDE_GRID_COLUMNS.desktop);
		});
	}
});

describe('tablet span-sum invariant (spec §9: six zones, --ride-strip-h: 140px)', () => {
	// This is the load-bearing assertion for finding 12: no (phase,
	// optional-zone-presence, emergency) combination may ever resolve to
	// more than 6 column-units at tablet width — the exact ceiling the
	// spec's six-zone wireframe draws, and the invariant whose absence let
	// NET/STOPS/SHIFT collapse into 96px columns.
	for (const phase of RIDE_PHASE_ORDER) {
		it(`${phase}: full zone set fits the tablet ceiling after merging`, () => {
			const sum = zoneSpanSum(tabletizeZones(fullZonesForPhase(phase)));
			expect(sum).toBeLessThanOrEqual(RIDE_GRID_COLUMNS.tablet);
		});

		it(`${phase}: EMERGENCY-absorbed zone set fits the tablet ceiling after merging`, () => {
			const sum = zoneSpanSum(tabletizeZones(withEmergency(fullZonesForPhase(phase))));
			expect(sum).toBeLessThanOrEqual(RIDE_GRID_COLUMNS.tablet);
		});
	}

	it('every resolved tablet zone has span exactly 1 (no double-width tile at 6 columns)', () => {
		for (const phase of RIDE_PHASE_ORDER) {
			for (const z of tabletizeZones(fullZonesForPhase(phase))) {
				expect(z.span ?? 1, `${phase}/${z.id}`).toBe(1);
			}
		}
	});
});

describe('tabletizeZones merges', () => {
	it('merges NET+SHIFT into one zone keeping the NET id (so navigateZone needs no new case)', () => {
		const zones = [zone('net', { label: 'NET', lines: [{ text: '4:12', size: 'value' }] }), zone('shift', { label: 'SHIFT', lines: [{ text: 'KE7ABC', size: 'value' }] })];
		const out = tabletizeZones(zones);
		expect(out.map((z) => z.id)).toEqual(['net']);
		expect(out[0].label).toBe('NET+SHIFT');
		expect(out[0].lines.map((l) => l.text)).toEqual(['4:12', 'KE7ABC']);
	});

	it('merges STOPS+SAG into LOGISTICS keeping the STOPS id', () => {
		const zones = [zone('stops', { lines: [{ text: '◆4 ⊘2', size: 'value' }] }), zone('sag', { lines: [{ text: '⬒ 2 open', size: 'value' }] })];
		const out = tabletizeZones(zones);
		expect(out.map((z) => z.id)).toEqual(['stops']);
		expect(out[0].label).toBe('LOGISTICS');
	});

	it('merges CLEARING+SAG into LOGISTICS keeping the CLEARING id (COLLAPSE phase)', () => {
		const zones = [zone('clearing'), zone('sag')];
		const out = tabletizeZones(zones);
		expect(out.map((z) => z.id)).toEqual(['clearing']);
		expect(out[0].label).toBe('LOGISTICS');
	});

	it('never drops the "no SAG units" warning line when merged into LOGISTICS', () => {
		const zones = [
			zone('stops', { lines: [{ text: '◆4', size: 'value' }] }),
			zone('sag', { tone: 'warning', lines: [{ text: '⬒ no SAG units', size: 'value', tone: 'warning' }] })
		];
		const out = tabletizeZones(zones);
		const warningLine = out[0].lines.find((l) => l.text === '⬒ no SAG units');
		expect(warningLine?.tone).toBe('warning');
	});

	it('is a no-op merge when only one side of a pair is present (e.g. EMERGENCY already removed SAG)', () => {
		const zones = [zone('stops'), zone('traffic', { span: 3 })];
		const out = tabletizeZones(zones);
		expect(out.map((z) => z.id).sort()).toEqual(['stops', 'traffic']);
	});

	it('caps a merged zone at 3 lines (the tablet row cannot fit a 4th)', () => {
		const zones = [
			zone('net', { lines: [{ text: 'a', size: 'value' }, { text: 'b', size: 'body' }] }),
			zone('shift', { lines: [{ text: 'c', size: 'body' }, { text: 'd', size: 'label' }] })
		];
		const out = tabletizeZones(zones);
		expect(out[0].lines.length).toBeLessThanOrEqual(3);
	});
});

describe('tabletizeZones WX reduction (spec §9: "WX -> glyph + heat index only")', () => {
	it('keeps the heat-index line and drops the plain temperature-only line', () => {
		const wx = zone('wx', {
			lines: [
				{ text: '94°F', size: 'value' },
				{ text: 'HI 101°F', size: 'body', tone: 'warning' },
				{ text: 'HEAT', size: 'label', tone: 'warning' }
			]
		});
		const [out] = tabletizeZones([wx]);
		expect(out.lines).toHaveLength(1);
		expect(out.lines[0].text).toContain('HI 101');
	});

	it('falls back to the first line when there is no heat-index reading', () => {
		const wx = zone('wx', { lines: [{ text: '58°F', size: 'value' }] });
		const [out] = tabletizeZones([wx]);
		expect(out.lines).toHaveLength(1);
		expect(out.lines[0].text).toContain('58°F');
	});
});

describe('tabletizeZones span clamp', () => {
	it('clamps every span down to 1, including an EMERGENCY-expanded span-3 zone', () => {
		const zones = [zone('lastRider', { span: 2 }), zone('traffic', { span: 3 })];
		const out = tabletizeZones(zones);
		for (const z of out) expect(z.span ?? 1).toBe(1);
	});
});

describe('zoneSpanSum', () => {
	it('treats a missing span as 1', () => {
		expect(zoneSpanSum([{ }, { span: 2 }, { span: 1 }])).toBe(4);
	});

	it('sums an empty list to 0', () => {
		expect(zoneSpanSum([])).toBe(0);
	});
});
