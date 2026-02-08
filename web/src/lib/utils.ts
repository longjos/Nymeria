export function timeAgo(dateStr: string): string {
	const now = Date.now();
	const then = new Date(dateStr).getTime();
	const seconds = Math.floor((now - then) / 1000);

	if (seconds < 60) return `${seconds}s ago`;
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	return `${days}d ago`;
}

export function formatSpeed(kmh: number | undefined): string {
	if (!kmh) return '';
	return `${Math.round(kmh)} km/h`;
}

export function formatAltitude(m: number | undefined): string {
	if (!m) return '';
	return `${Math.round(m)} m`;
}

export function formatCoord(lat: number, lon: number): string {
	const latDir = lat >= 0 ? 'N' : 'S';
	const lonDir = lon >= 0 ? 'E' : 'W';
	return `${Math.abs(lat).toFixed(4)}${latDir} ${Math.abs(lon).toFixed(4)}${lonDir}`;
}

export function formatCourse(deg: number | undefined): string {
	if (deg === undefined || deg === 0) return '';
	const dirs = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'];
	const idx = Math.round(deg / 45) % 8;
	return `${Math.round(deg)}° ${dirs[idx]}`;
}

export function stationDisplayName(callsign: string, ssid: number): string {
	return ssid > 0 ? `${callsign}-${ssid}` : callsign;
}

export function stationKey(station: { callsign: string; ssid: number }): string {
	return station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign;
}
