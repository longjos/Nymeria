import { writable, type Readable } from 'svelte/store';
import { api } from '$lib/api';

/** Whether the server has what3words enabled and an API key configured.
 * Loaded once at boot (Observer+ endpoint, so it works for every logged-in
 * role) and refreshed after a Settings save. Components read this store;
 * nobody fetches /w3w/status ad hoc. */
const _w3wConfigured = writable(false);
export const w3wConfigured: Readable<boolean> = _w3wConfigured;

/** Fetch and apply the current what3words status. Safe to call repeatedly
 * (e.g. after a Settings save) — always 200, never throws. */
export async function loadW3WStatus(): Promise<void> {
	try {
		const status = await api.w3wStatus();
		_w3wConfigured.set(status.configured);
	} catch {
		_w3wConfigured.set(false);
	}
}
