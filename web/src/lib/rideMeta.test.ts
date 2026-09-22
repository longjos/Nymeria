import { describe, it, expect } from 'vitest';
import {
	tierStyle,
	tierById,
	highestTier,
	STOP_GLYPHS,
	enumLabel,
	ageState,
	ageText,
	heatIndexF,
	elapsedHM,
	phaseLabel,
	RIDE_PHASE_ORDER,
	SAG_REASON_LABELS,
	AGE_FRESH_MS,
	AGE_AGING_MS,
	SAG_BIKE_LABELS,
	SAG_BIKE_SHORT,
	SAG_BIKE_GLYPHS,
	slotTakesRack,
	slotBike,
	unassignedRiders,
	activeUnloadedLegs,
	BIKE_ORDER
} from './rideMeta';
import type { PriorityTier } from './types';

function tier(rank: number, label: string, id = label.toLowerCase()): PriorityTier {
	return { id, label, rank, description: '' } as PriorityTier;
}

// The shipped Marin ARS ladder, used only as A ladder — never as THE ladder.
const marin = [
	tier(1, 'EMERGENCY'),
	tier(2, 'PRIORITY'),
	tier(3, 'HIGH'),
	tier(4, 'MEDIUM'),
	tier(5, 'LOW')
];

describe('tierStyle', () => {
	it('keys style by RANK, never by label — an agency can rename every tier', () => {
		const renamed = [
			tier(1, 'MAYDAY', 't1'),
			tier(2, 'URGENT', 't2'),
			tier(3, 'ROUTINE', 't3')
		];
		expect(tierStyle(renamed[0]).colorVar).toBe(tierStyle(marin[0]).colorVar);
		expect(tierStyle(renamed[1]).glyph).toBe(tierStyle(marin[1]).glyph);
		expect(tierStyle(renamed[2]).glyphFill).toBe(tierStyle(marin[2]).glyphFill);
	});

	it('gives rank 1 the only red, and rank 1 alone', () => {
		expect(tierStyle(marin[0]).colorVar).toBe('--color-ride-emergency');
		for (const t of marin.slice(1)) {
			expect(tierStyle(t).colorVar).not.toBe('--color-ride-emergency');
		}
	});

	// --color-ride-priority and --color-ride-high are separate token NAMES
	// that both alias --color-warning (app.css), so ranks 2 and 3 land on the
	// same amber and only the fill and the code tell them apart.
	it('separates ranks 2 and 3 by fill and code, not by hue', () => {
		const pr = tierStyle(marin[1]);
		const hi = tierStyle(marin[2]);
		expect(pr.colorVar).toBe('--color-ride-priority');
		expect(hi.colorVar).toBe('--color-ride-high');
		expect(pr.glyph).toBe(hi.glyph);
		expect(pr.glyphFill).toBe(true);
		expect(hi.glyphFill).toBe(false);
		expect(pr.code).not.toBe(hi.code);
	});

	// The code is the first two letters of the tier's OWN label, because the
	// label is agency data. That yields ME for MEDIUM where the spec's
	// example table writes MD — a deliberate trade: a name-keyed override
	// table would reintroduce exactly the hardcoded ladder rule 5 forbids.
	it('pairs every tier with a distinct two-letter code — the non-colour channel', () => {
		const codes = marin.map((t) => tierStyle(t).code);
		expect(codes).toEqual(['EM', 'PR', 'HI', 'ME', 'LO']);
		expect(new Set(codes).size).toBe(codes.length);
		expect(tierStyle(tier(2, 'Urgent')).code).toBe('UR');
	});

	it('falls back to ?? rather than an empty code for an unlabelled tier', () => {
		expect(tierStyle(tier(3, '')).code).toBe('??');
	});

	it('points softVar at its OWN hue, not at the red wash', () => {
		for (const t of marin) {
			const s = tierStyle(t);
			expect(s.softVar).toMatch(/^--color-ride-[a-z]+-soft$/);
		}
		expect(tierStyle(marin[3]).softVar).toBe('--color-ride-medium-soft');
		expect(tierStyle(marin[4]).softVar).toBe('--color-ride-low-soft');
	});

	it('treats any rank past 5 as the lowest tier', () => {
		expect(tierStyle(tier(9, 'INFO')).colorVar).toBe('--color-ride-low');
	});
});

describe('tierById / highestTier', () => {
	it('resolves by id', () => {
		expect(tierById(marin, 'high')?.label).toBe('HIGH');
		expect(tierById(marin, 'nope')).toBeUndefined();
	});

	it('picks the LOWEST rank number as highest urgency', () => {
		expect(highestTier(marin, ['low', 'priority', 'medium'])?.label).toBe('PRIORITY');
		expect(highestTier(marin, [])).toBeNull();
		expect(highestTier(marin, ['not-a-tier'])).toBeNull();
	});
});

describe('STOP_GLYPHS', () => {
	it('gives every stop state a distinct mark', () => {
		const marks = Object.values(STOP_GLYPHS);
		expect(new Set(marks).size).toBe(marks.length);
	});

	// U+2300–U+23FF (Misc Technical) holds HOURGLASS/STOPWATCH, which carry
	// emoji presentation: tofu on the stock Linux font stack, a colour emoji
	// on macOS/Windows. The set must stay in Geometric Shapes.
	it('uses no emoji-presentation codepoint (they render as tofu on Linux)', () => {
		for (const g of Object.values(STOP_GLYPHS)) {
			const cp = g.codePointAt(0)!;
			expect(cp < 0x2300 || cp > 0x23ff).toBe(true);
		}
	});
});

describe('ageState / ageText', () => {
	const now = Date.parse('2026-09-22T12:00:00Z');
	const ago = (ms: number) => new Date(now - ms).toISOString();

	it('names `never` separately from `stale` and never renders 0', () => {
		expect(ageState(null, now)).toBe('never');
		expect(ageState(undefined, now)).toBe('never');
		expect(ageState('not a date', now)).toBe('never');
		expect(ageText(null, now)).toBe('—');
		expect(ageText(undefined, now)).toBe('—');
	});

	it('walks fresh -> aging -> stale at the documented thresholds', () => {
		expect(ageState(ago(0), now)).toBe('fresh');
		expect(ageState(ago(AGE_FRESH_MS - 1), now)).toBe('fresh');
		expect(ageState(ago(AGE_FRESH_MS), now)).toBe('aging');
		expect(ageState(ago(AGE_AGING_MS - 1), now)).toBe('aging');
		expect(ageState(ago(AGE_AGING_MS), now)).toBe('stale');
	});

	it('reports whole minutes and never a negative age', () => {
		expect(ageText(ago(6 * 60_000), now)).toBe('6m');
		expect(ageText(ago(90_000), now)).toBe('1m');
		expect(ageText(new Date(now + 60_000).toISOString(), now)).toBe('0m');
	});
});

describe('heatIndexF', () => {
	it('returns null below 80F — the regression is undefined there', () => {
		expect(heatIndexF(79, 50)).toBeNull();
		expect(heatIndexF(70, 90)).toBeNull();
	});

	it('matches the NWS table within a degree', () => {
		// NWS heat index chart: 90F/70% ~= 105F, 100F/40% ~= 109F.
		expect(heatIndexF(90, 70)).toBeGreaterThanOrEqual(104);
		expect(heatIndexF(90, 70)).toBeLessThanOrEqual(106);
		expect(heatIndexF(100, 40)).toBeGreaterThanOrEqual(108);
		expect(heatIndexF(100, 40)).toBeLessThanOrEqual(110);
	});

	it('applies the low-humidity adjustment downward', () => {
		const adjusted = heatIndexF(95, 10)!;
		const unadjusted = heatIndexF(95, 14)!;
		expect(adjusted).toBeLessThan(unadjusted);
	});

	it('applies the high-humidity adjustment upward', () => {
		expect(heatIndexF(85, 90)!).toBeGreaterThan(heatIndexF(85, 85)!);
	});

	it('survives a missing humidity reading', () => {
		expect(heatIndexF(95, NaN)).toBeNull();
	});
});

describe('elapsedHM', () => {
	const now = Date.parse('2026-09-22T12:00:00Z');

	it('renders h:mm with a zero-padded minute', () => {
		expect(elapsedHM('2026-09-22T07:48:00Z', now)).toBe('4:12');
		expect(elapsedHM('2026-09-22T11:55:00Z', now)).toBe('0:05');
	});

	it('shows an em dash, not 0:00, when no net is open', () => {
		expect(elapsedHM(undefined, now)).toBe('—');
		expect(elapsedHM('nonsense', now)).toBe('—');
	});

	it('clamps a clock-skewed future start to 0:00 rather than going negative', () => {
		expect(elapsedHM('2026-09-22T12:30:00Z', now)).toBe('0:00');
	});
});

describe('phase vocabulary', () => {
	it('is the only place a phase id becomes display text', () => {
		expect(phaseLabel('mid-ride')).toBe('MID-RIDE');
		expect(phaseLabel('')).toBe('');
	});

	it('lists every phase once, in forward order', () => {
		expect(RIDE_PHASE_ORDER).toEqual(['pre-start', 'launched', 'mid-ride', 'closing', 'collapse', 'reconcile']);
		expect(new Set(RIDE_PHASE_ORDER).size).toBe(RIDE_PHASE_ORDER.length);
	});
});

describe('enumLabel', () => {
	it('prefers the table, then title-cases the raw enum', () => {
		expect(enumLabel('medical_minor', SAG_REASON_LABELS)).toBe('Medical (minor)');
		expect(enumLabel('rider_waving', SAG_REASON_LABELS)).toBe('Rider waving');
		expect(enumLabel(undefined)).toBe('');
		expect(enumLabel('')).toBe('');
	});
});

// --- bike disposition: a separate axis from rider disposition ---

describe('slotTakesRack', () => {
	it('only counts a bike travelling with its rider', () => {
		expect(slotTakesRack({ bike: 'with_rider' })).toBe(true);
		expect(slotTakesRack({ bike: 'none', hasBike: true })).toBe(false);
		expect(slotTakesRack({ bike: 'left_behind', hasBike: true })).toBe(false);
		expect(slotTakesRack({ bike: 'other_vehicle', hasBike: true })).toBe(false);
	});

	it('falls back to the legacy boolean for a slot written before the axis existed', () => {
		expect(slotTakesRack({ hasBike: true })).toBe(true);
		expect(slotTakesRack({ hasBike: false })).toBe(false);
		expect(slotTakesRack({ bike: '', hasBike: true })).toBe(true);
	});
});

describe('slotBike', () => {
	it('never returns an empty disposition the UI would have to guess at', () => {
		expect(slotBike({ hasBike: true })).toBe('with_rider');
		expect(slotBike({ hasBike: false })).toBe('none');
		expect(slotBike({ bike: 'left_behind', hasBike: true })).toBe('left_behind');
		expect(slotBike({ bike: 'nonsense', hasBike: false })).toBe('none');
	});

	it('has a label, a short form and a glyph for every disposition', () => {
		for (const d of ['with_rider', 'none', 'left_behind', 'other_vehicle']) {
			expect(SAG_BIKE_LABELS[d]).toBeTruthy();
			expect(SAG_BIKE_SHORT[d]).toBeTruthy();
			expect(SAG_BIKE_GLYPHS[d]).toBeTruthy();
		}
	});
});

// --- "who is waiting for a ride" ---

describe('unassignedRiders', () => {
	it('counts only riders with nobody coming for them', () => {
		expect(unassignedRiders({ slots: [{ disposition: 'waiting' }] })).toBe(1);
		// Reserved on a leg that is already rolling: waiting, but not waiting on NCS.
		expect(unassignedRiders({ slots: [{ disposition: 'waiting', legId: 'leg1' }] })).toBe(0);
		expect(unassignedRiders({ slots: [{ disposition: 'loaded', legId: 'leg1' }] })).toBe(0);
		expect(unassignedRiders({ slots: [{ disposition: 'delivered' }] })).toBe(0);
	});

	it('counts the rider added after the van already loaded and left', () => {
		expect(
			unassignedRiders({
				slots: [
					{ disposition: 'loaded', legId: 'leg1' },
					{ disposition: 'waiting' }
				]
			})
		).toBe(1);
	});
});

describe('activeUnloadedLegs', () => {
	it('offers the legs a late rider can still join', () => {
		const legs = [
			{ id: 'a', status: 'dispatched' },
			{ id: 'b', status: 'enroute' },
			{ id: 'c', status: 'onscene' },
			{ id: 'd', status: 'loaded' },
			{ id: 'e', status: 'delivered' },
			{ id: 'f', status: 'released' }
		];
		expect(activeUnloadedLegs(legs).map((l) => l.id)).toEqual(['a', 'b', 'c']);
	});
});

describe('BIKE_ORDER', () => {
	it('defaults to the common answer and buries the rare one', () => {
		// with_rider is the default and by far the most common; other_vehicle
		// (shuttles, bike-rack trucks) is real on very large rides but rare, so
		// it must never be the default or the first alternative tabbed into.
		expect(BIKE_ORDER[0]).toBe('with_rider');
		expect(BIKE_ORDER[BIKE_ORDER.length - 1]).toBe('other_vehicle');
		expect(BIKE_ORDER[1]).not.toBe('other_vehicle');
	});

	it('covers every disposition exactly once', () => {
		expect([...BIKE_ORDER].sort()).toEqual(Object.keys(SAG_BIKE_LABELS).sort());
		expect(new Set(BIKE_ORDER).size).toBe(BIKE_ORDER.length);
	});
});
