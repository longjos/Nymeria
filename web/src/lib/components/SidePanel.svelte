<script lang="ts">
	import type { Snippet } from 'svelte';
	import CloseButton from './CloseButton.svelte';

	let {
		open = false,
		onClose,
		onBack,
		backLabel = 'Messages',
		hideBackButton = false,
		onTransitionEnd,
		children
	}: {
		open?: boolean;
		onClose?: () => void;
		/** When set, Escape and a header Back control return to the previous view instead of closing. */
		onBack?: () => void;
		/** Label shown next to the back chevron (e.g. "Alerts" when backing out of the NWS Alerts detail view). */
		backLabel?: string;
		/**
		 * Keep `onBack` wired to Escape but do not render the chrome back
		 * button — for views that draw their own in-content back control and
		 * would otherwise show two identical ones (the NWS alert detail).
		 */
		hideBackButton?: boolean;
		onTransitionEnd?: () => void;
		children?: Snippet;
	} = $props();

	function handleTransitionEnd() {
		onTransitionEnd?.();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!open) return;
		// A modal above the panel owns Escape. `inert` on body children does not
		// silence a window listener, and stopPropagation cannot help — both
		// listeners are on window — so check the DOM for an open modal instead.
		if (document.querySelector('[aria-modal="true"], [data-blocks-escape="true"]')) return;
		if (e.key !== 'Escape') return;
		if (onBack) onBack();
		else onClose?.();
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="side-panel desktop-only"
	class:open
	ontransitionend={handleTransitionEnd}
>
	<div class="panel-header" class:has-back={!!onBack && !hideBackButton}>
		{#if onBack && !hideBackButton}
			<button class="back-btn" onclick={onBack} aria-label="Back to {backLabel}">
				<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
					<path d="M10 12L6 8l4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
				</svg>
				{backLabel}
			</button>
		{/if}
		<CloseButton onClick={() => onClose?.()} label="Close panel" bordered />
	</div>
	<div class="panel-body">
		{#if children}
			{@render children()}
		{/if}
	</div>
</div>

<style>
	.side-panel {
		position: fixed;
		top: 0;
		right: var(--rail-width);
		/* The ride strip is a full-width bottom band (spec §9); the panel
		   insets off the same runtime-published token the strip publishes,
		   defaulting to 0px outside bike-ride mode. */
		bottom: var(--ride-strip-h, 0px);
		width: var(--panel-width);
		background: var(--color-bg);
		border-left: 1px solid var(--color-primary);
		z-index: var(--z-panel);
		transform: translateX(100%);
		transition: transform var(--duration-slow) var(--ease-out);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.side-panel.open {
		transform: translateX(0);
	}

	.panel-header {
		display: flex;
		justify-content: flex-end;
		padding: var(--space-sm) var(--space-md);
		flex-shrink: 0;
	}

	.panel-header.has-back {
		justify-content: space-between;
		align-items: center;
	}

	.back-btn {
		display: flex;
		align-items: center;
		gap: 4px;
		min-height: 44px;
		padding: 4px var(--space-sm) 4px var(--space-xs);
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.back-btn:hover {
		color: var(--color-accent);
	}

	.panel-body {
		flex: 1;
		overflow-y: auto;
		overflow-x: hidden;
	}
</style>
