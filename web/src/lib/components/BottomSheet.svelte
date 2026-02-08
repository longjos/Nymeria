<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { SheetState } from '$lib/stores/ui';

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

	let dragging = $state(false);
	let startY = $state(0);
	let startTranslate = $state(0);
	let currentTranslate = $state(0);
	let startTime = $state(0);
	let sheetEl: HTMLDivElement;

	function snapY(s: SheetState): number {
		const vh = window.innerHeight;
		switch (s) {
			case 'peek': return vh - 60;
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
			const peekY = vh - 60;
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

	.sheet-peek {
		padding: 0 var(--space-md);
		flex-shrink: 0;
	}

	.sheet-content {
		flex: 1;
		overflow-y: auto;
		padding: 0 var(--space-md);
		overscroll-behavior: contain;
	}
</style>
