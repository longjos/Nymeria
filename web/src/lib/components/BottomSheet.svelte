<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { SheetState } from '$lib/stores/ui';

	// Peek height: handle 20 + peek status row 32 + nav rail 58 + 12 padding.
	// Hoisted so the snap arithmetic and the published CSS token can never drift apart.
	// Measured, not budgeted: at 112 the status row's real 32px min-height pushed
	// the rail 10px past the viewport and clipped the bottom of every FAB.
	const PEEK_CONTENT_H = 122;

	let {
		sheetLevel = 'peek' as SheetState,
		onStateChange,
		peekContent,
		children
	}: {
		sheetLevel?: SheetState;
		onStateChange?: (s: SheetState) => void;
		peekContent?: Snippet;
		children?: Snippet;
	} = $props();

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

	let dragging = $state(false);
	let startY = $state(0);
	let startTranslate = $state(0);
	let currentTranslate = $state(0);
	let startTime = $state(0);
	let sheetEl: HTMLDivElement;

	$effect(() => {
		safeBottom = measureSafeBottom();
		const onResize = () => { safeBottom = measureSafeBottom(); };
		window.addEventListener('resize', onResize);
		return () => window.removeEventListener('resize', onResize);
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

	function snapY(s: SheetState): number {
		const vh = window.innerHeight;
		switch (s) {
			case 'peek': return vh - PEEK_H;
			case 'half': return vh * 0.5;
			case 'full': return vh * 0.1;
		}
	}

	let translateY = $derived(dragging ? currentTranslate : snapY(sheetLevel));

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
		const vh = window.innerHeight;
		currentTranslate = Math.max(vh * 0.1, Math.min(vh - 20, next));
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
			const vh = window.innerHeight;
			const peekY = vh - PEEK_H;
			const halfY = vh * 0.5;
			const fullY = vh * 0.1;
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
	style="transform: translateY({translateY}px); transition: {dragging ? 'none' : `transform var(--duration-slow) var(--ease-out)`}"
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
		bottom: 0;
		left: 0;
		right: 0;
		height: 100vh;
		height: 100dvh;
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
