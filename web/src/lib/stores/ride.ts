// Bike-ride mode client state (WP6, docs/ride-strip-spec.md). One net at a
// time — every writable here is reset/reloaded on activeNetId identity
// change, exactly like netcontrol.ts's `lastNetId` subscription.
//
// Governing design facts this file must never violate:
//   - Rider accounting is EDGE-based. No bib ever appears in a zone
//     descriptor — bibs live in the SAG request record and the exception log.
//   - The priority ladder is agency-configurable; always read it from
//     rideLadder (net's effective ladder), never hardcode a tier name.
//   - Medical data may already be redacted server-side; this file never
//     assumes PatientName/Bib survive that redaction.
import { writable, derived, get } from 'svelte/store';
import { browser } from '$app/environment';
import { api, ApiError } from '$lib/api';
import { wsClient } from './stations';
import {
	activeNet, activeNetId, activeCheckIns, netMetrics,
	orderedCheckpoints, progressElements, hasCheckpoints, netAnnotations
} from './netcontrol';
import { wxInAreaAlerts } from './wxAlerts';
import { weatherStations, weatherUnits } from './weather';
import { rosterCallsigns } from './rosterScope';
import { openNetControl, openWeather, openAnnotations, commandPaletteOpen, netControlRequestedTab, courseRequestedTab } from './ui';
import { paletteSeed } from './commandpalette';
import { showToast } from './toast';
import { secondClock, minuteClock, startClock } from './clock';
import { rideInterruptQueue } from './interrupts';
import { currentUser } from './session';
import type {
	NetProfileView, RidePhaseStatus, RidePhaseID, CourseState, SAGBoard, MedicalNotification,
	SupplyRequest, HandoffItem, ShiftBriefing, CloseoutStatus, Accounting, PriorityTier,
	ShutoffPoint, StationClosureState, SAGLocation
} from '$lib/types';
import {
	tierById, ageState, ageText, elapsedHM, heatIndexF, tierStyle,
	STOP_GLYPHS, PENDING_GLYPH, SAG_REASON_LABELS, enumLabel,
	unassignedRiders
} from '$lib/rideMeta';
import { countdown, clock } from '$lib/wxAlertTime';
import { convertTemp, formatTempShort } from '$lib/units';
import { buildRouteIndex, parseLineString, projectStops, type RouteIndex } from '$lib/routeDistance';

const ACKED_KEY = 'nymeria_ride_acked';

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

// ---- primary state ----

export const rideProfile = writable<NetProfileView | null>(null);
export const ridePhase = writable<RidePhaseStatus | null>(null);
export const courseState = writable<CourseState | null>(null);
export const sagBoard = writable<SAGBoard | null>(null);
export const medicalOpen = writable<MedicalNotification[]>([]);
export const supplyAll = writable<SupplyRequest[]>([]);
export const handoffOpen = writable<HandoffItem[]>([]);
export const briefing = writable<ShiftBriefing | null>(null);
export const closeout = writable<CloseoutStatus | null>(null);
export const accounting = writable<Accounting | null>(null);
export const rideAcked = writable<Map<string, string>>(loadAcked());
export const rideAnnouncement = writable<string>('');

/**
 * The strip's width class (spec §9): `desktop` is >=1200w (the 128px, 8-zone
 * template); `tablet` is 769-1199w (the 140px, 6-zone template achieved by
 * MERGING zones, not by clipping columns). RideStrip.svelte is the only
 * writer — it already runs the `min-width: 1200px` matchMedia query for the
 * strip's height contract, this just publishes that same boolean so the zone
 * SET (computed in this file) can be a function of it too, instead of the
 * zone list being fixed and only the CSS reacting to width.
 */
export type RideViewportClass = 'desktop' | 'tablet';
export const rideViewportClass = writable<RideViewportClass>('desktop');

/**
 * One-shot handoff from the command palette's `s`/"sag " accelerator (see
 * commandParser.ts + NetControlPanel.svelte's `case 'sag':`) to
 * SagBoard.svelte: a SAG request is a multi-field record the palette cannot
 * commit from one typed line, so it hands off to the real composer instead,
 * prefilled with whatever free text followed "sag ". SagBoard consumes and
 * resets this to null, mirroring stores/ui.ts's netControlRequestedTab.
 */
export const sagComposerSeed = writable<{ reason?: string } | null>(null);

if (browser) {
	rideAcked.subscribe((m) => {
		try {
			localStorage.setItem(ACKED_KEY, JSON.stringify(Array.from(m.entries())));
		} catch {
			// per-device convenience only
		}
	});
}

// ---- announcement batching: <= 1 per 60s, tier escalations concatenate ----

let announceQueue: string[] = [];
let announceTimer: ReturnType<typeof setTimeout> | null = null;

function scheduleAnnounceCooldown() {
	announceTimer = setTimeout(() => {
		announceTimer = null;
		if (announceQueue.length > 0) flushAnnouncement();
	}, 60_000);
}

function flushAnnouncement() {
	if (announceQueue.length === 0) return;
	rideAnnouncement.set(announceQueue.join(' — '));
	announceQueue = [];
	scheduleAnnounceCooldown();
}

function queueAnnouncement(text: string, escalation = false) {
	if (announceTimer) {
		// Within the 60s cooldown: the latest message replaces earlier
		// unflushed ones, except a tier escalation, which concatenates.
		announceQueue = escalation ? [...announceQueue, text] : [text];
		return;
	}
	announceQueue = [text];
	flushAnnouncement();
}

// ---- derived: identity ----

export const rideMode = derived(activeNet, (n) => n?.profile === 'bike-ride');

/** Rank 1 = most urgent. NEVER hardcoded — read from the net's effective ladder. */
export const rideLadder = derived(rideProfile, (p) => p?.effectivePriorityTiers ?? []);

export const rideHasCourse = derived([courseState, hasCheckpoints], ([c, has]) => has && (c?.stations.length ?? 0) > 0);

// ---- derived: open ride records (SAG / medical / supply), edge-based, never a bib ----

export interface RideOpenItem {
	id: string;
	kind: 'sag' | 'medical' | 'supply';
	tier: PriorityTier;
	createdAt: string;
	where: string;
	summary: string;
	acked: boolean;
}

function sagLocationText(loc: SAGLocation | undefined, milesRemaining?: number | null): string {
	if (!loc) return '';
	const base = loc.description || loc.route || loc.kind;
	if (milesRemaining != null) return `${base} · mi ${milesRemaining.toFixed(1)} to go`;
	return base;
}

export const rideOpenItems = derived(
	[sagBoard, medicalOpen, supplyAll, rideLadder, rideAcked],
	([$sag, $med, $sup, $ladder, $acked]): RideOpenItem[] => {
		if ($ladder.length === 0) return [];
		const fallbackTier = $ladder[$ladder.length - 1];
		const resolve = (id: string): PriorityTier => tierById($ladder, id) ?? fallbackTier;
		const items: RideOpenItem[] = [];

		for (const r of $sag?.requests ?? []) {
			if (r.status === 'complete' || r.status === 'cancelled') continue;
			items.push({
				id: r.id,
				kind: 'sag',
				tier: resolve(r.priority),
				createdAt: r.createdAt,
				where: sagLocationText(r.pickup, r.pickup.milesRemaining),
				summary: `SAG ${enumLabel(r.reason, SAG_REASON_LABELS) || 'requested'}`.trim(),
				acked: $acked.has(r.id)
			});
		}
		for (const m of $med) {
			if (m.status === 'departed' || m.status === 'released' || m.status === 'cancelled') continue;
			items.push({
				id: m.id,
				kind: 'medical',
				tier: resolve(m.priority),
				createdAt: m.createdAt,
				where: m.milesRemaining != null ? `${m.location} · mi ${m.milesRemaining.toFixed(1)} to go` : m.location,
				summary: m.chiefComplaint || 'Medical',
				acked: $acked.has(m.id)
			});
		}
		for (const s of $sup) {
			if (s.status === 'delivered' || s.status === 'cancelled' || s.status === 'merged') continue;
			items.push({
				id: s.id,
				kind: 'supply',
				tier: resolve(s.priority),
				createdAt: s.createdAt,
				where: s.milesRemaining != null ? `${s.location} · mi ${s.milesRemaining.toFixed(1)} to go` : s.location,
				summary: s.items.map((i) => i.item).join(', ') || 'Supply request',
				acked: $acked.has(s.id)
			});
		}
		return items;
	}
);

export const rideTierCounts = derived([rideOpenItems, rideLadder], ([items, ladder]) => {
	const byTier = new Map<string, { tier: PriorityTier; count: number; oldestAt: string }>();
	for (const it of items) {
		const cur = byTier.get(it.tier.id);
		if (!cur) {
			byTier.set(it.tier.id, { tier: it.tier, count: 1, oldestAt: it.createdAt });
		} else {
			cur.count++;
			if (Date.parse(it.createdAt) < Date.parse(cur.oldestAt)) cur.oldestAt = it.createdAt;
		}
	}
	return Array.from(byTier.values()).sort((a, b) => a.tier.rank - b.tier.rank);
});

export const rideTopTier = derived(rideTierCounts, (c) => c[0]?.tier ?? null);

/** The strip's EMERGENCY latch: the oldest rank-1 item not yet acked. Downgrades to rank-2 styling once acked (it simply drops out of this derived), but the record itself stays open until it goes terminal server-side. */
export const rideEmergency = derived(rideOpenItems, (items) => {
	const unacked = items.filter((i) => i.tier.rank === 1 && !i.acked);
	if (unacked.length === 0) return null;
	return unacked.reduce((a, b) => (Date.parse(a.createdAt) <= Date.parse(b.createdAt) ? a : b));
});

export const rideActiveInterrupt = derived([rideOpenItems, rideInterruptQueue], ([items, q]) => {
	if (q.length === 0) return null;
	return items.find((i) => i.id === q[0]) ?? null;
});

export const rideUnackedTraffic = derived(activeCheckIns, (cis) => cis.filter((ci) => ci.traffic !== 'none').length);

// ---- derived: pending / awaiting-reply (B8's briefing-adjacent data) ----

const pendingFirstSeen = new Map<string, number>();

export const ridePending = derived([handoffOpen, minuteClock], ([items, _m]) => {
	const now = Date.now();
	let oldestAt: string | null = null;
	let overdue = 0;
	let escalate = false;
	const seenNow = new Set<string>();
	for (const h of items) {
		seenNow.add(h.id);
		if (!oldestAt || Date.parse(h.createdAt) < Date.parse(oldestAt)) oldestAt = h.createdAt;
		if (!h.dueAt) continue;
		const due = Date.parse(h.dueAt);
		if (now < due) continue;
		overdue++;
		const interval = due - Date.parse(h.createdAt);
		const twiceOverdueAt = interval > 0 ? due + interval : due;
		if (now >= twiceOverdueAt) {
			if (!pendingFirstSeen.has(h.id)) pendingFirstSeen.set(h.id, now);
			// 30s stability gate on the auto-derived escalation (spec §4.6).
			if (now - (pendingFirstSeen.get(h.id) ?? now) >= 30_000) escalate = true;
		}
	}
	for (const id of Array.from(pendingFirstSeen.keys())) {
		if (!seenNow.has(id)) pendingFirstSeen.delete(id);
	}
	return { count: items.length, oldestAt, overdue, escalate };
});

// ---- derived: SAG summary ----

/**
 * `open` used to mean "not complete or cancelled", which counted every job a
 * van was already driving and reported it to the strip as work outstanding.
 * The number that changes an NCS decision is how many jobs are BLOCKED ON
 * NCS — riders with no vehicle assigned to them — and `ridersWaiting` is now
 * the same set counted per rider (spec section 2, B5: "how many humans are at
 * the roadside and for how long"). `inMotion` carries the rest, so nothing is
 * hidden; it is just not the headline.
 */
export const rideSagSummary = derived(sagBoard, (board) => {
	const requests = board?.requests ?? [];
	const vehicles = board?.vehicles ?? [];
	const active = requests.filter((r) => r.status !== 'complete' && r.status !== 'cancelled');
	const needing = active.filter((r) => unassignedRiders(r) > 0);
	let ridersWaiting = 0;
	let oldestOpenAt: string | null = null;
	for (const r of needing) {
		ridersWaiting += unassignedRiders(r);
		if (!oldestOpenAt || Date.parse(r.createdAt) < Date.parse(oldestOpenAt)) oldestOpenAt = r.createdAt;
	}
	const open = needing;
	const inMotion = active.length - needing.length;
	const seatsAvail = vehicles.reduce((s, v) => s + v.availableSeats, 0);
	const seats = vehicles.reduce((s, v) => s + v.seats, 0);
	const racksAvail = vehicles.reduce((s, v) => s + v.availableRacks, 0);
	const racks = vehicles.reduce((s, v) => s + v.rackSlots, 0);
	const noUnits = vehicles.filter((v) => v.checkInStatus !== 'released').length === 0;
	return { open: open.length, inMotion, active: active.length, ridersWaiting, oldestOpenAt, seatsAvail, seats, racksAvail, racks, noUnits };
});

// ---- derived: stops / course closure ----

export const rideStops = derived([courseState, orderedCheckpoints], ([c, cps]) => {
	const stations = c?.stations ?? [];
	const total = stations.length;
	let open = 0, closed = 0, awaitingSweep = 0, atCapacity = 0;
	let blockingLabel = '';
	let readyToClose = 0;
	const sorted = [...stations].sort((a, b) => a.sequenceNumber - b.sequenceNumber);
	for (const st of sorted) {
		if (st.closure.state === 'closed') closed++;
		else if (st.closure.state === 'riders_clear') {
			// Riders gone, sweep not yet past: the state the backend's
			// ErrSweepNotPassed gate actually refuses a close on.
			awaitingSweep++;
			if (!blockingLabel) blockingLabel = st.label;
		} else if (st.closure.state === 'sweep_passed') readyToClose++;
		else open++;
		const cp = cps.find((x) => x.meta.annotationId === st.checkpointId);
		if (cp?.annotation.status === 'at-capacity') atCapacity++;
	}
	let waitingOn = '';
	let since: string | undefined;
	for (const st of sorted) {
		if (st.closure.state !== 'closed') {
			waitingOn = st.label;
			since = st.closure.ridersClearAt ?? st.closure.sweepPassedAt;
			break;
		}
	}
	return {
		total, open, closed, awaitingSweep, readyToClose, atCapacity, blockingLabel,
		allClosed: c?.allStationsClosed ?? false,
		clearing: { cleared: c?.stationsClosed ?? 0, total, waitingOn, since }
	};
});

// ---- derived: shutoffs ----

export const rideShutoffs = derived([courseState, secondClock], ([c, now]) => {
	const shutoffs = c?.shutoffs ?? [];
	const next = c?.nextShutoff ?? null;
	const nextTwo = shutoffs
		.filter((s) => s.status === 'planned')
		.sort((a, b) => Date.parse(a.scheduledAt) - Date.parse(b.scheduledAt))
		.slice(0, 2);
	const allFired = shutoffs.length > 0 && shutoffs.every((s) => s.status === 'fired' || s.status === 'cancelled');
	const none = shutoffs.length === 0;
	let cd: string | null = null;
	let tone: 'normal' | 'warning' = 'normal';
	if (next) {
		const msLeft = Date.parse(next.scheduledAt) - now;
		if (msLeft <= 90 * 60_000) cd = countdown(next.scheduledAt, now).text;
		if (msLeft <= 15 * 60_000) tone = 'warning';
	}
	return { next, nextTwo, allFired, none, countdown: cd, tone };
});

// ---- derived: the course rail's view model ----

const routeIndexCache = new Map<string, RouteIndex>();

function computeStopMiles(): Map<string, number> | undefined {
	try {
		const route = get(netAnnotations).find((a) => a.category === 'route');
		if (!route) return undefined;
		const cacheKey = `${route.id}:${route.updatedAt ?? ''}`;
		let idx = routeIndexCache.get(cacheKey);
		if (!idx) {
			const coords = parseLineString(route.geometry);
			if (!coords) return undefined;
			idx = buildRouteIndex(coords);
			routeIndexCache.set(cacheKey, idx);
		}
		const checkpoints = get(orderedCheckpoints);
		const anns = checkpoints.map((cp) => cp.annotation);
		const stops = projectStops(idx, anns);
		const out = new Map<string, number>();
		for (const s of stops) out.set(s.id, s.chainageMeters / 1609.344);
		return out.size > 0 ? out : undefined;
	} catch {
		return undefined;
	}
}

function hoursMinutes(hours: number): string {
	const total = Math.max(0, Math.round(hours * 60));
	const h = Math.floor(total / 60);
	const m = total % 60;
	return h > 0 ? `${h}h${String(m).padStart(2, '0')}m` : `${m}m`;
}

export const rideRail = derived(
	[orderedCheckpoints, progressElements, courseState, rideOpenItems],
	([cps, elements, c, openItems]) => {
		const stopMiles = computeStopMiles();
		const closures = new Map<string, StationClosureState>();
		// courseState.stations[].outOfOrder is the backend's own reconciliation
		// (internal/course.State: a later-sequence station closed while an
		// earlier one was still open) — the SAME field the course panel's
		// StationRow badges. The rail reads it rather than re-deriving a
		// position-vs-sequence mismatch itself, so the two surfaces can never
		// disagree about which stop is out of order.
		const outOfOrder = new Map<string, boolean>();
		for (const st of c?.stations ?? []) {
			closures.set(st.checkpointId, st.closure.state as StationClosureState);
			if (st.outOfOrder) outOfOrder.set(st.checkpointId, true);
		}

		const leadLabel = c?.config.leadLabel || 'LEAD';
		const sweepLabel = c?.config.sweepLabel || 'SWEEP';

		const leadEl = elements.find((e) => e.label.toLowerCase() === leadLabel.toLowerCase());
		const sweepEl = elements.find((e) => e.label.toLowerCase() === sweepLabel.toLowerCase());
		const leadMile = leadEl && stopMiles ? stopMiles.get(leadEl.lastCheckpointId) : undefined;
		const sweepMile =
			(c?.sweep.latestReport?.routeMile ?? undefined) ??
			(sweepEl && stopMiles ? stopMiles.get(sweepEl.lastCheckpointId) : undefined);

		const leadMileText = leadMile != null ? `mi ${leadMile.toFixed(1)}` : leadEl ? `CP${leadEl.lastCheckpointSeq}` : '—';
		const sweepMileText = sweepMile != null ? `mi ${sweepMile.toFixed(1)}` : sweepEl ? `CP${sweepEl.lastCheckpointSeq}` : '—';
		// Grafana's NoData rule (spec §6): `never` is a named state with its
		// own words, visually distinct from `stale` and never rendered as 0.
		const leadRead = leadMile != null || leadEl ? `${leadLabel} ${leadMileText}` : `${leadLabel} not reported`;
		const sweepRead = sweepMile != null || sweepEl ? `${sweepLabel} ${sweepMileText}` : `${sweepLabel} not reported`;
		// Spec §2/§6: the gap is asked for both ways on the radio, and a
		// never-reported edge must read "not reported", never a bare dash.
		const gapMiles = leadMile != null && sweepMile != null ? Math.max(0, leadMile - sweepMile) : null;
		const sweepMph = c?.sweep.latestReport?.estimatedSpeedMph ?? null;
		const gapText =
			gapMiles == null
				? ''
				: sweepMph && sweepMph > 0
					? `${gapMiles.toFixed(1)} mi / ~${hoursMinutes(gapMiles / sweepMph)}`
					: `${gapMiles.toFixed(1)} mi`;

		const incidentPins = openItems
			.filter((i) => !i.acked)
			.map((i) => ({ id: i.id, tier: i.tier, label: i.summary }));

		return {
			checkpoints: cps,
			elements,
			shutoffs: c?.shutoffs ?? [],
			closures,
			outOfOrder,
			stations: c?.stations ?? [],
			stopMiles,
			leadMile: leadMile ?? null,
			leadReported: leadMile != null || leadEl != null,
			leadMileText,
			sweepMileText,
			leadRead,
			sweepRead,
			gapText,
			sweepNextStationId: c?.sweep.nextStationId ?? '',
			leadLabel,
			sweepLabel,
			incidentPins
		};
	}
);

// ---- derived: sweep readout (B2) ----

export const rideSweep = derived([courseState, rideRail, rideProfile, secondClock], ([c, rail, profile, now]) => {
	const sweep = c?.sweep;
	const latest = sweep?.latestReport;
	const mile = latest?.routeMile ?? (sweep ? rail.stopMiles?.get(sweep.lastCheckpointId) : undefined) ?? null;
	const reportedAt = latest?.reportedAt ?? sweep?.lastPassageTime ?? null;
	const speed = latest?.estimatedSpeedMph ?? null;
	const total = profile?.rideConfig?.routes?.[0]?.distanceMiles ?? (rail.stopMiles ? Math.max(0, ...Array.from(rail.stopMiles.values())) : null);
	const toGo = mile != null && total != null ? Math.max(0, total - mile) : null;
	return { mile, reportedAt, speed, toGo, age: ageState(reportedAt, now), ageText: ageText(reportedAt, now) };
});

/**
 * B-LEAD's readout, the same three-state shape as rideSweep: the LAUNCHED
 * phase's big tile cannot render a bare em-dash over the literal words
 * "to go" when the lead has never been reported (Grafana NoData, spec §6).
 */
export const rideLead = derived([rideRail, rideProfile], ([rail, profile]) => {
	const total = profile?.rideConfig?.routes?.[0]?.distanceMiles ?? (rail.stopMiles ? Math.max(0, ...Array.from(rail.stopMiles.values())) : null);
	const toGo = rail.leadMile != null && total != null ? Math.max(0, total - rail.leadMile) : null;
	return { mile: rail.leadMile, reported: rail.leadReported, toGo };
});

// ---- derived: weather (B7) ----

export const rideWx = derived([weatherStations, rosterCallsigns, wxInAreaAlerts, weatherUnits], ([stations, roster, alerts, units]) => {
	const rosterStations = stations.filter((s) => roster.has(s.callsign));
	const pool = rosterStations.length > 0 ? rosterStations : stations;
	const reading = pool.length > 0 ? pool.reduce((a, b) => (Date.parse(a.lastHeard) > Date.parse(b.lastHeard) ? a : b)) : null;
	const temp = reading?.weather?.temperature;
	const tempText = temp != null ? formatTempShort(temp, units) : '';
	let heatIndexText = '';
	if (temp != null && reading?.weather?.humidity != null) {
		const tempF = convertTemp(temp, 'imperial');
		const hi = heatIndexF(tempF, reading.weather.humidity);
		if (hi != null) {
			const hiDisplay = units === 'imperial' ? hi : Math.round(((hi - 32) * 5) / 9);
			heatIndexText = `HI ${hiDisplay}°${units === 'imperial' ? 'F' : 'C'}`;
		}
	}
	const alert = alerts[0] ?? null;
	return { tempText, heatIndexText, alert, hidden: !reading && !alert };
});

// ---- derived: shift (B8) ----

export const rideShift = derived([activeNet, briefing, secondClock], ([net, brief, now]) => {
	const dueAt = brief?.shiftDueAt;
	let cd: string | null = null;
	let tone: 'normal' | 'warning' = 'normal';
	if (dueAt) {
		const msLeft = Date.parse(dueAt) - now;
		if (msLeft <= 90 * 60_000) cd = countdown(dueAt, now).text;
		if (msLeft <= 15 * 60_000) tone = 'warning';
	}
	return { callsign: net?.ncsCallsign ?? '', dueAt, countdown: cd, tone, hasShift: !!dueAt };
});

// ---- RideZone descriptors ----

export type RideZoneId =
	| 'net' | 'lead' | 'lastRider' | 'nextShutoff' | 'stops' | 'sag' | 'traffic' | 'wx' | 'shift'
	| 'checkedIn' | 'clearing' | 'unaccounted' | 'sagdTotal' | 'openIncidents' | 'unitsNotReleased';

export interface ZoneLine {
	text: string;
	size: 'value' | 'body' | 'label';
	tone?: 'normal' | 'muted' | 'warning' | 'success';
}

export interface RideZoneDescriptor {
	id: RideZoneId;
	label: string;
	lines: ZoneLine[];
	span?: 1 | 2 | 3;
	tone?: 'normal' | 'muted' | 'warning' | 'success';
	borderVar?: string;
	aria: string;
}

// ---- zone SET as a function of (phase, viewport) — spec §8 + §9 ----

/**
 * The candidate zone set per phase, at full (desktop) width — one source of
 * truth, read by both the real derivation below and ride.test.ts's
 * exhaustive span-sum check. `span` is the DESKTOP span; tablet clamps every
 * span to 1 (see `tabletizeZones`) so the six-zone template never depends on
 * a double-width tile fitting a six-column grid.
 *
 * Zones not listed for a phase are not merely hidden — they don't exist for
 * that phase, exactly as spec §8's table draws it (e.g. reconcile has no
 * `net` zone to merge SHIFT into).
 */
export const PHASE_ZONE_TEMPLATE: Record<RidePhaseID, { id: RideZoneId; span?: 1 | 2 | 3 }[]> = {
	'pre-start': [{ id: 'net' }, { id: 'checkedIn' }, { id: 'stops' }, { id: 'sag' }, { id: 'wx' }, { id: 'shift' }],
	launched: [{ id: 'net' }, { id: 'lead', span: 2 }, { id: 'stops' }, { id: 'sag' }, { id: 'traffic' }, { id: 'wx' }, { id: 'shift' }],
	'mid-ride': [{ id: 'net' }, { id: 'lastRider', span: 2 }, { id: 'nextShutoff' }, { id: 'stops' }, { id: 'sag' }, { id: 'traffic' }, { id: 'wx' }, { id: 'shift' }],
	closing: [{ id: 'net' }, { id: 'lastRider', span: 2 }, { id: 'nextShutoff', span: 2 }, { id: 'stops' }, { id: 'sag' }, { id: 'traffic' }, { id: 'wx' }, { id: 'shift' }],
	collapse: [{ id: 'net' }, { id: 'lastRider', span: 2 }, { id: 'clearing' }, { id: 'sag' }, { id: 'traffic' }, { id: 'wx' }, { id: 'shift' }],
	reconcile: [{ id: 'unaccounted' }, { id: 'sagdTotal' }, { id: 'openIncidents' }, { id: 'unitsNotReleased' }, { id: 'shift' }]
};

/**
 * Documented ceiling per viewport class, asserted by ride.test.ts against
 * EVERY (phase, viewport) combination PHASE_ZONE_TEMPLATE can produce —
 * including every optional zone present at once and an active EMERGENCY.
 * tablet=6 is spec §9's literal contract. desktop=10 is the template's own
 * computed worst case (CLOSING with NEXT SHUTOFF doubled and WX present:
 * 1+2+2+1+1+1+1+1); the desktop grid has no CSS row-wrap risk regardless
 * (`.ride-zones` is `grid-auto-flow: column`, which only ever adds columns),
 * so this ceiling exists to catch an accidental future zone-set bloat, not
 * to prevent a layout break.
 */
export const RIDE_GRID_COLUMNS: Record<RideViewportClass, number> = { desktop: 10, tablet: 6 };

export function zoneSpanSum(zones: { span?: 1 | 2 | 3 }[]): number {
	return zones.reduce((sum, z) => sum + (z.span ?? 1), 0);
}

/** Zone-id pairs merged at tablet width (spec §9): primary keeps its id
 *  (so `navigateZone` needs no new cases) and absorbs the secondary's lines. */
const TABLET_MERGES: { primary: RideZoneId; secondary: RideZoneId; label: string }[] = [
	{ primary: 'net', secondary: 'shift', label: 'NET+SHIFT' },
	{ primary: 'stops', secondary: 'sag', label: 'LOGISTICS' },
	// COLLAPSE's STOPS-equivalent is CLEARING; the same LOGISTICS merge applies.
	{ primary: 'clearing', secondary: 'sag', label: 'LOGISTICS' }
];

function mergeZonePair(a: RideZoneDescriptor, b: RideZoneDescriptor, label: string): RideZoneDescriptor {
	return {
		...a,
		label,
		// The tablet row buys a third line (spec §9 "tiles need a third
		// line" — 140px vs desktop's 128px); a 4th would be silently eaten
		// by RideZone's overflow:hidden, the exact class of bug this
		// finding is about. Cap defensively rather than trust every future
		// merge pair to self-limit.
		lines: [...a.lines, ...b.lines].slice(0, 3),
		aria: `${a.aria}. ${b.aria}.`
	};
}

/** "WX -> glyph + heat index only" (spec §9): drop the plain temperature
 * line and keep just the alert glyph (if any) and the heat index — the two
 * facts that change a decision, not the raw number the desktop tile leads
 * with. Falls back to whatever the first line was if there's no HI reading
 * (e.g. a reading with no humidity), so the tile is never blank. */
function reduceWxZone(z: RideZoneDescriptor): RideZoneDescriptor {
	const hiLine = z.lines.find((l) => l.text.startsWith('HI '));
	const alertLine = z.lines.find((l) => l.tone === 'warning' && !l.text.startsWith('HI '));
	const text = [alertLine ? '⚠' : '', (hiLine ?? z.lines[0])?.text ?? ''].filter(Boolean).join(' ') || '—';
	return { ...z, lines: [{ text, size: 'value', tone: alertLine || hiLine ? 'warning' : 'normal' }] };
}

/**
 * The tablet transform (spec §9): merge NET+SHIFT and STOPS/CLEARING+SAG,
 * reduce WX, then clamp every zone to span 1. The clamp is unconditional —
 * including the EMERGENCY-expanded TRAFFIC zone — so a six-zone tablet
 * template can never be asked to hold more than 6 column-units regardless
 * of phase or emergency state; this is what makes the "surplus flows into a
 * hidden second row" bug class impossible rather than merely unobserved.
 */
export function tabletizeZones(zones: RideZoneDescriptor[]): RideZoneDescriptor[] {
	let out = [...zones];
	for (const spec of TABLET_MERGES) {
		const ai = out.findIndex((z) => z.id === spec.primary);
		const bi = out.findIndex((z) => z.id === spec.secondary);
		if (ai < 0 || bi < 0) continue; // one side absent (e.g. EMERGENCY already absorbed SAG) — nothing to merge
		const merged = mergeZonePair(out[ai], out[bi], spec.label);
		out = out.filter((_z, i) => i !== ai && i !== bi);
		out.splice(Math.min(ai, bi), 0, merged);
	}
	out = out.map((z) => (z.id === 'wx' ? reduceWxZone(z) : z));
	out = out.map((z) => (z.span && z.span > 1 ? { ...z, span: 1 } : z));
	return out;
}

function trafficZone(counts: { tier: PriorityTier; count: number; oldestAt: string }[], unacked: number, pending: { overdue: number; oldestAt: string | null }): RideZoneDescriptor {
	if (counts.length === 0 && unacked === 0 && pending.overdue === 0) {
		return {
			id: 'traffic', label: 'TRAFFIC', span: 1, tone: 'muted',
			lines: [{ text: '✓ clear', size: 'value', tone: 'muted' }],
			aria: 'Traffic: clear'
		};
	}
	const lines: ZoneLine[] = [];
	const top = counts[0];
	if (top) {
		// tierStyle().code is the ONE place a tier label becomes its two-letter
		// code; re-deriving it here let the strip and SagBoard drift apart.
		lines.push({ text: `${tierStyle(top.tier).code} ${top.count}${counts.length > 1 ? ` +${counts.length - 1}` : ''}`, size: 'value' });
	}
	if (unacked > 0) lines.push({ text: `! ${unacked} unacked`, size: 'body', tone: 'warning' });
	if (pending.overdue > 0) lines.push({ text: `${PENDING_GLYPH} ${pending.overdue} pending`, size: 'label', tone: 'warning' });
	return {
		id: 'traffic', label: 'TRAFFIC', span: 1,
		borderVar: top ? tierStyle(top.tier).colorVar : undefined,
		lines,
		aria: `Traffic: ${counts.map((c) => `${c.count} ${c.tier.label}`).join(', ') || 'nothing open'}${unacked ? `, ${unacked} unacknowledged` : ''}${pending.overdue ? `, ${pending.overdue} pending reply` : ''}`
	};
}

export const rideZones = derived(
	[ridePhase, rideStops, rideRail, rideSweep, rideLead, rideShutoffs, rideSagSummary, rideTierCounts, rideUnackedTraffic, ridePending, rideWx, rideShift, netMetrics, closeout, accounting, minuteClock, rideViewportClass],
	([phase, stops, rail, sweep, lead, shutoffs, sag, tierCounts, unacked, pending, wx, shift, metrics, closeoutV, accountingV, _m, viewportClass]): RideZoneDescriptor[] => {
		const phaseId = (phase?.phase ?? 'pre-start') as RidePhaseID;

		const stopsZone: RideZoneDescriptor = stops.total === 0
			? { id: 'stops', label: 'STOPS', lines: [{ text: '—', size: 'value', tone: 'muted' }], aria: 'No stops configured' }
			: stops.allClosed
				? { id: 'stops', label: 'STOPS', tone: 'success', lines: [{ text: `⊘ all ${stops.total} closed`, size: 'value', tone: 'success' }], aria: `All ${stops.total} stops closed` }
				: {
						id: 'stops', label: 'STOPS',
						lines: [
							// Spec §4.4/§11: render a counter only when it is > 0.
							// "◆4 ⊘0 ⏳0" is three numbers where one is the signal.
							{
								text: [
									stops.open > 0 ? `${STOP_GLYPHS.open}${stops.open}` : '',
									stops.awaitingSweep > 0 ? `${STOP_GLYPHS.awaitingSweep}${stops.awaitingSweep}` : '',
									stops.readyToClose > 0 ? `${STOP_GLYPHS.readyToClose}${stops.readyToClose}` : '',
									stops.closed > 0 ? `${STOP_GLYPHS.closed}${stops.closed}` : ''
								].filter(Boolean).join(' '),
								size: 'value'
							},
							{ text: stops.blockingLabel ? `${stops.blockingLabel} awaits sweep` : '', size: 'body', tone: 'muted' }
						],
						aria: `Stops: ${stops.open} open, ${stops.awaitingSweep} awaiting sweep, ${stops.readyToClose} ready to close, ${stops.closed} closed${stops.blockingLabel ? `, ${stops.blockingLabel} is blocking` : ''}`
					};

		const wxZone: RideZoneDescriptor | null = wx.hidden
			? null
			: {
					id: 'wx', label: 'WX',
					lines: [
						{ text: wx.tempText || '—', size: 'value' },
						...(wx.heatIndexText ? [{ text: wx.heatIndexText, size: 'body' as const, tone: 'warning' as const }] : []),
						...(wx.alert ? [{ text: wx.alert.shortCode, size: 'label' as const, tone: 'warning' as const }] : [])
					],
					aria: `Weather: ${wx.tempText || 'no reading'}${wx.alert ? `, ${wx.alert.event}` : ''}`
				};

		const shiftZone: RideZoneDescriptor = {
			id: 'shift', label: 'SHIFT',
			lines: [
				{ text: shift.callsign || '—', size: 'value' },
				// countdown is null until relief is inside 90m (spec §6); the old
				// `shift.dueAt ? ...` guard rendered the literal string "−null".
				{ text: shift.countdown ? `−${shift.countdown}` : shift.dueAt ? `relief ${clock(shift.dueAt)}` : '', size: 'body', tone: shift.tone === 'warning' ? 'warning' : 'normal' }
			],
			aria: `Shift: ${shift.callsign || 'no NCS'}${shift.countdown ? `, relief in ${shift.countdown}` : shift.dueAt ? `, relief at ${clock(shift.dueAt)}` : ''}`
		};

		const trafficZ = trafficZone(tierCounts, unacked, pending);

		const sagZone: RideZoneDescriptor = sag.noUnits
			? { id: 'sag', label: 'SAG', tone: 'warning', lines: [{ text: '⬒ no SAG units', size: 'value', tone: 'warning' }], aria: 'No SAG units checked in' }
			: {
					id: 'sag', label: 'SAG',
					lines: [
						{
							text: sag.open > 0 ? `⬒ ${sag.open} waiting` : '⬒ none waiting',
							size: 'value',
							tone: sag.open > 0 ? 'warning' : undefined
						},
						{
							text:
								sag.open > 0
									? `${sag.ridersWaiting} roadside${sag.oldestOpenAt ? ` · ${ageText(sag.oldestOpenAt, Date.now())}` : ''}`
									: `${sag.inMotion} in motion`,
							size: 'body'
						},
						{ text: `${sag.seatsAvail} free of ${sag.seats} seats`, size: 'label', tone: 'muted' }
					],
					aria:
						sag.open > 0
							? `SAG: ${sag.open} request${sag.open === 1 ? '' : 's'} needing a vehicle, ${sag.ridersWaiting} riders at the roadside, ${sag.seatsAvail} of ${sag.seats} seats free`
							: `SAG: nobody waiting for a vehicle, ${sag.inMotion} in motion, ${sag.seatsAvail} of ${sag.seats} seats free`
				};

		const nextShutoffZone: RideZoneDescriptor | null = shutoffs.none
			? null
			: shutoffs.allFired
				? { id: 'nextShutoff', label: 'NEXT SHUTOFF', tone: 'success', lines: [{ text: '╫ all shutoffs fired', size: 'body', tone: 'success' }], aria: 'All shutoffs fired' }
				: shutoffs.next
					? {
							id: 'nextShutoff', label: 'NEXT SHUTOFF',
							lines: [
								{ text: `╫ ${shutoffs.next.name}`, size: 'value' },
								{
									text: `${clock(shutoffs.next.scheduledAt)}${shutoffs.countdown ? `  −${shutoffs.countdown}` : ''}`,
									size: 'body',
									tone: shutoffs.tone === 'warning' ? 'warning' : 'normal'
								}
							],
							aria: `Next shutoff ${shutoffs.next.name} at ${clock(shutoffs.next.scheduledAt)}${shutoffs.countdown ? `, in ${shutoffs.countdown}` : ''}`
						}
					: null;

		const lastRiderZone: RideZoneDescriptor = sweep.age === 'never'
			? {
					id: 'lastRider', label: 'LAST RIDER', span: 2, tone: 'muted',
					lines: [
						{ text: '▲ —', size: 'value', tone: 'muted' },
						{ text: `${rail.sweepLabel} not reported`, size: 'body', tone: 'muted' }
					],
					aria: `${rail.sweepLabel} position not reported`
				}
			: {
					id: 'lastRider', label: 'LAST RIDER', span: 2, tone: sweep.age === 'stale' ? 'muted' : 'normal',
					lines: [
						{ text: `▲ ${rail.sweepMileText}`, size: 'value', tone: sweep.age === 'stale' ? 'muted' : 'normal' },
						{ text: sweep.toGo != null ? `${sweep.toGo.toFixed(1)} to go` : '', size: 'body' },
						{
							text: `${sweep.speed != null ? `${sweep.speed.toFixed(1)}mph · ` : ''}${sweep.ageText}${sweep.age === 'stale' ? ' · stale' : ''}`,
							size: 'label',
							tone: sweep.age === 'stale' || sweep.age === 'aging' ? 'warning' : 'normal'
						}
					],
					aria: `${rail.sweepLabel} at ${rail.sweepMileText}${sweep.toGo != null ? `, ${sweep.toGo.toFixed(1)} miles remaining` : ''}${sweep.speed != null ? `, ${sweep.speed.toFixed(1)} miles per hour` : ''}, reported ${sweep.ageText} ago${sweep.age === 'stale' ? ', stale' : ''}`
				};

		// The second line used to be the bare string "to go" — no number in
		// front of it — and the never-reported case rendered "▼ —" over it.
		// Same named states as LAST RIDER (spec §6): a value, or the literal
		// words, never a dangling unit.
		const leadZone: RideZoneDescriptor = lead.reported
			? {
					id: 'lead', label: 'LEAD', span: 2,
					lines: [
						{ text: `▼ ${rail.leadMileText}`, size: 'value' },
						{ text: lead.toGo != null ? `${lead.toGo.toFixed(1)} to go` : '', size: 'body' }
					],
					aria: `${rail.leadLabel} at ${rail.leadMileText}${lead.toGo != null ? `, ${lead.toGo.toFixed(1)} miles remaining` : ''}`
				}
			: {
					id: 'lead', label: 'LEAD', span: 2, tone: 'muted',
					lines: [
						{ text: '▼ —', size: 'value', tone: 'muted' },
						{ text: `${rail.leadLabel} not reported`, size: 'body', tone: 'muted' }
					],
					aria: `${rail.leadLabel} position not reported`
				};

		const netZone: RideZoneDescriptor = {
			id: 'net', label: 'NET',
			lines: [{ text: elapsedHM(get(activeNet)?.openedAt, Date.now()), size: 'value' }],
			aria: `Net elapsed ${elapsedHM(get(activeNet)?.openedAt, Date.now())}`
		};

		const checkedInZone: RideZoneDescriptor = {
			id: 'checkedIn', label: 'CHECKED IN',
			lines: [{ text: `${metrics.totalIn} in`, size: 'value' }, { text: `${metrics.missing} missing`, size: 'body', tone: metrics.missing > 0 ? 'warning' : 'muted' }],
			aria: `${metrics.totalIn} checked in, ${metrics.missing} missing`
		};

		const clearingZone: RideZoneDescriptor = {
			id: 'clearing', label: 'CLEARING',
			lines: [
				{ text: `${stops.clearing.cleared} of ${stops.clearing.total} cleared`, size: 'value' },
				{ text: stops.clearing.waitingOn ? `waiting on ${stops.clearing.waitingOn}` : '', size: 'body', tone: 'muted' }
			],
			aria: `${stops.clearing.cleared} of ${stops.clearing.total} stations cleared${stops.clearing.waitingOn ? `, waiting on ${stops.clearing.waitingOn}` : ''}`
		};

		const unaccountedZone: RideZoneDescriptor = {
			id: 'unaccounted', label: 'UNACCOUNTED',
			lines: [{ text: `${accountingV?.counts.supported ?? 0}`, size: 'value' }],
			aria: `${accountingV?.counts.supported ?? 0} unaccounted riders`
		};
		const sagdTotalZone: RideZoneDescriptor = {
			id: 'sagdTotal', label: "SAG'D TOTAL",
			lines: [{ text: `${accountingV?.counts.byKind?.sag ?? 0}`, size: 'value' }],
			aria: `${accountingV?.counts.byKind?.sag ?? 0} riders SAG'd`
		};
		const openIncidentsZone: RideZoneDescriptor = {
			id: 'openIncidents', label: 'OPEN INCIDENTS',
			lines: [{ text: `${tierCounts.reduce((s, c) => s + c.count, 0)}`, size: 'value' }],
			aria: `${tierCounts.reduce((s, c) => s + c.count, 0)} open incidents`
		};
		const unitsItem = closeoutV?.items.find((i) => i.key === 'field_units_out');
		const unitsNotReleasedZone: RideZoneDescriptor = {
			id: 'unitsNotReleased', label: 'UNITS OUT',
			lines: [{ text: unitsItem ? `${unitsItem.count}/${unitsItem.total}` : '—', size: 'value' }],
			aria: `${unitsItem?.count ?? 0} of ${unitsItem?.total ?? 0} field units still out`
		};

		// One lookup, keyed by zone id, built once regardless of phase — every
		// *Zone above is already computed unconditionally. PHASE_ZONE_TEMPLATE
		// (§8 + §9) is the single source of truth for WHICH ids a phase shows;
		// this function only supplies the CONTENT for whichever ids get asked
		// for, so the zone SET can be resolved generically for any (phase,
		// viewport) pair instead of hand-listing it per phase per width.
		const zoneById: Partial<Record<RideZoneId, RideZoneDescriptor | null>> = {
			net: netZone,
			lead: leadZone,
			lastRider: lastRiderZone,
			// CLOSING doubles NEXT SHUTOFF's width and shows the next TWO —
			// the "next two" content lives in nextShutoffZone already; span is
			// applied by PHASE_ZONE_TEMPLATE's own {id:'nextShutoff', span:2}.
			nextShutoff: nextShutoffZone,
			stops: stopsZone,
			// SAG is spec-"dark" in PRE-START, with one exception: "no SAG
			// units at all" is the one absence PRE-START most needs to know
			// about (an operator can still fix it before launch). Every other
			// phase always shows SAG, in whichever of its two forms.
			// PRE-START swaps SAG out for CHECKED IN — but only while there is
			// genuinely no SAG activity. A net still marked pre-start with riders
			// in a vehicle used to show NOTHING about them anywhere on the strip.
			sag: phaseId === 'pre-start' && !sag.noUnits && sag.active === 0 ? null : sagZone,
			traffic: trafficZ,
			// COLLAPSE hides WX unless an alert is active — stricter than B7's
			// normal "hidden only with neither a reading nor an alert" rule.
			wx: phaseId === 'collapse' ? (wx.alert ? wxZone : null) : wxZone,
			shift: shiftZone,
			checkedIn: checkedInZone,
			clearing: clearingZone,
			unaccounted: unaccountedZone,
			sagdTotal: sagdTotalZone,
			openIncidents: openIncidentsZone,
			unitsNotReleased: unitsNotReleasedZone
		};

		const template = PHASE_ZONE_TEMPLATE[phaseId] ?? PHASE_ZONE_TEMPLATE['mid-ride'];
		let zones: (RideZoneDescriptor | null)[] = template.map(({ id, span }) => {
			const z = zoneById[id];
			if (!z) return null;
			return span ? { ...z, span } : z;
		});

		// EMERGENCY (any phase, any viewport): traffic zone absorbs SAG + WX
		// and spans 3. Applied BEFORE the tablet transform, which then clamps
		// every span back to 1 — see tabletizeZones's doc comment for why
		// that clamp is unconditional.
		const emergency = get(rideEmergency);
		if (emergency) {
			zones = zones.filter((z) => z && z.id !== 'sag' && z.id !== 'wx');
			zones = zones.map((z) => (z && z.id === 'traffic' ? { ...z, span: 3 as const, borderVar: '--color-ride-emergency' } : z));
		}

		let resolved = zones.filter((z): z is RideZoneDescriptor => z !== null);
		if (viewportClass === 'tablet') resolved = tabletizeZones(resolved);
		return resolved;
	}
);

// ---- actions ----

let flyToHandler: ((lat: number, lon: number, zoom?: number) => void) | null = null;
let flyToBoundsHandler: ((coords: { lat: number; lon: number }[]) => void) | null = null;

/** RideStrip.svelte registers the map-fly callbacks it receives as props. */
export function registerRideFlyTo(
	fly: ((lat: number, lon: number, zoom?: number) => void) | null,
	flyBounds: ((coords: { lat: number; lon: number }[]) => void) | null = null
): void {
	flyToHandler = fly;
	flyToBoundsHandler = flyBounds;
}

let initialized = false;
let lastRideNetId: string | null = null;

/** Idempotent; registers WS handlers; starts the shared clock; reloads on net-identity change. */
export function initRideStore(): void {
	if (initialized) return;
	initialized = true;
	startClock();

	activeNetId.subscribe((id) => {
		if (id === lastRideNetId) return;
		lastRideNetId = id;
		rideProfile.set(null);
		ridePhase.set(null);
		courseState.set(null);
		sagBoard.set(null);
		medicalOpen.set([]);
		supplyAll.set([]);
		handoffOpen.set([]);
		briefing.set(null);
		closeout.set(null);
		accounting.set(null);
		if (id) {
			loadRideData(id).catch(() => {
				/* per-request failures are handled inside loadRideData */
			});
		}
	});

	wsClient.on('course_state', (msg) => courseState.set(msg.data as CourseState));
	wsClient.on('course_shutoff_fired', (msg) => {
		const sp = msg.data as ShutoffPoint;
		queueAnnouncement(`Shutoff ${sp.name} fired at ${new Date(sp.firedAt ?? Date.now()).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}`, true);
		showToast(`Shutoff fired: ${sp.name}`, 'info', 6000);
	});

	wsClient.on('sag_request_created', (msg) => upsertSagRequest(msg.data));
	wsClient.on('sag_request_updated', (msg) => upsertSagRequest(msg.data));
	wsClient.on('sag_vehicle_updated', (msg) => upsertSagVehicle(msg.data));

	wsClient.on('ride_medical_created', (msg) => upsertMedical(msg.data as MedicalNotification));
	wsClient.on('ride_medical_updated', (msg) => upsertMedical(msg.data as MedicalNotification));
	wsClient.on('ride_supply_created', (msg) => upsertSupply(msg.data as SupplyRequest));
	wsClient.on('ride_supply_updated', (msg) => upsertSupply(msg.data as SupplyRequest));

	wsClient.on('ride_handoff_item_created', (msg) => upsertHandoff(msg.data as HandoffItem));
	wsClient.on('ride_handoff_item_updated', (msg) => upsertHandoff(msg.data as HandoffItem));
	wsClient.on('ride_shift_handoff', () => {
		const netId = get(activeNetId);
		if (netId) api.rideBriefing(netId).then((b) => briefing.set(b)).catch(() => {});
	});

	wsClient.on('ride_phase_updated', (msg) => {
		const st = msg.data as RidePhaseStatus;
		const prev = get(ridePhase);
		ridePhase.set(st);
		if (prev && prev.phase !== st.phase) {
			queueAnnouncement(`Ride phase ${st.phase.toUpperCase()}`, true);
			if (st.phase === 'reconcile') {
				const netId = get(activeNetId);
				if (netId) {
					api.rideCloseout(netId).then((c) => closeout.set(c)).catch(() => {});
					api.rideAccounting(netId).then((a) => accounting.set(a)).catch(() => {});
				}
			}
		}
	});

	// Restore-safe: a rank-1 item already open when the store loads must not
	// be silently dropped (mirrors wxAlerts' restore exception).
	rideOpenItems.subscribe((items) => {
		const acked = get(rideAcked);
		rideInterruptQueue.update((q) => {
			const next = [...q];
			for (const it of items) {
				if (it.tier.rank === 1 && !acked.has(it.id) && !next.includes(it.id)) next.push(it.id);
			}
			return next.filter((id) => items.some((it) => it.id === id));
		});
	});
}

export function upsertSagRequest(data: unknown) {
	sagBoard.update((b) => {
		if (!b) return b;
		const r = data as SAGBoard['requests'][number];
		const idx = b.requests.findIndex((x) => x.id === r.id);
		const requests = idx >= 0 ? b.requests.map((x, i) => (i === idx ? r : x)) : [...b.requests, r];
		return { ...b, requests };
	});
}

export function upsertSagVehicle(data: unknown) {
	sagBoard.update((b) => {
		if (!b) return b;
		const v = data as SAGBoard['vehicles'][number];
		const idx = b.vehicles.findIndex((x) => x.checkInId === v.checkInId);
		const vehicles = idx >= 0 ? b.vehicles.map((x, i) => (i === idx ? v : x)) : [...b.vehicles, v];
		return { ...b, vehicles };
	});
}

export function upsertMedical(m: MedicalNotification) {
	medicalOpen.update((list) => {
		const terminal = m.status === 'departed' || m.status === 'released' || m.status === 'cancelled';
		const idx = list.findIndex((x) => x.id === m.id);
		if (terminal) return idx >= 0 ? list.filter((x) => x.id !== m.id) : list;
		return idx >= 0 ? list.map((x, i) => (i === idx ? m : x)) : [...list, m];
	});
}

export function upsertSupply(s: SupplyRequest) {
	supplyAll.update((list) => {
		const idx = list.findIndex((x) => x.id === s.id);
		return idx >= 0 ? list.map((x, i) => (i === idx ? s : x)) : [...list, s];
	});
}

export function upsertHandoff(h: HandoffItem) {
	handoffOpen.update((list) => {
		const idx = list.findIndex((x) => x.id === h.id);
		if (h.status !== 'open') return idx >= 0 ? list.filter((x) => x.id !== h.id) : list;
		return idx >= 0 ? list.map((x, i) => (i === idx ? h : x)) : [...list, h];
	});
}

/** Every failure here is per-request, never throws — arrays normalize with `?? []`. */
export async function loadRideData(netId: string): Promise<void> {
	const results = await Promise.allSettled([
		api.netProfile(netId),
		api.ridePhase(netId),
		api.courseState(netId),
		api.sagBoard(netId),
		api.rideMedical(netId, true),
		api.rideSupply(netId),
		api.rideHandoff(netId),
		api.rideBriefing(netId)
	]);
	const [profileR, phaseR, courseR, sagR, medR, supR, handoffR, briefR] = results;
	if (profileR.status === 'fulfilled') rideProfile.set(profileR.value);
	if (phaseR.status === 'fulfilled') ridePhase.set(phaseR.value);
	else ridePhase.set(null);
	if (courseR.status === 'fulfilled') courseState.set(courseR.value);
	if (sagR.status === 'fulfilled') sagBoard.set(sagR.value);
	if (medR.status === 'fulfilled') medicalOpen.set(medR.value ?? []);
	// supplyAll really is ALL supply requests (unfiltered) — it must match
	// upsertSupply's behavior below, which never drops a terminal request off
	// a live WS update. rideOpenItems already does its own per-item status
	// filtering, so pre-filtering here only made the initial load and a
	// later WS update disagree about what the store holds.
	if (supR.status === 'fulfilled') supplyAll.set(supR.value ?? []);
	if (handoffR.status === 'fulfilled') handoffOpen.set(handoffR.value ?? []);
	if (briefR.status === 'fulfilled') briefing.set(briefR.value);

	if (phaseR.status === 'fulfilled' && phaseR.value.phase === 'reconcile') {
		try {
			closeout.set(await api.rideCloseout(netId));
			accounting.set(await api.rideAccounting(netId));
		} catch {
			// advisory close-out data — the strip degrades to '—' tiles on failure
		}
	}
}

export async function setPhase(to: RidePhaseID, reason?: string): Promise<RidePhaseStatus> {
	const netId = get(activeNetId);
	if (!netId) throw new Error('no active net');
	try {
		const st = await api.setRidePhase(netId, { phase: to, reason });
		ridePhase.set(st);
		queueAnnouncement(`Ride phase ${st.phase.toUpperCase()}`, true);
		return st;
	} catch (e) {
		if (e instanceof ApiError) {
			throw { status: e.status, error: e.message, code: (e.body as any)?.code };
		}
		throw e;
	}
}

export async function markSweepPassed(cpId: string): Promise<void> {
	const netId = get(activeNetId);
	if (!netId) return;
	const label = get(courseState)?.stations.find((s) => s.checkpointId === cpId)?.label ?? cpId;
	try {
		await api.stationSweepPassed(netId, cpId);
		showToast(`Sweep passed ${label}`, 'success');
	} catch (e) {
		showToast(e instanceof ApiError ? e.message : 'Sweep passage failed', 'error', 8000);
		throw e;
	}
}

/**
 * Close a rest stop. Feedback split (spec §7 "every action has a visible
 * response", exactly one of them):
 *   - success and UNEXPECTED failures toast here, so every caller gets them;
 *   - the two KNOWN gate refusals (`sweep_not_passed`, `ncs_or_admin_required`)
 *     toast nothing and rethrow, because the caller renders them better than a
 *     toast can — StationRow opens its inline gate explainer with the real
 *     override form, NetControlPanel's palette path says which tab to use.
 * This used to toast the gate refusal here AND in both callers (two toasts for
 * one refusal), and offered the override through window.prompt() — an
 * unstyled, unvalidated native modal collecting a reason that is written to
 * the audit log.
 */
export async function closeStation(cpId: string, opts?: { override: boolean; reason: string }): Promise<void> {
	const netId = get(activeNetId);
	if (!netId) return;
	const label = get(courseState)?.stations.find((s) => s.checkpointId === cpId)?.label ?? cpId;
	try {
		await api.stationClose(netId, cpId, opts ?? {});
		showToast(opts?.override ? `${label} closed by override` : `${label} closed`, 'success');
	} catch (e) {
		const code = e instanceof ApiError ? (e.body as Record<string, unknown>)?.code : undefined;
		if (code !== 'sweep_not_passed' && code !== 'ncs_or_admin_required' && code !== 'illegal_transition') {
			showToast(e instanceof ApiError ? e.message : 'Station close failed', 'error', 8000);
		}
		throw e;
	}
}

export async function ackEmergency(id: string): Promise<void> {
	const netId = get(activeNetId);
	rideAcked.update((m) => {
		const copy = new Map(m);
		copy.set(id, new Date().toISOString());
		return copy;
	});
	rideInterruptQueue.update((q) => q.filter((qid) => qid !== id));

	const item = get(rideOpenItems).find((i) => i.id === id);
	if (!netId || !item) return;

	const user = get(currentUser);
	try {
		if (item.kind === 'medical') {
			const m = get(medicalOpen).find((x) => x.id === id);
			if (m && !m.readBackAt) {
				await api.medicalReadback(netId, id, { confirmed: true, readBackBy: user?.name });
			}
		}
		await api.addNetNote(netId, {
			content: `ACK ${item.tier.label}: ${item.summary}`,
			category: 'comms',
			severity: 'info'
		});
		showToast(`Acknowledged ${item.tier.label} — logged`, 'success');
	} catch {
		showToast('Ack logged locally only — server unreachable', 'error');
	}
}

export function navigateZone(id: RideZoneId | 'stop' | 'course-import', arg?: string): void {
	const rail = get(rideRail);
	switch (id) {
		case 'net':
			openNetControl();
			break;
		case 'lead': {
			openNetControl();
			netControlRequestedTab.set('timeline');
			break;
		}
		case 'lastRider': {
			const st = get(courseState)?.sweep.latestReport;
			if (flyToHandler && st?.lat != null && st?.lon != null) flyToHandler(st.lat, st.lon, 13);
			openNetControl();
			netControlRequestedTab.set('timeline');
			break;
		}
		case 'nextShutoff': {
			const next = get(rideShutoffs).next;
			if (flyToHandler && next) flyToHandler(next.lat, next.lon, 14);
			openNetControl();
			netControlRequestedTab.set('course');
			courseRequestedTab.set('shutoffs');
			break;
		}
		case 'stops':
		case 'clearing':
			openNetControl();
			netControlRequestedTab.set('course');
			courseRequestedTab.set('stops');
			break;
		case 'stop': {
			const cp = rail.checkpoints.find((c) => c.meta.annotationId === arg);
			if (flyToHandler && cp) {
				try {
					const geom = JSON.parse(cp.annotation.geometry);
					if (geom.type === 'Point' && flyToHandler) flyToHandler(geom.coordinates[1], geom.coordinates[0], 15);
				} catch {
					/* ignore unparsable geometry */
				}
			}
			openNetControl();
			netControlRequestedTab.set('locations');
			break;
		}
		case 'sag':
			openNetControl();
			netControlRequestedTab.set('sag');
			break;
		case 'traffic':
		case 'openIncidents':
			openNetControl();
			netControlRequestedTab.set('situation');
			break;
		case 'wx':
			openWeather('alerts');
			break;
		case 'shift':
			openNetControl();
			netControlRequestedTab.set('situation');
			break;
		case 'checkedIn':
			openNetControl();
			netControlRequestedTab.set('roster');
			break;
		case 'unaccounted':
		case 'sagdTotal':
		case 'unitsNotReleased':
			openNetControl();
			netControlRequestedTab.set('situation');
			break;
		case 'course-import':
			openAnnotations();
			break;
	}
}

export function seedPalette(text: string, lockedTierId?: string): void {
	paletteSeed.set({ text, lockedTierId });
	commandPaletteOpen.set(true);
}
