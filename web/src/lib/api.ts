import type {
	Station, Message, Conversation, HealthResponse, TransportStatus,
	SessionUser, PublicUser, ConfigResponse, Annotation, ActivityResponse
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
	annotations: () => get<Annotation[]>('/annotations'),
	createAnnotation: (ann: Partial<Annotation>) => post<Annotation>('/annotations', ann),
	updateAnnotation: (id: string, ann: Partial<Annotation>) => put<Annotation>(`/annotations/${id}`, ann),
	deleteAnnotation: (id: string) => del<{ status: string }>(`/annotations/${id}`),

	// Activity
	activity: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return get<ActivityResponse>(`/activity${qs}`);
	},
	activityExportUrl: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return `${BASE}/activity/export${qs}`;
	}
};
