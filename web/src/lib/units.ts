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
