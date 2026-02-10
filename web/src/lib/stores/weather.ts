import { writable, derived } from 'svelte/store';
import { stations } from './stations';
import { api } from '$lib/api';
import type { WeatherReading, WeatherConfig, WeatherAlertThreshold, Station } from '$lib/types';
import type { UnitSystem } from '$lib/units';

/** Stations that have weather data, derived from the main stations store. */
export const weatherStations = derived(stations, ($stations) => {
	const result: Station[] = [];
	for (const s of $stations.values()) {
		if (s.weather) {
			result.push(s);
		}
	}
	return result.sort((a, b) => a.callsign.localeCompare(b.callsign));
});

/** Historical weather readings for a selected station (loaded on demand). */
export const weatherReadings = writable<WeatherReading[]>([]);

/** Weather config (alert thresholds, retention). */
export const weatherConfig = writable<WeatherConfig>({ retentionDays: 7, units: 'metric' });

/** Current unit system derived from weather config. */
export const weatherUnits = derived(weatherConfig, ($cfg) => ($cfg.units || 'metric') as UnitSystem);

/** Currently selected weather station callsign. */
export const selectedWeatherStation = writable<string | null>(null);

/** Loading state for weather readings. */
export const weatherReadingsLoading = writable<boolean>(false);

let configLoaded = false;

export function initWeatherStore(): void {
	if (configLoaded) return;
	configLoaded = true;
	api.weatherConfig().then((cfg) => {
		weatherConfig.set(cfg);
	}).catch(() => {
		// Config not available, use defaults
	});
}

export async function loadWeatherReadings(callsign: string, since?: string): Promise<void> {
	selectedWeatherStation.set(callsign);
	weatherReadingsLoading.set(true);
	try {
		const params: Record<string, string> = {};
		if (since) params.since = since;
		const readings = await api.weatherReadings(callsign, params);
		weatherReadings.set(readings);
	} catch {
		weatherReadings.set([]);
	} finally {
		weatherReadingsLoading.set(false);
	}
}

/** Check if a weather metric value exceeds configured alert thresholds. */
export function isAlertTriggered(
	metric: string,
	value: number | undefined,
	thresholds: Record<string, WeatherAlertThreshold> | undefined
): boolean {
	if (value === undefined || !thresholds) return false;
	const t = thresholds[metric];
	if (!t) return false;
	if (t.min !== undefined && value < t.min) return true;
	if (t.max !== undefined && value > t.max) return true;
	return false;
}
