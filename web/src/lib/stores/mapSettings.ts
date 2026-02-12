import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export type StationAgeFilter = 'all' | '15m' | '30m' | '1h' | '2h' | '4h' | '8h';
export type TrackDuration = '30m' | '1h' | '2h' | '5h' | '12h' | '24h' | 'all';

export interface MapSettings {
	stationAgeFilter: StationAgeFilter;
	trackDuration: TrackDuration;
	showTracks: boolean;
	showDRCones: boolean;
	showWeatherOverlay: boolean;
	showDFOverlay: boolean;
}

const DEFAULTS: MapSettings = {
	stationAgeFilter: 'all',
	trackDuration: 'all',
	showTracks: true,
	showDRCones: true,
	showWeatherOverlay: false,
	showDFOverlay: false,
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
