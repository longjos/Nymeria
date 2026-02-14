import { writable, derived } from 'svelte/store';
import type { Net, NetCheckIn, NetMission, NetEvent, NetNote, StationCategory, Annotation, CheckpointWithPassages, CheckpointPassage } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';
import { annotations, annotationList } from './annotations';

export const activeNet = writable<Net | null>(null);
export const opsView = writable<{ lat: number; lon: number; zoom: number } | null>(null);
export const checkIns = writable<NetCheckIn[]>([]);
export const missions = writable<NetMission[]>([]);
export const timeline = writable<NetEvent[]>([]);
export const notes = writable<NetNote[]>([]);
export const checkpoints = writable<CheckpointWithPassages[]>([]);

// Net-scoped annotation views derived from the global annotation store.
export const activeNetId = derived(activeNet, ($net) => $net?.id ?? '');

export const netAnnotations = derived(
	[annotations, activeNetId],
	([$anns, $netId]) => {
		if (!$netId) return [];
		return [...$anns.values()]
			.filter(a => a.netId === $netId)
			.sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0));
	}
);

export const netLocationAnnotations = derived(
	netAnnotations,
	($na) => $na.filter(a => a.type === 'point' && a.geometry)
);

export const annotationsByName = derived(
	netAnnotations,
	($na) => {
		const map = new Map<string, Annotation>();
		for (const a of $na) {
			map.set(a.label.toLowerCase(), a);
			if (a.shortName) map.set(a.shortName.toLowerCase(), a);
		}
		return map;
	}
);

// Pinned check-ins — ordered by the net's pinnedStations list.
export const pinnedCheckIns = derived(
	[activeNet, checkIns],
	([$net, $cis]) => {
		if (!$net?.pinnedStations?.length) return [];
		const ciMap = new Map($cis.map(ci => [ci.callsign, ci]));
		return $net.pinnedStations.map(cs => ciMap.get(cs)).filter(Boolean) as NetCheckIn[];
	}
);

// Hover state for cross-component highlighting (mission card → map + roster)
export const hoveredMissionId = writable<string | null>(null);

// Hover state for operator highlighting (assign picker → map)
export const hoveredCheckInId = writable<string | null>(null);

// Check-ins linked to the currently hovered mission
export const highlightedCheckIns = derived(
	[hoveredMissionId, checkIns],
	([$hovered, $cis]) => {
		if (!$hovered) return new Set<string>();
		return new Set($cis.filter(ci => ci.missionIds?.includes($hovered)).map(ci => ci.id));
	}
);

// Annotations linked to the currently hovered mission
export const highlightedAnnotations = derived(
	[hoveredMissionId, annotationList],
	([$hovered, $anns]) => {
		if (!$hovered) return new Set<string>();
		return new Set($anns.filter(a => a.missionIds?.includes($hovered)).map(a => a.id));
	}
);

// --- Checkpoint derived stores ---

export const orderedCheckpoints = derived(checkpoints, ($cps) =>
	[...$cps].sort((a, b) => a.meta.sequenceNumber - b.meta.sequenceNumber)
);

export const hasCheckpoints = derived(checkpoints, ($cps) => $cps.length > 0);

export const progressElements = derived(checkpoints, ($cps) => {
	// Compute element positions from passage data.
	const seqMap = new Map<string, number>();
	for (const cp of $cps) {
		seqMap.set(cp.meta.annotationId, cp.meta.sequenceNumber);
	}

	const elemMap = new Map<string, { label: string; lastCheckpointId: string; lastCheckpointSeq: number; lastPassageTime: string }>();
	for (const cp of $cps) {
		for (const p of cp.passages) {
			const seq = seqMap.get(p.checkpointId);
			if (seq == null) continue;
			const existing = elemMap.get(p.label);
			if (!existing || seq > existing.lastCheckpointSeq) {
				elemMap.set(p.label, {
					label: p.label,
					lastCheckpointId: p.checkpointId,
					lastCheckpointSeq: seq,
					lastPassageTime: p.passageTime,
				});
			}
		}
	}

	return [...elemMap.values()].sort((a, b) => b.lastCheckpointSeq - a.lastCheckpointSeq);
});

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
	const lines: { operator: NetCheckIn; mission: NetMission }[] = [];
	for (const ci of $cis) {
		if (!ci.missionIds?.length || ci.lat == null || ci.lon == null) continue;
		for (const mid of ci.missionIds) {
			const mission = missionMap.get(mid);
			if (mission && mission.lat != null && mission.lon != null) {
				lines.push({ operator: ci, mission });
			}
		}
	}
	return lines;
});

// Category counts for active operators.
export const categoryCounts = derived(checkIns, ($cis) => {
	const active = $cis.filter(ci => ci.status !== 'released');
	const counts = new Map<StationCategory, number>();
	for (const ci of active) {
		const cat = (ci.category || 'general') as StationCategory;
		counts.set(cat, (counts.get(cat) || 0) + 1);
	}
	return counts;
});

// Notes grouped by checkInId — latest first.
export const notesByCheckIn = derived(notes, ($notes) => {
	const map = new Map<string, NetNote[]>();
	for (const n of $notes) {
		if (!n.checkInId) continue;
		const arr = map.get(n.checkInId) || [];
		arr.push(n);
		map.set(n.checkInId, arr);
	}
	// Sort each group: latest first.
	for (const arr of map.values()) {
		arr.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
	}
	return map;
});

// Notes grouped by missionId — latest first.
export const notesByMission = derived(notes, ($notes) => {
	const map = new Map<string, NetNote[]>();
	for (const n of $notes) {
		if (!n.missionId) continue;
		const arr = map.get(n.missionId) || [];
		arr.push(n);
		map.set(n.missionId, arr);
	}
	for (const arr of map.values()) {
		arr.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
	}
	return map;
});

// Pinned/urgent notes (net-wide, not tied to a specific operator or mission, sorted latest first).
export const pinnedNotes = derived(notes, ($notes) =>
	$notes
		.filter((n) => n.pinned)
		.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
);

// --- Situation Board derived stores ---

export type AttentionReason = 'missing' | 'emergency' | 'stale' | 'rollcall';

export interface AttentionItem {
	checkIn: NetCheckIn;
	reason: AttentionReason;
	detail: string; // human-readable explanation
	action: string; // primary action label
}

export const attentionItems = derived(checkIns, ($cis): AttentionItem[] => {
	const now = Date.now();
	const items: AttentionItem[] = [];
	for (const ci of $cis) {
		if (ci.status === 'released') continue;
		if (ci.status === 'missing') {
			const mins = Math.floor((now - new Date(ci.lastHeard).getTime()) / 60000);
			items.push({ checkIn: ci, reason: 'missing', detail: `Last heard ${mins}m ago`, action: 'Locate' });
			continue; // don't double-count
		}
		if (ci.traffic === 'emergency') {
			items.push({ checkIn: ci, reason: 'emergency', detail: 'Emergency traffic', action: 'Respond' });
			continue;
		}
		const elapsed = now - new Date(ci.lastHeard).getTime();
		if (elapsed > STALE_THRESHOLD_MS) {
			const mins = Math.floor(elapsed / 60000);
			items.push({ checkIn: ci, reason: 'stale', detail: `Stale position (${mins}m)`, action: 'Check' });
			continue;
		}
		if (ci.missedRollCalls >= 2) {
			items.push({ checkIn: ci, reason: 'rollcall', detail: `${ci.missedRollCalls} missed roll calls`, action: 'Call' });
		}
	}
	// Sort: missing first, then emergency, stale, rollcall
	const reasonOrder: Record<AttentionReason, number> = { missing: 0, emergency: 1, stale: 2, rollcall: 3 };
	items.sort((a, b) => reasonOrder[a.reason] - reasonOrder[b.reason]);
	return items;
});

export const activeMissions = derived(missions, ($ms) =>
	$ms.filter(m => m.status !== 'complete')
		.sort((a, b) => {
			const prio: Record<string, number> = { emergency: 0, priority: 1, welfare: 2, routine: 3 };
			return (prio[a.priority] ?? 3) - (prio[b.priority] ?? 3);
		})
);

export const recentSignificantEvents = derived(timeline, ($tl) => {
	const noise = new Set(['position_update']);
	return [...$tl]
		.filter(e => !noise.has(e.type))
		.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
		.slice(0, 10);
});

// Situation metrics — computed counts for the metrics summary bar.
export interface NetMetrics {
	totalIn: number;
	available: number;
	assigned: number;      // assigned + enroute + onscene
	missing: number;
	stale: number;         // last heard >20m, not released, not already missing
	missionsActive: number;
	missionsDone: number;
}

const STALE_THRESHOLD_MS = 20 * 60 * 1000; // 20 minutes

export const netMetrics = derived([checkIns, missions], ([$cis, $ms]): NetMetrics => {
	const active = $cis.filter(ci => ci.status !== 'released');
	const now = Date.now();
	let available = 0;
	let assigned = 0;
	let missing = 0;
	let stale = 0;

	for (const ci of active) {
		switch (ci.status) {
			case 'available': available++; break;
			case 'assigned':
			case 'enroute':
			case 'onscene': assigned++; break;
			case 'missing': missing++; break;
		}
		if (ci.traffic === 'emergency') {
			// Emergency traffic operators count toward missing for attention purposes
			// but only if not already counted
			if (ci.status !== 'missing') missing++;
		}
		// Stale: >20m since last heard, not released, not already flagged missing
		if (ci.status !== 'missing' && ci.status !== 'released') {
			const elapsed = now - new Date(ci.lastHeard).getTime();
			if (elapsed > STALE_THRESHOLD_MS) stale++;
		}
	}

	return {
		totalIn: active.length,
		available,
		assigned,
		missing,
		stale,
		missionsActive: $ms.filter(m => m.status !== 'complete').length,
		missionsDone: $ms.filter(m => m.status === 'complete').length,
	};
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
		// Sync ops view from net data.
		if (n.opsViewLat != null && n.opsViewLon != null && n.opsViewZoom != null) {
			opsView.set({ lat: n.opsViewLat, lon: n.opsViewLon, zoom: n.opsViewZoom });
		}
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

	wsClient.on('checkpoint_passage', (msg) => {
		const passage = msg.data as CheckpointPassage;
		if (!passage) return;
		checkpoints.update((list) =>
			list.map((cp) => {
				if (cp.meta.annotationId !== passage.checkpointId) return cp;
				return {
					...cp,
					passages: [...cp.passages, passage],
					passageCount: cp.passageCount + 1,
					latestPassage: passage.passageTime,
				};
			})
		);
		// Also add a synthetic timeline entry.
		timeline.update((list) => [...list, {
			id: passage.id,
			netId: passage.netId,
			type: 'checkpoint_passage',
			callsign: passage.reportedBy || '',
			summary: `${passage.label} passed checkpoint`,
			details: '',
			createdAt: passage.passageTime,
		}]);
	});

	wsClient.on('checkpoint_meta_updated', (msg) => {
		const meta = msg.data as any;
		if (!meta) return;
		checkpoints.update((list) =>
			list.map((cp) => {
				if (cp.meta.annotationId !== meta.annotationId) return cp;
				return { ...cp, meta: { ...cp.meta, ...meta } };
			})
		);
	});

	wsClient.on('net_timeline_entry', (msg) => {
		const data = msg.data as any;
		if (!data) return;

		// If the data has 'content' and 'category', it's a NetNote (emitted as timeline entry).
		// Deduplicate against notes already added optimistically from API responses.
		if (data.content && data.category) {
			const note = data as NetNote;
			notes.update((list) => {
				if (list.some((n) => n.id === note.id)) return list;
				return [...list, note];
			});
		}

		// Always push to timeline (logEvent creates a separate NetEvent).
		// The timeline entry is a NetEvent with type/summary/callsign fields.
		if (data.type && data.summary) {
			timeline.update((list) => [...list, data as NetEvent]);
		}
	});
}

export async function loadNetData(netId: string): Promise<void> {
	try {
		const data = await api.net(netId);
		checkIns.set(data.checkIns || []);
		missions.set(data.missions || []);

		// Merge net-scoped annotations into the global annotation store.
		const netAnns = data.annotations || [];
		if (netAnns.length > 0) {
			annotations.update((m) => {
				for (const a of netAnns) m.set(a.id, a);
				return new Map(m);
			});
		}

		// Hydrate ops view from persisted net data.
		const n = data.net;
		if (n && n.opsViewLat != null && n.opsViewLon != null && n.opsViewZoom != null) {
			opsView.set({ lat: n.opsViewLat, lon: n.opsViewLon, zoom: n.opsViewZoom });
		}

		const [events, netNotes, cps] = await Promise.all([
			api.netEvents(netId),
			api.netNotes(netId),
			api.getCheckpoints(netId).catch(() => []),
		]);
		timeline.set(events || []);
		notes.set(netNotes || []);
		checkpoints.set(cps || []);
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
	checkpoints.set([]);
	opsView.set(null);
}
