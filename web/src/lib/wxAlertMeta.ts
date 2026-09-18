// Metadata tables and pure helpers for NWS Alerts (internal/wxalert).
// Mirrors annotationMeta.ts in style: plain data + small pure functions, no
// store imports. Per BUILD-PLAN.md §1 rows 33/34, tier and shortCode are
// derived server-side ONLY — this file reads `alert.tier` / `alert.shortCode`
// from the payload and never re-derives them. `tierRank`/`severityRank`
// exist purely for client-side sorting/filtering, not classification.
import type { WxAlert, WxTier, WxSeverity, WxNotifyClass } from './types';
import { clock } from './wxAlertTime';

export const TIER_ORDER: WxTier[] = ['warning', 'watch', 'advisory', 'statement'];

const TIER_RANK: Record<WxTier, number> = { warning: 3, watch: 2, advisory: 1, statement: 0 };

/** Total order over tiers: warning=3 .. statement=0. For sorting/filtering only. */
export function tierRank(t: WxTier): number {
	return TIER_RANK[t] ?? 0;
}

const SEVERITY_RANK: Record<WxSeverity, number> = { Extreme: 4, Severe: 3, Moderate: 2, Minor: 1, Unknown: 0 };

/** Total order over severities: Extreme=4 .. Unknown=0. */
export function severityRank(s: WxSeverity): number {
	return SEVERITY_RANK[s] ?? 0;
}

const NOTIFY_RANK: Record<WxNotifyClass, number> = { interrupt: 3, toast: 2, badge: 1, panel: 0 };

/** Total order over notify classes: interrupt=3 .. panel=0. */
export function notifyRank(c: WxNotifyClass): number {
	return NOTIFY_RANK[c] ?? 0;
}

export interface TierMeta {
	label: string;
	/** CSS custom property name, always '--color-wx-*' — never '--color-warning'/'--color-error' directly. */
	colorVar: string;
	softVar: string;
	dashVar: string;
	/** SVG path for a 16x16 viewBox (WxTierGlyph). */
	glyph: string;
	glyphFill: boolean;
	ariaWord: string;
}

export const tierMeta: Record<WxTier, TierMeta> = {
	warning: {
		label: 'Warning',
		colorVar: '--color-wx-warning',
		softVar: '--color-wx-warning-soft',
		dashVar: '--wx-dash-warning',
		// Filled incident triangle; WxTierGlyph draws the "!" as a second path in --color-bg.
		glyph: 'M8 1.5l6.5 13H1.5L8 1.5z',
		glyphFill: true,
		ariaWord: 'Warning'
	},
	watch: {
		label: 'Watch',
		colorVar: '--color-wx-watch',
		softVar: '--color-wx-watch-soft',
		dashVar: '--wx-dash-watch',
		// Open diamond with a centre dot.
		glyph: 'M8 1.5L14.5 8 8 14.5 1.5 8 8 1.5zM8 8h.01',
		glyphFill: false,
		ariaWord: 'Watch'
	},
	advisory: {
		label: 'Advisory',
		colorVar: '--color-wx-advisory',
		softVar: '--color-wx-advisory-soft',
		dashVar: '--wx-dash-advisory',
		// Open circle with an "i".
		glyph: 'M8 2a6 6 0 100 12A6 6 0 008 2zM8 7v4M8 5h.01',
		glyphFill: false,
		ariaWord: 'Advisory'
	},
	statement: {
		label: 'Statement',
		colorVar: '--color-wx-statement',
		softVar: '--color-wx-statement-soft',
		dashVar: '--wx-dash-statement',
		// Note/rectangle glyph.
		glyph: 'M3 3h10v10H3zM5.5 6.5h5M5.5 9.5h5',
		glyphFill: false,
		ariaWord: 'Statement'
	}
};

export interface SeverityStyle {
	weight: number;
	fillOpacity: number;
	hatch: boolean;
	fontWeight: 500 | 600 | 700;
	uppercase: boolean;
}

/** Map-stroke-weight/fill and row-typography styling driven by severity, tier-gated (§3.2 of the UX doc). */
export function severityStyle(tier: WxTier, s: WxSeverity): SeverityStyle {
	const base: SeverityStyle = (() => {
		switch (s) {
			case 'Extreme':
				return { weight: 3, fillOpacity: 0.14, hatch: tier === 'warning', fontWeight: 700, uppercase: true };
			case 'Severe':
				return { weight: 2.5, fillOpacity: 0.12, hatch: tier === 'warning', fontWeight: 700, uppercase: false };
			case 'Moderate':
				return { weight: 2, fillOpacity: 0.08, hatch: false, fontWeight: 600, uppercase: false };
			default: // Minor | Unknown
				return { weight: 1.5, fillOpacity: 0.05, hatch: false, fontWeight: 500, uppercase: false };
		}
	})();
	// Advisory/statement polygons are stroke-only on the map — never filled.
	if (tier === 'advisory' || tier === 'statement') return { ...base, fillOpacity: 0 };
	return base;
}

/** Keep identical to internal/wxalert/tier.go FloorText(). Asserted equal in wxAlertMeta.test.ts. */
export const FLOOR_TEXT =
	'An Extreme-severity, Immediate-urgency alert inside the watch area always shows at least a toast. Nothing on this page can turn that off.';

/** Group in -> near -> ended; within a group TierRank desc, SeverityRank desc, endsAt asc, sent desc (BUILD-PLAN §4.5). */
function alertCompare(a: WxAlert, b: WxAlert): number {
	const group = (x: WxAlert): number => (x.state !== 'active' ? 2 : x.proximity === 'in' ? 0 : 1);
	const g = group(a) - group(b);
	if (g !== 0) return g;
	const tr = tierRank(b.tier) - tierRank(a.tier);
	if (tr !== 0) return tr;
	const sr = severityRank(b.severity) - severityRank(a.severity);
	if (sr !== 0) return sr;
	const ends = Date.parse(a.endsAt) - Date.parse(b.endsAt);
	if (ends !== 0) return ends;
	return Date.parse(b.sent) - Date.parse(a.sent);
}

/** Sort alerts the way the server already sorts a snapshot — used to re-sort after a client-side filter, never to re-derive order from scratch. */
export function sortAlerts(alerts: WxAlert[]): WxAlert[] {
	return [...alerts].sort(alertCompare);
}

/** First value of a CAP parameter, or undefined. `parameters` is never nil but a given key may be absent. */
export function param(a: WxAlert, key: string): string | undefined {
	const v = a.parameters[key];
	return v && v.length > 0 ? v[0] : undefined;
}

/** 'Kent County' | 'Kent County +2' — from the alert's resolved zone refs. '' when there are none (polygon alert). */
export function zoneLabel(a: WxAlert): string {
	if (!a.zones.length) return '';
	const first = a.zones[0].name || a.zones[0].ugc;
	return a.zones.length > 1 ? `${first} +${a.zones.length - 1}` : first;
}

/**
 * 'Adams, IN; Allen, IN; Van Wert, OH' -> 'Adams, Allen IN · Van Wert OH'.
 * Groups consecutive "Name, ST" segments of `areaDesc` that share a state so
 * the two-letter code isn't repeated once per county/zone; segments with no
 * trailing state code (multi-zone or marine areaDesc strings, e.g. 'Dubuque;
 * Jackson; Jo Daviess') pass through as their own groups, unmerged. '' in,
 * '' out — the row falls back to `affects.summary`/'in watch area' itself.
 * Used by WxAlertRow's second line (P1-1) so three simultaneous alerts of
 * the same event are distinguishable by area, not just by event name.
 */
export function compressAreaDesc(areaDesc: string): string {
	if (!areaDesc) return '';
	const segments = areaDesc
		.split(';')
		.map((s) => s.trim())
		.filter(Boolean);
	if (segments.length === 0) return '';

	const groups: string[] = [];
	let pendingNames: string[] = [];
	let pendingState = '';

	const flush = (): void => {
		if (pendingNames.length === 0) return;
		groups.push(pendingState ? `${pendingNames.join(', ')} ${pendingState}` : pendingNames.join(', '));
		pendingNames = [];
		pendingState = '';
	};

	for (const seg of segments) {
		const m = /^(.*),\s*([A-Z]{2})$/.exec(seg);
		const name = m ? m[1].trim() : seg;
		const state = m ? m[2] : '';
		if (state && state === pendingState) {
			pendingNames.push(name);
			continue;
		}
		flush();
		pendingNames = [name];
		pendingState = state;
	}
	flush();

	return groups.join(' · ');
}

const COMPASS_8 = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'];

/** 'N' | 'NE' | … | '' when `deg` is absent. */
export function compass(deg?: number): string {
	if (deg === undefined || deg === null || !Number.isFinite(deg)) return '';
	const idx = Math.round((((deg % 360) + 360) % 360) / 45) % 8;
	return COMPASS_8[idx];
}

/** The toast/list line for an alert — one string reused by showToast() and the row's line-2 fallback. */
export function toastLine(a: WxAlert): string {
	if (a.state !== 'active') {
		return `${a.event} ${a.endedReason === 'cancelled' ? 'cancelled' : 'expired'} · NWS`;
	}
	if (a.proximity === 'near') {
		return `${a.shortCode} · ${Math.round(a.distanceMiles)} mi ${compass(a.bearingDeg)} of course · until ${clock(a.endsAt)} · NWS`;
	}
	return `${a.shortCode} · ${a.affects.summary || 'in your watch area'} · until ${clock(a.endsAt)} · NWS`;
}

function hhmm(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return `${String(d.getHours()).padStart(2, '0')}${String(d.getMinutes()).padStart(2, '0')}`;
}

function zoneAbbrev(a: WxAlert): string {
	if (!a.zones.length) return '';
	const name = (a.zones[0].name || a.zones[0].ugc)
		.replace(/\s+County$/i, ' CO')
		.replace(/\s+Parish$/i, ' PARISH')
		.toUpperCase()
		.trim();
	return a.zones.length > 1 ? `${name} +${a.zones.length - 1}` : name;
}

function responseWord(a: WxAlert): string {
	switch (a.response) {
		case 'Shelter':
			return 'SEEK SHELTER';
		case 'Evacuate':
			return 'EVACUATE';
		case 'Avoid':
			return 'AVOID AREA';
		default:
			return '';
	}
}

const BULLETIN_MAX = 67;

/**
 * APRS-bulletin-length summary for the relay composer, e.g.
 * 'TOR WARN KENT CO UNTIL 1545 SEEK SHELTER -W8ABC'. Always <= 67 chars: a
 * many-zone alert already collapses to "+N" (see zoneAbbrev) so it rarely
 * needs the fallback truncation below, but a long zone name or NCS callsign
 * still can — the code, the deadline and the "-callsign" suffix are kept
 * intact and the zone text is shortened first, then the response word is
 * dropped, then (last resort) the whole line is hard-truncated.
 */
export function bulletinSummary(a: WxAlert, ncs: string): string {
	const zone = zoneAbbrev(a);
	const until = `UNTIL ${hhmm(a.endsAt)}`;
	const resp = responseWord(a);
	const tail = ` -${ncs}`;

	let line = [a.shortCode, zone, until, resp].filter(Boolean).join(' ') + tail;
	if (line.length <= BULLETIN_MAX) return line;

	line = [a.shortCode, zone, until].filter(Boolean).join(' ') + tail;
	if (line.length <= BULLETIN_MAX) return line;

	const fixed = [a.shortCode, until].filter(Boolean).join(' ');
	const budget = BULLETIN_MAX - fixed.length - tail.length - (zone ? 1 : 0);
	const shortZone = budget > 0 ? zone.slice(0, budget) : '';
	line = [a.shortCode, shortZone, until].filter(Boolean).join(' ') + tail;
	return line.length <= BULLETIN_MAX ? line : line.slice(0, BULLETIN_MAX);
}

function minutesPhrase(ms: number): string {
	const totalMin = Math.max(0, Math.round(ms / 60000));
	if (totalMin < 60) return `${totalMin} minute${totalMin === 1 ? '' : 's'}`;
	const h = Math.floor(totalMin / 60);
	if (h < 24) {
		const m = totalMin % 60;
		return m > 0 ? `${h} hour${h === 1 ? '' : 's'} ${m} minute${m === 1 ? '' : 's'}` : `${h} hour${h === 1 ? '' : 's'}`;
	}
	const d = Math.floor(h / 24);
	return `${d} day${d === 1 ? '' : 's'}`;
}

/**
 * The one string shown AND announced for an interrupt-class alert
 * (WxInterruptBanner's `aria-describedby` body and `role="alertdialog"`
 * content are the same text — screen readers hear exactly what's on screen).
 * `now` defaults to `Date.now()` for callers, but is a parameter so the
 * sentence is exactly reproducible in tests.
 */
export function announceSentence(a: WxAlert, fetchedClock: string, now: number = Date.now()): string {
	const msLeft = Date.parse(a.endsAt) - now;
	const phrase = msLeft > 0 ? `${minutesPhrase(msLeft)} from now` : 'now';
	return `${a.event} in your watch area, affects ${a.affects.summary || 'the watch area'}, ends ${clock(a.endsAt)}, ${phrase}. Source ${a.senderName}, fetched ${fetchedClock}.`;
}
