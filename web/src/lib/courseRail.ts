/**
 * Pure view-model functions for the course progress rail
 * (docs/course-rail-spec.md). No stores, no DOM: everything here is unit-tested
 * against fixtures and the real course, and the rail component only draws.
 *
 * The rail answers one question — "where is it ALONG THE COURSE, in the miles
 * we speak on the air?" — so every x on it goes through one axis (§B1).
 */
import type { RouteIndex } from './routeDistance';

const METERS_PER_MILE = 1609.344;

// --- the axis -----------------------------------------------------------------

export interface RailAxis {
	/** 'mile' when a course line exists; 'index' (evenly spaced stops, NOT to
	 *  scale) only when it does not. The two are never mixed. */
	mode: 'mile' | 'index';
	totalMeters: number | null;
	totalMiles: number | null;
	/** Chainage -> 0..100 along the rail. Null on an index axis: a distance
	 *  means nothing without a line to measure it on. */
	pct(chainageMeters: number): number | null;
	/** A stop's position, whichever the mode. Null if the stop is not placed. */
	pctForStop(stopId: string): number | null;
}

/**
 * The one axis every rail layer is placed on.
 *
 * B3: the old axis ended at the furthest NUMBERED stop, so with the finish
 * unnumbered the rail ended at the last rest stop and "to go" read 0 with
 * riders still seven miles out. The course line knows its own length.
 *
 * `stopIdsInSequence` is used only on an index axis (no line), to space stops
 * evenly in sequence order.
 */
export function buildRailAxis(
	idx: RouteIndex | null,
	stopChainages: Map<string, number>,
	stopIdsInSequence: string[] = []
): RailAxis {
	if (idx && idx.totalMeters > 0) {
		const total = idx.totalMeters;
		const pct = (m: number) => Math.min(100, Math.max(0, (m / total) * 100));
		return {
			mode: 'mile',
			totalMeters: total,
			totalMiles: total / METERS_PER_MILE,
			pct,
			pctForStop: (id) => {
				const m = stopChainages.get(id);
				return m == null ? null : pct(m);
			}
		};
	}
	const n = stopIdsInSequence.length;
	const pos = new Map(stopIdsInSequence.map((id, i) => [id, n <= 1 ? 50 : (i / (n - 1)) * 100]));
	return {
		mode: 'index',
		totalMeters: null,
		totalMiles: null,
		pct: () => null,
		pctForStop: (id) => pos.get(id) ?? null
	};
}

// --- lead and sweep -----------------------------------------------------------

/** One logged passage, as edgePositions needs it. */
export interface EdgePassage {
	label: string;
	checkpointId: string;
	seq: number;
	/** ISO time the passage was logged. */
	at: string;
}

/** A sweep report. `mile` is null when the report named no mile. */
export interface EdgeReport {
	mile: number | null;
	at: string;
}

export interface EdgePosition {
	/** Null when the edge is at a stop that is not on the course line. */
	mile: number | null;
	at: string;
	source: 'passage' | 'report' | 'sweep-passed';
	checkpointId?: string;
	seq?: number;
}

export interface Edges {
	lead: EdgePosition | null;
	sweep: EdgePosition | null;
	/** Sweep reported ahead of lead — a data problem to surface, not a spread. */
	inverted: boolean;
	/** Lead minus sweep, miles; null when either is unplaced or inverted. */
	spreadMiles: number | null;
}

/** Sweep ahead of lead by less than this is the same place, not an inversion. */
const INVERSION_TOLERANCE_MI = 0.05;

const norm = (s: string) => s.trim().toLowerCase();
const t = (iso: string) => {
	const v = Date.parse(iso);
	return Number.isNaN(v) ? -Infinity : v;
};

/**
 * Where LEAD and SWEEP are, from REPORTED sources only.
 *
 * The user's rule, settled: "We'd always take the reported sweep over the
 * GPS." Sweep is the operational line behind which the course is clear;
 * closing stops and declaring the course clear are made on what a human
 * reported, never on where a van happens to be. So nothing here takes a
 * position fix, and the input type has no field for one (spec R0.5).
 *
 * Between reported sources, the NEWEST wins (B2): the old code let any sweep
 * report with a mile beat every later passage, so a 09:40 "mile 10" outranked
 * an 11:05 passage at mile 35.4 and the readout and the marker sat 25 miles
 * apart. Passages are matched per label newest-first rather than
 * highest-stop-first (B18), so a passage logged at the wrong stop can be
 * corrected by logging the right one.
 */
export function edgePositions(input: {
	passages: EdgePassage[];
	sweepReport: EdgeReport | null;
	/** Stops an operator MARKED sweep-passed (the Sweep passed… action). That
	 *  is a human report of where sweep was, even with no passage logged. */
	sweepPassed?: { checkpointId: string; seq: number; at: string }[];
	stopMiles: Map<string, number>;
	leadLabel?: string;
	sweepLabel?: string;
}): Edges {
	const leadKey = norm(input.leadLabel || 'LEAD');
	const sweepKey = norm(input.sweepLabel || 'SWEEP');

	const newestPassage = (key: string): EdgePosition | null => {
		let best: EdgePassage | null = null;
		for (const p of input.passages) {
			if (norm(p.label) !== key) continue;
			if (!best || t(p.at) > t(best.at)) best = p;
		}
		if (!best) return null;
		return {
			mile: input.stopMiles.get(best.checkpointId) ?? null,
			at: best.at,
			source: 'passage',
			checkpointId: best.checkpointId,
			seq: best.seq
		};
	};

	const lead = newestPassage(leadKey);
	let sweep = newestPassage(sweepKey);
	for (const m of input.sweepPassed ?? []) {
		if (!sweep || t(m.at) > t(sweep.at)) {
			sweep = {
				mile: input.stopMiles.get(m.checkpointId) ?? null,
				at: m.at,
				source: 'sweep-passed',
				checkpointId: m.checkpointId,
				seq: m.seq
			};
		}
	}
	const r = input.sweepReport;
	// A report with no mile cannot place sweep, so it never displaces a
	// passage that can.
	if (r && r.mile != null && (!sweep || t(r.at) > t(sweep.at))) {
		sweep = { mile: r.mile, at: r.at, source: 'report' };
	}

	let inverted = false;
	let spreadMiles: number | null = null;
	if (lead?.mile != null && sweep?.mile != null) {
		if (sweep.mile > lead.mile + INVERSION_TOLERANCE_MI) inverted = true;
		else spreadMiles = Math.max(0, lead.mile - sweep.mile);
	}
	return { lead, sweep, inverted, spreadMiles };
}

// --- layout: pixels, run at the rail's real width --------------------------------

export interface StopPx {
	id: string;
	px: number;
}

export interface DodgedStop {
	id: string;
	/** Where the stop really is on the line — the leader tick points here. */
	truePx: number;
	/** Where its glyph is drawn, at least `minPx` from its neighbours. */
	drawPx: number;
	dodged: boolean;
}

/**
 * Spread stops that are too close to read, without hiding or reordering any.
 *
 * B10: nothing kept stops apart, so on a long course two stops 0.8 mi apart
 * were drawn 6 px apart at tablet width — one glyph printed over the other,
 * and the LEAD/SWEEP chevron appeared to sit on the wrong stop. Close stops
 * are laid out at `minPx` spacing centred on their mean; a stop moved off its
 * true position keeps a leader tick back to it. Groups that collide after
 * spreading are merged and re-spread, and a group is shifted, not squashed,
 * to stay on the track.
 */
export function dodgeStops(stops: StopPx[], widthPx: number, minPx: number): DodgedStop[] {
	const sorted = stops.slice().sort((a, b) => a.px - b.px);
	type Group = { members: StopPx[]; start: number };
	const layout = (g: Group) => {
		const n = g.members.length;
		const mean = g.members.reduce((s, m) => s + m.px, 0) / n;
		let start = mean - ((n - 1) * minPx) / 2;
		// Shift, don't squash, to keep the whole group on the track.
		start = Math.max(0, Math.min(start, widthPx - (n - 1) * minPx));
		g.start = start;
	};
	const groups: Group[] = [];
	for (const s of sorted) {
		const g: Group = { members: [s], start: s.px };
		layout(g);
		groups.push(g);
		// Merge backwards while this group now collides with the previous one.
		while (groups.length > 1) {
			const cur = groups[groups.length - 1];
			const prev = groups[groups.length - 2];
			const prevEnd = prev.start + (prev.members.length - 1) * minPx;
			if (cur.start - prevEnd >= minPx - 1e-9) break;
			prev.members.push(...cur.members);
			layout(prev);
			groups.pop();
		}
	}
	const out: DodgedStop[] = [];
	for (const g of groups) {
		g.members.forEach((m, i) => {
			const drawPx = g.members.length === 1 ? m.px : g.start + i * minPx;
			out.push({ id: m.id, truePx: m.px, drawPx, dodged: Math.abs(drawPx - m.px) > 1e-9 });
		});
	}
	return out;
}

export interface PipPx {
	id: string;
	px: number;
	/** Reveal order inside a cluster: lower first (category, then freshness). */
	rank: number;
}

export interface PipCluster {
	/** Where the pip or badge is drawn. */
	px: number;
	/** Member ids, in reveal order. One member = a plain pip. */
	members: string[];
	/** The span of true positions the badge covers, for its label. */
	fromPx: number;
	toPx: number;
}

/**
 * Merge roster pips closer than `mergePx` into count badges (spec R13).
 * Clustering is in PIXELS at the rail's real width, not in miles: the rail
 * is an overview and the map is the zoom, so fifteen people inside a mile is
 * rightly one badge at 1400 px and may span two miles at 800 px.
 *
 * A pip is never drawn ON a stop glyph (within `stopRadiusPx` of a stop
 * centre): it is >150 m from that stop or it would have been folded into it,
 * so it is pushed just clear, on the side it is already on.
 */
export function clusterPips(
	pips: PipPx[],
	mergePx: number,
	stopCentresPx: number[] = [],
	stopRadiusPx = 0
): PipCluster[] {
	const sorted = pips.slice().sort((a, b) => a.px - b.px);
	const clusters: PipPx[][] = [];
	for (const p of sorted) {
		const last = clusters[clusters.length - 1];
		if (last && p.px - last[last.length - 1].px < mergePx) last.push(p);
		else clusters.push([p]);
	}
	return clusters.map((members) => {
		let px = members.reduce((s, m) => s + m.px, 0) / members.length;
		for (const c of stopCentresPx) {
			if (Math.abs(px - c) < stopRadiusPx) px = px >= c ? c + stopRadiusPx : c - stopRadiusPx;
		}
		return {
			px,
			members: members
				.slice()
				.sort((a, b) => a.rank - b.rank)
				.map((m) => m.id),
			fromPx: members[0].px,
			toPx: members[members.length - 1].px
		};
	});
}
