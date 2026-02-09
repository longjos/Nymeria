import { writable, derived } from 'svelte/store';
import type { Bulletin, Message } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';

export const bulletins = writable<Bulletin[]>([]);

export const bulletinList = derived(bulletins, ($b) =>
	[...$b].sort((a, b) => {
		if (a.bulletinId !== b.bulletinId) return a.bulletinId.localeCompare(b.bulletinId);
		return a.from.localeCompare(b.from);
	})
);

export const announcements = derived(bulletins, ($b) =>
	$b.filter((b) => b.isAnnouncement)
);

export const regularBulletins = derived(bulletins, ($b) =>
	$b.filter((b) => !b.isAnnouncement)
);

let initialized = false;

function isBulletinAddressed(msg: Message): boolean {
	return msg.to?.startsWith('BLN') || msg.to?.startsWith('ANN');
}

function refreshBulletins(): void {
	api.bulletins().then((list) => {
		bulletins.set(list ?? []);
	}).catch(() => {});
}

export function initBulletinStore(): void {
	if (initialized) return;
	initialized = true;

	refreshBulletins();

	wsClient.on('message_received', (data) => {
		const m = data.message as Message;
		if (m && isBulletinAddressed(m)) {
			refreshBulletins();
		}
	});

	wsClient.on('message_sent', (data) => {
		const m = data.message as Message;
		if (m && isBulletinAddressed(m)) {
			refreshBulletins();
		}
	});
}
