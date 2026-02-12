/**
 * Dead reckoning projection utilities for moving APRS stations.
 * Pure math — no Leaflet dependency.
 */

/** Minimum speed (km/h) to consider a station "moving" */
export const MOVING_THRESHOLD_KMH = 1.0;

/** Maximum age (minutes) before DR projection is hidden */
export const DR_MAX_MINUTES = 15;

/** How often (ms) the DR layer updates on the client */
export const DR_UPDATE_INTERVAL_MS = 30000;

/** Earth radius in km */
const R = 6371;

/** Degrees to radians */
function toRad(deg: number): number {
	return (deg * Math.PI) / 180;
}

/** Radians to degrees */
function toDeg(rad: number): number {
	return (rad * 180) / Math.PI;
}

/**
 * Returns true if the station is considered moving.
 * Speed must exceed threshold and course must be valid (> 0).
 */
export function isMoving(speed: number | undefined, course: number | undefined): boolean {
	if (speed == null || course == null) return false;
	return speed > MOVING_THRESHOLD_KMH && course > 0;
}

/**
 * Project a lat/lon forward along a bearing for a given distance.
 * Uses spherical Earth model (Vincenty-like forward formula).
 *
 * @param lat - starting latitude (degrees)
 * @param lon - starting longitude (degrees)
 * @param speedKmh - speed in km/h
 * @param courseDeg - course in degrees (0 = north, clockwise)
 * @param elapsedMinutes - time to project forward
 * @returns projected [lat, lon] in degrees
 */
export function projectPosition(
	lat: number,
	lon: number,
	speedKmh: number,
	courseDeg: number,
	elapsedMinutes: number
): [number, number] {
	const d = (speedKmh * (elapsedMinutes / 60)) / R; // angular distance
	const brng = toRad(courseDeg);
	const lat1 = toRad(lat);
	const lon1 = toRad(lon);

	const lat2 = Math.asin(
		Math.sin(lat1) * Math.cos(d) + Math.cos(lat1) * Math.sin(d) * Math.cos(brng)
	);
	const lon2 =
		lon1 +
		Math.atan2(
			Math.sin(brng) * Math.sin(d) * Math.cos(lat1),
			Math.cos(d) - Math.sin(lat1) * Math.sin(lat2)
		);

	return [toDeg(lat2), toDeg(lon2)];
}

export interface DRCone {
	/** Center projected position */
	center: [number, number];
	/** Left edge of uncertainty cone */
	left: [number, number][];
	/** Right edge of uncertainty cone */
	right: [number, number][];
	/** Confidence 0..1 (decays linearly over DR_MAX_MINUTES) */
	confidence: number;
	/** Elapsed minutes since last report */
	elapsedMinutes: number;
}

/**
 * Compute a dead reckoning cone polygon for a moving station.
 *
 * The cone widens over time to represent growing uncertainty:
 * - At 1 minute: +/- 5 degrees
 * - At DR_MAX_MINUTES: +/- 30 degrees
 * Confidence decays linearly from 1.0 to 0.1.
 *
 * Returns null if the station isn't stale enough (< 1 min) or too old (> DR_MAX_MINUTES).
 */
export function computeDRCone(
	lat: number,
	lon: number,
	speed: number,
	course: number,
	lastHeardMs: number,
	nowMs: number
): DRCone | null {
	const elapsedMs = nowMs - lastHeardMs;
	const elapsedMinutes = elapsedMs / 60000;

	// Only show DR between 1 and DR_MAX_MINUTES
	if (elapsedMinutes < 1 || elapsedMinutes > DR_MAX_MINUTES) return null;

	// Uncertainty angle grows from 5 to 30 degrees over the window
	const t = (elapsedMinutes - 1) / (DR_MAX_MINUTES - 1);
	const halfAngle = 5 + t * 25;

	// Confidence decays linearly
	const confidence = Math.max(0.1, 1.0 - (elapsedMinutes / DR_MAX_MINUTES) * 0.9);

	// Project center line
	const center = projectPosition(lat, lon, speed, course, elapsedMinutes);

	// Build cone edges with intermediate points for smooth curve
	const steps = 4;
	const left: [number, number][] = [[lat, lon]];
	const right: [number, number][] = [[lat, lon]];

	for (let i = 1; i <= steps; i++) {
		const frac = i / steps;
		const mins = elapsedMinutes * frac;
		const angle = halfAngle * frac; // cone widens progressively

		left.push(projectPosition(lat, lon, speed, course - angle, mins));
		right.push(projectPosition(lat, lon, speed, course + angle, mins));
	}

	return { center, left, right, confidence, elapsedMinutes };
}
