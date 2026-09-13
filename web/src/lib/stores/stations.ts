import { writable, derived } from 'svelte/store';
import type { Station } from '$lib/types';
import { api } from '$lib/api';
import { WSClient } from '$lib/ws';

export const stations = writable<Map<string, Station>>(new Map());

// Decorate-sort-undecorate: parse each lastHeard once instead of allocating two
// Date objects per comparison (~2*N*logN allocations per emission). Ordering is
// identical — the comparator yields the same numbers, Array#sort is stable, and
// an unparseable lastHeard yields NaN exactly as new Date(...).getTime() did.
export const stationList = derived(stations, ($stations) => {
	const arr: Array<[number, Station]> = [];
	for (const s of $stations.values()) arr.push([Date.parse(s.lastHeard), s]);
	arr.sort((a, b) => b[0] - a[0]);
	return arr.map((p) => p[1]);
});

export const wsClient = new WSClient();
let initialized = false;

// Inbound station traffic from APRS-IS is a firehose: one store write per packet
// means an O(N) Map clone plus a full derived cascade (sort -> filter -> map
// marker rebuild) for every single packet. Coalesce all station mutations into
// one write per animation frame. Nothing can paint faster than a frame, so the
// skipped intermediate states were never observable, and no code reads the
// stations map synchronously outside of a subscription.
const pendingUpserts = new Map<string, Station>();
const pendingRemovals = new Set<string>();
let flushScheduled = false;

function scheduleFlush(): void {
	if (flushScheduled) return;
	flushScheduled = true;
	if (typeof requestAnimationFrame === 'function') requestAnimationFrame(flushPending);
	else setTimeout(flushPending, 16);
}

function flushPending(): void {
	flushScheduled = false;
	if (!pendingUpserts.size && !pendingRemovals.size) return;
	stations.update((m) => {
		const next = new Map(m);
		for (const [k, s] of pendingUpserts) next.set(k, s);
		for (const k of pendingRemovals) next.delete(k);
		pendingUpserts.clear();
		pendingRemovals.clear();
		return next;
	});
}

function queueUpsert(s: Station): void {
	const k = stationKey(s);
	// Cross-delete keeps last-write-wins exact when an update and a removal for
	// the same station land in the same frame.
	pendingRemovals.delete(k);
	pendingUpserts.set(k, s);
	scheduleFlush();
}

function queueRemoval(s: Station): void {
	const k = stationKey(s);
	pendingUpserts.delete(k);
	pendingRemovals.add(k);
	scheduleFlush();
}

export function connectWS(token?: string): void {
	wsClient.connect(undefined, token);
}

export function initStationStore(): void {
	if (initialized) return;
	initialized = true;

	// Load initial data
	api.stations().then((list) => {
		stations.set(new Map(list.map((s) => [stationKey(s), s])));
	}).catch(() => {
		// API not available yet
	});

	wsClient.on('station_new', (msg) => {
		const s = msg.station as Station;
		if (!s) return;
		queueUpsert(s);
	});

	wsClient.on('station_update', (msg) => {
		const s = msg.station as Station;
		if (!s) return;
		queueUpsert(s);
	});

	wsClient.on('station_removed', (msg) => {
		const s = msg.station as Station;
		if (!s) return;
		queueRemoval(s);
	});
}

function stationKey(s: Station): string {
	return s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
}
