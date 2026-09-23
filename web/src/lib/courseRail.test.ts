import { describe, it, expect } from 'vitest';
import { buildRailAxis, edgePositions, dodgeStops, clusterPips, type EdgePassage } from './courseRail';
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

// --- dodgeStops -----------------------------------------------------------------

describe('dodgeStops', () => {
	it('leaves well-separated stops exactly where they are', () => {
		const got = dodgeStops([{ id: 'a', px: 100 }, { id: 'b', px: 300 }], 800, 18);
		expect(got.map((s) => s.drawPx)).toEqual([100, 300]);
		expect(got.every((s) => s.dodged === false)).toBe(true);
	});

	it('B10: spreads close stops to the minimum spacing, keeping order and their true x', () => {
		// The 100-miler's 30.4 / 31.2 mi pair is 6 px apart at 800 px.
		const got = dodgeStops([{ id: 's4', px: 300 }, { id: 's5', px: 306 }], 800, 18);
		const [a, b] = got;
		expect(b.drawPx - a.drawPx).toBeCloseTo(18, 6);
		expect(a.drawPx).toBeLessThan(b.drawPx); // order preserved
		expect((a.drawPx + b.drawPx) / 2).toBeCloseTo(303, 6); // centred on their mean
		expect(a.truePx).toBe(300); // the leader tick still points at the truth
		expect(a.dodged && b.dodged).toBe(true);
	});

	it('merges cascading groups: dodging one pair can push into a third stop', () => {
		const got = dodgeStops(
			[{ id: 'a', px: 100 }, { id: 'b', px: 104 }, { id: 'c', px: 125 }],
			800,
			18
		);
		for (let i = 1; i < got.length; i++) {
			expect(got[i].drawPx - got[i - 1].drawPx).toBeGreaterThanOrEqual(18 - 1e-9);
		}
	});

	it('never pushes a stop off either end of the track', () => {
		const got = dodgeStops([{ id: 'a', px: 0 }, { id: 'b', px: 2 }, { id: 'c', px: 4 }], 800, 18);
		expect(got[0].drawPx).toBeGreaterThanOrEqual(0);
		const end = dodgeStops([{ id: 'x', px: 796 }, { id: 'y', px: 800 }], 800, 18);
		expect(end[1].drawPx).toBeLessThanOrEqual(800);
		expect(end[1].drawPx - end[0].drawPx).toBeCloseTo(18, 6);
	});

	it('sorts by position whatever order it is given', () => {
		const got = dodgeStops([{ id: 'late', px: 500 }, { id: 'early', px: 50 }], 800, 18);
		expect(got.map((s) => s.id)).toEqual(['early', 'late']);
	});
});

// --- clusterPips ----------------------------------------------------------------

describe('clusterPips', () => {
	it('acceptance 9: fifteen members within one mile at 1400 px are ONE badge', () => {
		// Day 1 is 47.25 mi, so one mile is ~29.6 px at 1400 px.
		const pxPerMile = 1400 / 47.25;
		const pips = Array.from({ length: 15 }, (_, i) => ({ id: `m${i}`, px: 400 + (i / 14) * pxPerMile, rank: i }));
		const got = clusterPips(pips, 10);
		expect(got).toHaveLength(1);
		expect(got[0].members).toHaveLength(15);
	});

	it('keeps members that are further apart than the merge distance separate', () => {
		const got = clusterPips([{ id: 'a', px: 100, rank: 0 }, { id: 'b', px: 140, rank: 0 }], 10);
		expect(got).toHaveLength(2);
		expect(got.map((c) => c.members.length)).toEqual([1, 1]);
	});

	it('lists a cluster\'s members by rank (medical and SAG first, then fresher)', () => {
		const got = clusterPips(
			[
				{ id: 'gen', px: 100, rank: 7 },
				{ id: 'med', px: 102, rank: 0 },
				{ id: 'sag', px: 104, rank: 1 }
			],
			10
		);
		expect(got[0].members).toEqual(['med', 'sag', 'gen']);
	});

	it('pushes a pip off a stop glyph, to the side it is already on', () => {
		// A pip this close to a stop is > 150 m from it (or it would have been
		// folded into the stop), so it must not sit ON the glyph.
		const got = clusterPips([{ id: 'p', px: 203, rank: 0 }], 10, [200], 9);
		expect(got[0].px).toBeGreaterThanOrEqual(209);
		const left = clusterPips([{ id: 'q', px: 197, rank: 0 }], 10, [200], 9);
		expect(left[0].px).toBeLessThanOrEqual(191);
	});
});
