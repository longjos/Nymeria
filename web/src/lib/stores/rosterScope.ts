import { derived } from 'svelte/store';
import { activeNet, activeCheckIns, operatorsWithPosition } from './netcontrol';
import { stationList } from './stations';
import { mapSettings } from './mapSettings';
import { stationKey } from '$lib/utils';

/**
 * "Roster only" is an APP-WIDE scope, not a map setting (#106). The toggle lives
 * in the map palette because that is where an operator first reaches for it, but
 * every station surface — map, station list, inline search, command palette —
 * reads the same derived scope from here. A list of 300 stations next to a map of
 * 12 is the bug this module exists to prevent.
 */

/**
 * Roster callsigns for the currently OPEN net: the checked-in operators plus the
 * tracked devices linked to them (an operator checked in as W4ABC may have their
 * position arriving from W4ABC-9). Tracked callsigns are stored verbatim and may
 * carry an SSID, so membership is tested against both a station's bare callsign
 * and its SSID-suffixed key — see isRosterStation.
 *
 * Scoped to the open net by id: the netcontrol store keeps the previous net's
 * check-ins in $checkIns until the new net's data loads, so an unscoped roster
 * would scope the app to the *last* net's operators.
 *
 * Identity stability matters here. Any check-in change (status, note, roll call,
 * position) re-emits $activeCheckIns, and a fresh Set would invalidate every
 * downstream $derived — rebuilding every map marker and re-filtering every list
 * on a packet that did not change roster membership. The Set is only ever read
 * via .has(), so returning the cached instance is indistinguishable from a fresh
 * one.
 */
let prevRoster: Set<string> | null = null;
export const rosterCallsigns = derived(
	[activeNet, activeCheckIns],
	([$net, $checkIns], set) => {
		const next = new Set<string>();
		const openNetId = $net?.status === 'open' ? $net.id : null;
		for (const ci of openNetId ? $checkIns : []) {
			if (ci.netId !== openNetId) continue;
			next.add(ci.callsign);
			for (const ts of ci.trackedStations ?? []) next.add(ts.callsign);
		}
		if (prevRoster && prevRoster.size === next.size) {
			let same = true;
			for (const c of next) {
				if (!prevRoster.has(c)) { same = false; break; }
			}
			// Returning the cached instance is NOT enough to stop the cascade:
			// Svelte's safe_not_equal reports every object value as changed, so a
			// plain `return prevRoster` still notifies every subscriber. Emitting
			// nothing is the only way to actually suppress the downstream
			// recomputation this cache exists for.
			if (same) return;
		}
		prevRoster = next;
		set(next);
	},
	new Set<string>()
);

/**
 * The scope only applies while a net is actually running. Must test the status,
 * not just for a net object: the netcontrol store keeps the closed net in
 * $activeNet after a net_updated close event, so `$activeNet !== null` stays
 * true and yesterday's roster would keep filtering today's netless session.
 */
export const netIsOpen = derived(activeNet, ($net) => $net?.status === 'open');

/** Is this station on the roster? Tests the bare callsign and the SSID key. */
export function isRosterStation(
	roster: Set<string>,
	station: { callsign: string; ssid: number }
): boolean {
	return roster.has(station.callsign) || roster.has(stationKey(station));
}

/**
 * What the scope would actually leave visible, counted in mapped stations rather
 * than raw check-ins: a roster of voice-only operators has check-ins but nothing
 * on the map, and enabling the scope on that would blank every surface.
 */
export const rosterMappedCount = derived(
	[netIsOpen, rosterCallsigns, stationList, activeNet, operatorsWithPosition],
	([$netIsOpen, $roster, $stations, $net, $operators]) => {
		if (!$netIsOpen) return 0;
		let n = 0;
		for (const s of $stations) {
			if (s.position && isRosterStation($roster, s)) n++;
		}
		// Manually placed check-ins draw on their own (unfiltered) operator layer,
		// so they keep the map populated even with no roster station heard.
		const openNetId = $net?.id ?? null;
		for (const ci of $operators) {
			if (ci.netId === openNetId) n++;
		}
		return n;
	}
);

export const rosterFilterAvailable = derived(
	[netIsOpen, rosterMappedCount],
	([$netIsOpen, $count]) => $netIsOpen && $count > 0
);

/** The persisted toggle AND a roster worth filtering to. */
export const rosterFilterActive = derived(
	[mapSettings, rosterFilterAvailable],
	([$settings, $available]) => $settings.showRosterOnly && $available
);

/**
 * Convenience for list surfaces: the roster subset of every heard station, plus
 * how many the scope is hiding, so each surface can say so out loud. When the
 * scope is off this passes $stationList straight through — identity included, so
 * nothing downstream recomputes on a netless day.
 */
export const rosterScopedStations = derived(
	[rosterFilterActive, rosterCallsigns, stationList],
	([$active, $roster, $stations]) => {
		if (!$active) return { stations: $stations, hidden: 0, scoped: false };
		const kept = $stations.filter((s) => isRosterStation($roster, s));
		return { stations: kept, hidden: $stations.length - kept.length, scoped: true };
	}
);
