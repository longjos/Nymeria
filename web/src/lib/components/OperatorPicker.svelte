<script lang="ts">
	import type { NetCheckIn } from '$lib/types';
	import { statusColors } from '$lib/stores/commandpalette';

	/**
	 * The one operator picker. Used from the mission card ("+ Assign", single
	 * pick) and from the mission create form (multi-select). Both render the
	 * identical row, so picking an operator before the mission exists looks
	 * exactly like picking one afterwards.
	 */
	let {
		candidates,
		selectedIds = [],
		mode = 'single',
		emptyLabel = 'No available operators',
		ariaLabel = 'Assign operator',
		onSelect,
		onHover,
		onHoverEnd,
		onClose
	}: {
		candidates: NetCheckIn[];
		selectedIds?: string[];
		mode?: 'single' | 'multi';
		emptyLabel?: string;
		ariaLabel?: string;
		onSelect: (ci: NetCheckIn) => void;
		onHover?: (ci: NetCheckIn) => void;
		onHoverEnd?: () => void;
		onClose?: () => void;
	} = $props();

	// In single mode an already-assigned operator is not a candidate at all;
	// in multi mode they stay visible with a check so the picker doubles as
	// the record of what has been chosen.
	const rows = $derived(
		mode === 'single' ? candidates.filter((c) => !selectedIds.includes(c.id)) : candidates
	);

	let activeIndex = $state(0);
	let rowEls = $state<Array<HTMLButtonElement | undefined>>([]);

	function focusRow(i: number) {
		const clamped = Math.max(0, Math.min(rows.length - 1, i));
		activeIndex = clamped;
		rowEls[clamped]?.focus();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (rows.length === 0 && e.key !== 'Escape') return;
		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				e.stopPropagation();
				focusRow(activeIndex + 1);
				break;
			case 'ArrowUp':
				e.preventDefault();
				e.stopPropagation();
				focusRow(activeIndex - 1);
				break;
			case 'Home':
				e.preventDefault();
				e.stopPropagation();
				focusRow(0);
				break;
			case 'End':
				e.preventDefault();
				e.stopPropagation();
				focusRow(rows.length - 1);
				break;
			case 'Enter':
			case ' ':
				// Never let Enter bubble to the mission form's submit handler:
				// inside the picker Enter means "toggle this operator".
				e.preventDefault();
				e.stopPropagation();
				if (rows[activeIndex]) onSelect(rows[activeIndex]);
				break;
			case 'Escape':
				// Only consume Escape when this picker actually has something to
				// close (the mission-card popover). In the create form the picker
				// is inline, so Escape must bubble to the form's own handler and
				// cancel the draft — the same as the <select> it replaced.
				if (onClose) {
					e.stopPropagation();
					onClose();
				}
				break;
		}
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
<div
	class="assign-picker"
	role="listbox"
	aria-label={ariaLabel}
	aria-multiselectable={mode === 'multi'}
	onkeydown={handleKeydown}
	tabindex="-1"
>
	{#each rows as ci, i (ci.id)}
		{@const selected = selectedIds.includes(ci.id)}
		<button
			type="button"
			class="assign-option"
			role="option"
			aria-selected={selected}
			tabindex={i === activeIndex ? 0 : -1}
			bind:this={rowEls[i]}
			onfocus={() => (activeIndex = i)}
			onclick={() => onSelect(ci)}
			onmouseenter={() => onHover?.(ci)}
			onmouseleave={() => onHoverEnd?.()}
		>
			<span class="mission-op-dot" style="background: {statusColors[ci.status] ?? '#6b7280'}"></span>
			{ci.callsign}
			{#if ci.tacticalCall}
				<span class="assign-option-tactical">"{ci.tacticalCall}"</span>
			{/if}
			{#if ci.lat != null}<span class="assign-option-pos">GPS</span>{/if}
			{#if mode === 'multi'}
				<span class="assign-option-check" aria-hidden="true">{selected ? '✓' : ''}</span>
			{/if}
		</button>
	{/each}
	{#if rows.length === 0}
		<div class="assign-empty" role="status">{emptyLabel}</div>
	{/if}
</div>

<style>
	.assign-picker {
		display: flex;
		flex-direction: column;
		gap: 1px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		max-height: 160px;
		overflow-y: auto;
		min-width: 0;
	}

	.assign-option {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 12px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		font-size: 0.8rem;
		text-align: left;
		cursor: pointer;
		min-height: 44px;
	}

	.assign-option:hover {
		background: var(--color-primary);
	}

	.assign-option:last-of-type {
		border-bottom: none;
	}

	.assign-option:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
	}

	.assign-option[aria-selected='true'] {
		background: color-mix(in srgb, var(--color-accent) 14%, transparent);
	}

	.assign-empty {
		padding: 10px 12px;
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	.mission-op-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.assign-option-tactical {
		font-size: 0.7rem;
		color: var(--color-accent);
	}

	.assign-option-pos {
		font-size: 0.6rem;
		font-weight: 700;
		color: #22c55e;
		margin-left: auto;
		letter-spacing: 0.04em;
	}

	.assign-option-check {
		margin-left: 8px;
		color: var(--color-accent);
		font-weight: 700;
	}
</style>
