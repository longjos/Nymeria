import { describe, it, expect } from 'vitest';
import { buildRouteIndex } from './routeDistance';
import { buildRailModel, type RailInput } from './railModel';
import type { Annotation, CheckpointWithPassages, CourseState } from './types';

const MI = 1609.344;
const NOW = Date.parse('2026-09-23T12:00:00Z');
const ago = (min: number) => new Date(NOW - min * 60_000).toISOString();

// ~9.1 km east-west line at lat 35: one-way, no shared road.
const LAT0 = 35;
const M_PER_DEG_LON = 91085.8;
const idx = buildRouteIndex(Array.from({ length: 11 }, (_, i) => [-86 + i * 0.01, LAT0] as [number, number]));
const lonAt = (m: number) => -86 + m / M_PER_DEG_LON;

function ann(id: string, category: string, m: number, status = 'active', label = id): Annotation {
	return {
		id,
		type: 'point',
		label,
		geometry: JSON.stringify({ type: 'Point', coordinates: [lonAt(m), LAT0] }),
		createdAt: ago(60),
		updatedAt: ago(60),
		category: category as Annotation['category'],
		status,
		priority: 'routine',
		missionIds: []
	};
}

function cp(a: Annotation, seq: number, passages: { label: string; at: string }[] = []): CheckpointWithPassages {
	return {
		annotation: a,
		meta: { annotationId: a.id, netId: 'n', sequenceNumber: seq },
		passages: passages.map((p, i) => ({
			id: `${a.id}-p${i}`,
			checkpointId: a.id,
			netId: 'n',
			label: p.label,
			passageTime: p.at,
			direction: '',
			reportedBy: 'test'
		})),
		passageCount: passages.length
	};
}

const rs1 = ann('rs1', 'aid', 2000);
const rs2 = ann('rs2', 'aid', 5000);
const fin = ann('fin', 'finish', idx.totalMeters);

function input(over: Partial<RailInput> = {}): RailInput {
	return {
		routeIndex: idx,
		checkpoints: [cp(rs1, 1), cp(rs2, 2), cp(fin, 3)],
		checkIns: [],
		rosterMemory: new Map(),
		nowMs: NOW,
		...over
	};
}

function closure(checkpointId: string, state: string, sweepPassedAt?: string) {
	return {
		checkpointId,
		label: checkpointId,
		category: 'aid',
		sequenceNumber: 0,
		outOfOrder: false,
		passageCount: 0,
		closure: {
			netId: 'n',
			checkpointId,
			division: '',
			state,
			sweepPassedAt,
			ridersClearBy: '',
			sweepPassedBy: '',
			closedBy: '',
			closedByOverride: false,
			overrideReason: '',
			reopenCount: 0,
			note: '',
			updatedAt: ago(5)
		}
	};
}

function course(over: Partial<CourseState> = {}): CourseState {
	return {
		netId: 'n',
		config: { leadLabel: 'LEAD', sweepLabel: 'SWEEP' } as CourseState['config'],
		stations: [],
		clearThroughSeq: 0,
		clearThroughLabel: '',
		stationsOpen: 0,
		stationsClosed: 0,
		allStationsClosed: false,
		sweep: { lastCheckpointId: '', lastCheckpointSeq: 0, nextStationId: '', nextStationLabel: '' },
		shutoffs: [],
		supportedExceptions: 0,
		unsupportedExceptions: 0,
		rerouteCountTotal: 0,
		updatedAt: ago(1),
		...over
	} as CourseState;
}

describe('buildRailModel — stops', () => {
	it('places stops on one mile axis spanning the whole course', () => {
		const m = buildRailModel(input());
		expect(m.axis.mode).toBe('mile');
		expect(m.stops.map((s) => s.id)).toEqual(['rs1', 'rs2', 'fin']);
		expect(m.stops[0].mile).toBeCloseTo(2000 / MI, 2);
		expect(m.stops[0].pct).toBeCloseTo((2000 / idx.totalMeters) * 100, 1);
		expect(m.stops[2].pct).toBeCloseTo(100, 1);
	});

	it('B5: reads each stop from its LIVE annotation, not the checkpoint snapshot', () => {
		const moved = { ...rs1, status: 'at-capacity', label: 'Maxwell (moved)' };
		const m = buildRailModel(input({ liveAnnotations: new Map([['rs1', moved]]) }));
		expect(m.stops[0].label).toBe('Maxwell (moved)');
		expect(m.stops[0].state).toBe('at capacity');
	});

	it("B6: colours each stop from its OWN category's palette", () => {
		// The checkpoint palette paints "active" amber; an open aid station is green.
		const m = buildRailModel(input());
		expect(m.stops[0].color.toLowerCase()).toBe('#22c55e');
	});

	it('closure state outranks annotation status', () => {
		const m = buildRailModel(
			input({ courseState: course({ stations: [closure('rs1', 'sweep_passed', ago(30)), closure('rs2', 'closed')] as never }) })
		);
		expect(m.stops[0].state).toBe('sweep passed, ready to close');
		expect(m.stops[1].state).toBe('closed');
	});

	it('without a course line: an honest index axis, stops evenly spaced', () => {
		const m = buildRailModel(input({ routeIndex: null }));
		expect(m.axis.mode).toBe('index');
		expect(m.stops.map((s) => s.pct)).toEqual([0, 50, 100]);
		expect(m.stops.every((s) => s.mile === null)).toBe(true);
	});
});

describe('buildRailModel — lead and sweep (reported only)', () => {
	it('a stop marked sweep-passed places sweep; lead comes from its passage', () => {
		const m = buildRailModel(
			input({
				checkpoints: [cp(rs1, 1), cp(rs2, 2, [{ label: 'LEAD', at: ago(6) }]), cp(fin, 3)],
				courseState: course({ stations: [closure('rs1', 'sweep_passed', ago(40))] as never })
			})
		);
		expect(m.edges.lead?.checkpointId).toBe('rs2');
		expect(m.edges.sweep).toMatchObject({ checkpointId: 'rs1', source: 'sweep-passed' });
		expect(m.leadPct).toBeCloseTo(m.stops[1].pct!, 6);
		expect(m.sweepPct).toBeCloseTo(m.stops[0].pct!, 6);
	});

	it('uses the configured labels', () => {
		const m = buildRailModel(
			input({
				checkpoints: [cp(rs1, 1, [{ label: 'Tail', at: ago(3) }]), cp(rs2, 2), cp(fin, 3)],
				courseState: course({ config: { leadLabel: 'Front', sweepLabel: 'Tail' } as CourseState['config'] })
			})
		);
		expect(m.sweepLabel).toBe('Tail');
		expect(m.edges.sweep?.checkpointId).toBe('rs1');
	});

	it('never takes a position from the roster, even from the sweep unit', () => {
		// The sweep unit reported, and its van is now well ahead: the SWEEP
		// marker stays on the report (the user's rule).
		const m = buildRailModel(
			input({
				courseState: course({
					sweep: {
						lastCheckpointId: '',
						lastCheckpointSeq: 0,
						nextStationId: '',
						nextStationLabel: '',
						latestReport: { routeMile: 1, reportedAt: ago(20), checkInId: 'van' } as never
					}
				}),
				checkIns: [{ id: 'van', callsign: 'VAN', category: 'sag', lat: LAT0, lon: lonAt(8000), lastHeard: ago(1), source: 'aprs' }]
			})
		);
		expect(m.edges.sweep?.mile).toBe(1);
		expect(m.roster[0].isSweepUnit).toBe(true);
		expect(m.roster[0].chainageMeters).toBeCloseTo(8000, -1);
	});
});

describe('buildRailModel — roster, gates, incidents, weather', () => {
	it('folds members at a stop into its staffing count and counts the unplaceable', () => {
		const m = buildRailModel(
			input({
				checkIns: [
					{ id: 'a', callsign: 'A', category: 'marshal', lat: LAT0, lon: lonAt(2030), lastHeard: ago(1), source: 'aprs' },
					{ id: 'b', callsign: 'B', category: 'sag', lat: LAT0 + 0.05, lon: lonAt(4000), lastHeard: ago(1), source: 'aprs' },
					{ id: 'c', callsign: 'C', category: 'general', lastHeard: ago(1), source: 'voice' }
				]
			})
		);
		expect(m.staffing.get('rs1')).toBe(1);
		expect(m.stops[0].staffed).toBe(1);
		expect(m.unplaced).toEqual({ offCourse: 1, noPosition: 1, noCourse: 0 });
	});

	it('draws a shutoff only where it really is, and never a cancelled one', () => {
		const m = buildRailModel(
			input({
				courseState: course({
					shutoffs: [
						{ id: 'g1', name: 'Gate 1', routeMile: 3, status: 'planned' },
						{ id: 'g2', name: 'No mile', status: 'planned' },
						{ id: 'g3', name: 'Cancelled', routeMile: 4, status: 'cancelled' }
					] as never
				})
			})
		);
		expect(m.gates.map((g) => g.id)).toEqual(['g1']);
		expect(m.gates[0].pct).toBeCloseTo(((3 * MI) / idx.totalMeters) * 100, 1);
	});

	it('B1: drops an incident that is not on the course instead of drawing it mid-course', () => {
		const tier = { id: 'high', label: 'High', rank: 3 } as never;
		const m = buildRailModel(
			input({
				incidents: [
					{ id: 'ok', label: 'SAG', tier, chainageMeters: 3000 },
					{ id: 'off', label: 'Beyond the finish', tier, chainageMeters: idx.totalMeters + 5000 }
				]
			})
		);
		expect(m.incidents.map((i) => i.id)).toEqual(['ok']);
	});

	it('brackets a weather alert across its stops on the same axis', () => {
		const m = buildRailModel(
			input({
				wxAlerts: [
					{ id: 'w1', event: 'Severe Thunderstorm Warning', shortCode: 'SVR', tier: 'warning', affects: { checkpointSeqRange: [1, 2] } },
					{ id: 'w2', event: 'Statement', shortCode: 'SPS', tier: 'statement', affects: { checkpointSeqRange: [1, 3] } }
				] as never
			})
		);
		expect(m.wx).toHaveLength(1); // statements are not bracketed
		expect(m.wx[0].fromPct).toBeCloseTo(m.stops[0].pct!, 6);
		expect(m.wx[0].toPct).toBeCloseTo(m.stops[1].pct!, 6);
	});
});
