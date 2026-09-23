/**
 * The course line and the numbered stops placed on it — the one place both
 * are computed, so the course rail, the ride strip and SAG placement can never
 * disagree about where a stop is.
 *
 * Before this, stores/ride.ts and stores/sagMap.ts each kept their own route
 * index cache and each placed stops from `checkpoints[].annotation`: a SNAPSHOT
 * taken when the checkpoints were loaded. Annotation updates never reached it,
 * so dragging a mis-placed stop, or setting it At capacity, changed nothing on
 * the rail until a reload (course-rail-spec B5). This store reads each stop's
 * LIVE annotation by id.
 *
 * It imports only netcontrol and annotations, so both ride.ts and sagMap.ts
 * can depend on it without a cycle (sagMap imports ride; course.ts, the
 * course-closure store, imports ride too — hence a separate module).
 */
import { derived } from 'svelte/store';
import { netAnnotations, orderedCheckpoints } from './netcontrol';
import { annotations } from './annotations';
import {
	buildRouteIndex,
	parseLineString,
	projectStopsBySequence,
	type PlacedStop,
	type RouteIndex,
	type SequencedStop
} from '$lib/routeDistance';
import type { Annotation } from '$lib/types';

const METERS_PER_MILE = 1609.344;
const routeIndexCache = new Map<string, RouteIndex>();

export interface CourseRoute {
	index: RouteIndex | null;
	name: string;
	/** How many route lines the net has; >1 means a request must say which. */
	count: number;
}

/** The course this net is running. */
export const courseRoute = derived(netAnnotations, (anns): CourseRoute => {
	const routes = anns.filter((a) => a.category === 'route');
	const route = routes[0];
	if (!route) return { index: null, name: '', count: 0 };
	try {
		const key = `${route.id}:${route.updatedAt ?? ''}`;
		let idx = routeIndexCache.get(key);
		if (!idx) {
			const coords = parseLineString(route.geometry);
			if (!coords) return { index: null, name: route.label ?? '', count: routes.length };
			idx = buildRouteIndex(coords);
			routeIndexCache.set(key, idx);
		}
		return { index: idx, name: route.shortName || route.label || '', count: routes.length };
	} catch {
		return { index: null, name: route.label ?? '', count: routes.length };
	}
});

/** A numbered stop, placed on the course, with its LIVE annotation. */
export interface CourseStop extends PlacedStop {
	seq: number;
	annotation: Annotation;
}

function pointOf(a: Annotation): { lat: number; lon: number } | null {
	try {
		const g = typeof a.geometry === 'string' ? JSON.parse(a.geometry) : a.geometry;
		if (g?.type !== 'Point' || !Array.isArray(g.coordinates)) return null;
		const [lon, lat] = g.coordinates;
		if (!Number.isFinite(lat) || !Number.isFinite(lon)) return null;
		return { lat, lon };
	} catch {
		return null;
	}
}

/**
 * Every numbered stop that sits on the course line, keyed by annotation id,
 * each on the leg its sequence number implies (projectStopsBySequence). Stops
 * that are numbered but not on the line are absent.
 */
export const courseStops = derived(
	[courseRoute, orderedCheckpoints, annotations],
	([route, cps, live]): Map<string, CourseStop> => {
		const out = new Map<string, CourseStop>();
		if (!route.index) return out;
		const byId = new Map<string, { seq: number; annotation: Annotation }>();
		const input: SequencedStop[] = [];
		for (const cp of cps) {
			const ann = live.get(cp.meta.annotationId) ?? cp.annotation;
			const pt = pointOf(ann);
			if (!pt) continue;
			byId.set(ann.id, { seq: cp.meta.sequenceNumber, annotation: ann });
			input.push({ id: ann.id, seq: cp.meta.sequenceNumber, ...pt });
		}
		try {
			for (const [id, placed] of projectStopsBySequence(route.index, input)) {
				const extra = byId.get(id);
				if (extra) out.set(id, { ...placed, ...extra });
			}
		} catch {
			// A malformed line is "no stops placed", not a broken strip.
		}
		return out;
	}
);

/** Stop miles by annotation id — the unit the rail and the tiles speak. */
export const courseStopMiles = derived(courseStops, (stops) => {
	const out = new Map<string, number>();
	for (const [id, s] of stops) out.set(id, s.chainageMeters / METERS_PER_MILE);
	return out;
});
