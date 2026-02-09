import { writable, get } from 'svelte/store';

export type PanelMode = 'closed' | 'stations' | 'detail' | 'messages' | 'convo' | 'transports' | 'activity' | 'annotations' | 'netcontrol' | 'bulletins' | 'ics309';
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
			default: closePanel();
		}
	}
}
