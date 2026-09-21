import { describe, it, expect } from 'vitest';
import { countdown, countdownTone, clock, clockWithDay, clockWithSeconds, endsLabel, progress, fetchedLabel, linkStateText } from './wxAlertTime';
import type { WxLinkStatus } from './types';

// Fixed instant: 2026-09-17T20:00:00Z == 4:00 PM America/Detroit (EDT).
const NOW = Date.parse('2026-09-17T20:00:00Z');
const TZ = 'America/Detroit';

function iso(offsetMs: number): string {
	return new Date(NOW + offsetMs).toISOString();
}

describe('countdown', () => {
	it('shows whole minutes between 5 minutes and 1 hour', () => {
		expect(countdown(iso(42 * 60_000), NOW).text).toBe('42m');
	});

	it('shows Xm Ys under 5 minutes, never bare m:ss (P1-4: reads as a clock time otherwise)', () => {
		expect(countdown(iso(4 * 60_000 + 59_000), NOW).text).toBe('4m 59s');
	});

	it('shows hours and minutes between 1 and 24 hours', () => {
		expect(countdown(iso(2 * 3_600_000 + 10 * 60_000), NOW).text).toBe('2h 10m');
	});

	it('shows days and hours beyond 24 hours', () => {
		expect(countdown(iso(26 * 3_600_000), NOW).text).toBe('1d 2h');
	});

	it('reads "expired" once past', () => {
		expect(countdown(iso(-1000), NOW).text).toBe('expired');
	});

	it('is empty for a missing timestamp', () => {
		expect(countdown(null, NOW).text).toBe('');
		expect(countdown(undefined, NOW).text).toBe('');
	});

	// P1-4 (wx-alerts-populated-review.md): countdown formats that mislead.
	// "4:59" next to "updated 3:09 PM" reads as a clock time; "115m" forces
	// arithmetic nobody does in their head. Table-driven over every precision
	// bucket the formatter can land in.
	const cases: Array<{ name: string; offsetMs: number; want: string }> = [
		{ name: 'seconds-only, single digit', offsetMs: 7_000, want: '7s' },
		{ name: 'sub-minute (well under a minute)', offsetMs: 45_000, want: '45s' },
		{ name: 'under 5 minutes renders "4m 59s", not "4:59"', offsetMs: 4 * 60_000 + 59_000, want: '4m 59s' },
		{ name: 'exactly 5 minutes switches to whole-minutes', offsetMs: 5 * 60_000, want: '5m' },
		{ name: 'minutes under an hour', offsetMs: 42 * 60_000, want: '42m' },
		{ name: '115 minutes renders "1h 55m", not "115m"', offsetMs: 115 * 60_000, want: '1h 55m' },
		{ name: 'exactly one hour', offsetMs: 60 * 60_000, want: '1h 00m' },
		{ name: 'hours under a day', offsetMs: 5 * 3_600_000 + 5 * 60_000, want: '5h 05m' },
		{ name: 'multi-day', offsetMs: 3 * 24 * 3_600_000 + 4 * 3_600_000, want: '3d 4h' },
		{ name: 'already expired, just past', offsetMs: -1, want: 'expired' },
		{ name: 'already expired, well past', offsetMs: -60 * 60_000, want: 'expired' },
		{ name: 'expired exactly at the deadline (msLeft === 0)', offsetMs: 0, want: 'expired' }
	];

	it.each(cases)('$name', ({ offsetMs, want }) => {
		expect(countdown(iso(offsetMs), NOW).text).toBe(want);
	});
});

describe('countdownTone', () => {
	it('is normal well before the deadline', () => {
		expect(countdownTone(iso(42 * 60_000), NOW)).toBe('normal');
	});

	it('is soon inside the last 10 minutes', () => {
		expect(countdownTone(iso(9 * 60_000 + 59_000), NOW)).toBe('soon');
	});

	it('is expired once past', () => {
		expect(countdownTone(iso(-1000), NOW)).toBe('expired');
	});
});

describe('clock / clockWithSeconds', () => {
	it('formats hour:minute with AM/PM in the given timezone', () => {
		expect(clock(iso(42 * 60_000), TZ)).toBe('4:42 PM');
	});

	it('formats hour:minute:second with AM/PM', () => {
		expect(clockWithSeconds('2026-09-17T20:21:07Z', TZ)).toBe('4:21:07 PM');
	});

	it('is empty for a missing/unparseable timestamp', () => {
		expect(clock(null)).toBe('');
		expect(clock('not-a-date')).toBe('');
	});
});

describe('endsLabel', () => {
	it('reads ENDS <clock> while still in the future', () => {
		expect(endsLabel(iso(42 * 60_000), NOW, TZ)).toBe('ENDS 4:42 PM');
	});

	it('reads ENDED <clock> · <age> ago once past', () => {
		expect(endsLabel(iso(-12 * 60_000), NOW, TZ)).toBe('ENDED 3:48 PM · 12 min ago');
	});

	it('is empty for a missing timestamp', () => {
		expect(endsLabel(null, NOW, TZ)).toBe('');
	});
});

describe('progress', () => {
	it('is 0.5 halfway between effective and ends', () => {
		expect(progress(iso(-42 * 60_000), iso(42 * 60_000), NOW)).toBeCloseTo(0.5);
	});

	it('clamps to 1 once past ends', () => {
		expect(progress(iso(-60 * 60_000), iso(-1000), NOW)).toBe(1);
	});

	it('clamps to 0 before effective', () => {
		expect(progress(iso(60_000), iso(3_600_000), NOW)).toBe(0);
	});

	it('never produces NaN for bad data (ends before effective)', () => {
		const p = progress(iso(60_000), iso(-60_000), NOW);
		expect(Number.isNaN(p)).toBe(false);
		expect(p).toBe(1);
	});
});

function status(overrides: Partial<WxLinkStatus>): WxLinkStatus {
	return {
		state: 'live',
		enabled: true,
		contactConfigured: true,
		consecutiveFailures: 0,
		fromCache: false,
		regionCount: 0,
		inAreaCount: 0,
		nearbyCount: 0,
		zonesCached: 0,
		zonesMissing: 0,
		sounds: true,
		...overrides
	};
}

describe('fetchedLabel', () => {
	it('reads "updated <clock> · <age> ago" when live', () => {
		const s = status({ state: 'live', lastSuccessAt: iso(-40_000) });
		expect(fetchedLabel(s, NOW, TZ)).toBe('updated 3:59 PM · 40 s ago');
	});

	it('reads "last update … retrying…" when stale', () => {
		const s = status({ state: 'stale', lastSuccessAt: iso(-6 * 60_000) });
		expect(fetchedLabel(s, NOW, TZ)).toBe('last update 3:54 PM · 6 min ago · retrying…');
	});

	it('is empty when there has been no attempt yet', () => {
		expect(fetchedLabel(status({ lastSuccessAt: undefined, lastAttemptAt: undefined }), NOW, TZ)).toBe('');
	});
});

describe('linkStateText', () => {
	it('reads the down sentence with "since <clock>"', () => {
		const s = status({ state: 'down', lastSuccessAt: iso(-22 * 60_000) });
		expect(linkStateText(s, NOW, TZ)).toBe(
			'NWS unreachable since 3:38 PM · showing last known alerts · new warnings will NOT arrive'
		);
	});

	it('reads the off sentence verbatim', () => {
		expect(linkStateText(status({ state: 'off', enabled: false }), NOW, TZ)).toBe(
			'NWS alerts are turned off · internet required · enable in Settings'
		);
	});

	it('prefixes "From cache ·" before the first successful poll after boot', () => {
		const s = status({ state: 'live', fromCache: true, lastSuccessAt: iso(-40_000) });
		expect(linkStateText(s, NOW, TZ)).toBe('From cache · NWS · updated 3:59 PM · 40 s ago');
	});

	it('never prefixes "From cache ·" for the down sentence', () => {
		const s = status({ state: 'down', fromCache: true, lastSuccessAt: iso(-22 * 60_000) });
		expect(linkStateText(s, NOW, TZ).startsWith('NWS unreachable')).toBe(true);
	});
});

describe('linkStateText — off is a status, not a failure', () => {
	const base = {
		enabled: false,
		contactConfigured: false,
		consecutiveFailures: 0,
		fromCache: false,
		regionCount: 0,
		inAreaCount: 0,
		nearbyCount: 0,
		zonesCached: 0,
		zonesMissing: 0,
		sounds: false
	};
	const now = Date.parse('2026-09-18T15:00:00Z');

	it('says turned off, never "unreachable", when disabled', () => {
		const text = linkStateText({ ...base, state: 'off', reason: 'disabled' } as any, now);
		expect(text).toContain('turned off');
		expect(text.toLowerCase()).not.toContain('unreachable');
		expect(text).toContain('Settings');
	});

	it('names the contact fix rather than claiming it is off', () => {
		const text = linkStateText({ ...base, state: 'off', reason: 'contactMissing' } as any, now);
		expect(text).toContain('contact');
		expect(text).not.toContain('turned off');
	});

	it('distinguishes a failed start from being switched off', () => {
		const text = linkStateText({ ...base, state: 'off', reason: 'initFailed' } as any, now);
		expect(text).toContain('failed to start');
		expect(text).not.toContain('turned off');
	});

	it('still reports a genuine outage as unreachable', () => {
		const text = linkStateText(
			{ ...base, state: 'down', lastSuccessAt: '2026-09-18T14:00:00Z' } as any,
			now
		);
		expect(text).toContain('unreachable');
		expect(text).toContain('NOT');
	});
});

describe('linkStateText — cold start is not a healthy link', () => {
	const base = {
		enabled: true,
		contactConfigured: true,
		consecutiveFailures: 0,
		fromCache: true,
		regionCount: 0,
		inAreaCount: 0,
		nearbyCount: 0,
		zonesCached: 0,
		zonesMissing: 0,
		sounds: false
	};
	const now = Date.parse('2026-09-18T15:00:00Z');

	it('says nothing is being monitored, and never claims it is off', () => {
		const text = linkStateText({ ...base, state: 'off', reason: 'noWatchArea' } as any, now);
		expect(text).toContain('nothing is being monitored');
		expect(text).not.toContain('turned off');
	});

	it('does not imply a successful update', () => {
		const text = linkStateText({ ...base, state: 'off', reason: 'noWatchArea' } as any, now);
		expect(text).not.toContain('updated');
		expect(text).not.toContain('ago');
	});
});

describe('countdownTone urgent bucket', () => {
	it('escalates to urgent inside five minutes', () => {
		expect(countdownTone(iso(4 * 60_000 + 59_000), NOW)).toBe('urgent');
		expect(countdownTone(iso(30_000), NOW)).toBe('urgent');
	});

	it('is still only soon at five minutes and one second', () => {
		expect(countdownTone(iso(5 * 60_000 + 1_000), NOW)).toBe('soon');
	});
});

describe('clockWithDay', () => {
	it('omits the weekday for a time later today', () => {
		expect(clockWithDay(iso(2 * 3600_000), NOW)).toBe(clock(iso(2 * 3600_000)));
	});

	it('adds the weekday once the time is more than a day out', () => {
		const far = iso(3 * 24 * 3600_000);
		const got = clockWithDay(far, NOW);
		expect(got).toMatch(/^(Sun|Mon|Tue|Wed|Thu|Fri|Sat) /);
		expect(got.endsWith(clock(far))).toBe(true);
	});

	it('adds the weekday for a different calendar day even within 24 hours', () => {
		// 23:30 local "tomorrow" style case: same 24h window, different day.
		const d = new Date(NOW);
		d.setDate(d.getDate() + 1);
		d.setHours(9, 0, 0, 0);
		const isoStr = d.toISOString();
		expect(clockWithDay(isoStr, NOW)).toMatch(/^(Sun|Mon|Tue|Wed|Thu|Fri|Sat) /);
	});

	it('is empty for a missing or unparseable time', () => {
		expect(clockWithDay(null, NOW)).toBe('');
		expect(clockWithDay('nope', NOW)).toBe('');
	});
});
