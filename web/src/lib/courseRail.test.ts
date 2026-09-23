import { describe, it, expect } from 'vitest';
import { buildRailAxis, edgePositions, type EdgePassage } from './courseRail';
import { buildRouteIndex } from './routeDistance';

const MI = 1609.344;

// --- buildRailAxis ------------------------------------------------------------

describe('buildRailAxis', () => {
	// ~9.1 km east-west line at lat 35.
	const line: [number, number][] = Array.from({ length: 11 }, (_, i) => [-86 + i * 0.01, 35]);
	const idx = buildRouteIndex(line);

	it('spans the WHOLE course, not the furthest numbered stop', () => {
		// B3: the old axis was max(stop miles). With the finish unnumbered, a
		// stop at 40% made the rail end there and "to go" read 0 with riders
		// still out. The route line knows its own length.
		const axis = buildRailAxis(idx, new Map([['s1', idx.totalMeters * 0.4]]));
		expect(axis.mode).toBe('mile');
		expect(axis.totalMeters).toBe(idx.totalMeters);
		expect(axis.totalMiles).toBeCloseTo(idx.totalMeters / MI, 6);
		expect(axis.pct(idx.totalMeters * 0.4)).toBeCloseTo(40, 6);
		expect(axis.pct(idx.totalMeters)).toBeCloseTo(100, 6);
	});

	it('clamps to the ends rather than drawing off the rail', () => {
		const axis = buildRailAxis(idx, new Map());
		expect(axis.pct(-50)).toBe(0);
		expect(axis.pct(idx.totalMeters + 50)).toBe(100);
	});

	it('places stops through the same pct as everything else', () => {
		const axis = buildRailAxis(idx, new Map([['s1', idx.totalMeters / 2]]));
		expect(axis.pctForStop('s1')).toBeCloseTo(50, 6);
		expect(axis.pctForStop('missing')).toBeNull();
	});

	it('without a route line is honest about not being to scale', () => {
		const axis = buildRailAxis(null, new Map(), ['a', 'b', 'c']);
		expect(axis.mode).toBe('index');
		expect(axis.totalMiles).toBeNull();
		// Evenly spaced by sequence, inset so the ends are not on the border.
		expect(axis.pctForStop('a')).toBeCloseTo(0, 6);
		expect(axis.pctForStop('b')).toBeCloseTo(50, 6);
		expect(axis.pctForStop('c')).toBeCloseTo(100, 6);
		// A chainage means nothing without a line: never placed by distance.
		expect(axis.pct(1000)).toBeNull();
	});

	it('handles a single stop on an index axis without dividing by zero', () => {
		const axis = buildRailAxis(null, new Map(), ['only']);
		expect(axis.pctForStop('only')).toBe(50);
	});
});

// --- edgePositions ------------------------------------------------------------

const miles = new Map([
	['cp1', 13.2],
	['cp2', 24.1],
	['cp3', 35.4],
	['fin', 47.25]
]);
const seq: Record<string, number> = { cp1: 1, cp2: 2, cp3: 3, fin: 4 };
const pass = (label: string, cp: string, at: string): EdgePassage => ({
	label,
	checkpointId: cp,
	seq: seq[cp],
	at
});

describe('edgePositions', () => {
	it('never reported: both edges null, no spread, not inverted', () => {
		const e = edgePositions({ passages: [], sweepReport: null, stopMiles: miles });
		expect(e.lead).toBeNull();
		expect(e.sweep).toBeNull();
		expect(e.spreadMiles).toBeNull();
		expect(e.inverted).toBe(false);
	});

	it('B2: the newest report wins — a later passage beats an older report', () => {
		// 09:40 report "mile 10", 11:05 SWEEP passage at CP3 (mi 35.4). The old
		// code let ANY report beat every passage, so the readout said mi 10
		// while the marker sat at CP3: 25 miles apart, on the radio.
		const e = edgePositions({
			passages: [pass('SWEEP', 'cp3', '2026-09-23T11:05:00Z')],
			sweepReport: { mile: 10, at: '2026-09-23T09:40:00Z' },
			stopMiles: miles
		});
		expect(e.sweep).toMatchObject({ mile: 35.4, source: 'passage', checkpointId: 'cp3' });
	});

	it('B2, other way round: a newer report beats an older passage', () => {
		const e = edgePositions({
			passages: [pass('SWEEP', 'cp1', '2026-09-23T09:00:00Z')],
			sweepReport: { mile: 18.6, at: '2026-09-23T09:30:00Z' },
			stopMiles: miles
		});
		expect(e.sweep).toMatchObject({ mile: 18.6, source: 'report' });
	});

	it('a report with no mile cannot place sweep and does not erase a passage', () => {
		const e = edgePositions({
			passages: [pass('SWEEP', 'cp2', '2026-09-23T09:00:00Z')],
			sweepReport: { mile: null, at: '2026-09-23T10:00:00Z' },
			stopMiles: miles
		});
		expect(e.sweep).toMatchObject({ mile: 24.1, source: 'passage' });
	});

	it('B18: the newest passage wins, not the highest stop number', () => {
		// A LEAD passage mis-logged at CP3, then correctly logged at CP2 later:
		// "highest sequence ever" could never be corrected.
		const e = edgePositions({
			passages: [
				pass('LEAD', 'cp3', '2026-09-23T09:00:00Z'),
				pass('LEAD', 'cp2', '2026-09-23T09:05:00Z')
			],
			sweepReport: null,
			stopMiles: miles
		});
		expect(e.lead).toMatchObject({ mile: 24.1, checkpointId: 'cp2' });
	});

	it('B18: labels match case- and space-insensitively, as the rail does', () => {
		const e = edgePositions({
			passages: [
				pass('Sweep', 'cp1', '2026-09-23T09:00:00Z'),
				pass(' SWEEP ', 'cp2', '2026-09-23T10:00:00Z')
			],
			sweepReport: null,
			stopMiles: miles
		});
		expect(e.sweep).toMatchObject({ checkpointId: 'cp2' });
	});

	it('honours configured labels', () => {
		const e = edgePositions({
			passages: [pass('SWEEP 1', 'cp2', '2026-09-23T10:00:00Z'), pass('SWEEP', 'cp3', '2026-09-23T11:00:00Z')],
			sweepReport: null,
			stopMiles: miles,
			sweepLabel: 'Sweep 1'
		});
		expect(e.sweep).toMatchObject({ checkpointId: 'cp2' });
	});

	it('ignores passages by anyone who is not lead or sweep', () => {
		const e = edgePositions({
			passages: [pass('RIDER 334', 'cp3', '2026-09-23T11:00:00Z')],
			sweepReport: null,
			stopMiles: miles
		});
		expect(e.lead).toBeNull();
		expect(e.sweep).toBeNull();
	});

	it('spread is lead minus sweep in miles', () => {
		const e = edgePositions({
			passages: [pass('LEAD', 'cp3', '2026-09-23T10:00:00Z'), pass('SWEEP', 'cp1', '2026-09-23T10:00:00Z')],
			sweepReport: null,
			stopMiles: miles
		});
		expect(e.spreadMiles).toBeCloseTo(35.4 - 13.2, 6);
		expect(e.inverted).toBe(false);
	});

	it('B12: sweep reported AHEAD of lead is inverted, never a calm 0.0', () => {
		const e = edgePositions({
			passages: [pass('LEAD', 'cp1', '2026-09-23T09:00:00Z'), pass('SWEEP', 'cp3', '2026-09-23T11:00:00Z')],
			sweepReport: null,
			stopMiles: miles
		});
		expect(e.inverted).toBe(true);
		expect(e.spreadMiles).toBeNull();
	});

	it('a passage at a stop that is not on the course keeps its stop, with no mile', () => {
		const e = edgePositions({
			passages: [pass('LEAD', 'cp2', '2026-09-23T10:00:00Z')],
			sweepReport: null,
			stopMiles: new Map([['cp1', 13.2]]) // cp2 not projected
		});
		expect(e.lead).toMatchObject({ mile: null, checkpointId: 'cp2', seq: 2 });
		expect(e.spreadMiles).toBeNull();
	});
});
