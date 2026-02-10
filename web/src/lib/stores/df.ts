import { derived } from 'svelte/store';
import { stations } from './stations';
import type { Station } from '$lib/types';

/** Stations that have DF data and a position, derived from the main stations store. */
export const dfStations = derived(stations, ($stations) => {
	const result: Station[] = [];
	for (const s of $stations.values()) {
		if (s.df && s.position) {
			result.push(s);
		}
	}
	return result.sort((a, b) => a.callsign.localeCompare(b.callsign));
});
