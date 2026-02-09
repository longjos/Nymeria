import type {
	Station, Message, Conversation, Bulletin, HealthResponse, TransportStatus,
	SessionUser, PublicUser, ConfigResponse, Annotation, ActivityResponse,
	Net, NetCheckIn, NetMission, NetNote, NetEvent, NetSummary, TacticalAlias
} from './types';

const BASE = '/api';

const TOKEN_KEY = 'nymeria_token';

let authToken: string | null = null;

export function setAuthToken(token: string | null) {
	authToken = token;
	try {
		if (token) {
			localStorage.setItem(TOKEN_KEY, token);
		} else {
			localStorage.removeItem(TOKEN_KEY);
		}
	} catch {
		// localStorage may be unavailable (SSR, privacy mode)
	}
}

export function loadSavedToken(): string | null {
	try {
		return localStorage.getItem(TOKEN_KEY);
	} catch {
		return null;
	}
}

function headers(): Record<string, string> {
	const h: Record<string, string> = { 'Content-Type': 'application/json' };
	if (authToken) {
		h['Authorization'] = `Bearer ${authToken}`;
	}
	return h;
}

async function get<T>(path: string): Promise<T> {
	const res = await fetch(`${BASE}${path}`, { headers: headers() });
	if (!res.ok) throw new Error(`API error: ${res.status}`);
	return res.json();
}

async function post<T>(path: string, body: unknown): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method: 'POST',
		headers: headers(),
		body: JSON.stringify(body)
	});
	if (!res.ok) throw new Error(`API error: ${res.status}`);
	return res.json();
}

async function put<T>(path: string, body: unknown): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method: 'PUT',
		headers: headers(),
		body: JSON.stringify(body)
	});
	if (!res.ok) throw new Error(`API error: ${res.status}`);
	return res.json();
}

async function del<T>(path: string): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method: 'DELETE',
		headers: headers()
	});
	if (!res.ok) throw new Error(`API error: ${res.status}`);
	return res.json();
}

export const api = {
	// Public
	health: () => get<HealthResponse>('/health'),
	config: () => get<ConfigResponse>('/config'),

	// Session
	login: (name: string, pin?: string) => post<SessionUser>('/session', { name, pin }),
	session: () => get<SessionUser>('/session'),
	logout: () => del<{ status: string }>('/session'),

	// Users
	users: () => get<PublicUser[]>('/users'),
	updateUserRole: (id: string, role: string) => put<{ id: string; role: string }>(`/users/${id}/role`, { role }),
	removeUser: (id: string) => del<{ id: string; status: string }>(`/users/${id}`),

	// Stations
	stations: () => get<Station[]>('/stations'),
	stationsInBounds: (s: number, w: number, n: number, e: number) =>
		get<Station[]>(`/stations?bounds=${s},${w},${n},${e}`),
	searchStations: (q: string) => get<Station[]>(`/stations?q=${encodeURIComponent(q)}`),
	station: (callsign: string) => get<Station>(`/stations/${encodeURIComponent(callsign)}`),

	// Bulletins
	bulletins: () => get<Bulletin[]>('/bulletins'),

	// Messages
	conversations: () => get<Conversation[]>('/messages'),
	messages: (callsign: string) => get<Message[]>(`/messages/${encodeURIComponent(callsign)}`),
	sendMessage: (to: string, body: string) => post<Message>('/messages', { to, body }),
	claimConversation: (callsign: string, userId: string, userName: string) =>
		post<unknown>(`/messages/${encodeURIComponent(callsign)}/claim`, { userId, userName }),
	unclaimConversation: (callsign: string) =>
		del<unknown>(`/messages/${encodeURIComponent(callsign)}/claim`),

	// Transports
	transports: () => get<TransportStatus[]>('/transports'),

	// Annotations
	annotations: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return get<Annotation[]>(`/annotations${qs}`);
	},
	createAnnotation: (ann: Partial<Annotation>) => post<Annotation>('/annotations', ann),
	updateAnnotation: (id: string, ann: Partial<Annotation>) => put<Annotation>(`/annotations/${id}`, ann),
	deleteAnnotation: (id: string) => del<{ status: string }>(`/annotations/${id}`),
	changeAnnotationStatus: (id: string, status: string) => post<Annotation>(`/annotations/${id}/status`, { status }),

	// Activity
	activity: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return get<ActivityResponse>(`/activity${qs}`);
	},
	activityExportUrl: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return `${BASE}/activity/export${qs}`;
	},

	// Net Control
	nets: () => get<Net[]>('/nets'),
	net: (id: string) => get<{ net: Net; checkIns: NetCheckIn[]; missions: NetMission[] }>(`/nets/${id}`),
	createNet: (data: Partial<Net>) => post<Net>('/nets', data),
	openNet: (id: string) => post<Net>(`/nets/${id}/open`, {}),
	closeNet: (id: string) => post<{ net: Net; summary: NetSummary }>(`/nets/${id}/close`, {}),
	transferNCS: (id: string, callsign: string, userId: string) => post<Net>(`/nets/${id}/transfer`, { callsign, userId }),
	checkIn: (netId: string, callsign: string, traffic?: string) => post<NetCheckIn>(`/nets/${netId}/checkin`, { callsign, traffic }),
	updateCheckIn: (netId: string, ciId: string, data: Partial<NetCheckIn>) => put<NetCheckIn>(`/nets/${netId}/checkin/${ciId}`, data),
	checkOut: (netId: string, ciId: string) => post<{ status: string }>(`/nets/${netId}/checkout/${ciId}`, {}),
	createMission: (netId: string, data: Partial<NetMission>) => post<NetMission>(`/nets/${netId}/missions`, data),
	updateMission: (netId: string, mId: string, data: Partial<NetMission>) => put<NetMission>(`/nets/${netId}/missions/${mId}`, data),
	addNetNote: (netId: string, data: { checkInId?: string; content: string }) => post<NetNote>(`/nets/${netId}/notes`, data),
	netEvents: (netId: string) => get<NetEvent[]>(`/nets/${netId}/events`),
	netNotes: (netId: string) => get<NetNote[]>(`/nets/${netId}/notes`),
	initiateRollCall: (netId: string) => post<{ status: string }>(`/nets/${netId}/rollcall`, {}),
	recordRollCallResponse: (netId: string, ciId: string) => post<{ status: string }>(`/nets/${netId}/rollcall/${ciId}`, {}),
	searchOperators: (q: string) => get<Station[]>(`/nets/search?q=${encodeURIComponent(q)}`),
	assignMission: (netId: string, ciId: string, missionId: string) =>
		post<NetCheckIn>(`/nets/${netId}/checkin/${ciId}/assign`, { missionId }),
	unassignMission: (netId: string, ciId: string) =>
		del<NetCheckIn>(`/nets/${netId}/checkin/${ciId}/assign`),
	addTrackedStation: (netId: string, ciId: string, callsign: string) =>
		post<NetCheckIn>(`/nets/${netId}/checkin/${ciId}/devices`, { callsign }),
	removeTrackedStation: (netId: string, ciId: string, callsign: string) =>
		del<NetCheckIn>(`/nets/${netId}/checkin/${ciId}/devices/${encodeURIComponent(callsign)}`),
	rosterExportUrl: (netId: string) => `${BASE}/nets/${netId}/roster/export`,

	// Tactical Aliases
	tacticalAliases: () => get<TacticalAlias[]>('/tactical'),
	setTacticalAlias: (callsign: string, alias: string) =>
		put<TacticalAlias>(`/tactical/${encodeURIComponent(callsign)}`, { alias }),
	deleteTacticalAlias: (callsign: string) =>
		del<{ status: string }>(`/tactical/${encodeURIComponent(callsign)}`)
};
