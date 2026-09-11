import { writable, get } from 'svelte/store';

export type PanelMode = 'closed' | 'stations' | 'detail' | 'messages' | 'convo' | 'transports' | 'activity' | 'annotations' | 'netcontrol' | 'bulletins' | 'ics309' | 'weather' | 'telemetry' | 'df' | 'packets' | 'settings';
export type DetailTab = 'info' | 'messages' | 'track';
export type SheetState = 'peek' | 'half' | 'full';
export type ConnectionState = 'connected' | 'disconnected' | 'reconnecting';

export const selectedStation = writable<string | null>(null);
export const panelMode = writable<PanelMode>('closed');
export const detailTab = writable<DetailTab>('info');
export const searchOpen = writable<boolean>(false);
export const searchQuery = writable<string>('');
export const sheetState = writable<SheetState>('peek');
export const connectionState = writable<ConnectionState>('disconnected');
export const commandPaletteOpen = writable<boolean>(false);

export function toggleCommandPalette(): void {
	commandPaletteOpen.update(v => !v);
}

export function selectStation(key: string): void {
	selectedStation.set(key);
	panelMode.set('detail');
	detailTab.set('info');
	sheetState.set('half');
}

export function closePanel(): void {
	panelMode.set('closed');
	selectedStation.set(null);
	sheetState.set('peek');
}

export function openStationList(): void {
	panelMode.set('stations');
	sheetState.set('half');
}

export function openMessages(): void {
	panelMode.set('messages');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openConversation(callsign: string): void {
	selectedStation.set(callsign);
	panelMode.set('convo');
	detailTab.set('messages');
	sheetState.set('half');
}

export function openTransports(): void {
	panelMode.set('transports');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openActivity(): void {
	panelMode.set('activity');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openAnnotations(): void {
	panelMode.set('annotations');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openNetControl(): void {
	panelMode.set('netcontrol');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openBulletins(): void {
	panelMode.set('bulletins');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openWeather(): void {
	panelMode.set('weather');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openTelemetry(): void {
	panelMode.set('telemetry');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openDF(): void {
	panelMode.set('df');
	selectedStation.set(null);
	sheetState.set('half');
}

export function openPackets(): void {
	panelMode.set('packets');
	selectedStation.set(null);
	sheetState.set('half');
}

/** Section key SettingsPanel should auto-expand and scroll to on next mount
 * (e.g. a "Settings →" hint link deep-linking into the what3words section).
 * Consumed (reset to null) by SettingsPanel once applied. */
export const settingsOpenSection = writable<string | null>(null);

/**
 * Snapshot of an in-progress mission draft in NetControlPanel, saved right
 * before navigating to Settings (e.g. the what3words "Settings →" hint
 * link). Settings and Net Control are mutually exclusive panels — opening
 * one unmounts the other — and NetControlPanel's mission-draft fields are
 * plain component $state with no other persistence, so without this the
 * whole draft (title, description, priority, assignee, chosen location)
 * would silently vanish while the NCS is just trying to add an API key.
 * Consumed (reset to null) by NetControlPanel's onMount once restored, and
 * scoped to the net it was captured in so it never leaks into a different
 * net's draft.
 */
export interface MissionDraftSnapshot {
	netId: string;
	title: string;
	desc: string;
	priority: string;
	assign: string;
	locLabel: string;
	locLat: number | null;
	locLon: number | null;
	locSource: string;
	locNearId: string | null;
	locWords: string;
	locNear: string;
	locConfirmed: boolean;
	/** Normalized Plus Code / MGRS string (#94) — '' for every other source. */
	locCode: string;
	selectedAnnotationIds: string[];
}
export const missionDraftBackup = writable<MissionDraftSnapshot | null>(null);

export function openSettings(section?: string): void {
	settingsOpenSection.set(section ?? null);
	panelMode.set('settings');
	selectedStation.set(null);
	sheetState.set('full');
}

export function openICS309(netId?: string): void {
	ics309NetId.set(netId ?? null);
	panelMode.set('ics309');
	selectedStation.set(null);
	sheetState.set('full');
}

export const ics309NetId = writable<string | null>(null);

/** Toggle a panel: if it's already open, close it; otherwise open it. */
export function togglePanel(mode: PanelMode): void {
	if (get(panelMode) === mode) {
		closePanel();
	} else {
		switch (mode) {
			case 'stations': openStationList(); break;
			case 'messages': openMessages(); break;
			case 'transports': openTransports(); break;
			case 'activity': openActivity(); break;
			case 'annotations': openAnnotations(); break;
			case 'netcontrol': openNetControl(); break;
			case 'bulletins': openBulletins(); break;
			case 'ics309': openICS309(); break;
			case 'weather': openWeather(); break;
			case 'telemetry': openTelemetry(); break;
			case 'df': openDF(); break;
			case 'packets': openPackets(); break;
			case 'settings': openSettings(); break;
			default: closePanel();
		}
	}
}
