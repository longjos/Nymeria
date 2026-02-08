import { writable, get } from 'svelte/store';

export type PanelMode = 'closed' | 'stations' | 'detail' | 'messages' | 'convo';
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
