import { writable } from 'svelte/store';
import { browser } from '$app/environment';
import type { WxMapMode } from '$lib/types';

export type StationAgeFilter = 'all' | '15m' | '30m' | '1h' | '2h' | '4h' | '8h';
export type TrackDuration = '30m' | '1h' | '2h' | '5h' | '12h' | '24h' | 'all';
/** How much of the SAG leg spaghetti to draw. Drawing every leg on a busy ride
 *  is unreadable; drawing none hides the assignment the operator just made. */
export type SagLegLines = 'selected' | 'all' | 'off';

export interface MapSettings {
	stationAgeFilter: StationAgeFilter;
	trackDuration: TrackDuration;
	showTracks: boolean;
	showDRCones: boolean;
	showCallsigns: boolean;
	showRosterOnly: boolean;
	showWeatherOverlay: boolean;
	showDFOverlay: boolean;
	/** NWS Alerts map rendering — map only, never changes what notifies (§10.3). */
	showWxAlerts: WxMapMode;
	/** SAG pickups, dropoffs and vehicle chits. ON by default: a ride net
	 *  exists to run SAG, and asking the operator to discover the toggle
	 *  during their first emergency is not a default. Non-ride nets never
	 *  mount the layer, so this costs them nothing. */
	showSagOverlay: boolean;
	sagLegLines: SagLegLines;
	showSagDock: boolean;
}

const DEFAULTS: MapSettings = {
	stationAgeFilter: 'all',
	trackDuration: 'all',
	showTracks: true,
	showDRCones: true,
	showCallsigns: false,
	showRosterOnly: false,
	showWeatherOverlay: false,
	showDFOverlay: false,
	showWxAlerts: 'watches',
	showSagOverlay: true,
	sagLegLines: 'selected',
	showSagDock: true,
};

const STORAGE_KEY = 'nymeria_map_settings';

function loadSettings(): MapSettings {
	if (!browser) return { ...DEFAULTS };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return { ...DEFAULTS };
		const saved = JSON.parse(raw) as Partial<MapSettings>;
		return { ...DEFAULTS, ...saved };
	} catch {
		return { ...DEFAULTS };
	}
}

export const mapSettings = writable<MapSettings>(loadSettings());

if (browser) {
	mapSettings.subscribe((val) => {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(val));
	});
}

export function updateMapSetting<K extends keyof MapSettings>(key: K, value: MapSettings[K]): void {
	mapSettings.update((s) => ({ ...s, [key]: value }));
}

/**
 * The `SAG focus` preset (docs/sag-map-spec.md §4) — one button that clears the
 * map of everything a SAG decision does not need, and a second press that puts
 * it all back exactly as the operator had it.
 *
 * The snapshot lives here rather than in a component so that the dock and
 * `MapPalette` cannot each hold their own idea of what "before" was. It is
 * deliberately NOT persisted: a returning operator should come back to their
 * real settings, not to a half-remembered filtered map.
 */
const SAG_FOCUS_PRESET: Partial<MapSettings> = {
	showTracks: false,
	showDRCones: false,
	showCallsigns: false,
	showWeatherOverlay: false,
	showDFOverlay: false,
	stationAgeFilter: '1h',
};

let preSagFocusSettings: MapSettings | null = null;

export const sagFocusPresetOn = writable<boolean>(false);

export function toggleSagFocusPreset(): void {
	if (preSagFocusSettings) {
		const restore = preSagFocusSettings;
		preSagFocusSettings = null;
		mapSettings.set(restore);
		sagFocusPresetOn.set(false);
		return;
	}
	mapSettings.update((s) => {
		preSagFocusSettings = { ...s };
		return { ...s, ...SAG_FOCUS_PRESET };
	});
	sagFocusPresetOn.set(true);
}

export const SAG_LEG_LINE_LABELS: Record<SagLegLines, string> = {
	selected: 'Selected',
	all: 'All',
	off: 'Off',
};

export const AGE_FILTER_MS: Record<StationAgeFilter, number> = {
	'all': Infinity,
	'15m': 15 * 60 * 1000,
	'30m': 30 * 60 * 1000,
	'1h': 60 * 60 * 1000,
	'2h': 2 * 60 * 60 * 1000,
	'4h': 4 * 60 * 60 * 1000,
	'8h': 8 * 60 * 60 * 1000,
};

export const TRACK_DURATION_MS: Record<TrackDuration, number> = {
	'30m': 30 * 60 * 1000,
	'1h': 60 * 60 * 1000,
	'2h': 2 * 60 * 60 * 1000,
	'5h': 5 * 60 * 60 * 1000,
	'12h': 12 * 60 * 60 * 1000,
	'24h': 24 * 60 * 60 * 1000,
	'all': Infinity,
};

export const AGE_FILTER_LABELS: Record<StationAgeFilter, string> = {
	'all': 'All',
	'15m': '15 min',
	'30m': '30 min',
	'1h': '1 hour',
	'2h': '2 hours',
	'4h': '4 hours',
	'8h': '8 hours',
};

export const TRACK_DURATION_LABELS: Record<TrackDuration, string> = {
	'30m': '30 min',
	'1h': '1 hour',
	'2h': '2 hours',
	'5h': '5 hours',
	'12h': '12 hours',
	'24h': '24 hours',
	'all': 'Full history',
};

export const WX_MAP_MODE_LABELS: Record<WxMapMode, string> = {
	off: 'Off',
	warnings: 'Warnings',
	watches: 'Warnings + watches',
	all: 'All',
};
