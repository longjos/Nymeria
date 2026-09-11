import type { Annotation } from '$lib/types';

/**
 * Shared "reveal and hold" behaviour for a marker click that should bring an
 * annotation row into view inside whichever list panel currently owns it
 * (AnnotationPanel or LocationManager). Both the gate-planning logic and the
 * frame-polling sequencer live here so the two components stay structurally
 * identical instead of diverging copy-paste implementations.
 */

/** How long the highlight is held at full strength after the row is revealed. */
export const HIGHLIGHT_HOLD_MS = 2500;
/** Fade in/out of the held state. Must match the CSS transition duration. */
export const HIGHLIGHT_FADE_MS = 200;
/** Bounded wait for the row to render after expanding a set / clearing filters. */
export const FOCUS_MAX_FRAMES = 30; // ~500ms at 60fps
/** Mobile sheet slides in; re-align once it has settled. */
export const SHEET_SETTLE_MS = 380;

export interface FocusGates {
	collapsedBatchIds: ReadonlySet<string>;
	filterCategory: string; // '' = no filter
	filterBatchId: string; // '' = no filter
}

export interface FocusPlan {
	/** Batch id whose collapse must be undone, or null. */
	expandBatchId: string | null;
	/** Clear the category filter to reveal the row. */
	clearCategoryFilter: boolean;
	/** Clear the batch filter to reveal the row. */
	clearBatchFilter: boolean;
	/** True when at least one gate had to be overridden (drives the toast). */
	overrode: boolean;
}

/**
 * Work out which UI gates currently hide `ann` from the list. Pure: it reads
 * state and returns intent, it never writes. The caller applies the plan.
 */
export function planFocusReveal(ann: Annotation, gates: FocusGates): FocusPlan {
	const expandBatchId =
		ann.batchId && gates.collapsedBatchIds.has(ann.batchId) ? ann.batchId : null;
	const clearCategoryFilter =
		!!gates.filterCategory && (ann.category || 'general') !== gates.filterCategory;
	const clearBatchFilter =
		!!gates.filterBatchId && (ann.batchId || '') !== gates.filterBatchId;

	return {
		expandBatchId,
		clearCategoryFilter,
		clearBatchFilter,
		overrode: expandBatchId !== null || clearCategoryFilter || clearBatchFilter,
	};
}

export interface RevealDeps {
	/** Returns the row element for `id`, or null if not rendered yet. */
	find: (id: string) => HTMLElement | null;
	/** Schedules one frame; returns a cancel handle. */
	nextFrame: (cb: () => void) => number;
	cancelFrame: (h: number) => void;
	/** true when the user asked for reduced motion. */
	reducedMotion: boolean;
	scrollTo: (el: HTMLElement, smooth: boolean) => void;
	/** Applies/removes the held highlight. */
	setHighlight: (id: string | null) => void;
	setTimer: (cb: () => void, ms: number) => number;
	clearTimer: (h: number) => void;
	/** Called exactly once, when the sequence has settled (found or gave up). */
	done: (found: boolean) => void;
}

/**
 * Poll for the row up to `maxFrames` animation frames. On success, scroll it
 * into view, apply the highlight, schedule a corrective re-scroll (covers a
 * mobile sheet still animating) and schedule the highlight to clear after the
 * hold. On exhaustion, report failure and apply nothing.
 *
 * Returns a cancel function. Idempotent: calling cancel after done() is a
 * no-op. Cancelling stops the frame poll and the pending re-scroll, but it
 * never clears the hold timer — the caller's `setTimer` records that handle
 * itself (see AnnotationPanel/LocationManager) so a *new* reveal sequence can
 * clear a stale one; the sequencer here has no opinion on that.
 */
export function revealAnnotation(
	id: string,
	deps: RevealDeps,
	maxFrames = FOCUS_MAX_FRAMES
): () => void {
	let cancelled = false;
	let frameHandle: number | null = null;
	let settleHandle: number | null = null;
	let doneCalled = false;

	function finish(found: boolean) {
		if (doneCalled) return;
		doneCalled = true;
		deps.done(found);
	}

	function poll(attemptsLeft: number) {
		if (cancelled) return;
		const el = deps.find(id);
		if (el) {
			deps.scrollTo(el, !deps.reducedMotion);
			deps.setHighlight(id);
			settleHandle = deps.setTimer(() => {
				settleHandle = null;
				if (cancelled) return;
				deps.scrollTo(el, false);
			}, SHEET_SETTLE_MS);
			deps.setTimer(() => {
				deps.setHighlight(null);
			}, HIGHLIGHT_HOLD_MS);
			finish(true);
			return;
		}

		const remaining = attemptsLeft - 1;
		if (remaining <= 0) {
			finish(false);
			return;
		}
		frameHandle = deps.nextFrame(() => {
			frameHandle = null;
			poll(remaining);
		});
	}

	poll(maxFrames);

	return () => {
		cancelled = true;
		if (frameHandle !== null) {
			deps.cancelFrame(frameHandle);
			frameHandle = null;
		}
		if (settleHandle !== null) {
			deps.clearTimer(settleHandle);
			settleHandle = null;
		}
	};
}
