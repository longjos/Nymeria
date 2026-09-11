import { describe, it, expect } from 'vitest';
import {
	normalizeWords,
	isFullAddress,
	looksLikePartial,
	formatWords,
	w3wLocationLabel,
	isW3WLocation,
	extractWords,
	w3wSuffix,
	cachedReverse,
	putReverse,
} from './w3w';

// Shared test-vector table with internal/geocode/w3w/words_test.go (Go) — a
// comment there points back at this file. Keep both in sync.
const VECTORS: Array<{
	input: string;
	fullAddress: boolean;
	partial: boolean;
}> = [
	{ input: 'filled.count.soap', fullAddress: true, partial: false },
	{ input: '///filled.count.soap', fullAddress: true, partial: false },
	{ input: '/filled.count.soap', fullAddress: true, partial: false },
	{ input: '  Filled.Count.Soap  ', fullAddress: true, partial: false },
	{ input: 'filled.count.', fullAddress: false, partial: true },
	{ input: 'filled.count', fullAddress: false, partial: true },
	{ input: 'filled.', fullAddress: false, partial: true },
	{ input: 'filled', fullAddress: false, partial: false },
	{ input: 'filled.count.soap.extra', fullAddress: false, partial: false },
	{ input: 'filled.count.s0ap', fullAddress: false, partial: false },
	{ input: 'filled count soap', fullAddress: false, partial: false },
	{ input: 'déjà.vu.vraiment', fullAddress: true, partial: false },
	{ input: '旅。行。者', fullAddress: true, partial: false },
	{ input: '', fullAddress: false, partial: false },
	{ input: 'Aid Station 3', fullAddress: false, partial: false },
	{ input: 'Mile 22.5', fullAddress: false, partial: false },
	{ input: 'CP-1', fullAddress: false, partial: false },
	{ input: 'N. Trailhead', fullAddress: false, partial: false },
	{ input: '41.8781, -87.6298', fullAddress: false, partial: false },
	// normalizeWords strips every leading slash, not just up to three.
	{ input: '/////filled.count.soap', fullAddress: true, partial: false },
];

describe('normalizeWords / isFullAddress / looksLikePartial (shared vector table)', () => {
	for (const v of VECTORS) {
		it(`${JSON.stringify(v.input)} -> full=${v.fullAddress} partial=${v.partial}`, () => {
			expect(isFullAddress(v.input)).toBe(v.fullAddress);
			expect(looksLikePartial(v.input)).toBe(v.partial);
		});
	}

	it('normalizes prefix, case, whitespace and CJK separators', () => {
		expect(normalizeWords('///Filled.Count.Soap')).toBe('filled.count.soap');
		expect(normalizeWords('/filled.count.soap')).toBe('filled.count.soap');
		expect(normalizeWords('  Filled.Count.Soap  ')).toBe('filled.count.soap');
		expect(normalizeWords('旅。行。者')).toBe('旅.行.者');
		expect(normalizeWords('旅・行・者')).toBe('旅.行.者');
	});

	it('never false-positives on realistic annotation labels', () => {
		for (const label of ['Aid Station 3', 'Mile 22.5', 'CP-1', 'N. Trailhead', '41.8781, -87.6298']) {
			expect(isFullAddress(label)).toBe(false);
			expect(looksLikePartial(label)).toBe(false);
		}
	});
});

describe('formatWords', () => {
	it('always yields exactly three leading slashes', () => {
		expect(formatWords('a.b.c')).toBe('///a.b.c');
		expect(formatWords('/a.b.c')).toBe('///a.b.c');
		expect(formatWords('///a.b.c')).toBe('///a.b.c');
	});
});

describe('w3wLocationLabel', () => {
	it('appends the nearest place when given', () => {
		expect(w3wLocationLabel('filled.count.soap', 'Bayswater')).toBe(
			'///filled.count.soap · near Bayswater'
		);
	});

	it('omits the near clause when place is empty or absent', () => {
		expect(w3wLocationLabel('filled.count.soap', '')).toBe('///filled.count.soap');
		expect(w3wLocationLabel('filled.count.soap')).toBe('///filled.count.soap');
	});
});

describe('isW3WLocation / extractWords / w3wSuffix round-trip', () => {
	it('round-trips a plain w3w location string', () => {
		const stored = '///filled.count.soap';
		expect(isW3WLocation(stored)).toBe(true);
		expect(extractWords(stored)).toBe('filled.count.soap');
		expect(w3wSuffix(stored)).toBeNull();
	});

	it('round-trips a w3w location string with a near suffix', () => {
		const stored = '///filled.count.soap · near Bayswater, London';
		expect(isW3WLocation(stored)).toBe(true);
		expect(extractWords(stored)).toBe('filled.count.soap');
		expect(w3wSuffix(stored)).toBe('Bayswater, London');
	});

	it('is false/null for a non-w3w stored location', () => {
		expect(isW3WLocation('Aid Station 3')).toBe(false);
		expect(extractWords('Aid Station 3')).toBeNull();
		expect(w3wSuffix('Aid Station 3')).toBeNull();
	});
});

describe('reverse cache', () => {
	it('put/get round-trips', () => {
		putReverse(51.520847, -0.195521, 'filled.count.soap');
		expect(cachedReverse(51.520847, -0.195521)).toBe('filled.count.soap');
	});

	it('rounds the cache key to 6 decimal places', () => {
		putReverse(51.5208470001, -0.1955210001, 'filled.count.soap');
		expect(cachedReverse(51.520847, -0.195521)).toBe('filled.count.soap');
	});

	it('misses return undefined, never throw', () => {
		expect(cachedReverse(1.234567, 2.345678)).toBeUndefined();
	});

	it('evicts the oldest entry once the 256 cap is exceeded', () => {
		for (let i = 0; i < 300; i++) {
			putReverse(i, i, `word.${i}.evict`);
		}
		// The earliest-inserted entries should have been evicted.
		expect(cachedReverse(0, 0)).toBeUndefined();
		expect(cachedReverse(1, 1)).toBeUndefined();
		// The most recent entries should still be present.
		expect(cachedReverse(299, 299)).toBe('word.299.evict');
		expect(cachedReverse(250, 250)).toBe('word.250.evict');
	});
});
