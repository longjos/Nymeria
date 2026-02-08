import { writable } from 'svelte/store';
import type { TransportStatus } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';

export const transports = writable<TransportStatus[]>([]);

let initialized = false;

export function initTransportStore(): void {
	if (initialized) return;
	initialized = true;

	api.transports().then((list) => {
		transports.set(list);
	}).catch(() => {});

	wsClient.on('transport_status', (msg) => {
		const list = msg.transports as TransportStatus[];
		if (list) transports.set(list);
	});
}
