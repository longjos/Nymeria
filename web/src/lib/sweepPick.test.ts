import { describe, it, expect } from 'vitest';
import { sweepCandidates, filterStops } from './sweepPick';
import type { StationView } from './types';

function st(seq: number, label: string, state: StationView['closure']['state'] = 'open'): StationView {
	return {
		checkpointId: `cp${seq}`,
		label,
		category: 'aid',
		sequenceNumber: seq,
		outOfOrder: false,
		passageCount: 0,
		closure: { netId: 'n', checkpointId: `cp${seq}`, division: '', state, ridersClearBy: '', sweepPassedBy: '' }
	} as StationView;
}

const course = [
	st(3, 'Rest Stop Eakin Elementary', 'riders_clear'),
	st(1, 'Start — Shelbyville'),
	st(2, 'Rest Stop Maxwell Chapel', 'sweep_passed'),
	st(4, 'Rest Stop Flat Creek'),
	st(5, 'Finish', 'closed')
];

describe('sweepCandidates', () => {
	it('offers only stops the sweep has not passed, in course order', () => {
		expect(sweepCandidates(course).map((s) => s.sequenceNumber)).toEqual([1, 3, 4]);
	});
});

describe('filterStops', () => {
	const c = sweepCandidates(course);

	it('an empty query keeps everything', () => {
		expect(filterStops(c, '  ')).toHaveLength(3);
	});

	it('matches any word of the name, case-insensitively', () => {
		expect(filterStops(c, 'flat').map((s) => s.sequenceNumber)).toEqual([4]);
		expect(filterStops(c, 'EAKIN elem').map((s) => s.sequenceNumber)).toEqual([3]);
	});

	it('a bare number or #number is the stop number, exactly — "3" is not "13"', () => {
		const many = sweepCandidates([...course, st(13, 'Rest Stop Wartrace')]);
		expect(filterStops(many, '3').map((s) => s.sequenceNumber)).toEqual([3]);
		expect(filterStops(many, '#13').map((s) => s.sequenceNumber)).toEqual([13]);
	});

	it('a number inside a name still matches as text', () => {
		const withNum = [st(7, 'Hwy 64 Church')];
		expect(filterStops(withNum, 'hwy 64')).toHaveLength(1);
	});
});
