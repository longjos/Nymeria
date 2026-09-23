/**
 * The course rail's view model: plain data in, everything the rail draws out
 * (docs/course-rail-spec.md). Pure — no stores, no DOM.
 *
 * There are three rails — the ride strip, the SituationBoard panel and the
 * agency dashboard — and the dashboard keeps its own data, deliberately
 * separate from the main app's stores. One model built from plain inputs is
 * what lets all three show the same course the same way: before this, the
 * panels were evenly spaced while the strip was to scale, so one course had
 * two shapes depending on where you looked (spec B15).
 *
 * LEAD and SWEEP are REPORTED positions only (passages, sweep reports, stops
 * marked sweep-passed). The user's rule, settled: "We'd always take the
 * reported sweep over the GPS." The roster input never feeds them.
 */
import { buildRailAxis, edgePositions, type EdgePassage, type Edges, type RailAxis } from './courseRail';
import { projectStopsBySequence, type RouteIndex, type SequencedStop } from './routeDistance';
import {
	projectRosterOnCourse,
	type RosterCheckIn,
	type RosterMemory,
	type RosterOnCourse,
	type RosterStation,
	type RosterStopPoint
} from './rosterCourse';
import { statusColor } from './annotationMeta';
import { STOP_GLYPHS } from './rideMeta';
import type {
	Annotation,
	AnnotationCategory,
	CheckpointWithPassages,
	CourseState,
	PriorityTier,
	RidePhaseID,
	WxAlert
} from './types';

const MI = 1609.344;

export interface RailStopModel {
	id: string;
	seq: number;
	label: string;
	shortName?: string;
	/** Null when the stop is not on the course line (or there is no line). */
	mile: number | null;
	/** 0..100 along the rail; null only if the stop cannot be placed at all. */
	pct: number | null;
	glyph: string;
	/** Spoken state: 'open', 'at capacity', 'awaiting sweep', … */
	state: string;
	color: string;
	outOfOrder: boolean;
	/** Roster members within 150 m of this stop. */
	staffed: number;
	passageCount: number;
}

export interface RailGateModel {
	id: string;
	name: string;
	mile: number;
	status: string;
	pct: number;
}

export interface RailIncidentInput {
	id: string;
	label: string;
	tier: PriorityTier;
	chainageMeters: number;
}

export interface RailIncidentModel extends RailIncidentInput {
	pct: number;
}

export interface RailWxModel {
	id: string;
	event: string;
	shortCode: string;
	tier: string;
	fromSeq: number;
	toSeq: number;
	fromPct: number;
	toPct: number;
}

export interface RailModel {
	axis: RailAxis;
	/** For mile -> point (exact), so any marker can fly the map to itself. */
	routeIndex: RouteIndex | null;
	leadLabel: string;
	sweepLabel: string;
	edges: Edges;
	leadPct: number | null;
	sweepPct: number | null;
	stops: RailStopModel[];
	gates: RailGateModel[];
	incidents: RailIncidentModel[];
	wx: RailWxModel[];
	roster: RosterOnCourse[];
	staffing: Map<string, number>;
	/** Roster members the rail cannot show, and why — counted, never hidden. */
	unplaced: { offCourse: number; noPosition: number; noCourse: number };
	phase: RidePhaseID | null;
	sweepSpeedMph: number | null;
}

export interface RailInput {
	routeIndex: RouteIndex | null;
	checkpoints: CheckpointWithPassages[];
	/** Live annotations by id. A stop is drawn from here when present, so a
	 *  moved or re-statused stop updates at once (B5). */
	liveAnnotations?: Map<string, Annotation>;
	courseState?: CourseState | null;
	checkIns: RosterCheckIn[];
	stationsByCallsign?: Map<string, RosterStation>;
	/** Continuity memory for the roster projection; the caller keeps it. */
	rosterMemory: RosterMemory;
	/** Open incidents that ALREADY have a chainage. Unplaceable ones are the
	 *  caller's to leave out — never drawn at a made-up place (B1). */
	incidents?: RailIncidentInput[];
	wxAlerts?: WxAlert[];
	phase?: RidePhaseID | null;
	nowMs: number;
}

function pointOf(a: Annotation): { lat: number; lon: number } | null {
	try {
		const g = typeof a.geometry === 'string' ? JSON.parse(a.geometry) : a.geometry;
		if (g?.type !== 'Point' || !Array.isArray(g.coordinates)) return null;
		const [lon, lat] = g.coordinates;
		return Number.isFinite(lat) && Number.isFinite(lon) ? { lat, lon } : null;
	} catch {
		return null;
	}
}

function stopState(closure: string | undefined, status: string): { glyph: string; state: string } {
	if (closure === 'closed') return { glyph: STOP_GLYPHS.closed, state: 'closed' };
	if (closure === 'sweep_passed') return { glyph: STOP_GLYPHS.readyToClose, state: 'sweep passed, ready to close' };
	if (closure === 'riders_clear') return { glyph: STOP_GLYPHS.awaitingSweep, state: 'awaiting sweep' };
	if (status === 'at-capacity') return { glyph: STOP_GLYPHS.atCapacity, state: 'at capacity' };
	if (status === 'open' || status === 'active') return { glyph: STOP_GLYPHS.open, state: 'open' };
	if (status === 'closed') return { glyph: STOP_GLYPHS.closed, state: 'closed' };
	return { glyph: STOP_GLYPHS.planned, state: 'planned' };
}

export function buildRailModel(input: RailInput): RailModel {
	const { routeIndex: idx, courseState: c } = input;
	const cps = input.checkpoints.slice().sort((a, b) => a.meta.sequenceNumber - b.meta.sequenceNumber);
	const live = (cp: CheckpointWithPassages) => input.liveAnnotations?.get(cp.meta.annotationId) ?? cp.annotation;

	// Stops: each on the leg its sequence number implies (B11), from its live point.
	const seqd: SequencedStop[] = [];
	const stopPoints: RosterStopPoint[] = [];
	for (const cp of cps) {
		const pt = pointOf(live(cp));
		if (!pt) continue;
		seqd.push({ id: cp.meta.annotationId, seq: cp.meta.sequenceNumber, ...pt });
		stopPoints.push({ id: cp.meta.annotationId, seq: cp.meta.sequenceNumber, ...pt });
	}
	const placed = idx ? projectStopsBySequence(idx, seqd) : new Map();
	const stopChainage = new Map<string, number>();
	for (const [id, p] of placed) stopChainage.set(id, p.chainageMeters);
	const stopMiles = new Map(Array.from(stopChainage, ([id, m]) => [id, m / MI]));

	const axis = buildRailAxis(idx, stopChainage, cps.map((cp) => cp.meta.annotationId));

	// Roster.
	const roster = projectRosterOnCourse({
		checkIns: input.checkIns,
		stationsByCallsign: input.stationsByCallsign ?? new Map(),
		routeIndex: idx,
		stops: stopPoints,
		nowMs: input.nowMs,
		memory: input.rosterMemory,
		sweepUnitCheckInId: c?.sweep?.latestReport?.checkInId || undefined
	});
	const staffing = new Map<string, number>();
	for (const r of roster) if (r.state === 'at-stop' && r.stopId) staffing.set(r.stopId, (staffing.get(r.stopId) ?? 0) + 1);

	// Closure state and the backend's own out-of-order reconciliation.
	const closureById = new Map<string, string>();
	const backendOutOfOrder = new Set<string>();
	for (const st of c?.stations ?? []) {
		closureById.set(st.checkpointId, st.closure.state);
		if (st.outOfOrder) backendOutOfOrder.add(st.checkpointId);
	}

	const stops: RailStopModel[] = cps.map((cp) => {
		const a = live(cp);
		const id = cp.meta.annotationId;
		const st = stopState(closureById.get(id), a.status);
		return {
			id,
			seq: cp.meta.sequenceNumber,
			label: a.label,
			shortName: a.shortName,
			mile: stopMiles.get(id) ?? null,
			pct: axis.pctForStop(id),
			glyph: st.glyph,
			state: st.state,
			// B6: its OWN category's palette — the checkpoint palette paints
			// "active" amber, so a healthy course of aid stations read as warnings.
			color: statusColor(a.category as AnnotationCategory, a.status),
			outOfOrder: backendOutOfOrder.has(id) || (placed.get(id)?.outOfSequence ?? false),
			staffed: staffing.get(id) ?? 0,
			passageCount: cp.passageCount
		};
	});

	// Lead and sweep: reported sources only, newest wins.
	const leadLabel = c?.config?.leadLabel || 'LEAD';
	const sweepLabel = c?.config?.sweepLabel || 'SWEEP';
	const passages: EdgePassage[] = cps.flatMap((cp) =>
		(cp.passages ?? []).map((p) => ({
			label: p.label,
			checkpointId: cp.meta.annotationId,
			seq: cp.meta.sequenceNumber,
			at: p.passageTime
		}))
	);
	const latest = c?.sweep?.latestReport;
	const edges = edgePositions({
		passages,
		sweepReport: latest ? { mile: latest.routeMile ?? null, at: latest.reportedAt } : null,
		sweepPassed: (c?.stations ?? [])
			.filter((s) => !!s.closure.sweepPassedAt)
			.map((s) => ({ checkpointId: s.checkpointId, seq: s.sequenceNumber, at: s.closure.sweepPassedAt as string })),
		stopMiles,
		leadLabel,
		sweepLabel
	});
	const edgePct = (e: Edges['lead']) => {
		if (!e) return null;
		if (e.mile != null) {
			const p = axis.pct(e.mile * MI);
			if (p != null) return p;
		}
		return e.checkpointId ? axis.pctForStop(e.checkpointId) : null;
	};

	// Gates and incidents: drawn only where they really are.
	const gates: RailGateModel[] = [];
	for (const s of c?.shutoffs ?? []) {
		if (s.status === 'cancelled' || s.routeMile == null) continue;
		const pct = axis.pct(s.routeMile * MI);
		if (pct == null) continue;
		gates.push({ id: s.id, name: s.name, mile: s.routeMile, status: s.status, pct });
	}
	const incidents: RailIncidentModel[] = [];
	const total = axis.totalMeters;
	for (const i of input.incidents ?? []) {
		if (total == null || i.chainageMeters < 0 || i.chainageMeters > total) continue;
		const pct = axis.pct(i.chainageMeters);
		if (pct != null) incidents.push({ ...i, pct });
	}

	// Weather brackets across their stops, on the same axis as everything else.
	const pctBySeq = new Map(stops.map((s) => [s.seq, s.pct]));
	const wx: RailWxModel[] = [];
	for (const a of input.wxAlerts ?? []) {
		if (a.tier === 'statement' || a.affects?.checkpointSeqRange?.length !== 2) continue;
		const [lo, hi] = a.affects.checkpointSeqRange;
		const from = pctBySeq.get(lo);
		const to = pctBySeq.get(hi);
		if (from == null || to == null) continue;
		wx.push({ id: a.id, event: a.event, shortCode: a.shortCode, tier: a.tier, fromSeq: lo, toSeq: hi, fromPct: Math.min(from, to), toPct: Math.max(from, to) });
	}

	return {
		axis,
		routeIndex: idx,
		leadLabel,
		sweepLabel,
		edges,
		leadPct: edgePct(edges.lead),
		sweepPct: edgePct(edges.sweep),
		stops,
		gates,
		incidents,
		wx,
		roster,
		staffing,
		unplaced: {
			offCourse: roster.filter((r) => r.state === 'off-course').length,
			noPosition: roster.filter((r) => r.state === 'no-position').length,
			noCourse: roster.filter((r) => r.state === 'no-course').length
		},
		phase: input.phase ?? null,
		sweepSpeedMph: latest?.estimatedSpeedMph ?? null
	};
}
