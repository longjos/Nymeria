// The ONE ticking interval app-wide. Promoted out of stores/wxAlerts.ts
// (which used to own a private `setInterval` + `wxClock`/`wxMinute` pair) so
// every age chip, countdown and "Xm ago" label across the app — wx alerts,
// the ride status strip, anything else that ticks — reads the same clock.
// Two independent 1s intervals would let two "6m ago" chips silently drift
// apart over a multi-hour shift; a single shared store makes that
// structurally impossible.
import { writable, derived } from 'svelte/store';
import { browser } from '$app/environment';

/** 1 s tick, started by startClock(). */
export const secondClock = writable<number>(Date.now());
/** Rows only need to re-render once a minute except under a countdown threshold. */
export const minuteClock = derived(secondClock, ($t) => Math.floor($t / 60000));

let started = false;

/** Idempotent; browser-only. Call from every store that needs a ticking clock
 * (initWxAlertStore(), initRideStore(), …) — the first caller wins, every
 * later call is a no-op. */
export function startClock(): void {
	if (started || !browser) return;
	started = true;
	setInterval(() => secondClock.set(Date.now()), 1000);
}
