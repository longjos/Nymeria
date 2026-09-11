import { describe, it, expect } from 'vitest';
import {
	planFocusReveal,
	revealAnnotation,
	HIGHLIGHT_HOLD_MS,
	SHEET_SETTLE_MS,
	FOCUS_MAX_FRAMES,
	type RevealDeps,
} from './annotationFocus';
import type { Annotation } from './types';

function makeAnnotation(overrides: Partial<Annotation> = {}): Annotation {
	return {
		id: 'a1',
		type: 'point',
		label: 'Aid Station 1',
		geometry: '{"type":"Point","coordinates":[0,0]}',
		createdAt: '2026-01-01T00:00:00Z',
		updatedAt: '2026-01-01T00:00:00Z',
		category: 'aid',
		status: 'active',
		priority: 'routine',
		missionIds: [],
		...overrides,
	};
}

describe('planFocusReveal', () => {
	it('returns a no-op plan for an unfiltered, ungrouped annotation', () => {
		const ann = makeAnnotation();
		const plan = planFocusReveal(ann, {
			collapsedBatchIds: new Set(),
			filterCategory: '',
			filterBatchId: '',
		});
		expect(plan).toEqual({
			expandBatchId: null,
			clearCategoryFilter: false,
			clearBatchFilter: false,
			overrode: false,
		});
	});

	it('expands the containing batch when that batch is collapsed', () => {
		const ann = makeAnnotation({ batchId: 'b1' });
		const plan = planFocusReveal(ann, {
			collapsedBatchIds: new Set(['b1']),
			filterCategory: '',
			filterBatchId: '',
		});
		expect(plan.expandBatchId).toBe('b1');
		expect(plan.overrode).toBe(true);
	});

	it('does not expand a batch that is already expanded', () => {
		const ann = makeAnnotation({ batchId: 'b1' });
		const plan = planFocusReveal(ann, {
			collapsedBatchIds: new Set(['b2']),
			filterCategory: '',
			filterBatchId: '',
		});
		expect(plan.expandBatchId).toBeNull();
		expect(plan.overrode).toBe(false);
	});

	it('clears the category filter when the annotation category does not match', () => {
		const ann = makeAnnotation({ category: 'hazard' });
		const plan = planFocusReveal(ann, {
			collapsedBatchIds: new Set(),
			filterCategory: 'aid',
			filterBatchId: '',
		});
		expect(plan.clearCategoryFilter).toBe(true);
		expect(plan.overrode).toBe(true);
	});

	it('keeps the category filter when the annotation already matches it', () => {
		const ann = makeAnnotation({ category: 'aid' });
		const plan = planFocusReveal(ann, {
			collapsedBatchIds: new Set(),
			filterCategory: 'aid',
			filterBatchId: '',
		});
		expect(plan.clearCategoryFilter).toBe(false);
		expect(plan.overrode).toBe(false);
	});

	it('clears the batch filter when the annotation belongs to a different set', () => {
		const ann = makeAnnotation({ batchId: 'b1' });
		const plan = planFocusReveal(ann, {
			collapsedBatchIds: new Set(),
			filterCategory: '',
			filterBatchId: 'b2',
		});
		expect(plan.clearBatchFilter).toBe(true);
		expect(plan.overrode).toBe(true);
	});

	it('clears both filters and expands the set at once', () => {
		const ann = makeAnnotation({ batchId: 'b1', category: 'hazard' });
		const plan = planFocusReveal(ann, {
			collapsedBatchIds: new Set(['b1']),
			filterCategory: 'aid',
			filterBatchId: 'b2',
		});
		expect(plan).toEqual({
			expandBatchId: 'b1',
			clearCategoryFilter: true,
			clearBatchFilter: true,
			overrode: true,
		});
	});

	it('treats a missing category as general when matching the category filter', () => {
		const ann = makeAnnotation({ category: undefined as any });
		const plan = planFocusReveal(ann, {
			collapsedBatchIds: new Set(),
			filterCategory: 'general',
			filterBatchId: '',
		});
		expect(plan.clearCategoryFilter).toBe(false);
	});

	it('sets overrode only when at least one gate was overridden', () => {
		const ann = makeAnnotation({ batchId: 'b1', category: 'aid' });
		const noOverride = planFocusReveal(ann, {
			collapsedBatchIds: new Set(),
			filterCategory: 'aid',
			filterBatchId: 'b1',
		});
		expect(noOverride.overrode).toBe(false);

		const withOverride = planFocusReveal(ann, {
			collapsedBatchIds: new Set(),
			filterCategory: 'aid',
			filterBatchId: 'other',
		});
		expect(withOverride.overrode).toBe(true);
	});
});

/**
 * A controllable fake of the reveal sequencer's environment: frames and
 * timers are queues drained explicitly by the test rather than real timers,
 * so tests never race real animation frames or wall-clock time.
 */
function makeFakeDeps(opts: {
	findResults: (HTMLElement | null)[] | (() => HTMLElement | null);
	reducedMotion?: boolean;
}) {
	let findCallCount = 0;
	const frameQueue: (() => void)[] = [];
	const timers = new Map<number, { cb: () => void; ms: number }>();
	let nextTimerId = 1;

	const scrollCalls: Array<{ el: HTMLElement; smooth: boolean }> = [];
	const highlightCalls: (string | null)[] = [];
	const doneCalls: boolean[] = [];

	const findResults = opts.findResults;
	const find = (id: string): HTMLElement | null => {
		const result =
			typeof findResults === 'function'
				? findResults()
				: (findResults[findCallCount] ?? findResults[findResults.length - 1] ?? null);
		findCallCount++;
		return result;
	};

	const deps: RevealDeps = {
		find,
		nextFrame: (cb) => {
			frameQueue.push(cb);
			return frameQueue.length;
		},
		cancelFrame: (h) => {
			// Mark the queued callback as a no-op rather than splicing, so
			// handles stay stable for the test's flushFrame indexing.
			const idx = h - 1;
			if (frameQueue[idx]) frameQueue[idx] = () => {};
		},
		reducedMotion: opts.reducedMotion ?? false,
		scrollTo: (el, smooth) => scrollCalls.push({ el, smooth }),
		setHighlight: (id) => highlightCalls.push(id),
		setTimer: (cb, ms) => {
			const id = nextTimerId++;
			timers.set(id, { cb, ms });
			return id;
		},
		clearTimer: (h) => {
			timers.delete(h);
		},
		done: (found) => doneCalls.push(found),
	};

	return {
		deps,
		scrollCalls,
		highlightCalls,
		doneCalls,
		getFindCallCount: () => findCallCount,
		/** Runs the next queued rAF callback, if any. */
		flushFrame: () => {
			const cb = frameQueue.shift();
			cb?.();
		},
		frameQueueLength: () => frameQueue.length,
		/** Fires the timer scheduled for exactly `ms`, if one is pending. */
		fireTimer: (ms: number) => {
			for (const [id, t] of timers) {
				if (t.ms === ms) {
					timers.delete(id);
					t.cb();
					return true;
				}
			}
			return false;
		},
		pendingTimerCount: () => timers.size,
	};
}

const fakeEl = {} as HTMLElement;

describe('revealAnnotation', () => {
	it('highlights and scrolls on the first frame when the row already exists', () => {
		const t = makeFakeDeps({ findResults: [fakeEl] });
		revealAnnotation('a1', t.deps);

		expect(t.scrollCalls).toEqual([{ el: fakeEl, smooth: true }]);
		expect(t.highlightCalls).toEqual(['a1']);
		expect(t.getFindCallCount()).toBe(1);
	});

	it('keeps polling until the row appears, then highlights', () => {
		let calls = 0;
		const t = makeFakeDeps({
			findResults: () => {
				calls++;
				return calls >= 4 ? fakeEl : null;
			},
		});
		const cancel = revealAnnotation('a1', t.deps);

		expect(t.highlightCalls).toEqual([]);
		t.flushFrame(); // attempt 2
		expect(t.highlightCalls).toEqual([]);
		t.flushFrame(); // attempt 3
		expect(t.highlightCalls).toEqual([]);
		t.flushFrame(); // attempt 4 -> found
		expect(t.highlightCalls).toEqual(['a1']);
		expect(t.doneCalls).toEqual([true]);
		cancel();
	});

	it('calls done(false) and never highlights after the frame budget is exhausted', () => {
		const t = makeFakeDeps({ findResults: () => null });
		revealAnnotation('a1', t.deps, 5);

		// One synchronous attempt, then up to 4 more via queued frames.
		for (let i = 0; i < 10 && t.frameQueueLength() > 0; i++) t.flushFrame();

		expect(t.getFindCallCount()).toBe(5);
		expect(t.highlightCalls).toEqual([]);
		expect(t.doneCalls).toEqual([false]);
	});

	it('calls done(true) as soon as the row is highlighted, not after the hold', () => {
		const t = makeFakeDeps({ findResults: [fakeEl] });
		revealAnnotation('a1', t.deps);

		// done() already fired synchronously; the hold timer is still pending.
		expect(t.doneCalls).toEqual([true]);
		expect(t.pendingTimerCount()).toBeGreaterThan(0);
	});

	it('clears the highlight after HIGHLIGHT_HOLD_MS', () => {
		const t = makeFakeDeps({ findResults: [fakeEl] });
		revealAnnotation('a1', t.deps);

		expect(t.highlightCalls).toEqual(['a1']);
		const fired = t.fireTimer(HIGHLIGHT_HOLD_MS);
		expect(fired).toBe(true);
		expect(t.highlightCalls).toEqual(['a1', null]);
	});

	it('re-scrolls once at SHEET_SETTLE_MS with smooth scrolling disabled', () => {
		const t = makeFakeDeps({ findResults: [fakeEl] });
		revealAnnotation('a1', t.deps);

		expect(t.scrollCalls).toEqual([{ el: fakeEl, smooth: true }]);
		const fired = t.fireTimer(SHEET_SETTLE_MS);
		expect(fired).toBe(true);
		expect(t.scrollCalls).toEqual([
			{ el: fakeEl, smooth: true },
			{ el: fakeEl, smooth: false },
		]);
	});

	it('scrolls without smooth behaviour when reducedMotion is true', () => {
		const t = makeFakeDeps({ findResults: [fakeEl], reducedMotion: true });
		revealAnnotation('a1', t.deps);

		expect(t.scrollCalls).toEqual([{ el: fakeEl, smooth: false }]);
	});

	it('cancel() stops the frame poll before the row appears', () => {
		const t = makeFakeDeps({ findResults: () => null });
		const cancel = revealAnnotation('a1', t.deps, 10);

		const callsBeforeCancel = t.getFindCallCount();
		cancel();
		t.flushFrame();
		t.flushFrame();

		expect(t.getFindCallCount()).toBe(callsBeforeCancel);
		expect(t.doneCalls).toEqual([]);
	});

	it('cancel() after the highlight is applied does not clear the held highlight', () => {
		const t = makeFakeDeps({ findResults: [fakeEl] });
		const cancel = revealAnnotation('a1', t.deps);

		expect(t.highlightCalls).toEqual(['a1']);
		cancel();
		expect(t.highlightCalls).toEqual(['a1']);

		// The hold timer itself is untouched by cancel() — it still fires later.
		expect(t.fireTimer(HIGHLIGHT_HOLD_MS)).toBe(true);
		expect(t.highlightCalls).toEqual(['a1', null]);
	});

	it('calls done exactly once in every path', () => {
		const found = makeFakeDeps({ findResults: [fakeEl] });
		revealAnnotation('a1', found.deps);
		expect(found.doneCalls.length).toBe(1);

		const exhausted = makeFakeDeps({ findResults: () => null });
		revealAnnotation('a1', exhausted.deps, 3);
		for (let i = 0; i < 10 && exhausted.frameQueueLength() > 0; i++) exhausted.flushFrame();
		expect(exhausted.doneCalls.length).toBe(1);
	});
});
