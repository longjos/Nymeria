<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { SheetState } from '$lib/stores/ui';

	// Peek height: handle 20 + peek status row 32 + nav rail 58 + 12 padding.
	// Hoisted so the snap arithmetic and the published CSS token can never drift apart.
	// Measured, not budgeted: at 112 the status row's real 32px min-height pushed
	// the rail 10px past the viewport and clipped the bottom of every FAB.
	const PEEK_CONTENT_BASE_H = 122;

	let {
		sheetLevel = 'peek' as SheetState,
		onStateChange,
		peekContent,
		children,
		peekExtraH = 0
	}: {
		sheetLevel?: SheetState;
		onStateChange?: (s: SheetState) => void;
		peekContent?: Snippet;
		children?: Snippet;
		/** Extra px to reserve at peek height when peekContent renders more than
		 * the baseline status row (e.g. the NWS "soonest alert" strip) — so that
		 * content isn't pushed off-screen and the nav rail below it stays put. */
		peekExtraH?: number;
	} = $props();

	// Reactive so a caller can grow/shrink peekContent (e.g. the wx alert strip
	// mounting/unmounting) without a full remount of the sheet.
	let PEEK_CONTENT_H = $derived(PEEK_CONTENT_BASE_H + peekExtraH);

	// env(safe-area-inset-bottom) is only legible to CSS, but the snap arithmetic is
	// in JS — so measure it off a throwaway probe instead of guessing 34px. Without
	// this the rail's label row renders inside the home-indicator strip.
	let safeBottom = $state(0);

	function measureSafeBottom(): number {
		const probe = document.createElement('div');
		probe.style.cssText =
			'position:fixed;bottom:0;left:0;width:0;height:env(safe-area-inset-bottom);' +
			'visibility:hidden;pointer-events:none';
		document.body.appendChild(probe);
		const h = probe.getBoundingClientRect().height;
		probe.remove();
		return h;
	}

	// Name kept uppercase: the drag/snap handlers below read PEEK_H and are deliberately
	// left byte-for-byte unchanged. It is now reactive because the inset can change
	// (orientation) and the published token must always equal the rendered height.
	let PEEK_H = $derived(PEEK_CONTENT_H + safeBottom);

	// P0-3 / P1-5: below this height, fractional snap points (half=50vh, full=10vh)
	// leave the sheet mostly off the bottom of a landscape phone (852x393 → full at
	// y=39, i.e. almost the whole sheet). Short viewports get absolute snap points
	// instead. Keep in sync with the (min-height: 500px) breakpoints in app.css and
	// +page.svelte's isDesktop matchMedia query — all three define the same
	// "is this a phone-height viewport" line.
	const SHORT_VH_BREAKPOINT = 500;

	// P1-5: snapY() used to read window.innerHeight directly inside a $derived, and
	// the only resize listener refreshed safeBottom — so neither Safari's URL-bar
	// collapse nor a rotation (same safe-area inset, different height) invalidated
	// the snap math, leaving a stale translateY. vh is now its own reactive value,
	// refreshed alongside safeBottom off the same listener. visualViewport is
	// preferred where available — it tracks the true visible height (e.g. a
	// collapsing URL bar) more closely than innerHeight.
	let vh = $state(window.visualViewport?.height ?? window.innerHeight);

	let dragging = $state(false);
	let startY = $state(0);
	let startTranslate = $state(0);
	let currentTranslate = $state(0);
	let startTime = $state(0);
	let sheetEl: HTMLDivElement;

	$effect(() => {
		const read = () => {
			vh = window.visualViewport?.height ?? window.innerHeight;
			safeBottom = measureSafeBottom();
		};
		read();
		window.addEventListener('resize', read);
		window.visualViewport?.addEventListener('resize', read);
		return () => {
			window.removeEventListener('resize', read);
			window.visualViewport?.removeEventListener('resize', read);
		};
	});

	// Publish the peek height as a CSS token at runtime so overlays that sit above
	// the sheet (Toast) can position themselves against it. BottomSheet only renders
	// on mobile, so this never applies on desktop.
	// The token is the inset-free height: Toast adds env(safe-area-inset-bottom)
	// itself, so publishing PEEK_H here would double-count the inset.
	$effect(() => {
		const root = document.documentElement;
		const prev = root.style.getPropertyValue('--sheet-peek');
		root.style.setProperty('--sheet-peek', `${PEEK_CONTENT_H}px`);
		return () => {
			if (prev) root.style.setProperty('--sheet-peek', prev);
			else root.style.removeProperty('--sheet-peek');
		};
	});

	// P0-3: on a short viewport (landscape phone), fractional snap points collapse
	// into each other — full=10vh on a 393px-tall screen leaves 40px of content.
	// Absolute snap points instead, clamped so peek >= half >= full always holds
	// (peekY floors at halfY rather than ever going negative when PEEK_H > vh).
	function snapY(s: SheetState): number {
		const short = vh < SHORT_VH_BREAKPOINT;
		const fullY = short ? 40 : vh * 0.1;
		const halfY = short ? Math.max(fullY, vh - 200) : vh * 0.5;
		const peekY = Math.max(halfY, vh - PEEK_H);
		switch (s) {
			case 'peek': return peekY;
			case 'half': return halfY;
			case 'full': return fullY;
		}
	}

	let translateY = $derived(dragging ? currentTranslate : snapY(sheetLevel));

	// P0-1: the sheet's rendered height follows the *resolved* snap point, not the
	// live drag position — recomputing height (a layout property) on every
	// touchmove would fight the transform-only drag for the compositor. The box is
	// therefore correct at rest and only slightly stale for the duration of a drag,
	// which is intentional (see BottomSheet in the mobile review, P0-1).
	let heightSnapPx = $derived(snapY(sheetLevel));

	function onTouchStart(e: TouchEvent) {
		dragging = true;
		startY = e.touches[0].clientY;
		startTranslate = snapY(sheetLevel);
		currentTranslate = startTranslate;
		startTime = Date.now();
	}

	function onTouchMove(e: TouchEvent) {
		if (!dragging) return;
		const dy = e.touches[0].clientY - startY;
		const next = startTranslate + dy;
		currentTranslate = Math.max(snapY('full'), Math.min(vh - 20, next));
	}

	function onTouchEnd(e: TouchEvent) {
		if (!dragging) return;
		dragging = false;

		const dy = currentTranslate - startTranslate;
		const dt = Date.now() - startTime;
		const velocity = Math.abs(dy) / dt; // px/ms

		let newLevel: SheetState;

		if (velocity > 0.5) {
			// Flick
			newLevel = dy > 0 ? 'peek' : 'full';
		} else {
			// Snap to nearest
			const peekY = snapY('peek');
			const halfY = snapY('half');
			const fullY = snapY('full');
			const y = currentTranslate;

			const dPeek = Math.abs(y - peekY);
			const dHalf = Math.abs(y - halfY);
			const dFull = Math.abs(y - fullY);

			if (dPeek <= dHalf && dPeek <= dFull) newLevel = 'peek';
			else if (dHalf <= dFull) newLevel = 'half';
			else newLevel = 'full';
		}

		onStateChange?.(newLevel);
	}
</script>

<div
	class="bottom-sheet mobile-only"
	bind:this={sheetEl}
	style="transform: translateY({translateY}px);
		height: calc(100dvh - {heightSnapPx}px);
		transition: {dragging
		? 'none'
		: `transform var(--duration-slow) var(--ease-out), height var(--duration-slow) var(--ease-out)`}"
>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="sheet-handle"
		ontouchstart={onTouchStart}
		ontouchmove={onTouchMove}
		ontouchend={onTouchEnd}
	>
		<div class="handle-bar"></div>
	</div>
	<div class="sheet-peek">
		{#if peekContent}
			{@render peekContent()}
		{/if}
	</div>
	<div class="sheet-content" style="overscroll-behavior: contain">
		{#if children}
			{@render children()}
		{/if}
	</div>
</div>

<style>
	.bottom-sheet {
		position: fixed;
		/* P0-1: top: 0 (not bottom: 0) is load-bearing now that height is variable.
		   translateY(snapY) is the sole positioning mechanism — it assumes the
		   box's untransformed top is 0 and height spans exactly the visible
		   remainder (100dvh - snapY), so the transform lands the visible top at
		   snapY and the visible bottom flush with the viewport. Anchoring with
		   bottom: 0 instead would let height changes ALSO reposition the box
		   (bottom-anchored boxes grow/shrink from the bottom, moving their top),
		   double-counting the offset already applied by the transform — the
		   sheet ends up translated by 2x snapY, i.e. entirely off-screen. */
		top: 0;
		left: 0;
		right: 0;
		/* height is set inline from the resolved snap point (heightSnapPx), so the
		   sheet only ever occupies the space it visibly draws into — a footer or
		   any other bottom-anchored control inside .sheet-content can never
		   resolve past the real bottom edge again. min-height is only the
		   drag-time floor before the first inline height is computed. */
		min-height: var(--sheet-peek);
		max-height: 100dvh;
		background: var(--color-bg);
		border-top-left-radius: var(--radius-lg);
		border-top-right-radius: var(--radius-lg);
		box-shadow: var(--shadow-sheet);
		z-index: var(--z-sheet);
		pointer-events: auto;
		display: flex;
		flex-direction: column;
		will-change: transform;
	}

	.sheet-handle {
		display: flex;
		justify-content: center;
		padding: 8px 0;
		cursor: grab;
		touch-action: none;
		flex-shrink: 0;
	}

	.handle-bar {
		width: 36px;
		height: 4px;
		background: var(--color-text-muted);
		border-radius: var(--radius-full);
		opacity: 0.5;
	}

	/* Side padding must survive the safe-area addition, so the inset goes on
	   padding-bottom only — never a padding shorthand. PEEK_H adds the same inset
	   to the snap height, which is what lifts the rail clear of the home indicator. */
	.sheet-peek {
		padding: 0 var(--space-md);
		padding-bottom: env(safe-area-inset-bottom);
		flex-shrink: 0;
	}

	.sheet-content {
		flex: 1;
		overflow-y: auto;
		padding: 0 var(--space-md);
		padding-bottom: env(safe-area-inset-bottom);
		overscroll-behavior: contain;
	}
</style>
