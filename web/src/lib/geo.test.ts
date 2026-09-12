import { describe, it, expect } from 'vitest';
import { annotationPickPoint } from './geo';
import type { Annotation } from './types';

function makeAnnotation(geometry: unknown): Pick<Annotation, 'geometry'> {
	return { geometry: (typeof geometry === 'string' ? geometry : JSON.stringify(geometry)) as string };
}

describe('annotationPickPoint', () => {
	it('returns the point itself for Point geometry, ignoring the clicked fallback', () => {
		const ann = makeAnnotation({ type: 'Point', coordinates: [-121.5, 39.25] });
		const result = annotationPickPoint(ann, { lat: 0, lon: 0 });
		expect(result).toEqual({ lat: 39.25, lon: -121.5 });
	});

	it('prefers the clicked point on a LineString', () => {
		const ann = makeAnnotation({
			type: 'LineString',
			coordinates: [
				[-121.4, 39.2],
				[-121.35, 39.25],
				[-121.3, 39.3],
			],
		});
		const fallback = { lat: 39.3, lon: -121.4 };
		const result = annotationPickPoint(ann, fallback);
		expect(result).toEqual(fallback);
	});

	it('falls back to the LineString midpoint vertex when no click point is given (odd vertex count)', () => {
		const ann = makeAnnotation({
			type: 'LineString',
			coordinates: [
				[-121.4, 39.2],
				[-121.35, 39.25],
				[-121.3, 39.3],
			],
		});
		const result = annotationPickPoint(ann);
		// Math.floor(3 / 2) === 1 -> the middle vertex
		expect(result).toEqual({ lat: 39.25, lon: -121.35 });
	});

	it('falls back to the LineString midpoint vertex when no click point is given (even vertex count)', () => {
		const ann = makeAnnotation({
			type: 'LineString',
			coordinates: [
				[-121.4, 39.2],
				[-121.35, 39.25],
				[-121.3, 39.3],
				[-121.25, 39.35],
			],
		});
		const result = annotationPickPoint(ann);
		// Math.floor(4 / 2) === 2
		expect(result).toEqual({ lat: 39.3, lon: -121.3 });
	});

	it('uses the outer-ring centroid for a Polygon with no click point', () => {
		const ann = makeAnnotation({
			type: 'Polygon',
			coordinates: [[[0, 0], [0, 2], [2, 2], [2, 0], [0, 0]]],
		});
		const result = annotationPickPoint(ann);
		expect(result).toEqual({ lat: 1, lon: 1 });
	});

	it('prefers the clicked point on a Polygon', () => {
		const ann = makeAnnotation({
			type: 'Polygon',
			coordinates: [[[0, 0], [0, 2], [2, 2], [2, 0], [0, 0]]],
		});
		const fallback = { lat: 0.1, lon: 0.1 };
		const result = annotationPickPoint(ann, fallback);
		expect(result).toEqual(fallback);
	});

	it('returns the fallback for unparseable geometry', () => {
		const ann = makeAnnotation('not json');
		const fallback = { lat: 5, lon: 6 };
		expect(annotationPickPoint(ann, fallback)).toEqual(fallback);
	});

	it('returns null for unparseable geometry with no fallback', () => {
		const ann = makeAnnotation('not json');
		expect(annotationPickPoint(ann)).toBeNull();
	});

	it('returns null for an empty LineString', () => {
		const ann = makeAnnotation({ type: 'LineString', coordinates: [] });
		expect(annotationPickPoint(ann)).toBeNull();
	});

	it('accepts an already-parsed geometry object as well as a JSON string', () => {
		const geo = { type: 'Point', coordinates: [10, 20] };
		const asString = makeAnnotation(geo);
		const asObject: Pick<Annotation, 'geometry'> = { geometry: geo as unknown as string };
		expect(annotationPickPoint(asString)).toEqual({ lat: 20, lon: 10 });
		expect(annotationPickPoint(asObject)).toEqual({ lat: 20, lon: 10 });
	});
});
