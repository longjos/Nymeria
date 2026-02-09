import { writable, derived } from 'svelte/store';
import { api } from '$lib/api';
import { wsClient } from '$lib/stores/stations';
import type { TacticalAlias } from '$lib/types';

// Map of callsign → TacticalAlias
export const tacticalAliases = writable<Map<string, TacticalAlias>>(new Map());

// Derived helper: returns a lookup function
export const getTacticalAlias = derived(tacticalAliases, ($aliases) => {
	return (callsign: string): string | null => {
		const a = $aliases.get(callsign);
		return a ? a.alias : null;
	};
});

// Derived list for iteration
export const tacticalAliasList = derived(tacticalAliases, ($aliases) =>
	Array.from($aliases.values()).sort((a, b) => a.callsign.localeCompare(b.callsign))
);

let initialized = false;

export function initTacticalStore() {
	if (initialized) return;
	initialized = true;

	api.tacticalAliases().then((aliases) => {
		const map = new Map<string, TacticalAlias>();
		for (const a of aliases) {
			map.set(a.callsign, a);
		}
		tacticalAliases.set(map);
	}).catch(() => {
		// Silently fail on initial load
	});

	wsClient.on('tactical_set', (msg) => {
		const alias = msg.data as TacticalAlias;
		if (!alias?.callsign) return;
		tacticalAliases.update((m) => {
			const next = new Map(m);
			next.set(alias.callsign, alias);
			return next;
		});
	});

	wsClient.on('tactical_removed', (msg) => {
		const callsign = (msg.data as { callsign?: string })?.callsign;
		if (!callsign) return;
		tacticalAliases.update((m) => {
			const next = new Map(m);
			next.delete(callsign);
			return next;
		});
	});
}
