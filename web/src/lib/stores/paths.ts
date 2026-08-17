import { writable } from 'svelte/store';
import { api } from '$lib/api';
import { wsClient } from './stations';

const DEFAULT_PATH = 'WIDE1-1,WIDE2-1';

export const messagePath = writable(DEFAULT_PATH);
export const beaconPath = writable(DEFAULT_PATH);

export function setPaths(message: string | undefined | null, beacon: string | undefined | null) {
	if (message != null) messagePath.set(message);
	if (beacon != null) beaconPath.set(beacon);
}

export async function loadPaths(): Promise<void> {
	try {
		const cfg = await api.config();
		setPaths(cfg.messagePath ?? DEFAULT_PATH, cfg.beaconPath ?? DEFAULT_PATH);
	} catch {
		// keep defaults
	}
}

let initialized = false;

export function initPathStore(): void {
	if (initialized) return;
	initialized = true;
	loadPaths();
	wsClient.on('paths_updated', (msg) => {
		setPaths(
			typeof msg.messagePath === 'string' ? msg.messagePath : undefined,
			typeof msg.beaconPath === 'string' ? msg.beaconPath : undefined
		);
	});
}
