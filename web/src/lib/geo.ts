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
