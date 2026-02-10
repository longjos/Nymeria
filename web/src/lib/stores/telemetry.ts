import { writable, derived } from 'svelte/store';
import { stations } from './stations';
import { api } from '$lib/api';
import type { TelemetryReading, TelemetryParams, Station } from '$lib/types';

/** Stations that have telemetry data, derived from the main stations store. */
export const telemetryStations = derived(stations, ($stations) => {
	const result: Station[] = [];
	for (const s of $stations.values()) {
		if (s.telemetry) {
			result.push(s);
		}
	}
	return result.sort((a, b) => a.callsign.localeCompare(b.callsign));
});

/** Historical telemetry readings for a selected station (loaded on demand). */
export const telemetryReadings = writable<TelemetryReading[]>([]);

/** Telemetry params for the selected station (loaded with readings). */
export const telemetryParams = writable<TelemetryParams | null>(null);

/** Currently selected telemetry station callsign. */
export const selectedTelemetryStation = writable<string | null>(null);

/** Loading state for telemetry readings. */
export const telemetryReadingsLoading = writable<boolean>(false);

export async function loadTelemetryReadings(callsign: string, since?: string): Promise<void> {
	selectedTelemetryStation.set(callsign);
	telemetryReadingsLoading.set(true);
	try {
		const params: Record<string, string> = {};
		if (since) params.since = since;
		const resp = await api.telemetryReadings(callsign, params);
		telemetryReadings.set(resp.readings);
		telemetryParams.set(resp.params);
	} catch {
		telemetryReadings.set([]);
		telemetryParams.set(null);
	} finally {
		telemetryReadingsLoading.set(false);
	}
}

/** Apply a telemetry equation: value = a*x^2 + b*x + c. */
export function applyEquation(params: TelemetryParams | null, channel: number, raw: number): number {
	if (!params || channel < 0 || channel > 4) return raw;
	const [a, b, c] = params.equations[channel];
	if (a === 0 && b === 0 && c === 0) return raw;
	return a * raw * raw + b * raw + c;
}

/** Get the display name for an analog channel. */
export function channelName(params: TelemetryParams | null, channel: number): string {
	if (!params || !params.paramNames[channel]) return `Analog ${channel + 1}`;
	return params.paramNames[channel];
}

/** Get the unit label for an analog channel. */
export function channelUnit(params: TelemetryParams | null, channel: number): string {
	if (!params || !params.unitLabels[channel]) return '';
	return params.unitLabels[channel];
}
