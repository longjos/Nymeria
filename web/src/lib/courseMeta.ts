// Display vocabulary for the course-closure surface (WP7, part 2): the
// three-fact station ladder, shutoff status, and rider-exception kinds. Pure
// data, no store imports — same discipline as wxAlertMeta.ts / rideMeta.ts.
//
// Ages reuse rideMeta.ts's ageState/ageText (identical fresh/aging/stale/
// never thresholds — see docs/ride-strip-spec.md §6) rather than duplicating
// them here.
import type { StationClosureState, ShutoffStatus } from './types';

export interface StationLadderMeta {
	label: string;
	word: string;
	/** 16x16 SVG path, same shape contract as RideTierGlyph's glyphs. */
	glyph: string;
	glyphFill: boolean;
	colorVar: string;
}

/** Keyed by store.StationClosure.State. Labels are the ladder words shown
 * next to every glyph — never invent a synonym elsewhere in the UI. */
export const stationLadderMeta: Record<StationClosureState, StationLadderMeta> = {
	open: {
		label: 'Open',
		word: 'Open',
		// open diamond
		glyph: 'M8 1.5l6.5 6.5L8 14.5 1.5 8z',
		glyphFill: false,
		colorVar: '--color-text-muted'
	},
	riders_clear: {
		label: 'Riders clear',
		word: 'Riders clear',
		// filled diamond with a bar — "awaiting sweep"
		glyph: 'M8 1.5l6.5 6.5L8 14.5 1.5 8z M5.5 8h5',
		glyphFill: true,
		colorVar: '--color-warning'
	},
	sweep_passed: {
		label: 'Sweep passed',
		word: 'Sweep passed',
		// filled diamond with a check
		glyph: 'M8 1.5l6.5 6.5L8 14.5 1.5 8z M5.5 8.2l1.7 1.7 3.3-3.6',
		glyphFill: true,
		// StationRow tints the ladder WORD with this, not just the glyph, so
		// it has to clear AA as text: --color-info is 4.32:1 (app.css §text-safe).
		colorVar: '--color-info-text'
	},
	closed: {
		label: 'Closed',
		word: 'Closed',
		// circle-slash
		glyph: 'M8 2a6 6 0 100 12 6 6 0 000-12z M3.5 3.5l9 9',
		glyphFill: false,
		colorVar: '--color-success'
	}
};

export interface ShutoffStatusMeta {
	label: string;
	glyph: string;
	dashed: boolean;
	colorVar: string;
}

/** Keyed by store.ShutoffPoint.Status. Same gate glyph for all three — the
 * dashed rail (fired) and strike-through text (cancelled) carry the rest. */
export const shutoffStatusMeta: Record<ShutoffStatus, ShutoffStatusMeta> = {
	planned: { label: 'Planned', glyph: 'M8 1v14 M3 5h10 M3 11h10', dashed: false, colorVar: '--color-text' },
	// Same text-safety rule as stationLadderMeta: ShutoffBoard colours the
	// status word with colorVar, and --color-error is 4.16:1.
	fired: { label: 'Fired', glyph: 'M8 1v14 M3 5h10 M3 11h10', dashed: true, colorVar: '--color-error-text' },
	cancelled: { label: 'Cancelled', glyph: 'M8 1v14 M3 5h10 M3 11h10', dashed: false, colorVar: '--color-text-muted' }
};

/** internal/course's RiderException.Kind values (store.go, course.go
 * ValidKinds). `short` is used where a chip has no room for the full label. */
export type RiderExceptionKind = 'sag' | 'medical' | 'dnf' | 'declined_sag' | 'shutoff_reroute' | 'other';

export const riderKindMeta: Record<RiderExceptionKind, { label: string; short: string }> = {
	sag: { label: "SAG'd", short: 'SAG' },
	medical: { label: 'Medical', short: 'Med' },
	dnf: { label: 'DNF', short: 'DNF' },
	declined_sag: { label: 'Declined SAG', short: 'Decl.' },
	shutoff_reroute: { label: 'Rerouted at shutoff', short: 'Reroute' },
	other: { label: 'Other', short: 'Other' }
};

export function riderKindLabel(kind: string): string {
	return riderKindMeta[kind as RiderExceptionKind]?.label ?? kind;
}
