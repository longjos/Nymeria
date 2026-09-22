// Ride-mode panel gating (WP7, part 2): which top-level UI a net's profile
// wants mounted. Pure data — no store imports — same discipline as
// wxAlertMeta.ts. The panel ids below are internal/netprofile's contract
// (Profile.panels); this file is the ONLY place that maps a backend panel id
// to a frontend target. Never hardcode a profile's panel list in a
// component — read netProfile.ts's `profilePanelIds` (from
// GET /nets/{id}/profile) and pass it through the functions here.
import type { PanelMode } from './stores/ui';

/**
 * Backend panel id -> where it lives in this frontend. `tab` is a hint for
 * components that hosts an internal sub-nav (NetControlPanel's tabs); the
 * panel-level gate is always just `mode`.
 *
 * Course and SAG/supply/medical live as NetControlPanel tabs, not separate
 * top-level panels (mirroring the precedent the SAG surface already set —
 * see NetControlPanel.svelte's `'sag'` tab, gated directly on `$rideMode`).
 * `ride-status` has no target here: the ride strip's own visibility is that
 * surface's concern, and it reads `profileHasPanel('ride-status')` directly.
 */
export const PANEL_ID_TARGETS: Record<string, { mode: PanelMode; tab?: string }> = {
	stations: { mode: 'stations' },
	messages: { mode: 'messages' },
	netcontrol: { mode: 'netcontrol' },
	annotations: { mode: 'annotations' },
	checkpoints: { mode: 'netcontrol', tab: 'locations' },
	weather: { mode: 'weather', tab: 'stations' },
	wxalerts: { mode: 'weather', tab: 'alerts' },
	telemetry: { mode: 'telemetry' },
	activity: { mode: 'activity' },
	'rest-stops': { mode: 'netcontrol', tab: 'course' },
	shutoffs: { mode: 'netcontrol', tab: 'course' },
	'sag-board': { mode: 'netcontrol', tab: 'sag' },
	supply: { mode: 'netcontrol', tab: 'sag' },
	medical: { mode: 'netcontrol', tab: 'sag' }
	// 'ride-status' intentionally has no target — see doc comment above.
};

/**
 * Every PanelMode named by at least one entry above is part of the profile
 * contract and CAN be hidden. A mode no registry panel list ever names (df,
 * packets, bulletins, ics309, settings, transports, detail, convo, closed,
 * …) is an app-level tool the backend profile system does not model, and
 * must never be hidden by it — hiding something the data never mentions
 * would be gating on silence, not on data.
 */
const GATED_MODES: ReadonlySet<PanelMode> = new Set(Object.values(PANEL_ID_TARGETS).map((t) => t.mode));

/**
 * Always visible regardless of profile: connection health and app settings
 * are not "APRS-heavy" net-control UI, and the mobile peek row's
 * ConnectionStatus button already opens Transports. Kept as an explicit set
 * (rather than relying only on GATED_MODES) so the reason is documented at
 * the call site, not just implied by an id list nobody wrote.
 */
export const INFRA_MODES: ReadonlySet<PanelMode> = new Set(['settings', 'transports']);

/**
 * `null` in (no net, or the profile hasn't loaded yet) -> `null` out, which
 * callers read as "no gating — show everything". Otherwise the Set of
 * PanelModes the given panel ids reach.
 */
export function visibleModesFor(panels: string[] | null): Set<PanelMode> | null {
	if (panels == null) return null;
	const set = new Set<PanelMode>();
	for (const id of panels) {
		const target = PANEL_ID_TARGETS[id];
		if (target) set.add(target.mode);
	}
	return set;
}

/**
 * Whether `mode` should render given a `visibleModesFor(...)` result. A mode
 * outside the profile contract (see GATED_MODES) or in INFRA_MODES is always
 * visible; otherwise it must be named by the current profile's panel ids.
 */
export function panelModeVisible(visible: Set<PanelMode> | null, mode: PanelMode): boolean {
	if (visible == null) return true;
	if (INFRA_MODES.has(mode)) return true;
	if (!GATED_MODES.has(mode)) return true;
	return visible.has(mode);
}

/** Whether a specific backend panel id is present in a profile's panel list
 * (or `null`, meaning "not loaded — assume present" so a slow profile fetch
 * never flashes UI away). Used for within-panel tab gating, e.g.
 * WeatherPanel's Stations tab or the future NetControlPanel Course tab's
 * own Shutoffs sub-section. */
export function hasPanel(panels: string[] | null, id: string): boolean {
	return panels == null || panels.includes(id);
}

/** The UI target a backend panel id maps to, if any. */
export function profileTargetFor(id: string): { mode: PanelMode; tab?: string } | undefined {
	return PANEL_ID_TARGETS[id];
}
