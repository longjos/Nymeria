// Metadata tables and pure helpers for bike-ride mode's status strip. Same
// shape/discipline as wxAlertMeta.ts: plain data + small pure functions, no
// store imports, `now` always an explicit parameter.
import type { PriorityTier, RidePhaseID } from './types';
import { STALE_THRESHOLD_MS } from './stores/netcontrol';

export interface RideTierStyle {
	colorVar: string;
	/** Text-safe counterpart of colorVar. Use this whenever the token colors a
	    word or a numeral: --color-ride-emergency and --color-ride-medium fail
	    WCAG AA at normal text size (spec section 3). */
	textVar: string;
	softVar: string;
	glyph: string;
	glyphFill: boolean;
	/** Draw a punched-out "!" in --color-bg, as WxTierGlyph does for its warning tier. */
	punch: boolean;
	ariaWord: string;
	/** First two letters of the tier's own (agency-configurable) label, upper-cased. */
	code: string;
	label: string;
	rank: number;
	id: string;
}

/**
 * Style is keyed by RANK POSITION in the net's effective ladder, never by
 * tier id or label text — the five names are agency data (GET
 * /nets/{id}/profile). rank 1 -> filled octagon + punched '!' +
 * --color-ride-emergency; rank 2 -> filled triangle + punched '!' +
 * --color-ride-priority; rank 3 -> open triangle + --color-ride-high; rank 4
 * -> open diamond + --color-ride-medium; rank >= 5 -> open circle +
 * --color-ride-low.
 */
export function tierStyle(tier: PriorityTier): RideTierStyle {
	const code = tier.label.slice(0, 2).toUpperCase() || '??';
	const base = { label: tier.label, rank: tier.rank, id: tier.id, code };
	switch (tier.rank) {
		case 1:
			return {
				...base,
				colorVar: '--color-ride-emergency',
				textVar: '--color-ride-emergency-text',
				softVar: '--color-ride-emergency-soft',
				glyph: 'M5.5 1.5h5l3.5 3.5v5l-3.5 3.5h-5L2 10V5l3.5-3.5z',
				glyphFill: true,
				punch: true,
				ariaWord: 'Emergency'
			};
		case 2:
			return {
				...base,
				colorVar: '--color-ride-priority',
				textVar: '--color-ride-priority-text',
				softVar: '--color-ride-priority-soft',
				glyph: 'M8 1.5l6.5 13H1.5L8 1.5z',
				glyphFill: true,
				punch: true,
				ariaWord: 'Priority'
			};
		case 3:
			return {
				...base,
				colorVar: '--color-ride-high',
				textVar: '--color-ride-high-text',
				softVar: '--color-ride-priority-soft',
				glyph: 'M8 1.5l6.5 13H1.5L8 1.5z',
				glyphFill: false,
				punch: false,
				ariaWord: 'High'
			};
		case 4:
			return {
				...base,
				colorVar: '--color-ride-medium',
				textVar: '--color-ride-medium-text',
				softVar: '--color-ride-medium-soft',
				glyph: 'M8 1.5L14.5 8 8 14.5 1.5 8 8 1.5z',
				glyphFill: false,
				punch: false,
				ariaWord: 'Medium'
			};
		default:
			return {
				...base,
				colorVar: '--color-ride-low',
				textVar: '--color-ride-low-text',
				softVar: '--color-ride-low-soft',
				glyph: 'M8 2a6 6 0 100 12A6 6 0 008 2z',
				glyphFill: false,
				punch: false,
				ariaWord: 'Low'
			};
	}
}

export function tierById(ladder: PriorityTier[], id: string): PriorityTier | undefined {
	return ladder.find((t) => t.id === id);
}

/** The tier with the LOWEST rank number among `ids` (rank 1 = most urgent), or null. */
export function highestTier(ladder: PriorityTier[], ids: string[]): PriorityTier | null {
	let best: PriorityTier | null = null;
	for (const id of ids) {
		const t = tierById(ladder, id);
		if (!t) continue;
		if (!best || t.rank < best.rank) best = t;
	}
	return best;
}

/**
 * The ONE rest-stop glyph table — the ride strip's B4 zone, CourseRailView's stop
 * nodes and the course panel's rest-stop board all read it, so a stop can
 * never be drawn with two different marks on two surfaces.
 *
 * `awaitingSweep` was U+23F3 HOURGLASS and the board used U+231B: both are
 * emoji-presentation codepoints, so they render as tofu on the stock Linux
 * font stack (verified by canvas pixel signature in headless Chrome) and as a
 * colour emoji on macOS/Windows, either way breaking a deliberately
 * monochrome set. U+25C8 and U+2299 are geometric siblings of ◇/◆/⊘ that
 * render at the same weight everywhere.
 */
export const STOP_GLYPHS = {
	planned: '◇',
	open: '◆',
	awaitingSweep: '◈',
	readyToClose: '⊙',
	atCapacity: '⊛',
	closed: '⊘'
} as const;

/** B6's pending-reply mark. U+23F1 STOPWATCH is tofu for the same reason. */
export const PENDING_GLYPH = '◷';

/** internal/ride's SAGRequest.Reason values, in on-air language. */
export const SAG_REASON_LABELS: Record<string, string> = {
	mechanical: 'Mechanical',
	flat: 'Flat',
	fatigue: 'Fatigue',
	medical_minor: 'Medical (minor)',
	weather: 'Weather',
	cutoff: 'Cut-off',
	other: 'Other'
};

/** Humanise a backend enum for display; unknown values become Title Case. */
export function enumLabel(value: string | undefined, table?: Record<string, string>): string {
	if (!value) return '';
	if (table && table[value]) return table[value];
	return value.replace(/_/g, ' ').replace(/^./, (c) => c.toUpperCase());
}

export const AGE_FRESH_MS = 10 * 60_000;
/** === STALE_THRESHOLD_MS in netcontrol.ts, imported rather than duplicated. */
export const AGE_AGING_MS = STALE_THRESHOLD_MS;

export type AgeState = 'fresh' | 'aging' | 'stale' | 'never';

export function ageState(iso: string | null | undefined, now: number): AgeState {
	if (!iso) return 'never';
	const t = Date.parse(iso);
	if (Number.isNaN(t)) return 'never';
	const age = now - t;
	if (age < AGE_FRESH_MS) return 'fresh';
	if (age < AGE_AGING_MS) return 'aging';
	return 'stale';
}

/** '6m' | '17m' | '43m' | '—'. Never renders '0m' for a never-reported value. */
export function ageText(iso: string | null | undefined, now: number): string {
	if (!iso) return '—';
	const t = Date.parse(iso);
	if (Number.isNaN(t)) return '—';
	const mins = Math.max(0, Math.floor((now - t) / 60000));
	return `${mins}m`;
}

/**
 * NOAA Rothfusz regression heat index, °F. Returns null below 80°F or
 * without a relative-humidity reading — the backend does not ship a
 * heat-index field, so this is derived here, pure and unit-tested
 * (rideMeta.test.ts).
 */
export function heatIndexF(tempF: number, rh: number): number | null {
	if (tempF < 80 || rh == null || Number.isNaN(rh)) return null;
	const T = tempF;
	const R = rh;
	let hi =
		-42.379 +
		2.04901523 * T +
		10.14333127 * R -
		0.22475541 * T * R -
		0.00683783 * T * T -
		0.05481717 * R * R +
		0.00122874 * T * T * R +
		0.00085282 * T * R * R -
		0.00000199 * T * T * R * R;

	// Low relative-humidity adjustment (NWS): RH < 13% and 80 <= T <= 112.
	if (R < 13 && T >= 80 && T <= 112) {
		hi -= ((13 - R) / 4) * Math.sqrt((17 - Math.abs(T - 95)) / 17);
	}
	// High relative-humidity adjustment: RH > 85% and 80 <= T <= 87.
	if (R > 85 && T >= 80 && T <= 87) {
		hi += ((R - 85) / 10) * ((87 - T) / 5);
	}
	return Math.round(hi);
}

/** '4:12' for the NET zone — elapsed hours:minutes since `fromIso`. */
export function elapsedHM(fromIso: string | undefined, now: number): string {
	if (!fromIso) return '—';
	const t = Date.parse(fromIso);
	if (Number.isNaN(t)) return '—';
	const ms = Math.max(0, now - t);
	const totalMin = Math.floor(ms / 60000);
	const h = Math.floor(totalMin / 60);
	const m = totalMin % 60;
	return `${h}:${String(m).padStart(2, '0')}`;
}

/** The ONLY place a phase id becomes display text. */
export function phaseLabel(p: RidePhaseID | ''): string {
	return p ? p.toUpperCase() : '';
}

/** Canonical forward order — mirrors internal/ride/phase.Order. Read-only. */
export const RIDE_PHASE_ORDER: RidePhaseID[] = ['pre-start', 'launched', 'mid-ride', 'closing', 'collapse', 'reconcile'];

// --- SAG (internal/ride) display vocabulary ---

/** internal/ride's SAGLocation.Kind values, in on-air language. Pickup and
 * dropoff each use a subset (see GET /nets/{id}/sag/config's pickupKinds /
 * dropoffKinds) — never hardcode which subset applies, read it from there. */
export const SAG_LOCATION_KIND_LABELS: Record<string, string> = {
	course: 'On the course',
	reststop: 'Rest stop',
	checkpoint: 'Checkpoint',
	start: 'Start',
	finish: 'Finish',
	next_reststop: 'Next rest stop',
	hospital: 'Hospital',
	other: 'Other'
};

/** A location kind this annotation category answers for the picker. */
export const SAG_LOCATION_KIND_ANNOTATION_CATEGORY: Record<string, string> = {
	reststop: 'aid',
	checkpoint: 'checkpoint',
	start: 'start',
	finish: 'finish'
};

/** internal/ride's SAGRequest.Status values, in on-air language. */
export const SAG_STATUS_LABELS: Record<string, string> = {
	open: 'Open',
	assigned: 'Assigned',
	enroute: 'En route',
	onscene: 'On scene',
	transporting: 'Transporting',
	partial: 'Partial',
	complete: 'Complete',
	cancelled: 'Cancelled'
};

export const SAG_TERMINAL_STATUSES = new Set(['complete', 'cancelled']);

/** internal/ride's SAGSlot.Disposition values, in on-air language. */
export const SAG_DISPOSITION_LABELS: Record<string, string> = {
	waiting: 'Waiting',
	loaded: 'Loaded',
	delivered: 'Delivered',
	self_resolved: 'Fixed own flat, rode on',
	declined: 'Declined SAG',
	not_found: 'Not found on scene',
	handed_off: 'Handed off to medical',
	cancelled: 'Cancelled'
};

/**
 * internal/ride's bike dispositions, in on-air language. The bike is a
 * SEPARATE axis from the rider: the rider can be transported while the bike
 * stays at the rest stop, or travel in a different vehicle entirely. Only
 * 'with_rider' consumes one of the vehicle's rack slots.
 */
export const SAG_BIKE_LABELS: Record<string, string> = {
	with_rider: 'Bike on the rack',
	none: 'No bike',
	left_behind: 'Bike left behind',
	other_vehicle: 'Bike on another vehicle'
};

/**
 * The ONE display order for bike dispositions — mirrors
 * ride.BikeDispositionOrder, and every picker reads it rather than declaring
 * its own array, so the two can never drift.
 *
 * `with_rider` is first because it is the default and the overwhelmingly
 * common answer. `other_vehicle` is LAST and is never the default or the
 * first alternative an operator tabs into: shuttle trucks and bike-rack
 * trucks are real on very large rides but are not the norm, and a rare
 * option sitting high in a list is a mis-selection waiting to happen.
 */
export const BIKE_ORDER = ['with_rider', 'none', 'left_behind', 'other_vehicle'] as const;

/** The short form for a rider line, where the rider's name is already there. */
export const SAG_BIKE_SHORT: Record<string, string> = {
	with_rider: 'bike',
	none: 'no bike',
	left_behind: 'bike left behind',
	other_vehicle: 'bike separate'
};

/**
 * Geometric, monochrome, and never emoji-presentation — same rule the stop
 * glyphs follow. A bike that is NOT on the rack must be distinguishable from
 * one that is without relying on colour.
 */
export const SAG_BIKE_GLYPHS: Record<string, string> = {
	with_rider: '◉',
	none: '·',
	left_behind: '⊘',
	other_vehicle: '⇄'
};

/** Only a bike travelling with its rider occupies a rack. Mirrors ride.SlotTakesRack. */
export function slotTakesRack(slot: { bike?: string; hasBike?: boolean }): boolean {
	if (slot.bike && SAG_BIKE_LABELS[slot.bike]) return slot.bike === 'with_rider';
	return !!slot.hasBike;
}

/** Explicit disposition for a slot, falling back to the legacy boolean. */
export function slotBike(slot: { bike?: string; hasBike?: boolean }): string {
	if (slot.bike && SAG_BIKE_LABELS[slot.bike]) return slot.bike;
	return slot.hasBike ? 'with_rider' : 'none';
}

/**
 * The riders on a request who are standing at the roadside with nobody
 * coming for them. Mirrors ride.UnassignedRiders: NOT the count of waiting
 * slots, because a slot reserved on a leg already rolling is waiting but is
 * nobody's problem.
 */
export function unassignedRiders(r: { slots: { disposition: string; legId?: string }[] }): number {
	return r.slots.filter((s) => s.disposition === 'waiting' && !s.legId).length;
}

/**
 * The legs a LATE rider can still join — "SAG 4, make that TWO riders."
 * Mirrors ride.ActiveUnloadedLegs: a loaded van has physically left.
 */
export function activeUnloadedLegs<T extends { status: string }>(legs: T[]): T[] {
	return legs.filter((l) => l.status === 'dispatched' || l.status === 'enroute' || l.status === 'onscene');
}

/** internal/ride's SAGLeg.Status values, in on-air language. */
export const SAG_LEG_STATUS_LABELS: Record<string, string> = {
	dispatched: 'Dispatched',
	enroute: 'En route',
	onscene: 'On scene',
	loaded: 'Loaded',
	delivered: 'Delivered',
	released: 'Released'
};

// --- Supply / medical (internal/ride WP4) display vocabulary ---

export const SUPPLY_STATUS_LABELS: Record<string, string> = {
	draft: 'Draft',
	confirmed: 'Confirmed (transmitted)',
	relayed: 'Relayed',
	en_route: 'En route',
	delivered: 'Delivered',
	cancelled: 'Cancelled',
	merged: 'Merged'
};

export const SUPPLY_TERMINAL_STATUSES = new Set(['delivered', 'cancelled', 'merged']);

export const MEDICAL_STATUS_LABELS: Record<string, string> = {
	reported: 'Reported (awaiting read-back)',
	confirmed: 'Confirmed (transmitted)',
	ems_enroute: 'EMS en route',
	on_scene: 'On scene',
	departed: 'Departed',
	released: 'Released on scene',
	cancelled: 'Cancelled'
};

export const MEDICAL_TERMINAL_STATUSES = new Set(['departed', 'released', 'cancelled']);

export const MEDICAL_DESTINATION_LABELS: Record<string, string> = {
	hospital: 'Hospital',
	start: 'Start',
	finish: 'Finish',
	rest_stop: 'Rest stop',
	other: 'Other'
};
