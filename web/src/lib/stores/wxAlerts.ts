// NWS Alerts (internal/wxalert) client state. See BUILD-PLAN.md §3.1 (spec-frontend.md
// §3.1 as amended by BUILD-PLAN.md §1) for the full design; this file follows the
// canonical wire contract in BUILD-PLAN.md §2, not spec-frontend.md's own §1
// (superseded field names).
import { writable, derived, get } from 'svelte/store';
import { browser } from '$app/environment';
import { api } from '$lib/api';
import { wsClient } from './stations';
import { activeNet, checkIns } from './netcontrol';
import type { AttentionItem } from './netcontrol';
import { currentUser, canAdmin } from './session';
import { showToast, announce } from './toast';
import { openWeather, wxPanelTab, panelMode } from './ui';
import type { WxAlert, WxLinkStatus, WxFootprintSummary, WxEffectivePolicy, WxPolygonGeometry, WxNetAck } from '$lib/types';
import { tierRank, toastLine, sortAlerts } from '$lib/wxAlertMeta';
import { clock } from '$lib/wxAlertTime';

type WxApplyReason = 'poll' | 'footprint' | 'settings' | 'netwatch' | 'restore';

const OFF_STATUS: WxLinkStatus = {
	state: 'off',
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

// ---- localStorage (per-device conveniences, never the audit trail — that's the server activity log) ----

const ACKED_KEY = 'nymeria_wx_acked';
const MUTED_KEY = 'nymeria_wx_muted';

function loadAcked(): Map<string, string> {
	if (!browser) return new Map();
	try {
		const raw = localStorage.getItem(ACKED_KEY);
		if (!raw) return new Map();
		return new Map(JSON.parse(raw) as [string, string][]);
	} catch {
		return new Map();
	}
}

function loadMuted(): Set<string> {
	if (!browser) return new Set();
	try {
		const raw = localStorage.getItem(MUTED_KEY);
		if (!raw) return new Set();
		return new Set(JSON.parse(raw) as string[]);
	} catch {
		return new Set();
	}
}

// ---- state ----

export const wxAlertsById = writable<Map<string, WxAlert>>(new Map());
export const wxLinkStatus = writable<WxLinkStatus>(OFF_STATUS);
/** Snapshot-level footprint summary (net footprint, resolved zones, centroid). */
export const wxFootprint = writable<WxFootprintSummary | null>(null);
/** Effective notification policy for the current net (floorText, interrupt allowlist, …). */
export const wxPolicy = writable<WxEffectivePolicy | null>(null);
/** Cached zone polygons by UGC, filled lazily via ensureZoneGeometry(). */
export const wxZoneGeometry = writable<Map<string, WxPolygonGeometry>>(new Map());
/** alertId -> ISO ack time (localStorage 'nymeria_wx_acked'). */
export const wxAcked = writable<Map<string, string>>(loadAcked());
/** alertId (localStorage 'nymeria_wx_muted'). */
export const wxMuted = writable<Set<string>>(loadMuted());
/** Interrupt-class alert ids, FIFO — the banner shows the head, "+N more" for the rest. */
export const wxInterruptQueue = writable<string[]>([]);
/** Detail view target inside the NWS Alerts tab. */
export const wxSelectedAlertId = writable<string | null>(null);
/** Not persisted — resets each session. */
export const wxFilter = writable<'all' | 'warning' | 'watch' | 'advisory'>('all');
/** Not persisted. */
export const wxUnackedOnly = writable<boolean>(false);
/** Footprint outline shown for ~10 s after "Show on map" in the watch-area sheet. */
export const wxFootprintPreview = writable<WxPolygonGeometry | null>(null);
/** "Show on map" target — Map.svelte fits bounds then calls back to clear this. */
export const wxFocusAlertId = writable<string | null>(null);
/** 1 s tick, started by initWxAlertStore(). */
export const wxClock = writable<number>(Date.now());
/** Rows only need to re-render once a minute except under the 5-minute countdown threshold. */
export const wxMinute = derived(wxClock, ($t) => Math.floor($t / 60000));

if (browser) {
	wxAcked.subscribe((m) => {
		try {
			localStorage.setItem(ACKED_KEY, JSON.stringify(Array.from(m.entries())));
		} catch {
			// localStorage may be unavailable (SSR, privacy mode) — ack still works for the session.
		}
	});
	wxMuted.subscribe((s) => {
		try {
			localStorage.setItem(MUTED_KEY, JSON.stringify(Array.from(s)));
		} catch {
			// Same as above.
		}
	});
}

/**
 * Closing the side panel / bottom sheet (any panelMode -> 'closed', which
 * only ever happens via ui.ts's closePanel()) must drop any selected alert —
 * otherwise reopening the Weather panel resurrects the detail view with no
 * obvious way back to the list (P0-3). This is a one-way subscription
 * (wxAlerts.ts -> ui.ts): ui.ts must never import back from this file, or
 * the two stores become a circular ES module pair — see
 * wxWeatherAttentionItems' comment above for why that's a real TDZ risk.
 */
panelMode.subscribe((mode) => {
	if (mode === 'closed') wxSelectedAlertId.set(null);
});

// ---- derived ----

export const wxAlertList = derived(wxAlertsById, ($m) => sortAlerts(Array.from($m.values())));
export const wxInAreaAlerts = derived(wxAlertList, (l) => l.filter((a) => a.state === 'active' && a.proximity === 'in'));
export const wxNearbyAlerts = derived(wxAlertList, (l) => l.filter((a) => a.state === 'active' && a.proximity === 'near'));
export const wxExpiredAlerts = derived([wxAlertList, wxMinute], ([l, m]) =>
	l.filter((a) => a.state !== 'active' && a.endedAt && m * 60000 - Date.parse(a.endedAt) < 60 * 60000)
);
export const wxIsAcked = derived(wxAcked, ($acked) => (a: WxAlert) => $acked.has(a.id) || !!a.ackedForNet);
/** Watch tier and up, unacknowledged, IN the watch area — advisories/statements and net-acked alerts never count. */
export const wxUnackedCount = derived([wxInAreaAlerts, wxIsAcked], ([l, isAcked]) =>
	l.filter((a) => tierRank(a.tier) >= tierRank('watch') && !isAcked(a)).length
);
export const wxUnackedWarningsIn = derived([wxInAreaAlerts, wxIsAcked], ([l, isAcked]) => l.filter((a) => a.tier === 'warning' && !isAcked(a)));
/** Highest-sorted (list is already tier/severity sorted) active warning-tier IN alert, or null. */
export const wxActiveWarningIn = derived(wxInAreaAlerts, (l) => l.find((a) => a.tier === 'warning') ?? null);
export const wxActiveInterrupt = derived([wxInterruptQueue, wxAlertsById], ([q, m]) => (q.length ? (m.get(q[0]) ?? null) : null));
export const wxInterruptMore = derived(wxInterruptQueue, (q) => Math.max(0, q.length - 1));
export const wxIsNcs = derived([activeNet, currentUser, canAdmin], ([n, u, admin]) => admin || (!!n && !!u && n.ncsUserId === u.id));
/** Roster check-in ids currently inside an active warning-tier IN alert (roster stripe/glyph, attention list). */
export const weatherCheckInIds = derived(wxInAreaAlerts, (l) => {
	const set = new Set<string>();
	for (const a of l) {
		if (a.tier !== 'warning') continue;
		for (const s of a.affects.stations) if (s.checkInId) set.add(s.checkInId);
	}
	return set;
});
/** Active IN alerts (any tier) whose affects.stations names this callsign — WeatherStationCard/WeatherDetail's tier chip. */
export const wxAlertsForStation = derived(wxInAreaAlerts, (l) => (callsign: string) =>
	l.filter((a) => a.affects.stations.some((s) => s.kind === 'station' && s.id === callsign))
);

/**
 * "Needs Attention" rows for roster stations inside an active warning-tier IN
 * alert. Lives here (not stores/netcontrol.ts) deliberately: this store
 * already imports `checkIns`/`activeNet` from netcontrol.ts one-way, and
 * netcontrol.ts importing anything back from here would make the two stores
 * a circular ES module pair — a real TDZ crash risk, not just a lint nit.
 * SituationBoard.svelte merges this with netcontrol's own `attentionItems`
 * using `ATTENTION_REASON_ORDER`.
 */
export const wxWeatherAttentionItems = derived([checkIns, wxInAreaAlerts], ([$cis, $alerts]) => {
	const items: AttentionItem[] = [];
	for (const ci of $cis) {
		if (ci.status === 'released' || ci.status === 'missing' || ci.traffic === 'emergency') continue;
		const alert = $alerts.find(
			(a) => a.tier === 'warning' && a.affects.stations.some((s) => s.kind === 'station' && s.checkInId === ci.id)
		);
		if (!alert) continue;
		items.push({ checkIn: ci, reason: 'weather', detail: `Inside ${alert.event}`, action: 'Contact' });
	}
	return items;
});

function isAckedNow(a: WxAlert): boolean {
	return get(wxAcked).has(a.id) || !!a.ackedForNet;
}

function isMutedNow(id: string): boolean {
	return get(wxMuted).has(id);
}

function pruneAckedAndMuted(next: Map<string, WxAlert>): void {
	const cutoff = Date.now() - 24 * 60 * 60 * 1000;
	wxAcked.update((m) => {
		let changed = false;
		const copy = new Map(m);
		for (const [id, iso] of copy) {
			if (next.has(id)) continue;
			if (Date.parse(iso) < cutoff) {
				copy.delete(id);
				changed = true;
			}
		}
		return changed ? copy : m;
	});
	wxMuted.update((s) => {
		// Mute carries no timestamp; an alert missing from the snapshot entirely
		// (purged/superseded server-side) no longer needs its mute remembered.
		let changed = false;
		const copy = new Set(s);
		for (const id of copy) {
			if (next.has(id)) continue;
			copy.delete(id);
			changed = true;
		}
		return changed ? copy : s;
	});
}

function enqueueInterrupt(id: string): void {
	wxInterruptQueue.update((q) => (q.includes(id) ? q : [...q, id]));
}

function showWxToast(a: WxAlert): void {
	showToast(toastLine(a), 'wx', 8000, { label: 'View', run: () => openWxAlert(a.id) }, a.tier);
}

// Dedupe key per alert id — a version already rendered/toasted is never
// re-processed just because it arrived again in the same or a later snapshot.
const seenVersions = new Map<string, string>();

/**
 * The notification engine: dedupes by `updatedAt`, applies the restore rule
 * (never toast history except an unacked interrupt re-arming), the mute rule
 * (suppresses non-escalated updates only), and coalesces a multi-alert batch
 * into one toast. `notifyClass`/`notifyReason` are server-computed — this
 * function does not re-derive them.
 */
export function applyAlerts(
	alerts: WxAlert[] | null | undefined,
	status: WxLinkStatus,
	footprint: WxFootprintSummary | null,
	policy: WxEffectivePolicy,
	reason: WxApplyReason
): void {
	const list = alerts ?? [];
	const next = new Map(list.map((a) => [a.id, a] as const));

	wxAlertsById.set(next);
	wxLinkStatus.set(status);
	wxFootprint.set(footprint);
	wxPolicy.set(policy);

	pruneAckedAndMuted(next);

	wxInterruptQueue.update((q) => q.filter((id) => {
		const a = next.get(id);
		return !!a && a.state === 'active' && !isAckedNow(a);
	}));

	const toastBatch: WxAlert[] = [];
	for (const a of list) {
		if (seenVersions.get(a.id) === a.updatedAt) continue;
		seenVersions.set(a.id, a.updatedAt);

		if (reason === 'restore') {
			// Startup/reconnect: never toast history. Safety exception: an
			// active, IN, interrupt-class alert nobody acknowledged re-arms
			// the banner so a missed interrupt is never silently dropped.
			if (a.state === 'active' && a.notifyClass === 'interrupt' && !isAckedNow(a) && !isMutedNow(a.id)) {
				enqueueInterrupt(a.id);
			}
			continue;
		}
		if (a.notifyReason === 'none') continue;
		if (a.notifyReason === 'ended') {
			if (a.proximity === 'in' && tierRank(a.tier) >= tierRank('watch')) toastBatch.push(a);
			continue;
		}
		// Mute is "no more of the same", never "never again" — an escalation still notifies.
		if (isMutedNow(a.id) && a.notifyReason !== 'escalated') continue;

		switch (a.notifyClass) {
			case 'interrupt':
				enqueueInterrupt(a.id);
				break;
			case 'toast':
				toastBatch.push(a);
				break;
			default:
				// badge/panel — the derived counts pick this up, nothing to show now.
				break;
		}
	}

	if (toastBatch.length === 1) {
		showWxToast(toastBatch[0]);
	} else if (toastBatch.length > 1) {
		showToast(`${toastBatch.length} new NWS alerts in your area`, 'wx', 8000, { label: 'View', run: () => openWeather('alerts') }, 'watch');
	}
}

/**
 * Dispatches one WS message body to the store. Exported so tests can drive
 * the store deterministically without a real `wsClient` (see
 * stores/wxAlerts.test.ts) — `initWxAlertStore()` is the only other caller.
 */
export function handleWxMessage(type: string, data: unknown): void {
	switch (type) {
		case 'wx_alerts': {
			const d = data as {
				alerts: WxAlert[] | null;
				status: WxLinkStatus;
				footprint: WxFootprintSummary | null;
				policy: WxEffectivePolicy;
				reason?: string;
			} | null;
			if (!d) return;
			applyAlerts(d.alerts, d.status, d.footprint, d.policy, (d.reason as WxApplyReason) ?? 'poll');
			break;
		}
		case 'wx_link_status': {
			const status = data as WxLinkStatus | null;
			if (!status) return;
			const prev = get(wxLinkStatus);
			wxLinkStatus.set(status);
			if (prev.state !== 'down' && status.state === 'down') {
				announce(`NWS alerts link down since ${clock(status.lastSuccessAt ?? status.lastAttemptAt ?? null)}`);
			} else if (prev.state === 'down' && status.state !== 'down') {
				announce('NWS alerts link restored');
			}
			break;
		}
		case 'wx_alert_ack_net': {
			const d = data as { alertId: string; netId: string; ackedForNet: WxNetAck } | null;
			if (!d?.alertId) return;
			wxAlertsById.update((m) => {
				const a = m.get(d.alertId);
				if (!a) return m;
				const copy = new Map(m);
				copy.set(d.alertId, { ...a, ackedForNet: d.ackedForNet });
				return copy;
			});
			// Decision 5: NCS ack-for-net clears the banner for everyone, on every device.
			wxInterruptQueue.update((q) => q.filter((id) => id !== d.alertId));
			break;
		}
		default:
			break;
	}
}

let initialized = false;

/** Idempotent — call once from the approved-session init effect, after initWeatherStore(). */
export function initWxAlertStore(): void {
	if (initialized) return;
	initialized = true;

	api
		.wxAlerts()
		.then((snap) => applyAlerts(snap.alerts, snap.status, snap.footprint, snap.policy, 'restore'))
		.catch(() => {
			// Never overwrite a good status with a transient HTTP blip.
			// A disabled feature no longer lands here — the read endpoints
			// answer 200 with an 'off' status — so reaching this genuinely
			// means the Nymeria server could not be reached.
			if (get(wxLinkStatus) === OFF_STATUS) {
				wxLinkStatus.set({ ...OFF_STATUS, state: 'down', lastError: 'API unreachable' });
			}
		});

	wsClient.on('wx_alerts', (msg) => handleWxMessage('wx_alerts', msg.data));
	wsClient.on('wx_link_status', (msg) => handleWxMessage('wx_link_status', msg.data));
	wsClient.on('wx_alert_ack_net', (msg) => handleWxMessage('wx_alert_ack_net', msg.data));

	if (browser) setInterval(() => wxClock.set(Date.now()), 1000);
}

// ---- actions ----

/** Local acknowledgement: dequeues the banner, clears the row's NEW dot, logged server-side. */
export async function ackAlert(id: string): Promise<void> {
	wxAcked.update((m) => {
		const copy = new Map(m);
		copy.set(id, new Date().toISOString());
		return copy;
	});
	wxInterruptQueue.update((q) => q.filter((qid) => qid !== id));
	try {
		await api.ackWxAlert(id);
	} catch {
		// The local ack already happened; the server call only adds the audit trail.
	}
}

/** Acknowledges every currently-unacked IN alert (the panel's "Acknowledge all" affordance). */
export async function ackAllInArea(): Promise<void> {
	const isAcked = get(wxIsAcked);
	const targets = get(wxInAreaAlerts).filter((a) => !isAcked(a));
	await Promise.all(targets.map((a) => ackAlert(a.id)));
}

/** NCS/admin only (server-enforced) — clears the banner for everyone (decision 5); other devices update via the wx_alert_ack_net WS echo. */
export async function ackAlertForNet(id: string): Promise<void> {
	const updated = await api.ackWxAlertForNet(id);
	wxAlertsById.update((m) => {
		const copy = new Map(m);
		copy.set(id, updated);
		return copy;
	});
	wxInterruptQueue.update((q) => q.filter((qid) => qid !== id));
}

/** Refused (no-op) for an interrupt-class alert — an alert loud enough to trap focus cannot be silenced by mistake. */
export function muteAlert(id: string): void {
	const a = get(wxAlertsById).get(id);
	if (a?.notifyClass === 'interrupt') return;
	wxMuted.update((s) => new Set(s).add(id));
}

export function unmuteAlert(id: string): void {
	wxMuted.update((s) => {
		const copy = new Set(s);
		copy.delete(id);
		return copy;
	});
}

/** Opens the detail view for `id` inside the NWS Alerts tab of the Weather panel. */
export function openWxAlert(id: string): void {
	wxSelectedAlertId.set(id);
	wxPanelTab.set('alerts');
	openWeather('alerts');
}

/** Map.svelte fits bounds to the alert's geometry, then calls back to clear this. */
export function showAlertOnMap(id: string): void {
	wxFocusAlertId.set(id);
}

/** Fetches any of `ugcs` not already cached client-side. Failures are silent — the own-geometry fallback renders instead. */
export async function ensureZoneGeometry(ugcs: string[]): Promise<void> {
	const have = get(wxZoneGeometry);
	const missing = ugcs.filter((u) => !have.has(u));
	await Promise.all(
		missing.map(async (ugc) => {
			try {
				const zone = await api.wxZone(ugc);
				wxZoneGeometry.update((m) => {
					const copy = new Map(m);
					copy.set(ugc, zone.geometry);
					return copy;
				});
			} catch {
				// Silent — the map falls back to the own-geometry highlight for this zone.
			}
		})
	);
}

let footprintPreviewTimer: ReturnType<typeof setTimeout> | null = null;

/** Shows the net's watch-area outline on the map for ~10 s (the watch-area sheet's "Show on map" button). */
export async function showFootprintPreview(): Promise<void> {
	try {
		const { outline } = await api.wxFootprint();
		wxFootprintPreview.set(outline);
		if (footprintPreviewTimer) clearTimeout(footprintPreviewTimer);
		footprintPreviewTimer = setTimeout(() => wxFootprintPreview.set(null), 10_000);
	} catch {
		// Nothing to preview — leave the map as it was.
	}
}
