/**
 * The "Mark sweep passed" picker's list logic. Pure, so the dialog only lays
 * out what this decides.
 */
import type { StationView } from './types';

/** Stops the sweep can still be marked past, in course order. A stop already
 *  swept or closed is not offered: marking it again would move the sweep
 *  marker BACKWARDS on the most-asked-about number in the operation. */
export function sweepCandidates(stations: StationView[]): StationView[] {
	return stations
		.filter((s) => s.closure.state !== 'sweep_passed' && s.closure.state !== 'closed')
		.sort((a, b) => a.sequenceNumber - b.sequenceNumber);
}

/**
 * Narrow by what the operator types. A bare number ("3", "#3") is the stop
 * NUMBER, matched exactly — that is how stops are called on the radio, and
 * "3" must not also find stop 13. Anything else matches when every typed word
 * appears somewhere in the name, in any order and any case.
 */
export function filterStops(stops: StationView[], query: string): StationView[] {
	const q = query.trim().toLowerCase();
	if (!q) return stops;
	const num = /^#?(\d+)$/.exec(q);
	if (num) return stops.filter((s) => s.sequenceNumber === Number(num[1]));
	const words = q.split(/\s+/);
	return stops.filter((s) => {
		const name = s.label.toLowerCase();
		return words.every((w) => name.includes(w));
	});
}
