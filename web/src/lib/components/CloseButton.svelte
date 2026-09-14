<script lang="ts">
	/**
	 * The one dismiss button. The glyph `M4 4l8 8M12 4l-8 8` means "dismiss this
	 * thing" and lives here only — see #116 item 5. A control that destroys data
	 * needs its own glyph, not this one.
	 */
	let {
		onClick,
		label = 'Close',
		title = undefined,
		size = 44,
		iconSize = 16,
		tone = 'neutral',
		bordered = false
	}: {
		onClick: (e: MouseEvent) => void;
		/** Accessible name. Also the tooltip unless `title` overrides it. */
		label?: string;
		title?: string;
		/** Square target in px. 44 is the floor; smaller only where the row can't give it. */
		size?: number;
		iconSize?: number;
		/** `danger` tints the hover for controls that remove a row or record. */
		tone?: 'neutral' | 'danger';
		/** Draws the button as a surface chip rather than a bare glyph. */
		bordered?: boolean;
	} = $props();
</script>

<button
	type="button"
	class="close-button {tone}"
	class:bordered
	style="width: {size}px; height: {size}px;"
	onclick={onClick}
	aria-label={label}
	title={title ?? label}
>
	<svg width={iconSize} height={iconSize} viewBox="0 0 16 16" fill="none" aria-hidden="true">
		<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.75" stroke-linecap="round"/>
	</svg>
</button>

<style>
	.close-button {
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0;
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: color var(--duration-fast), background var(--duration-fast),
			border-color var(--duration-fast);
	}

	.close-button.bordered {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
	}

	.close-button:hover {
		color: var(--color-text);
		background: var(--color-primary);
	}

	.close-button.bordered:hover {
		background: var(--color-surface);
		border-color: var(--color-accent);
	}

	.close-button.danger:hover {
		color: var(--color-error);
		background: color-mix(in srgb, var(--color-error) 12%, transparent);
	}

	/* Inset so the ring survives a button flush against a panel edge or packed
	   into a dense action row — an outward offset gets clipped there. */
	.close-button:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
	}
</style>
