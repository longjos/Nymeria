<script module lang="ts">
	/**
	 * What the map should draw to show *what was measured*. A distance the
	 * operator reads aloud on the radio must be checkable against a shape:
	 * the badge and the rubber band can never disagree.
	 *
	 * `latlngs` / `from` / `to` are all [lat, lon] / {lat, lon} — Leaflet order,
	 * not GeoJSON order, because the only consumer is Leaflet.
	 */
	export type MeasureBand = {
		/** 'road' traces the real course vertices; 'direct' is one straight chord. */
		kind: 'road' | 'direct';
		/** Polyline vertices for 'road'. Empty for 'direct'. */
		latlngs: [number, number][];
		/** Endpoints for 'direct'. */
		from?: { lat: number; lon: number };
		to?: { lat: number; lon: number };
		/** Short amber tick from the origin to its snap point, when off-course. */
		snap?: { from: { lat: number; lon: number }; to: { lat: number; lon: number } };
	};

	export type DistanceOrigin = {
		lat: number;
		lon: number;
		label: string;
		/** Degrees true. Used to disambiguate an out-and-back overlap. */
		bearingDeg?: number;
		/** Age of the underlying fix in ms (GPS origins only). */
		ageMs?: number;
		stale?: boolean;
		/** 'gps' = the live own-position fix; 'pick' = long-press / station pick. */
		source?: 'gps' | 'pick';
	};

	const ACTIVE_COURSE_KEY = 'nymeria_active_course';
	const BRANCH_KEY = 'nymeria_course_branch';

	/** A ten-minute-old position on a bike course is a lie, not an answer. */
	const MAX_FIX_AGE_MS = 10 * 60 * 1000;

	// The loop-branch choice is deliberately session-scoped: a stale branch
	// from yesterday's ride would silently mis-answer today's. Clearing it at
	// module load is the whole of "session-scoped" — it is still written to
	// localStorage so it survives a panel remount within the session.
	if (typeof localStorage !== 'undefined') {
		try {
			localStorage.removeItem(BRANCH_KEY);
		} catch {
			/* private mode — the branch just lives in memory */
		}
	}
</script>

<script lang="ts">
	import type { Annotation } from '$lib/types';
	import { haversineMeters, annotationPickPoint } from '$lib/geo';
	import { formatGpsAge } from '$lib/stores/gps';
	import { unitSystem } from '$lib/stores/units';
	import { formatDistance, formatDistanceValue, formatDistanceSpoken } from '$lib/units';
	import {
		getRouteIndex, parseLineString, projectOnRoute, resolveCandidate,
		projectStops, nextStopsAhead, angularDiff,
		TOL_OFF_COURSE_M, ON_COURSE_M, STOP_SNAP_M
	} from '$lib/routeDistance';
	import type { RouteIndex, Candidate, Stop } from '$lib/routeDistance';

	let {
		origin = null,
		routeAnnotations = [],
		stopAnnotations = [],
		activeNetId = null,
		expanded = false,
		onToggle,
		onClear,
		onMeasureChange
	}: {
		origin?: DistanceOrigin | null;
		routeAnnotations?: Annotation[];
		stopAnnotations?: Annotation[];
		activeNetId?: string | null;
		expanded?: boolean;
		onToggle: () => void;
		onClear: () => void;
		onMeasureChange: (band: MeasureBand | null) => void;
	} = $props();

	type StopRow = { id: string; name: string; meters: number };

	type Answer = {
		/** null when there is no usable route index for this origin. */
		routeName: string;
		routeId: string;
		totalMeters: number;
		originChainage: number | null;
		originOffTrack: number;
		destName: string;
		destMeters: number | undefined;
		kind: 'road' | 'direct';
		stops: StopRow[];
		honesty: string;
		noStops: boolean;
		headingBack: boolean;
		band: MeasureBand | null;
		/** Non-empty means the pill must ask which branch of a loop we are on. */
		branches: Candidate[];
	};

	let answer = $state<Answer | null>(null);
	let pinnedStopId = $state<string | null>(null);
	let routeChooserOpen = $state(false);
	let wrapEl = $state<HTMLDivElement | undefined>(undefined);

	/** Last accepted chainage (m) — the continuity hint for the next projection.
	 *  It describes ONE origin on ONE route, so it is keyed to both: applying a
	 *  GPS chainage to a long-pressed rider on the far leg of an out-and-back
	 *  would silently convert an honest "which leg?" prompt into a confidently
	 *  wrong answer 30 miles out. */
	let lastChainage: number | null = null;
	let lastChainageAt = 0;
	let lastChainageRouteId: string | null = null;
	let lastOriginKey: string | null = null;
	/** Explicit loop-branch pick (m along the course), sticky until it stops fitting. */
	let branchChainage = $state<number | null>(loadBranch());
	/** The route the branch pick was made on; null = unknown (restored from storage). */
	let branchRouteId: string | null = null;

	function loadBranch(): number | null {
		try {
			const v = localStorage.getItem(BRANCH_KEY);
			return v ? Number(v) : null;
		} catch {
			return null;
		}
	}

	function saveBranch(v: number | null) {
		branchChainage = v;
		try {
			if (v == null) localStorage.removeItem(BRANCH_KEY);
			else localStorage.setItem(BRANCH_KEY, String(v));
		} catch {
			/* per-viewer convenience only */
		}
	}

	function loadActiveCourse(): string | null {
		try {
			return localStorage.getItem(ACTIVE_COURSE_KEY);
		} catch {
			return null;
		}
	}

	function saveActiveCourse(id: string) {
		try {
			localStorage.setItem(ACTIVE_COURSE_KEY, id);
		} catch {
			/* per-viewer convenience only */
		}
	}

	// --- route selection ---------------------------------------------------

	type RouteEntry = { ann: Annotation; idx: RouteIndex; off: number; cands: Candidate[] };

	function routeEntries(lat: number, lon: number): RouteEntry[] {
		const out: RouteEntry[] = [];
		for (const ann of routeAnnotations) {
			let idx: RouteIndex;
			try {
				// Lazy: the 79 KB LineString must be parsed once per edit, not
				// once per GPS tick — this runs at 1-2 Hz for the whole session.
				idx = getRouteIndex(ann.id, ann.updatedAt, () => parseLineString(ann.geometry));
			} catch {
				continue;
			}
			const cands = projectOnRoute(idx, lat, lon, TOL_OFF_COURSE_M);
			out.push({ ann, idx, off: cands.length ? cands[0].offTrackMeters : Infinity, cands });
		}
		return out;
	}

	function pickRoute(entries: RouteEntry[]): RouteEntry | null {
		if (entries.length === 0) return null;
		const stored = loadActiveCourse();
		if (stored) {
			const hit = entries.find((e) => e.ann.id === stored);
			if (hit) return hit;
		}
		const sorted = [...entries].sort((a, b) => a.off - b.off);
		const best = sorted[0];
		const second = sorted[1];
		// A near tie between two on-course lines is not a geometry question —
		// prefer the one scoped to the running net, then the longer course
		// (a spur rarely outranks the main route).
		if (second && best.off <= ON_COURSE_M && second.off <= ON_COURSE_M && second.off <= best.off * 2) {
			const tied = [best, second];
			const netScoped = tied.filter((e) => activeNetId && e.ann.netId === activeNetId);
			if (netScoped.length === 1) return netScoped[0];
			return tied[0].idx.totalMeters >= tied[1].idx.totalMeters ? tied[0] : tied[1];
		}
		return best;
	}

	// --- geometry helpers (metres everywhere; only formatters convert) ------

	/** Interpolated {lat, lon} at a chainage, clamped to the line's extent. */
	function pointAtChainage(idx: RouteIndex, m: number): { lat: number; lon: number } {
		const n = idx.cum.length;
		const t = Math.min(Math.max(m, 0), idx.totalMeters);
		let lo = 0;
		let hi = n - 1;
		while (lo < hi - 1) {
			const mid = (lo + hi) >> 1;
			if (idx.cum[mid] <= t) lo = mid;
			else hi = mid;
		}
		const seg = idx.cum[hi] - idx.cum[lo];
		// Zero-length segments are real in the source data (GPX track-segment
		// joins repeat a vertex) — guard the divide rather than emitting NaN.
		const f = seg > 0 ? (t - idx.cum[lo]) / seg : 0;
		return {
			lat: idx.lats[lo] + (idx.lats[hi] - idx.lats[lo]) * f,
			lon: idx.lons[lo] + (idx.lons[hi] - idx.lons[lo]) * f
		};
	}

	/** Vertices between two chainages, inclusive of interpolated endpoints. */
	function sliceLatLngs(idx: RouteIndex, a: number, b: number): [number, number][] {
		const lo = Math.min(a, b);
		const hi = Math.max(a, b);
		const p0 = pointAtChainage(idx, lo);
		const p1 = pointAtChainage(idx, hi);
		const pts: [number, number][] = [[p0.lat, p0.lon]];
		for (let i = 0; i < idx.cum.length; i++) {
			if (idx.cum[i] > lo && idx.cum[i] < hi) pts.push([idx.lats[i], idx.lons[i]]);
		}
		pts.push([p1.lat, p1.lon]);
		return pts;
	}

	/**
	 * Forward distance (m) from `o` to `c` along the course. On a closed route
	 * "ahead" wraps through the start/finish; on an open one it is the plain
	 * chainage difference.
	 */
	function aheadDelta(idx: RouteIndex, o: number, c: number, reverse: boolean): number {
		const total = idx.totalMeters;
		if (idx.closed && total > 0) {
			const d = reverse ? o - c : c - o;
			return ((d % total) + total) % total;
		}
		return Math.abs(c - o);
	}

	// Generic leading words a route planner puts on every stop. On the real
	// 48-mile course all three aid stations are named "Rest Stop <place>", so
	// keeping the prefix pushes the only distinguishing word past the pill's
	// truncation and every stop reads "REST STOP …". Strip it for display only —
	// the annotation keeps its full label everywhere else — and never strip it
	// down to nothing.
	const GENERIC_STOP_PREFIX = /^(rest\s*stop|aid\s*station|water\s*stop|check\s*point|checkpoint|stop)\b[\s:—-]*/i;

	function shortLabel(ann: { shortName?: string; label: string }): string {
		const raw = (ann.shortName || ann.label || '').trim();
		if (ann.shortName) return raw;
		const stripped = raw.replace(GENERIC_STOP_PREFIX, '').trim();
		return stripped || raw;
	}

	// --- the computation ---------------------------------------------------

	function compute(): Answer | null {
		const o = origin;
		if (!o) return null;

		// A new origin is a new question: everything remembered about the last
		// one (its chainage, and which leg of the loop it was on) describes a
		// different point and must not narrow this projection.
		const originKey = `${o.source ?? 'gps'}:${o.label}`;
		if (originKey !== lastOriginKey) {
			lastOriginKey = originKey;
			lastChainage = null;
			lastChainageAt = 0;
			lastChainageRouteId = null;
			if (branchChainage != null) saveBranch(null);
		}

		const entries = routeEntries(o.lat, o.lon);
		const chosen = pickRoute(entries);

		// No course at all. A GPS-derived origin renders nothing (the pill would
		// be noise); a deliberately picked one still gets a straight-line answer
		// to the nearest marked stop, because the operator asked.
		if (!chosen || chosen.cands.length === 0) {
			if (o.source === 'gps' && !chosen) return null;
			return directOnly(o, chosen);
		}

		const idx = chosen.idx;
		// Continuity only carries across the SAME route: a chainage from another
		// course's coordinate space is meaningless here, and pickRoute can switch
		// routes on its own as the origin moves.
		const sameRoute = lastChainageRouteId === chosen.ann.id;
		const carried = sameRoute ? lastChainage : null;
		const seed =
			branchRouteId === null || branchRouteId === chosen.ann.id ? branchChainage : null;
		const resolved = resolveCandidate(
			chosen.cands,
			{
				bearingDeg: o.bearingDeg,
				// Continuity: the previous accepted chainage, seeded by an explicit
				// loop-branch pick on a cold start.
				lastChainage: carried ?? seed ?? undefined,
				elapsedSeconds:
					carried != null && lastChainageAt ? (Date.now() - lastChainageAt) / 1000 : undefined
			},
			idx
		);

		if (resolved.ambiguous) {
			// Two equally valid answers ~60 miles apart. Showing either one is
			// worse than showing none: refuse until the operator picks a branch.
			return {
				routeName: chosen.ann.label,
				routeId: chosen.ann.id,
				totalMeters: idx.totalMeters,
				originChainage: null,
				originOffTrack: chosen.cands[0].offTrackMeters,
				destName: '',
				destMeters: undefined,
				kind: 'direct',
				stops: [],
				honesty: 'This course doubles back here — which leg are you on?',
				noStops: false,
				headingBack: false,
				band: null,
				branches: resolved.candidates.slice(0, 2)
			};
		}

		const cand = resolved.candidate ?? chosen.cands[0];
		const originChainage = cand.chainageMeters;
		lastChainage = originChainage;
		lastChainageAt = Date.now();
		lastChainageRouteId = chosen.ann.id;

		const offTrack = cand.offTrackMeters;
		// Heading back down the course reverses which stops count as "ahead".
		const headingBack =
			o.bearingDeg != null && angularDiff(cand.segBearing, o.bearingDeg) > 120;

		const stops = projectStops(idx, stopAnnotations, STOP_SNAP_M);
		// Everything still ahead, then the three the card shows. nextStopsAhead
		// owns the 50 m hysteresis, so rolling through an aid station does not
		// flicker the readout.
		const aheadAll = nextStopsAhead(stops, originChainage, idx, stops.length || 1, headingBack);
		const ahead = aheadAll.slice(0, 3);

		// A pinned destination is sticky until the origin rolls past it.
		if (pinnedStopId && !aheadAll.some((s) => s.id === pinnedStopId)) pinnedStopId = null;

		let destChainage: number;
		let destName: string;
		let destOffTrack: number;
		let destPoint: { lat: number; lon: number } | null = null;

		const pinned = pinnedStopId ? stops.find((s) => s.id === pinnedStopId) : undefined;
		const target = pinned ?? ahead[0];

		if (target) {
			destChainage = target.chainageMeters;
			destName = shortLabel(target);
			destOffTrack = target.offTrackMeters;
			const ann = stopAnnotations.find((a) => a.id === target.id);
			destPoint = ann ? annotationPickPoint(ann) : null;
		} else {
			// No marked stops ahead: the end of the line is always answerable,
			// and needs no annotations at all (the bare-KML case).
			destChainage = headingBack ? 0 : idx.totalMeters;
			destOffTrack = 0;
			const endPt = pointAtChainage(idx, destChainage);
			destPoint = endPt;
			const finishNear = stopAnnotations.find((a) => {
				if (a.category !== 'finish') return false;
				const p = annotationPickPoint(a);
				return p ? haversineMeters(p.lat, p.lon, endPt.lat, endPt.lon) <= STOP_SNAP_M : false;
			});
			destName = finishNear ? shortLabel(finishNear) : headingBack ? 'START' : 'END';
			if (finishNear) destName = shortLabel(finishNear) || 'FINISH';
		}

		const roadOK = offTrack <= TOL_OFF_COURSE_M && destOffTrack <= STOP_SNAP_M;
		const kind: 'road' | 'direct' = roadOK ? 'road' : 'direct';

		let meters: number;
		if (roadOK) {
			meters = aheadDelta(idx, originChainage, destChainage, headingBack);
		} else {
			meters = destPoint ? haversineMeters(o.lat, o.lon, destPoint.lat, destPoint.lon) : 0;
		}

		let honesty: string;
		if (offTrack > TOL_OFF_COURSE_M) honesty = 'Too far from the course to measure road miles.';
		else if (offTrack > ON_COURSE_M) honesty = `${Math.round(offTrack)} m off the course`;
		else honesty = `on course, ${Math.round(offTrack)} m off the line`;
		if (headingBack) honesty += ' (heading back)';

		const snapPt = pointAtChainage(idx, originChainage);
		const band: MeasureBand = roadOK
			? {
					kind: 'road',
					latlngs: idx.closed && destChainage < originChainage && !headingBack
						? [
								...sliceLatLngs(idx, originChainage, idx.totalMeters),
								...sliceLatLngs(idx, 0, destChainage)
							]
						: sliceLatLngs(idx, originChainage, destChainage),
					snap: offTrack > ON_COURSE_M ? { from: { lat: o.lat, lon: o.lon }, to: snapPt } : undefined
				}
			: {
					kind: 'direct',
					latlngs: [],
					from: { lat: o.lat, lon: o.lon },
					to: destPoint ?? { lat: o.lat, lon: o.lon }
				};

		return {
			routeName: chosen.ann.label,
			routeId: chosen.ann.id,
			totalMeters: idx.totalMeters,
			originChainage,
			originOffTrack: offTrack,
			destName,
			destMeters: meters,
			kind,
			stops: ahead.map((s) => ({
				id: s.id,
				name: shortLabel(s),
				meters: aheadDelta(idx, originChainage, s.chainageMeters, headingBack)
			})),
			honesty,
			noStops: stops.length === 0,
			headingBack,
			band,
			branches: []
		};
	}

	/** Straight-line-only answer: no usable course under the origin. */
	function directOnly(o: DistanceOrigin, chosen: RouteEntry | null): Answer | null {
		let best: { ann: Annotation; m: number; pt: { lat: number; lon: number } } | null = null;
		// Point geometry only: annotationPickPoint happily returns the centroid of
		// a staging polygon or the midpoint of a line, neither of which is a stop
		// you can ride to.
		const candidates = stopAnnotations.filter((a) => a.type === 'point');
		// A pinned destination is the operator's explicit answer to "which stop?"
		// and outranks proximity here exactly as it does on the road path.
		const pinnedAnn = pinnedStopId ? candidates.find((a) => a.id === pinnedStopId) : undefined;
		for (const a of pinnedAnn ? [pinnedAnn] : candidates) {
			const p = annotationPickPoint(a);
			if (!p) continue;
			const m = haversineMeters(o.lat, o.lon, p.lat, p.lon);
			if (!best || m < best.m) best = { ann: a, m, pt: p };
		}
		if (!best) return null;
		return {
			// Deliberately unnamed. Reaching here means the origin projects onto
			// no course, so `chosen` is whichever route selection fell through to
			// — naming it puts a course heading above a crow-flies number while
			// the operator may be nowhere near that course. The chooser still
			// works (it falls back to "Course") and routeId is kept for it.
			routeName: '',
			routeId: chosen?.ann.id ?? '',
			totalMeters: chosen?.idx.totalMeters ?? 0,
			originChainage: null,
			originOffTrack: Infinity,
			destName: shortLabel(best.ann),
			destMeters: best.m,
			kind: 'direct',
			stops: [],
			// "nearest", never "next": off the course there is no direction of
			// travel, so this stop may well be one the rider has already passed.
			honesty: chosen
				? 'Too far from the course to measure road miles — nearest marked stop.'
				: 'No course loaded — straight line to the nearest marked stop.',
			noStops: false,
			headingBack: false,
			band: { kind: 'direct', latlngs: [], from: { lat: o.lat, lon: o.lon }, to: best.pt },
			branches: []
		};
	}

	// --- throttled recompute (<= 2 Hz; GPS fixes arrive faster than that) ---

	let pendingTimer: ReturnType<typeof setTimeout> | null = null;
	let lastRunAt = 0;

	function schedule() {
		if (pendingTimer) return;
		const wait = Math.max(0, 500 - (Date.now() - lastRunAt));
		pendingTimer = setTimeout(() => {
			pendingTimer = null;
			lastRunAt = Date.now();
			try {
				answer = compute();
			} catch {
				answer = null;
			}
		}, wait);
	}

	$effect(() => {
		// Touch every input so the effect re-subscribes to all of them.
		void origin;
		void routeAnnotations;
		void stopAnnotations;
		void pinnedStopId;
		void branchChainage;
		void activeNetId;
		schedule();
		return () => {
			if (pendingTimer) {
				clearTimeout(pendingTimer);
				pendingTimer = null;
			}
		};
	});

	// The rubber band exists only while the card is open — the default view
	// stays clean, and an open card always shows what it measured.
	$effect(() => {
		onMeasureChange(expanded && answer ? answer.band : null);
	});

	// --- presentation ------------------------------------------------------

	let fixTooOld = $derived(
		!!origin && origin.source === 'gps' && (origin.ageMs ?? 0) > MAX_FIX_AGE_MS
	);
	let stale = $derived(!!origin && origin.source === 'gps' && !!origin.stale);
	let ambiguous = $derived(!!answer && answer.branches.length > 1);

	// The pill earns its slot only when it has something true to say: a course
	// is loaded (so "no fix" can teach the long-press gesture), or the operator
	// deliberately picked an origin and there is an answer for it.
	let showPill = $derived(
		routeAnnotations.length > 0
			? !origin || fixTooOld || !!answer
			: origin?.source === 'pick' && !!answer
	);

	let modeColor = $derived(
		answer?.kind === 'road' ? 'var(--color-success)' : 'var(--color-warning)'
	);

	let displayName = $derived.by(() => {
		const n = (answer?.destName || '').toUpperCase();
		return n.length > 14 ? n.slice(0, 13) + '…' : n;
	});

	let distanceText = $derived(
		answer ? formatDistanceValue(answer.destMeters, $unitSystem, answer.kind) : '--'
	);

	let spoken = $derived(
		answer
			? formatDistanceSpoken(answer.destMeters, $unitSystem, answer.kind, answer.destName)
			: 'No distance available'
	);

	let ageText = $derived(
		stale && origin?.ageMs != null ? formatGpsAge(origin.ageMs) + ' ago' : ''
	);

	/** "Mile 20.9 of 47.3" — always in the viewer's units. */
	let mileLine = $derived.by(() => {
		if (!answer || answer.originChainage == null) return '';
		const f = (m: number) =>
			$unitSystem === 'imperial' ? (m / 1609.344).toFixed(1) : (m / 1000).toFixed(1);
		const unit = $unitSystem === 'imperial' ? 'Mile' : 'Km';
		return `${unit} ${f(answer.originChainage)} of ${f(answer.totalMeters)}`;
	});

	function branchLabel(c: Candidate): string {
		const f = $unitSystem === 'imperial' ? c.chainageMeters / 1609.344 : c.chainageMeters / 1000;
		const unit = $unitSystem === 'imperial' ? 'MILE' : 'KM';
		return `${unit} ${f.toFixed(1)}`;
	}

	function pickBranch(c: Candidate) {
		saveBranch(c.chainageMeters);
		branchRouteId = answer?.routeId ?? null;
		lastChainage = c.chainageMeters;
		lastChainageAt = Date.now();
		lastChainageRouteId = branchRouteId;
		lastRunAt = 0;
		schedule();
	}

	function pinStop(id: string) {
		pinnedStopId = pinnedStopId === id ? null : id;
		lastRunAt = 0;
		schedule();
	}

	function chooseRoute(id: string) {
		saveActiveCourse(id);
		routeChooserOpen = false;
		pinnedStopId = null;
		saveBranch(null);
		branchRouteId = null;
		lastChainage = null;
		lastChainageAt = 0;
		lastChainageRouteId = null;
		lastRunAt = 0;
		schedule();
	}

	function onKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && expanded) {
			e.stopImmediatePropagation();
			routeChooserOpen = false;
			onToggle();
		}
	}

	function onDocPointerDown(e: MouseEvent) {
		if (!expanded) return;
		if (wrapEl && !wrapEl.contains(e.target as Node)) {
			routeChooserOpen = false;
			onToggle();
		}
	}

	function routeLengthText(a: Annotation): string {
		try {
			const idx = getRouteIndex(a.id, a.updatedAt, () => parseLineString(a.geometry));
			return formatDistanceValue(idx.totalMeters, $unitSystem, 'road');
		} catch {
			return '';
		}
	}
</script>

<!-- Escape is taken in the CAPTURE phase so an open card collapses without
     Map's own window handler also dropping a long-pressed distance origin. -->
<svelte:window onkeydowncapture={onKey} onmousedown={onDocPointerDown} />

{#if showPill}
	<!-- No live region on the wrapper: it holds the card's buttons, and a 1 Hz
	     recompute would re-announce every one of them. Only the number is live. -->
	<div class="ns-wrap" bind:this={wrapEl}>
			<div class="ns-row">
				{#if !origin || fixTooOld}
					<div class="ns-pill ns-muted">No fix — long-press the map</div>
				{:else if ambiguous && answer}
					<div class="ns-branch" role="group" aria-label="Which leg of the course are you on?">
						<span class="ns-branch-q">Which leg?</span>
						{#each answer.branches as b (b.chainageMeters)}
							<button class="ns-branch-btn" onclick={() => pickBranch(b)}>
								{branchLabel(b)}
							</button>
						{/each}
					</div>
				{:else if answer}
					<button
						class="ns-pill"
						class:is-stale={stale}
						style="border-color: {stale ? 'var(--color-warning)' : modeColor};"
						aria-expanded={expanded}
						aria-label={spoken}
						onclick={onToggle}
					>
						<span class="ns-name">{displayName}</span>
						<span class="ns-dist" aria-live="polite">
							{distanceText}{#if ageText}<span class="ns-age"> · {ageText}</span>{/if}
						</span>
						<span class="ns-badge" style="color: {modeColor}; border-color: {modeColor};">
							{answer.kind === 'road' ? 'ROAD' : 'DIRECT'}
						</span>
					</button>
					{#if origin?.source === 'pick'}
						<button class="ns-clear" onclick={onClear} aria-label="Measure from my position instead">
							×
						</button>
					{/if}
				{/if}
			</div>

			{#if expanded && answer && !ambiguous}
				<div class="ns-card">
					<div class="ns-card-head">
						<span class="ns-from">From {origin?.label ?? "my position"}</span>
						{#if mileLine}<span class="ns-mile">{mileLine}</span>{/if}
					</div>

					{#if answer.stops.length > 0}
						<div class="ns-stops" role="group" aria-label="Stops ahead">
							{#each answer.stops as s (s.id)}
								<button
									class="ns-stop"
									class:is-pinned={pinnedStopId === s.id}
									aria-pressed={pinnedStopId === s.id}
									onclick={() => pinStop(s.id)}
								>
									<span class="ns-stop-name">{s.name.toUpperCase()}</span>
									<span class="ns-stop-dist">
										{formatDistanceValue(s.meters, $unitSystem, answer.kind)}
									</span>
								</button>
							{/each}
						</div>
					{/if}

					<p class="ns-spoken">
						{formatDistance(answer.destMeters, $unitSystem, answer.kind)} to {answer.destName}
					</p>

					<p
						class="ns-honesty"
						class:is-warn={answer.originOffTrack > ON_COURSE_M}
					>
						{answer.honesty}
					</p>

					{#if answer.noStops}
						<p class="ns-empty">
							No stops marked on this course — set a category of Aid or Checkpoint on a
							pin in Annotations.
						</p>
					{/if}

					{#if routeAnnotations.length > 1}
						<button
							class="ns-route"
							aria-expanded={routeChooserOpen}
							onclick={() => (routeChooserOpen = !routeChooserOpen)}
						>
							{answer.routeName || 'Course'} ▾
						</button>
						{#if routeChooserOpen}
							<div class="ns-routes" role="group" aria-label="Choose a course">
								{#each routeAnnotations as r (r.id)}
									<button
										class="ns-route-opt"
										class:is-active={r.id === answer.routeId}
										onclick={() => chooseRoute(r.id)}
									>
										<span>{r.label}</span>
										<span class="ns-route-len">{routeLengthText(r)}</span>
									</button>
								{/each}
							</div>
						{/if}
					{:else if answer.routeName}
						<p class="ns-route-name">{answer.routeName}</p>
					{/if}
				</div>
		{/if}
	</div>
{/if}

<style>
	.ns-wrap {
		position: absolute;
		/* Next free slot in the hand-placed left map-chip column, below the
		   GPS status pill at 178px. */
		top: 226px;
		left: 10px;
		z-index: var(--z-toolbar);
		max-width: 300px;
	}

	.ns-row {
		display: flex;
		align-items: stretch;
		gap: 4px;
	}

	.ns-pill {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		min-height: 44px;
		padding: 0 10px;
		/* Opaque, never translucent — this is read in direct sunlight. */
		background: var(--color-bg-elevated, var(--color-surface));
		border: 1px solid var(--color-success);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-md);
		color: var(--color-text);
		cursor: pointer;
		text-align: left;
		font: inherit;
		transition: background var(--duration-fast) var(--ease-out);
	}

	.ns-pill:hover {
		background: var(--color-primary);
	}

	.ns-pill.ns-muted {
		cursor: default;
		border-color: var(--color-text-muted);
		color: var(--color-text-muted);
		font-size: 0.8rem;
	}

	.ns-pill.is-stale .ns-dist {
		color: var(--color-warning);
	}

	.ns-name {
		font-size: 13px;
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
		max-width: 11ch;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.ns-dist {
		font-size: 20px;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		line-height: 1.1;
	}

	.ns-age {
		font-size: 11px;
		font-weight: 500;
		color: var(--color-warning);
	}

	.ns-badge {
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		border: 1px solid currentColor;
		border-radius: var(--radius-sm);
		padding: 1px 5px;
	}

	.ns-clear {
		width: 28px;
		min-height: 44px;
		background: var(--color-bg-elevated, var(--color-surface));
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		color: var(--color-text-muted);
		font-size: 16px;
		line-height: 1;
		cursor: pointer;
	}

	.ns-clear:hover {
		color: var(--color-text);
	}

	.ns-branch {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		background: var(--color-bg-elevated, var(--color-surface));
		border: 1px solid var(--color-warning);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-md);
		padding: 4px 6px;
	}

	.ns-branch-q {
		font-size: 11px;
		color: var(--color-text-muted);
	}

	.ns-branch-btn {
		min-height: 44px;
		padding: 0 10px;
		background: var(--color-primary);
		color: var(--color-text);
		border: 1px solid var(--color-warning);
		border-radius: var(--radius-sm);
		font-size: 13px;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		cursor: pointer;
	}

	.ns-card {
		margin-top: 6px;
		width: 280px;
		max-width: calc(100vw - 20px);
		background: var(--color-bg-elevated, var(--color-surface));
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg);
		padding: var(--space-sm);
	}

	.ns-card-head {
		display: flex;
		justify-content: space-between;
		gap: var(--space-sm);
		font-size: 11px;
		color: var(--color-text-muted);
		margin-bottom: 6px;
	}

	.ns-mile {
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.ns-stops {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.ns-stop {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
		min-height: 44px;
		padding: 0 10px;
		background: var(--color-primary);
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		cursor: pointer;
	}

	.ns-stop.is-pinned {
		border-color: var(--color-accent);
	}

	.ns-stop-name {
		font-size: 12px;
		font-weight: 600;
		letter-spacing: 0.04em;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.ns-stop-dist {
		font-size: 14px;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.ns-spoken {
		margin-top: var(--space-sm);
		font-size: 12px;
		color: var(--color-text);
	}

	.ns-honesty {
		margin-top: 4px;
		font-size: 11px;
		color: var(--color-text-muted);
	}

	.ns-honesty.is-warn {
		color: var(--color-warning);
	}

	.ns-empty {
		margin-top: 6px;
		font-size: 11px;
		color: var(--color-text-muted);
	}

	.ns-route,
	.ns-route-opt {
		margin-top: 6px;
		width: 100%;
		min-height: 32px;
		background: transparent;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 11px;
		cursor: pointer;
		padding: 4px 6px;
	}

	.ns-route-opt {
		display: flex;
		justify-content: space-between;
		gap: var(--space-sm);
		min-height: 44px;
		align-items: center;
		text-align: left;
	}

	.ns-route-opt.is-active {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.ns-route-len {
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.ns-route-name {
		margin-top: 6px;
		font-size: 11px;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Narrow phones: the mobile FAB column owns the top-right, so the pill
	   only needs to stop growing into it. */
	@media (max-width: 400px) {
		.ns-wrap {
			max-width: calc(100vw - 80px);
		}

		.ns-dist {
			font-size: 18px;
		}

		.ns-name {
			max-width: 8ch;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.ns-pill {
			transition: none;
		}
	}
</style>
