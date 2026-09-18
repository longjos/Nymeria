// Time formatting for NWS Alerts (internal/wxalert). Every function takes
// `now` (epoch ms) as an explicit parameter — never `Date.now()` inside this
// module — so callers can drive re-renders off a single ticking store
// (stores/wxAlerts.ts `wxClock`/`wxMinute`) and so every function is
// deterministic in tests.
import type { WxLinkStatus } from './types';

export interface Countdown {
	/** Human string: '45s', '4m 59s', '42m', '1h 55m', '1d 2h', 'expired', or '' for no input. */
	text: string;
	/** Milliseconds remaining; NaN when `iso` is missing/unparseable. */
	msLeft: number;
}

/**
 * Countdown to `iso`, formatted at decreasing precision the closer it gets:
 * > 24h -> '1d 2h'; >= 1h -> '1h 55m'; >= 5m -> '42m'; < 5m -> 'Xm Ys'
 * ('4m 59s') or, under a minute, 'Xs' ('45s'); already past -> 'expired'.
 *
 * Deliberately never renders bare 'M:SS' (e.g. '4:59') — next to a clock
 * time like "3:09 PM" that reads as a time of day, not a countdown. See
 * wx-alerts-populated-review.md P1-4.
 */
export function countdown(iso: string | null | undefined, now: number): Countdown {
	if (!iso) return { text: '', msLeft: NaN };
	const target = Date.parse(iso);
	if (Number.isNaN(target)) return { text: '', msLeft: NaN };
	const msLeft = target - now;
	if (msLeft <= 0) return { text: 'expired', msLeft };

	const totalSec = Math.floor(msLeft / 1000);
	const h = Math.floor(totalSec / 3600);
	const m = Math.floor((totalSec % 3600) / 60);
	const s = totalSec % 60;

	if (h >= 24) {
		const d = Math.floor(h / 24);
		return { text: `${d}d ${h % 24}h`, msLeft };
	}
	if (h >= 1) return { text: `${h}h ${String(m).padStart(2, '0')}m`, msLeft };
	if (totalSec >= 300) return { text: `${Math.floor(totalSec / 60)}m`, msLeft };
	if (m > 0) return { text: `${m}m ${String(s).padStart(2, '0')}s`, msLeft };
	return { text: `${s}s`, msLeft };
}

/** Coarse urgency bucket for countdown styling (row/detail/banner countdown color). */
export function countdownTone(iso: string | null | undefined, now: number): 'normal' | 'soon' | 'expired' {
	if (!iso) return 'normal';
	const target = Date.parse(iso);
	if (Number.isNaN(target)) return 'normal';
	const msLeft = target - now;
	if (msLeft <= 0) return 'expired';
	if (msLeft <= 10 * 60 * 1000) return 'soon';
	return 'normal';
}

/** '3:45 PM' */
export function clock(iso: string | null | undefined, tz?: string): string {
	if (!iso) return '';
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleTimeString('en-US', {
		hour: 'numeric',
		minute: '2-digit',
		...(tz ? { timeZone: tz } : {})
	});
}

/** '3:21:07 PM' */
export function clockWithSeconds(iso: string | null | undefined, tz?: string): string {
	if (!iso) return '';
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleTimeString('en-US', {
		hour: 'numeric',
		minute: '2-digit',
		second: '2-digit',
		...(tz ? { timeZone: tz } : {})
	});
}

/** '12 min' / '3h 05m' / '< 1 min' — a spelled-unit "ago" phrase (endsLabel). */
function agoPhrase(ms: number): string {
	const totalMin = Math.max(0, Math.round(ms / 60000));
	if (totalMin < 1) return '< 1 min';
	if (totalMin < 60) return `${totalMin} min`;
	const h = Math.floor(totalMin / 60);
	const m = totalMin % 60;
	return m > 0 ? `${h}h ${m}m` : `${h}h`;
}

/** '40 s' / '6 min' / '3 h' — the short "ago" phrase used by fetchedLabel/linkStateText. */
function agoShort(ms: number): string {
	const totalSec = Math.max(0, Math.round(ms / 1000));
	if (totalSec < 60) return `${totalSec} s`;
	const totalMin = Math.round(totalSec / 60);
	if (totalMin < 60) return `${totalMin} min`;
	const h = Math.floor(totalMin / 60);
	return `${h} h`;
}

/**
 * 'ENDS 8:42 PM' while `iso` is in the future; 'ENDED 3:45 PM · 12 min ago'
 * once it has passed. Used by the detail view's timing hero (§6.5).
 */
export function endsLabel(iso: string | null | undefined, now: number, tz?: string): string {
	if (!iso) return '';
	const target = Date.parse(iso);
	if (Number.isNaN(target)) return '';
	if (target > now) return `ENDS ${clock(iso, tz)}`;
	return `ENDED ${clock(iso, tz)} · ${agoPhrase(now - target)} ago`;
}

/**
 * Fraction (0..1) of the way from `effectiveIso` to `endsIso`, clamped.
 * Bad data (missing/unparseable timestamps, or ends <= effective) reads as
 * "done" (1) rather than throwing or producing NaN — the progress bar should
 * never render broken.
 */
export function progress(effectiveIso: string | null | undefined, endsIso: string | null | undefined, now: number): number {
	const eff = effectiveIso ? Date.parse(effectiveIso) : NaN;
	const end = endsIso ? Date.parse(endsIso) : NaN;
	if (Number.isNaN(eff) || Number.isNaN(end) || end <= eff) return 1;
	return Math.min(1, Math.max(0, (now - eff) / (end - eff)));
}

/**
 * Provenance sentence fragment for "how fresh is this": 'updated 4:59 PM ·
 * 40 s ago' (live) or 'last update 3:54 PM · 6 min ago · retrying…' (stale).
 * '' when there has never been a successful/attempted fetch.
 */
export function fetchedLabel(status: WxLinkStatus, now: number, tz?: string): string {
	const at = status.lastSuccessAt ?? status.lastAttemptAt;
	if (!at) return '';
	const ago = agoShort(now - Date.parse(at));
	if (status.state === 'stale') {
		return `last update ${clock(at, tz)} · ${ago} ago · retrying…`;
	}
	return `updated ${clock(at, tz)} · ${ago} ago`;
}

/**
 * The four exact link-state sentences (spec §6.4), reused verbatim by the
 * provenance strip and the map link pill so the wording never drifts between
 * the two surfaces.
 */
export function linkStateText(status: WxLinkStatus, now: number, tz?: string): string {
	const prefix = status.fromCache && status.state !== 'down' ? 'From cache · ' : '';
	switch (status.state) {
		case 'live': {
			if (!status.lastSuccessAt) return `${prefix}NWS · updated · just now`;
			const t = clock(status.lastSuccessAt, tz);
			const age = agoShort(now - Date.parse(status.lastSuccessAt));
			return `${prefix}NWS · updated ${t} · ${age} ago`;
		}
		case 'stale': {
			const at = status.lastSuccessAt ?? status.lastAttemptAt;
			if (!at) return `${prefix}NWS · last update unknown · retrying…`;
			const t = clock(at, tz);
			const age = agoShort(now - Date.parse(at));
			return `${prefix}NWS · last update ${t} · ${age} ago · retrying…`;
		}
		case 'down': {
			const since = status.lastSuccessAt ?? status.lastAttemptAt;
			const t = since ? clock(since, tz) : 'unknown';
			return `NWS unreachable since ${t} · showing last known alerts · new warnings will NOT arrive`;
		}
		case 'off':
		default:
			switch (status.reason) {
				case 'noWatchArea':
					return 'NWS alerts on, but no watch area yet · nothing is being monitored · open a net or set home zones';
				case 'contactMissing':
					return 'NWS alerts need a contact · NWS requires one to allow requests · add it in Settings';
				case 'initFailed':
					return 'NWS alerts are on but failed to start · check the server log · not receiving alerts';
				case 'disabled':
				default:
					return 'NWS alerts are turned off · internet required · enable in Settings';
			}
	}
}
