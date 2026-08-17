import { describe, it, expect } from 'vitest';
	import {
		allCategories,
		canTransmitViaAPRS,
		categoryCanTransmitViaAPRS,
		categoryMeta,
		geometryMeta,
	} from './annotationMeta';

	describe('canTransmitViaAPRS', () => {
		it('allows only point geometry', () => {
			expect(canTransmitViaAPRS('point')).toBe(true);
			expect(canTransmitViaAPRS('line')).toBe(false);
			expect(canTransmitViaAPRS('area')).toBe(false);
		});

		it('rejects unknown geometry', () => {
			expect(canTransmitViaAPRS('')).toBe(false);
			expect(canTransmitViaAPRS('polygon')).toBe(false);
		});

		it('matches geometryMeta.aprsTx for every known type', () => {
			for (const [type, meta] of Object.entries(geometryMeta)) {
				expect(canTransmitViaAPRS(type)).toBe(meta.aprsTx);
			}
		});
	});

	describe('categoryCanTransmitViaAPRS', () => {
		it('is true when the category allows a point', () => {
			expect(categoryCanTransmitViaAPRS('incident')).toBe(true);
			expect(categoryCanTransmitViaAPRS('hazard')).toBe(true);
			expect(categoryCanTransmitViaAPRS('general')).toBe(true);
		});

		it('is false for line-only and area-only categories', () => {
			expect(categoryCanTransmitViaAPRS('route')).toBe(false);
			expect(categoryCanTransmitViaAPRS('boundary')).toBe(false);
		});

		it('covers every category via allowedGeometry', () => {
			for (const cat of allCategories) {
				const expected = categoryMeta[cat].allowedGeometry.includes('point');
				expect(categoryCanTransmitViaAPRS(cat)).toBe(expected);
			}
		});
	});
