import { writable, derived } from 'svelte/store';
import type { Station } from '$lib/types';
import { api } from '$lib/api';
import { WSClient } from '$lib/ws';

export const stations = writable<Map<string, Station>>(new Map());
export const stationList = derived(stations, ($stations) =>
	Array.from($stations.values()).sort(
		(a, b) => new Date(b.lastHeard).getTime() - new Date(a.lastHeard).getTime()
	)
);

export const wsClient = new WSClient();
let initialized = false;

export function initStationStore(): void {
	if (initialized) return;
	initialized = true;

	// Load initial data
	api.stations().then((list) => {
		stations.set(new Map(list.map((s) => [stationKey(s), s])));
	}).catch(() => {
		// API not available yet
	});

	// Connect WebSocket
	wsClient.connect();

	wsClient.on('station_new', (msg) => {
		const s = msg.station as Station;
		if (!s) return;
		stations.update((m) => {
			m.set(stationKey(s), s);
			return new Map(m);
		});
	});

	wsClient.on('station_update', (msg) => {
		const s = msg.station as Station;
		if (!s) return;
		stations.update((m) => {
			m.set(stationKey(s), s);
			return new Map(m);
		});
	});

	wsClient.on('station_removed', (msg) => {
		const s = msg.station as Station;
		if (!s) return;
		stations.update((m) => {
			m.delete(stationKey(s));
			return new Map(m);
		});
	});
}

function stationKey(s: Station): string {
	return s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
}
