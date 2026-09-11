import { writable, derived } from 'svelte/store';
import { browser } from '$app/environment';
import type { GpsStatus } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';

const DISABLED: GpsStatus = { enabled: false, connected: false, fix: null, ageMillis: null, stale: true };

export const gpsStatus = writable<GpsStatus>(DISABLED);

/** Ticks once per second so age-derived UI re-renders without new WS traffic. */
export const gpsClock = writable(Date.now());

let initialized = false;

export function initGpsStore(): void {
	if (initialized) return;
	initialized = true;

	api.gps().then((s) => gpsStatus.set(s)).catch(() => gpsStatus.set(DISABLED));

	wsClient.on('own_position', (msg) => {
		const s = msg.gps as GpsStatus | undefined;
		if (s) gpsStatus.set(s);
	});

	if (browser) setInterval(() => gpsClock.set(Date.now()), 1000);
}

/** Live age in ms, computed client-side from fix.receivedAt (clamped >= 0). */
export const gpsAgeMs = derived([gpsStatus, gpsClock], ([$s, $now]) =>
	$s.fix ? Math.max(0, $now - Date.parse($s.fix.receivedAt)) : null
);

/** True only when there is a usable, non-stale 2D/3D fix. */
export const gpsHasFix = derived(gpsStatus, ($s) => !!$s.fix && $s.fix.mode >= 2 && !$s.stale);

/** "12s" / "3m 07s" / "1h 04m" — shared by the status pill and the map tooltip. */
export function formatGpsAge(ms: number): string {
	const s = Math.max(0, Math.round(ms / 1000));
	if (s < 60) return `${s}s`;
	const m = Math.floor(s / 60);
	if (m < 60) return `${m}m ${String(s % 60).padStart(2, '0')}s`;
	const h = Math.floor(m / 60);
	return `${h}h ${String(m % 60).padStart(2, '0')}m`;
}

// --- Follow toggle — per viewer, persisted ---

const FOLLOW_KEY = 'nymeria_gps_follow';

function loadFollow(): boolean {
	if (!browser) return false;
	try {
		return localStorage.getItem(FOLLOW_KEY) === '1';
	} catch {
		return false;
	}
}

export const gpsFollow = writable<boolean>(loadFollow());

if (browser) {
	gpsFollow.subscribe((v) => {
		try {
			localStorage.setItem(FOLLOW_KEY, v ? '1' : '0');
		} catch {
			// localStorage may be unavailable (SSR, privacy mode) — follow still
			// works for the session, it just won't persist across reloads.
		}
	});
}
