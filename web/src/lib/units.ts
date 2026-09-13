import type { DistanceKind } from './routeDistance';

export type UnitSystem = 'metric' | 'imperial';

// --- Raw converters (for chart data) ---

export function convertTemp(c: number, units: UnitSystem): number {
	return units === 'imperial' ? c * 9 / 5 + 32 : c;
}

export function convertWindSpeed(ms: number, units: UnitSystem): number {
	// m/s → km/h or mph
	return units === 'imperial' ? ms * 2.237 : ms * 3.6;
}

export function convertPressure(hpa: number, units: UnitSystem): number {
	return units === 'imperial' ? hpa * 0.02953 : hpa;
}

export function convertRain(mm: number, units: UnitSystem): number {
	return units === 'imperial' ? mm * 0.03937 : mm;
}

// --- Formatted display strings ---

export function formatTemp(celsius: number | undefined, units: UnitSystem): string {
	if (celsius === undefined) return '--';
	if (units === 'imperial') {
		return `${(celsius * 9 / 5 + 32).toFixed(1)}°F`;
	}
	return `${celsius.toFixed(1)}°C`;
}

export function formatTempShort(celsius: number | undefined, units: UnitSystem): string {
	if (celsius === undefined) return '--';
	if (units === 'imperial') {
		return `${(celsius * 9 / 5 + 32).toFixed(1)}°`;
	}
	return `${celsius.toFixed(1)}°`;
}

export function formatWindSpeed(ms: number | undefined, units: UnitSystem): string {
	if (ms === undefined) return '--';
	if (units === 'imperial') {
		return `${(ms * 2.237).toFixed(0)} mph`;
	}
	return `${(ms * 3.6).toFixed(0)} km/h`;
}

export function formatWindSpeedValue(ms: number | undefined, units: UnitSystem): string {
	if (ms === undefined) return '--';
	if (units === 'imperial') {
		return `${(ms * 2.237).toFixed(0)}`;
	}
	return `${(ms * 3.6).toFixed(0)}`;
}

export function formatPressure(hpa: number | undefined, units: UnitSystem): string {
	if (hpa === undefined) return '--';
	if (units === 'imperial') {
		return `${(hpa * 0.02953).toFixed(2)}`;
	}
	return `${hpa.toFixed(1)}`;
}

export function formatRain(mm: number | undefined, units: UnitSystem): string {
	if (mm === undefined) return '--';
	if (units === 'imperial') {
		return `${(mm * 0.03937).toFixed(2)} in`;
	}
	return `${mm.toFixed(1)} mm`;
}

export function formatAltitude(m: number | undefined, units: UnitSystem): string {
	if (!m) return '';
	if (units === 'imperial') {
		return `${Math.round(m * 3.281)} ft`;
	}
	return `${Math.round(m)} m`;
}

export function formatSpeed(kmh: number | undefined, units: UnitSystem): string {
	if (!kmh) return '';
	if (units === 'imperial') {
		return `${Math.round(kmh * 0.6214)} mph`;
	}
	return `${Math.round(kmh)} km/h`;
}

// --- Unit labels (for chart axes) ---

export function tempUnit(units: UnitSystem): string {
	return units === 'imperial' ? '°F' : '°C';
}

export function windUnit(units: UnitSystem): string {
	return units === 'imperial' ? ' mph' : ' km/h';
}

export function pressureUnit(units: UnitSystem): string {
	return units === 'imperial' ? ' inHg' : ' hPa';
}

export function pressureLabel(units: UnitSystem): string {
	return units === 'imperial' ? 'inHg' : 'hPa';
}

export function rainUnit(units: UnitSystem): string {
	return units === 'imperial' ? ' in' : ' mm';
}

export function speedUnit(units: UnitSystem): string {
	return units === 'imperial' ? 'mph' : 'km/h';
}

// --- Distance ---
//
// Precision is deliberately capped at what the snap can actually support: a
// 50 m GPS offset costs up to ~20 m of along-course error, so anything past one
// decimal of a mile would be inventing accuracy.
//
// `kind` is a REQUIRED argument on every one of these, including the "value
// only" variant where it is not printed. That is on purpose: it makes an
// unlabelled distance type-impossible, and somebody is going to read this
// number aloud on a radio.

const METERS_PER_MILE = 1609.344;
const METERS_PER_FOOT = 0.3048;

function distanceParts(meters: number, units: UnitSystem): { num: string; short: string; long: string } {
	if (units === 'imperial') {
		const miles = meters / METERS_PER_MILE;
		if (miles < 0.1) {
			const ft = Math.round(meters / METERS_PER_FOOT / 10) * 10;
			return { num: `${ft}`, short: 'ft', long: ft === 1 ? 'foot' : 'feet' };
		}
		const num = miles < 9.95 ? miles.toFixed(1) : `${Math.round(miles)}`;
		return { num, short: 'mi', long: num === '1' ? 'mile' : 'miles' };
	}
	if (meters < 1000) {
		const m = Math.round(meters / 10) * 10;
		return { num: `${m}`, short: 'm', long: m === 1 ? 'metre' : 'metres' };
	}
	const km = meters / 1000;
	const num = km < 9.95 ? km.toFixed(1) : `${Math.round(km)}`;
	return { num, short: 'km', long: num === '1' ? 'kilometre' : 'kilometres' };
}

/** Bare magnitude with its unit symbol, e.g. '3.2 mi'. `kind` is unused in the
 *  output but required so no caller can format a distance without having decided
 *  whether it is a road measurement or a straight line. */
export function formatDistanceValue(
	meters: number | undefined,
	units: UnitSystem,
	kind: DistanceKind // eslint-disable-line -- required by design, see the block comment above
): string {
	if (meters === undefined || !Number.isFinite(meters)) return '--';
	void kind;
	const p = distanceParts(Math.max(0, meters), units);
	return `${p.num} ${p.short}`;
}

/** Labelled distance, e.g. '3.2 mi by road' or '2.8 mi direct'. */
export function formatDistance(
	meters: number | undefined,
	units: UnitSystem,
	kind: DistanceKind
): string {
	if (meters === undefined || !Number.isFinite(meters)) return '--';
	const value = formatDistanceValue(meters, units, kind);
	return kind === 'road' ? `${value} by road` : `${value} direct`;
}

/** The sentence the operator says into the microphone. The 'direct' form ends
 *  with the caveat that turns a wrong number into a valid lower bound. */
export function formatDistanceSpoken(
	meters: number | undefined,
	units: UnitSystem,
	kind: DistanceKind,
	destLabel: string
): string {
	if (meters === undefined || !Number.isFinite(meters)) return '--';
	const p = distanceParts(Math.max(0, meters), units);
	const value = `${p.num} ${p.long}`;
	return kind === 'road'
		? `${value} by road to ${destLabel}`
		: `${value} direct to ${destLabel} — road distance will be more`;
}
