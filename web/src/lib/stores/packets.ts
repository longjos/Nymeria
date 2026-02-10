import { writable, derived, get } from 'svelte/store';
import type { RawPacket, APRSPacketType } from '$lib/types';
import { wsClient } from '$lib/stores/stations';

const MAX_PACKETS = 500;

export const packets = writable<RawPacket[]>([]);
export const inspectorPaused = writable(false);
export const packetTypeFilter = writable<APRSPacketType | ''>('');
export const callsignFilter = writable('');
export const sourceFilter = writable('');
export const totalPacketCount = writable(0);

export const filteredPackets = derived(
	[packets, packetTypeFilter, callsignFilter, sourceFilter],
	([$packets, $typeFilter, $callFilter, $srcFilter]) => {
		let result = $packets;
		if ($typeFilter) {
			result = result.filter((p) => p.packetType === $typeFilter);
		}
		if ($callFilter) {
			const q = $callFilter.toUpperCase();
			result = result.filter(
				(p) =>
					p.from.call.toUpperCase().includes(q) ||
					p.to.call.toUpperCase().includes(q)
			);
		}
		if ($srcFilter) {
			result = result.filter((p) => p.source === $srcFilter);
		}
		return result;
	}
);

let initialized = false;

export function initPacketStore(): void {
	if (initialized) return;
	initialized = true;

	wsClient.on('packet', (msg) => {
		const pkt = msg as unknown as RawPacket;
		totalPacketCount.update((n) => n + 1);

		if (get(inspectorPaused)) return;

		packets.update((list) => {
			const next = [pkt, ...list];
			if (next.length > MAX_PACKETS) next.length = MAX_PACKETS;
			return next;
		});
	});
}

export function clearPackets(): void {
	packets.set([]);
	totalPacketCount.set(0);
}
