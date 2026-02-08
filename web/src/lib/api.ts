import type { Station, Message, Conversation, HealthResponse, TransportStatus } from './types';

const BASE = '/api';

async function get<T>(path: string): Promise<T> {
	const res = await fetch(`${BASE}${path}`);
	if (!res.ok) throw new Error(`API error: ${res.status}`);
	return res.json();
}

async function post<T>(path: string, body: unknown): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!res.ok) throw new Error(`API error: ${res.status}`);
	return res.json();
}

export const api = {
	health: () => get<HealthResponse>('/health'),
	stations: () => get<Station[]>('/stations'),
	stationsInBounds: (s: number, w: number, n: number, e: number) =>
		get<Station[]>(`/stations?bounds=${s},${w},${n},${e}`),
	searchStations: (q: string) => get<Station[]>(`/stations?q=${encodeURIComponent(q)}`),
	station: (callsign: string) => get<Station>(`/stations/${encodeURIComponent(callsign)}`),
	conversations: () => get<Conversation[]>('/messages'),
	messages: (callsign: string) => get<Message[]>(`/messages/${encodeURIComponent(callsign)}`),
	sendMessage: (to: string, body: string) => post<Message>('/messages', { to, body }),
	transports: () => get<TransportStatus[]>('/transports')
};
