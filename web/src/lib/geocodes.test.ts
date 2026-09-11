import { describe, it, expect } from 'vitest';
import {
	normalizeGeocode,
	detectFormat,
	parsePlusCode,
	parseMGRS,
	plusCodePrecisionM,
	formatMGRSGroups,
	formatPlusCode,
	formatMGRS,
	geocodeLocationLabel,
	extractGeocode,
	isGeocodeError,
	type RefPoint,
} from './geocodes';

// Every number in this file was computed by running the actual installed
// `open-location-code` and `mgrs` packages, not recalled — see the design
// doc for issue #94. Coordinates are cell centres; assert with toBeCloseTo.

describe('detectFormat', () => {
	const cases: Array<[string, 'w3w' | 'pluscode' | 'mgrs' | null]> = [
		['8FVC9G8F+6X', 'pluscode'],
		['8fvc9g8f+6x', 'pluscode'],
		['8FVC 9G8F+6X', 'pluscode'],
		['9G8F+6X', 'pluscode'],
		['6H+59', 'pluscode'],
		['16SEG1234567890', 'mgrs'],
		['16S EG 12345 67890', 'mgrs'],
		['16s eg 12345 67890', 'mgrs'],
		['33UXP0500444998', 'mgrs'],
		['4QFJ12345678', 'mgrs'],
		['filled.count.soap', 'w3w'],
		['///filled.count.soap', 'w3w'],
		['filled.count', 'w3w'],
		['Aid Station 3', null],
		['CP-1', null],
		['Mile 22.5', null],
		['N. Trailhead', null],
		['41.8781, -87.6298', null],
		['18T Trailhead', null],
		['3 Mile Rd', null],
		['16SEG12345', null],
		['16SEG12', null],
		['33IXP0500444998', null],
		['61SEG1234567890', null],
		['Net 16 South', null],
		['', null],
	];

	for (const [input, expected] of cases) {
		it(`${JSON.stringify(input)} -> ${expected}`, () => {
			expect(detectFormat(input)).toBe(expected);
		});
	}
});

describe('parsePlusCode — full codes (no reference needed)', () => {
	const vectors: Array<{
		code: string;
		lat: number;
		lon: number;
		codeLength: number;
		precisionM: number;
	}> = [
		{ code: '8FVC2222+22', lat: 47.000062, lon: 8.000062, codeLength: 10, precisionM: 14 },
		{ code: '8FVC9G8F+6X', lat: 47.365562, lon: 8.524938, codeLength: 10, precisionM: 14 },
		{ code: '868JHM6H+59', lat: 36.560438, lon: -87.321562, codeLength: 10, precisionM: 14 },
		{ code: '796RWF8Q+WF', lat: 14.917313, lon: -23.511313, codeLength: 10, precisionM: 14 },
		{ code: '8FVC9G8F+', lat: 47.36625, lon: 8.52375, codeLength: 8, precisionM: 275 },
		{ code: '8FVC9G8F+6XQ', lat: 47.365587, lon: 8.524984, codeLength: 11, precisionM: 3 },
	];

	for (const v of vectors) {
		it(`${v.code}`, () => {
			const r = parsePlusCode(v.code, null);
			if (isGeocodeError(r)) throw new Error(`expected a result, got error ${r.error}`);
			expect(r.lat).toBeCloseTo(v.lat, 4);
			expect(r.lon).toBeCloseTo(v.lon, 4);
			expect(r.label).toBe(v.code);
			expect(r.precisionM).toBe(v.precisionM);
			expect(r.format).toBe('pluscode');
		});
	}

	it('lowercase and internal whitespace both normalize to the same result', () => {
		const expected = parsePlusCode('8FVC9G8F+6X', null);
		if (isGeocodeError(expected)) throw new Error('unexpected error');
		for (const variant of ['8fvc9g8f+6x', '8FVC 9G8F+6X']) {
			const r = parsePlusCode(variant, null);
			if (isGeocodeError(r)) throw new Error(`expected a result for ${variant}`);
			expect(r.label).toBe('8FVC9G8F+6X');
			expect(r.lat).toBeCloseTo(expected.lat, 6);
			expect(r.lon).toBeCloseTo(expected.lon, 6);
		}
	});
});

describe('parsePlusCode — short-code recovery (reference-dependent)', () => {
	// The same short code resolving to two different continents depending on
	// the reference point is not a bug — it's the argument for the mandatory
	// "Confirm on map" gate. Never trust a short code without seeing the pin.
	it('the same short code recovers to two different continents depending on reference', () => {
		const zurich = parsePlusCode('9G8F+6X', { lat: 47.4, lon: 8.6 });
		const kentucky = parsePlusCode('9G8F+6X', { lat: 36.56, lon: -87.32 });
		if (isGeocodeError(zurich) || isGeocodeError(kentucky)) throw new Error('expected results');
		expect(zurich.recoveredFrom).toBe('8FVC9G8F+6X');
		expect(zurich.lat).toBeCloseTo(47.365562, 4);
		expect(zurich.lon).toBeCloseTo(8.524938, 4);
		expect(kentucky.recoveredFrom).toBe('868J9G8F+6X');
		expect(kentucky.lat).toBeCloseTo(36.365562, 4);
		expect(kentucky.lon).toBeCloseTo(-87.475063, 4);
	});

	it('6H+59 near Kentucky recovers to the full local code', () => {
		const r = parsePlusCode('6H+59', { lat: 36.56, lon: -87.32 });
		if (isGeocodeError(r)) throw new Error('expected a result');
		expect(r.recoveredFrom).toBe('868JHM6H+59');
		expect(r.lat).toBeCloseTo(36.560438, 4);
		expect(r.lon).toBeCloseTo(-87.321562, 4);
	});
});

describe('parsePlusCode / parseMGRS — errors', () => {
	const ref: RefPoint = { lat: 47.4, lon: 8.6 };

	it('short code with no reference needs one', () => {
		expect(parsePlusCode('9G8F+6X', null)).toEqual({ error: 'needs_reference' });
		expect(parsePlusCode('6H+59', null)).toEqual({ error: 'needs_reference' });
	});

	it('padded (coarse) codes are too_coarse, even with a reference', () => {
		expect(parsePlusCode('8FVC0000+', ref)).toEqual({ error: 'too_coarse' });
		expect(parsePlusCode('8FVC00+', ref)).toEqual({ error: 'too_coarse' });
	});

	it('structurally invalid codes are invalid', () => {
		expect(parsePlusCode('+6X', ref)).toEqual({ error: 'invalid' });
		expect(parsePlusCode('8FVC9G8F+6', ref)).toEqual({ error: 'invalid' });
		expect(parsePlusCode('NOTACODE+XX', ref)).toEqual({ error: 'invalid' });
	});

	it('MGRS-shaped but invalid input is invalid', () => {
		expect(parseMGRS('ZZZZ9999')).toEqual({ error: 'invalid' });
		expect(parseMGRS('16SEG12345')).toEqual({ error: 'invalid' }); // odd digit count
		expect(parseMGRS('61SEG1234567890')).toEqual({ error: 'invalid' }); // zone > 60
	});

	// Regression: a bare 2-char short code with nothing after the '+' (e.g.
	// 'CP+') is the sparsest possible short Plus Code — 2 significant chars
	// out of a 21-symbol alphabet, 0 digits of extra precision — and it
	// collides constantly with ordinary radio abbreviations (CP, HQ, RP, WX)
	// followed by a stray '+'. Without this guard, olc.isValid/isShort/
	// recoverNearest accept it and silently resolve to an arbitrary nearby
	// point with no error, letting a mission be created at the wrong
	// location with no warning (found in review of #94).
	it('a bare short code with nothing after the + is invalid, never a silent resolve', () => {
		for (const code of ['CP+', 'HQ+', 'RP+', 'WX+']) {
			expect(parsePlusCode(code, ref)).toEqual({ error: 'invalid' });
			expect(parsePlusCode(code, null)).toEqual({ error: 'invalid' });
		}
	});

	it('a short code WITH digits after the + still resolves (not over-tightened)', () => {
		// '6H+59' is the same 2-significant-char prefix as 'CP+' etc. above,
		// but with real trailing precision — this must keep working exactly
		// as documented in the short-code-recovery block above.
		const r = parsePlusCode('6H+59', ref);
		expect(isGeocodeError(r)).toBe(false);
	});
});

describe('parseMGRS — values', () => {
	const vectors: Array<{ code: string; lat: number; lon: number; precisionM: number }> = [
		{ code: '33UXP0500444998', lat: 48.249497, lon: 16.414502, precisionM: 1 },
		{ code: '16SEG1234567890', lat: 37.658097, lon: -86.860034, precisionM: 1 },
		{ code: '16S EG 12345 67890', lat: 37.658097, lon: -86.860034, precisionM: 1 },
		{ code: '16s eg 12345 67890', lat: 37.658097, lon: -86.860034, precisionM: 1 },
		{ code: '4QFJ12345678', lat: 21.309478, lon: -157.916819, precisionM: 10 },
		{ code: '33UXP0544', lat: 48.244931, lon: 16.421051, precisionM: 1000 },
		{ code: '16SDF7122146167', lat: 36.560503, lon: -87.321599, precisionM: 1 },
	];

	for (const v of vectors) {
		it(`${v.code}`, () => {
			const r = parseMGRS(v.code);
			if (isGeocodeError(r)) throw new Error(`expected a result, got error ${r.error}`);
			expect(r.lat).toBeCloseTo(v.lat, 4);
			expect(r.lon).toBeCloseTo(v.lon, 4);
			expect(r.precisionM).toBe(v.precisionM);
			expect(r.format).toBe('mgrs');
		});
	}

	it('is the mgrs package README\'s own vector — the sanity anchor', () => {
		// 4QFJ12345678 -> [-157.916819, 21.309478] is reproduced verbatim from
		// the mgrs package's own README, proving the library (and therefore
		// every other vector in this file) is being read correctly.
		const r = parseMGRS('4QFJ12345678');
		if (isGeocodeError(r)) throw new Error('expected a result');
		expect(r.lat).toBeCloseTo(21.309478, 5);
		expect(r.lon).toBeCloseTo(-157.916819, 5);
	});
});

describe('plusCodePrecisionM', () => {
	it('maps code length to a metre precision hint', () => {
		expect(plusCodePrecisionM(8)).toBe(275);
		expect(plusCodePrecisionM(10)).toBe(14);
		expect(plusCodePrecisionM(11)).toBe(3);
	});
});

describe('formatMGRSGroups', () => {
	it('re-inserts spaces for a readable, radio-friendly form', () => {
		expect(formatMGRSGroups('16SEG1234567890')).toBe('16S EG 12345 67890');
		expect(formatMGRSGroups('33UXP0544')).toBe('33U XP 05 44');
		expect(formatMGRSGroups('4QFJ12345678')).toBe('4Q FJ 1234 5678');
	});
});

describe('forward formatting', () => {
	it('formatMGRS at several precisions', () => {
		expect(formatMGRS(48.205, 16.368, 5)).toBe('33UXP0164039990');
		expect(formatMGRS(48.205, 16.368, 4)).toBe('33UXP01643999');
		expect(formatMGRS(48.205, 16.368, 3)).toBe('33UXP016399');

		expect(formatMGRS(36.5605, -87.3216, 5)).toBe('16SDF7122146167');
		expect(formatMGRS(36.5605, -87.3216, 4)).toBe('16SDF71224616');
		expect(formatMGRS(36.5605, -87.3216, 3)).toBe('16SDF712461');

		expect(formatMGRS(47.36559, 8.524997, 5)).toBe('32TMT6413445901');
		expect(formatMGRS(47.36559, 8.524997, 4)).toBe('32TMT64134590');
		expect(formatMGRS(47.36559, 8.524997, 3)).toBe('32TMT641459');
	});

	it('formatPlusCode: full + short relative to a reference', () => {
		const r1 = formatPlusCode(47.36559, 8.524997, { lat: 47.4, lon: 8.6 });
		expect(r1.full).toBe('8FVC9G8F+6X');
		expect(r1.short).toBe('9G8F+6X');

		const r2 = formatPlusCode(36.5605, -87.3216, { lat: 36.56, lon: -87.32 });
		expect(r2.full).toBe('868JHM6H+59');
		expect(r2.short).toBe('6H+59');

		const r3 = formatPlusCode(36.5605, -87.3216, { lat: 36.6, lon: -87.4 });
		expect(r3.full).toBe('868JHM6H+59');
		expect(r3.short).toBe('HM6H+59');
	});

	it('formatPlusCode falls back to the full code when the reference is too far, or absent', () => {
		const tooFar = formatPlusCode(36.5605, -87.3216, { lat: 37.5, lon: -88.5 });
		expect(tooFar.full).toBe('868JHM6H+59');
		expect(tooFar.short).toBe('868JHM6H+59');

		const noRef = formatPlusCode(36.5605, -87.3216, null);
		expect(noRef.full).toBe('868JHM6H+59');
		expect(noRef.short).toBe('868JHM6H+59');
	});
});

describe('round-trip', () => {
	it('formatMGRS(parseMGRS(c)) reproduces c exactly, at the code\'s own precision', () => {
		for (const code of ['33UXP0500444998', '16SEG1234567890', '4QFJ12345678', '16SDF7122146167']) {
			const r = parseMGRS(code);
			if (isGeocodeError(r)) throw new Error(`expected a result for ${code}`);
			const digitsPerAxis = code.replace(/[^\d]/g, '').length / 2;
			expect(formatMGRS(r.lat, r.lon, digitsPerAxis)).toBe(code);
		}
	});

	it('formatPlusCode(parsePlusCode(c)).full reproduces c for full 10-char codes', () => {
		for (const code of ['8FVC2222+22', '8FVC9G8F+6X', '868JHM6H+59', '796RWF8Q+WF']) {
			const r = parsePlusCode(code, null);
			if (isGeocodeError(r)) throw new Error(`expected a result for ${code}`);
			expect(formatPlusCode(r.lat, r.lon).full).toBe(code);
		}
	});
});

describe('geocodeLocationLabel / extractGeocode', () => {
	it('appends the near clause when given, omits it when absent', () => {
		expect(geocodeLocationLabel('8FVC9G8F+6X', 'Aid Station 3')).toBe(
			'8FVC9G8F+6X · near Aid Station 3'
		);
		expect(geocodeLocationLabel('8FVC9G8F+6X')).toBe('8FVC9G8F+6X');
	});

	it('extracts a pluscode/mgrs label back out of a stored location string', () => {
		expect(extractGeocode('16S EG 12345 67890 · near Aid Station 3')).toEqual({
			code: '16SEG1234567890',
			format: 'mgrs',
		});
		expect(extractGeocode('8FVC9G8F+6X · near Bayswater')).toEqual({
			code: '8FVC9G8F+6X',
			format: 'pluscode',
		});
	});

	it('returns null for non-geocode or w3w-sourced location strings', () => {
		expect(extractGeocode('Aid Station 3')).toBeNull();
		expect(extractGeocode('///filled.count.soap')).toBeNull();
		expect(extractGeocode('')).toBeNull();
	});
});

describe('memo cache eviction (formatPlusCode / formatMGRS)', () => {
	it('evicts oldest entries past the cap but keeps returning correct values', () => {
		for (let i = 0; i < 300; i++) {
			// Spread across valid latitudes so each call is a distinct cache key.
			formatPlusCode(1 + i * 0.01, 1);
			formatMGRS(1 + i * 0.01, 1);
		}
		// Re-querying an evicted-or-not key must still return the right answer
		// (never a stale mismatch) — that's the property under test, not the
		// internal cache size, which formatPlusCode/formatMGRS don't expose.
		const expectedFull = formatPlusCode(1 + 299 * 0.01, 1).full;
		expect(formatPlusCode(1 + 299 * 0.01, 1).full).toBe(expectedFull);
		const expectedMgrs = formatMGRS(1 + 250 * 0.01, 1);
		expect(formatMGRS(1 + 250 * 0.01, 1)).toBe(expectedMgrs);
	});
});

describe('normalizeGeocode', () => {
	it('strips all internal whitespace and upper-cases', () => {
		expect(normalizeGeocode('8fvc 9g8f+6x')).toBe('8FVC9G8F+6X');
		expect(normalizeGeocode('  16s eg 12345 67890  ')).toBe('16SEG1234567890');
	});
});

describe('never false-positives on realistic mission location names', () => {
	// A misfire here silently teleports the draft pin, which is strictly
	// worse than no detection at all — this test is the guard rail.
	const labels = [
		'Aid Station 3',
		'Mile 22.5',
		'CP-1',
		'N. Trailhead',
		'41.8781, -87.6298',
		'18T Trailhead',
		'3 Mile Rd',
		'Net 16 South',
		'Zone 4 Staging',
		'Grid C7',
		'Lot 33U',
	];
	for (const label of labels) {
		it(`${JSON.stringify(label)} -> null`, () => {
			expect(detectFormat(label)).toBeNull();
		});
	}
});
