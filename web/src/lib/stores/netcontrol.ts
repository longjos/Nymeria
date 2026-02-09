import { writable, derived } from 'svelte/store';
import type { Net, NetCheckIn, NetMission, NetEvent, NetNote } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';

export const activeNet = writable<Net | null>(null);
export const opsView = writable<{ lat: number; lon: number; zoom: number } | null>(null);
export const checkIns = writable<NetCheckIn[]>([]);
export const missions = writable<NetMission[]>([]);
export const timeline = writable<NetEvent[]>([]);
export const notes = writable<NetNote[]>([]);

// Smart sort: missing > emergency > stale(>20m) > available > working > released
function statusPriority(ci: NetCheckIn): number {
	if (ci.status === 'missing') return 0;
	if (ci.traffic === 'emergency') return 1;
	// Stale: last heard > 20 minutes ago
	const staleMs = Date.now() - new Date(ci.lastHeard).getTime();
	if (staleMs > 20 * 60 * 1000 && ci.status !== 'released') return 2;
	if (ci.status === 'available') return 3;
	if (ci.status === 'assigned' || ci.status === 'enroute') return 4;
	if (ci.status === 'onscene') return 5;
	if (ci.status === 'brb') return 6;
	if (ci.status === 'released') return 10;
	return 7;
}

export const sortedCheckIns = derived(checkIns, ($cis) =>
	[...$cis].sort((a, b) => statusPriority(a) - statusPriority(b))
);

export const activeCheckIns = derived(checkIns, ($cis) =>
	$cis.filter((ci) => ci.status !== 'released')
);

export const operatorsWithPosition = derived(checkIns, ($cis) =>
	$cis.filter((ci) => ci.lat != null && ci.lon != null && ci.status !== 'released')
);

export const missionsWithPosition = derived(missions, ($ms) =>
	$ms.filter((m) => m.lat != null && m.lon != null && m.status !== 'complete')
);

export const assignmentLines = derived([checkIns, missions], ([$cis, $ms]) => {
	const missionMap = new Map($ms.map((m) => [m.id, m]));
	return $cis
		.filter((ci) => ci.missionId && ci.lat != null && ci.lon != null)
		.map((ci) => ({ operator: ci, mission: missionMap.get(ci.missionId!) }))
		.filter((pair): pair is { operator: NetCheckIn; mission: NetMission } =>
			pair.mission != null && pair.mission.lat != null && pair.mission.lon != null
		);
});

let initialized = false;

export function initNetControlStore(): void {
	if (initialized) return;
	initialized = true;

	// Load active net on init.
	api.nets().then((nets) => {
		const open = nets.find((n) => n.status === 'open');
		if (open) {
			activeNet.set(open);
			loadNetData(open.id);
		}
	}).catch(() => {});

	// WebSocket handlers.
	wsClient.on('net_created', (msg) => {
		const n = msg.data as Net;
		if (n) activeNet.set(n);
	});

	wsClient.on('net_updated', (msg) => {
		const n = msg.data as Net;
		if (!n) return;
		activeNet.update((current) => {
			if (current && current.id === n.id) return n;
			if (n.status === 'open') return n;
			if (current && current.id === n.id && n.status !== 'open') return null;
			return current;
		});
	});

	wsClient.on('checkin_created', (msg) => {
		const ci = msg.data as NetCheckIn;
		if (!ci) return;
		checkIns.update((list) => [...list, ci]);
	});

	wsClient.on('checkin_updated', (msg) => {
		const ci = msg.data as NetCheckIn;
		if (!ci) return;
		checkIns.update((list) =>
			list.map((c) => (c.id === ci.id ? ci : c))
		);
	});

	wsClient.on('mission_created', (msg) => {
		const m = msg.data as NetMission;
		if (!m) return;
		missions.update((list) => [...list, m]);
	});

	wsClient.on('mission_updated', (msg) => {
		const m = msg.data as NetMission;
		if (!m) return;
		missions.update((list) =>
			list.map((existing) => (existing.id === m.id ? m : existing))
		);
	});

	wsClient.on('net_timeline_entry', (msg) => {
		const evt = msg.data as NetEvent;
		if (evt) {
			timeline.update((list) => [...list, evt]);
		}
	});
}

export async function loadNetData(netId: string): Promise<void> {
	try {
		const data = await api.net(netId);
		checkIns.set(data.checkIns || []);
		missions.set(data.missions || []);

		const events = await api.netEvents(netId);
		timeline.set(events || []);
	} catch {
		// Ignore fetch errors on init.
	}
}

export function clearNetControl(): void {
	activeNet.set(null);
	checkIns.set([]);
	missions.set([]);
	timeline.set([]);
	notes.set([]);
	opsView.set(null);
}
