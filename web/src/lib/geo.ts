import type { Annotation } from './types';

/** Great-circle distance in metres. */
export function haversineMeters(lat1: number, lon1: number, lat2: number, lon2: number): number {
	const R = 6371000;
	const toRad = Math.PI / 180;
	const dLat = (lat2 - lat1) * toRad;
	const dLon = (lon2 - lon1) * toRad;
	const a = Math.sin(dLat / 2) ** 2 +
		Math.cos(lat1 * toRad) * Math.cos(lat2 * toRad) * Math.sin(dLon / 2) ** 2;
	return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

/** Nearest point-annotation to (lat, lon) within maxMeters, or null. */
export function nearestAnnotation(
	lat: number, lon: number,
	anns: Annotation[],
	maxMeters = 100
): { annotation: Annotation; meters: number } | null {
	let best: { annotation: Annotation; meters: number } | null = null;
	for (const a of anns) {
		let geo: { type?: string; coordinates?: unknown };
		try {
			geo = typeof a.geometry === 'string' ? JSON.parse(a.geometry) : a.geometry;
		} catch {
			continue;
		}
		if (geo?.type !== 'Point' || !Array.isArray(geo.coordinates)) continue;
		const [lon2, lat2] = geo.coordinates as [number, number];
		if (typeof lat2 !== 'number' || typeof lon2 !== 'number') continue;
		const meters = haversineMeters(lat, lon, lat2, lon2);
		if (meters <= maxMeters && (!best || meters < best.meters)) {
			best = { annotation: a, meters };
		}
	}
	return best;
}

/** Mean coordinate of a set of point annotations, or null when none. Used to
 * focus what3words autosuggest on a net's location centroid when the map
 * viewport isn't available. */
export function annotationCentroid(anns: Annotation[]): { lat: number; lon: number } | null {
	let sumLat = 0;
	let sumLon = 0;
	let count = 0;
	for (const a of anns) {
		let geo: { type?: string; coordinates?: unknown };
		try {
			geo = typeof a.geometry === 'string' ? JSON.parse(a.geometry) : a.geometry;
		} catch {
			continue;
		}
		if (geo?.type !== 'Point' || !Array.isArray(geo.coordinates)) continue;
		const [lon, lat] = geo.coordinates as [number, number];
		if (typeof lat !== 'number' || typeof lon !== 'number') continue;
		sumLat += lat;
		sumLon += lon;
		count++;
	}
	if (count === 0) return null;
	return { lat: sumLat / count, lon: sumLon / count };
}

/** Representative coordinate for an annotation being used as a mission location.
 *  Point → its own coordinate (the fallback is ignored — the annotation IS the
 *  location). LineString/Polygon → the clicked point when one is supplied (the
 *  user picked a spot on the route/area), else the geometry's midpoint vertex /
 *  outer-ring centroid (the ring's closing vertex, which duplicates the first
 *  point, is excluded so it doesn't skew the mean). Unparseable or unsupported
 *  geometry → the fallback, else null. */
export function annotationPickPoint(
	ann: Pick<Annotation, 'geometry'>,
	fallback?: { lat: number; lon: number } | null
): { lat: number; lon: number } | null {
	let geo: { type?: string; coordinates?: unknown };
	try {
		geo = typeof ann.geometry === 'string' ? JSON.parse(ann.geometry) : ann.geometry;
	} catch {
		return fallback ?? null;
	}

	if (geo?.type === 'Point' && Array.isArray(geo.coordinates)) {
		const [lon, lat] = geo.coordinates as [number, number];
		if (typeof lat === 'number' && typeof lon === 'number') return { lat, lon };
	}

	if (fallback) return fallback;

	if (geo?.type === 'LineString' && Array.isArray(geo.coordinates)) {
		const coords = geo.coordinates as [number, number][];
		if (coords.length === 0) return null;
		const [lon, lat] = coords[Math.floor(coords.length / 2)];
		if (typeof lat !== 'number' || typeof lon !== 'number') return null;
		return { lat, lon };
	}

	if (geo?.type === 'Polygon' && Array.isArray(geo.coordinates)) {
		const rings = geo.coordinates as [number, number][][];
		const ring = rings[0];
		if (!Array.isArray(ring) || ring.length === 0) return null;
		let pts = ring;
		const first = pts[0];
		const last = pts[pts.length - 1];
		if (pts.length > 1 && first[0] === last[0] && first[1] === last[1]) {
			pts = pts.slice(0, -1);
		}
		if (pts.length === 0) return null;
		let sumLat = 0;
		let sumLon = 0;
		for (const [lon, lat] of pts) {
			if (typeof lat !== 'number' || typeof lon !== 'number') return null;
			sumLat += lat;
			sumLon += lon;
		}
		return { lat: sumLat / pts.length, lon: sumLon / pts.length };
	}

	return null;
}
