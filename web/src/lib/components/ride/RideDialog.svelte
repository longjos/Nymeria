<script lang="ts">
	/**
	 * The one modal shell for every ride-mode composer.
	 *
	 * Two things this exists to get right, both of which the three hand-rolled
	 * backdrops it replaces got wrong:
	 *
	 * 1. LAYER. Every shell these composers render in is transformed — SidePanel
	 *    is `translateX(0)` when open, BottomSheet carries an inline
	 *    `translateY` — and any transform other than `none` makes that ancestor
	 *    the containing block for `position: fixed` descendants. A hand-rolled
	 *    `position: fixed; inset: 0` backdrop therefore sized itself to the
	 *    PANEL, not the viewport: the dialog's title was clipped off the top and
	 *    its buttons sat behind the dashboard chrome. A real <dialog> opened
	 *    with showModal() lives in the top layer, which has no containing-block
	 *    ancestor, so it resolves against the viewport again. This is the same
	 *    fix NetControlPanel's close-net dialog already carries.
	 *
	 * 2. SCROLL. The box is a three-row grid: header, body, footer. The BODY is
	 *    the only scroller. The old markup made the whole dialog the scroller,
	 *    so a long form scrolled its own title and its own Cancel/Submit pair
	 *    out of reach — the operator had to scroll back up to commit.
	 *
	 * showModal() also supplies inertness, the Tab trap, the background scroll
	 * lock and Escape, so callers must not hand-roll those either.
	 */
	import type { Snippet } from 'svelte';

	let {
		title,
		titleId,
		onClose,
		closeDisabled = false,
		children,
		footer
	}: {
		title: string;
		/** Must match the id the caller wants announced; wired to aria-labelledby. */
		titleId: string;
		onClose: () => void;
		/** Blocks backdrop press, Escape and the × while a submit is in flight. */
		closeDisabled?: boolean;
		children: Snippet;
		footer: Snippet;
	} = $props();

	let dialogEl = $state<HTMLDialogElement | null>(null);

	function modalDialog(node: HTMLDialogElement) {
		node.showModal();
		return {
			destroy() {
				if (node.open) node.close();
			}
		};
	}

	function requestClose(): void {
		if (!closeDisabled) onClose();
	}
</script>

<!-- data-blocks-escape keeps SidePanel's own window-level Escape from closing
     the panel out from under the dialog: a native modal is implicitly
     aria-modal, so the attribute SidePanel looks for first is not present.
     The dialog has no padding and its three rows fill it, so a press whose
     target is the dialog itself can only have landed on the ::backdrop. -->
<dialog
	class="rd"
	bind:this={dialogEl}
	use:modalDialog
	data-blocks-escape="true"
	aria-labelledby={titleId}
	oncancel={(e) => {
		e.preventDefault();
		requestClose();
	}}
	onmousedown={(e) => {
		if (e.target === e.currentTarget) requestClose();
	}}
>
	<header class="rd-header">
		<h2 class="rd-title" id={titleId}>{title}</h2>
		<button class="rd-x" onclick={requestClose} disabled={closeDisabled} aria-label="Close">&times;</button>
	</header>

	<div class="rd-body">
		{@render children()}
	</div>

	<footer class="rd-footer">
		{@render footer()}
	</footer>
</dialog>

<style>
	.rd::backdrop {
		background: var(--color-scrim, rgba(0, 0, 0, 0.6));
	}

	.rd {
		/* Reset the UA <dialog> box: 1em padding, a solid border, and a
		   max-width/max-height that would otherwise fight the sizes below.
		   Centring is the UA's own inset:0 + margin:auto, left in place. */
		padding: 0;
		max-width: none;
		width: min(520px, 94vw);
		max-height: min(88vh, 760px);
		color: var(--color-text);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		overflow: hidden;
	}

	/* Qualified with [open]: an author `display` would beat the UA's
	   `dialog:not([open]) { display: none }` and render a closed dialog. */
	.rd[open] {
		display: flex;
		flex-direction: column;
	}

	.rd-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
		min-height: 52px;
		padding: 0 var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.rd-title {
		margin: 0;
		font-size: 0.95rem;
		font-weight: 600;
		line-height: 1.2;
	}

	.rd-x {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		/* Visually a glyph, but a real 44px target: this is pressed in a moving
		   vehicle. The negative margin keeps the optical edge on the padding. */
		width: 44px;
		height: 44px;
		margin-right: calc(var(--space-sm) * -1);
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 1.4rem;
		line-height: 1;
		cursor: pointer;
	}

	.rd-x:hover:not(:disabled) {
		color: var(--color-text);
	}

	.rd-x:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	/* The only scroller in the box. */
	.rd-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		overscroll-behavior: contain;
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.rd-footer {
		display: flex;
		gap: var(--space-sm);
		justify-content: flex-end;
		align-items: center;
		flex-wrap: wrap;
		padding: var(--space-sm) var(--space-md);
		padding-bottom: max(var(--space-sm), env(safe-area-inset-bottom));
		border-top: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	@media (max-width: 640px) {
		/* Bottom sheet. `fixed` resolves against the viewport here because the
		   top layer has no containing-block ancestor — the whole point of the
		   <dialog>. */
		.rd {
			width: 100%;
			position: fixed;
			inset: auto 0 0 0;
			margin: 0;
			max-height: 88vh;
			border-radius: var(--radius-lg) var(--radius-lg) 0 0;
			border-bottom: none;
		}
	}
</style>
