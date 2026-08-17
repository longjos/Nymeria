<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		open = false,
		onClose,
		onBack,
		onTransitionEnd,
		children
	}: {
		open?: boolean;
		onClose?: () => void;
		/** When set, Escape and a header Back control return to the previous view instead of closing. */
		onBack?: () => void;
		onTransitionEnd?: () => void;
		children?: Snippet;
	} = $props();

	function handleTransitionEnd() {
		onTransitionEnd?.();
	}

	function handleKeydown(e: KeyboardEvent) {
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
	<div class="panel-header" class:has-back={!!onBack}>
		{#if onBack}
			<button class="back-btn" onclick={onBack} aria-label="Back to conversations">
				<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
					<path d="M10 12L6 8l4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
				</svg>
				Messages
			</button>
		{/if}
		<button class="close-btn" onclick={onClose} aria-label="Close panel">
			<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
				<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
			</svg>
		</button>
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
		bottom: 0;
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
		padding: 4px 8px 4px 2px;
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

	.close-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: color var(--duration-fast), border-color var(--duration-fast);
	}

	.close-btn:hover {
		color: var(--color-text);
		border-color: var(--color-accent);
	}

	.panel-body {
		flex: 1;
		overflow-y: auto;
		overflow-x: hidden;
	}
</style>
