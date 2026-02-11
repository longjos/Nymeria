import { writable, derived, get } from 'svelte/store';
import { checkIns, missions, notes } from './netcontrol';
import { stationList } from './stations';
import { tacticalAliases } from './tactical';
import type { NetCheckIn, NetMission, NetNote, Station, NoteCategory, NoteSeverity } from '$lib/types';

// --- Types ---

export interface PaletteResult {
	type: 'checkin' | 'station';
	id: string;
	callsign: string;
	tacticalCall: string;
	operatorName: string;
	status: string;
	traffic: string;
	lastHeard: string;
	lat?: number;
	lon?: number;
	missionIds: string[];
	trackedStations: { callsign: string }[];
	comment?: string;
	score: number;
	/** Original check-in if type=checkin */
	checkIn?: NetCheckIn;
	/** Original station if type=station */
	station?: Station;
}

export type PaletteFilter = 'all' | 'roster' | 'tactical';

// --- Stores ---

export const paletteQuery = writable<string>('');
export const paletteFilter = writable<PaletteFilter>('all');
export const recentCallsigns = writable<string[]>(loadRecents());

// --- Recents persistence (session storage) ---

function loadRecents(): string[] {
	try {
		const raw = sessionStorage.getItem('nymeria_palette_recents');
		return raw ? JSON.parse(raw) : [];
	} catch {
		return [];
	}
}

function saveRecents(recents: string[]) {
	try {
		sessionStorage.setItem('nymeria_palette_recents', JSON.stringify(recents));
	} catch { /* ignore */ }
}

export function recordInteraction(callsign: string) {
	recentCallsigns.update((list) => {
		const filtered = list.filter((c) => c !== callsign);
		const next = [callsign, ...filtered].slice(0, 10);
		saveRecents(next);
		return next;
	});
}

// --- Fuzzy scoring ---

function fuzzyScore(query: string, target: string): number {
	if (!query || !target) return 0;
	const q = query.toLowerCase();
	const t = target.toLowerCase();

	// Exact match
	if (t === q) return 1000;

	// Prefix match
	if (t.startsWith(q)) return 800 + (q.length / t.length) * 100;

	// Contains match
	const idx = t.indexOf(q);
	if (idx >= 0) return 500 + (q.length / t.length) * 100;

	// Character-order match (fuzzy)
	let qi = 0;
	let consecutive = 0;
	let maxConsecutive = 0;
	for (let ti = 0; ti < t.length && qi < q.length; ti++) {
		if (t[ti] === q[qi]) {
			qi++;
			consecutive++;
			if (consecutive > maxConsecutive) maxConsecutive = consecutive;
		} else {
			consecutive = 0;
		}
	}
	if (qi === q.length) {
		return 200 + maxConsecutive * 30 + (q.length / t.length) * 50;
	}

	return 0;
}

function scoreResult(query: string, item: PaletteResult): number {
	if (!query) return 0;
	const fields = [
		{ text: item.callsign, weight: 1.0 },
		{ text: item.tacticalCall, weight: 0.9 },
		{ text: item.operatorName, weight: 0.8 },
		{ text: item.comment ?? '', weight: 0.4 },
	];

	let best = 0;
	for (const f of fields) {
		if (!f.text) continue;
		const s = fuzzyScore(query, f.text) * f.weight;
		if (s > best) best = s;
	}

	// Bonus for checked-in operators
	if (item.type === 'checkin') best += 100;

	// Bonus for emergency/missing
	if (item.status === 'missing') best += 50;
	if (item.traffic === 'emergency') best += 40;

	return best;
}

// --- Derived results ---

export const paletteResults = derived(
	[paletteQuery, paletteFilter, checkIns, stationList, tacticalAliases],
	([$query, $filter, $checkIns, $stations, $aliases]) => {
		const q = $query.trim();

		// Build unified list of candidates
		const candidates: PaletteResult[] = [];
		const seenCallsigns = new Set<string>();

		// Checked-in operators first
		for (const ci of $checkIns) {
			seenCallsigns.add(ci.callsign);
			candidates.push({
				type: 'checkin',
				id: ci.id,
				callsign: ci.callsign,
				tacticalCall: ci.tacticalCall,
				operatorName: ci.operatorName,
				status: ci.status,
				traffic: ci.traffic,
				lastHeard: ci.lastHeard,
				lat: ci.lat,
				lon: ci.lon,
				missionIds: ci.missionIds ?? [],
				trackedStations: ci.trackedStations ?? [],
				score: 0,
				checkIn: ci,
			});
		}

		// APRS stations (not already in check-ins) — only if filter allows
		if ($filter !== 'roster') {
			for (const st of $stations) {
				const key = st.ssid > 0 ? `${st.callsign}-${st.ssid}` : st.callsign;
				if (seenCallsigns.has(key) || seenCallsigns.has(st.callsign)) continue;
				const alias = $aliases.get(st.callsign);
				candidates.push({
					type: 'station',
					id: key,
					callsign: key,
					tacticalCall: alias?.alias ?? '',
					operatorName: '',
					status: '',
					traffic: '',
					lastHeard: st.lastHeard,
					lat: st.position?.lat,
					lon: st.position?.lon,
					missionIds: [],
					trackedStations: [],
					comment: st.comment,
					score: 0,
					station: st,
				});
			}
		}

		// Tactical filter: only items with a tactical call
		let filtered = $filter === 'tactical'
			? candidates.filter((c) => c.tacticalCall)
			: candidates;

		// If no query, show all (sorted by lastHeard)
		if (!q) {
			return filtered
				.sort((a, b) => {
					// Checkins first
					if (a.type !== b.type) return a.type === 'checkin' ? -1 : 1;
					return new Date(b.lastHeard).getTime() - new Date(a.lastHeard).getTime();
				})
				.slice(0, 20);
		}

		// Score and sort
		for (const c of filtered) {
			c.score = scoreResult(q, c);
		}

		return filtered
			.filter((c) => c.score > 0)
			.sort((a, b) => b.score - a.score)
			.slice(0, 20);
	}
);

// --- Empty state data ---

export const emptyStateData = derived(
	[recentCallsigns, checkIns, notes],
	([$recents, $checkIns, $notes]) => {
		const activeOps = $checkIns
			.filter((ci) => ci.status !== 'released')
			.sort((a, b) => new Date(b.lastHeard).getTime() - new Date(a.lastHeard).getTime())
			.slice(0, 5);

		const pinned = $notes
			.filter((n) => n.pinned)
			.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
			.slice(0, 3);

		return {
			recentCallsigns: $recents.slice(0, 5),
			activeOperators: activeOps,
			pinnedNotes: pinned,
		};
	}
);

// --- Note category & severity metadata (shared with NetControlPanel) ---

export const noteCategoryMeta: Record<NoteCategory, { label: string; color: string }> = {
	general: { label: 'Gen', color: '#6b7280' },
	medical: { label: 'Med', color: '#ef4444' },
	logistical: { label: 'Log', color: '#3b82f6' },
	tactical: { label: 'Tac', color: '#8b5cf6' },
	weather: { label: 'Wx', color: '#06b6d4' },
	resource: { label: 'Res', color: '#f59e0b' },
	hazard: { label: 'Haz', color: '#f97316' },
	comms: { label: 'Com', color: '#6b7280' },
};

export const severityMeta: Record<NoteSeverity, { label: string; color: string }> = {
	info: { label: 'Info', color: '#6b7280' },
	routine: { label: 'Routine', color: '#22c55e' },
	priority: { label: 'Priority', color: '#f59e0b' },
	urgent: { label: 'Urgent', color: '#ef4444' },
};

export const statusColors: Record<string, string> = {
	available: '#22c55e',
	assigned: '#3b82f6',
	enroute: '#8b5cf6',
	onscene: '#06b6d4',
	brb: '#f59e0b',
	missing: '#ef4444',
	released: '#6b7280',
};

export const missionPriorityColors: Record<string, string> = {
	routine: '#22c55e',
	priority: '#f59e0b',
	urgent: '#ef4444',
	emergency: '#ef4444',
};

export const trafficMeta: Record<string, { label: string; color: string }> = {
	none: { label: 'None', color: '#6b7280' },
	routine: { label: 'Routine', color: '#22c55e' },
	priority: { label: 'Priority', color: '#f59e0b' },
	welfare: { label: 'Welfare', color: '#3b82f6' },
	emergency: { label: 'Emergency', color: '#ef4444' },
};

export const stationCategoryMeta: Record<string, { label: string; short: string; color: string }> = {
	general:  { label: 'General',  short: 'GEN', color: '#6b7280' },
	command:  { label: 'Command',  short: 'CMD', color: '#eab308' },
	medical:  { label: 'Medical',  short: 'MED', color: '#ef4444' },
	sag:      { label: 'SAG',      short: 'SAG', color: '#f97316' },
	marshal:  { label: 'Marshal',  short: 'MAR', color: '#3b82f6' },
	fixed:    { label: 'Fixed',    short: 'FIX', color: '#14b8a6' },
	mobile:   { label: 'Mobile',   short: 'MOB', color: '#8b5cf6' },
	tactical: { label: 'Tactical', short: 'TAC', color: '#6366f1' },
};
