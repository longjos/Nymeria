import type { Station, Message, HealthResponse, TransportStatus } from './types';

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
	station: (callsign: string) => get<Station>(`/stations/${callsign}`),
	messages: () => get<Message[]>('/messages'),
	sendMessage: (to: string, body: string) => post<Message>('/messages', { to, body }),
	transports: () => get<TransportStatus[]>('/transports')
};
