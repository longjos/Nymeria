<script lang="ts">
	import type { NetMission } from '$lib/types';

	/**
	 * Mirror of OperatorPicker for the other direction: pick a MISSION from an
	 * operator's roster card. Same shell, same keyboard model, mission rows.
	 * The shell CSS is duplicated deliberately — Svelte scoped styles cannot be
	 * shared and hoisting them into app.css would leak globals.
	 */
	let {
		missions,
		excludeIds = [],
		emptyLabel = 'No available missions',
		ariaLabel = 'Assign mission',
		onSelect,
		onClose
	}: {
		missions: NetMission[];
		excludeIds?: string[];
		emptyLabel?: string;
		ariaLabel?: string;
		onSelect: (m: NetMission) => void;
		onClose?: () => void;
	} = $props();

	// keep in sync with NetControlPanel.svelte trafficColors
	const trafficColors: Record<string, string> = {
		routine: '#22c55e',
		priority: '#f59e0b',
		welfare: '#3b82f6',
		emergency: '#ef4444'
	};

	const rows = $derived(missions.filter((m) => !excludeIds.includes(m.id)));

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
				e.preventDefault();
				e.stopPropagation();
				if (rows[activeIndex]) onSelect(rows[activeIndex]);
				break;
			case 'Escape':
				e.stopPropagation();
				onClose?.();
				break;
		}
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
<div
	class="assign-picker"
	role="listbox"
	aria-label={ariaLabel}
	onkeydown={handleKeydown}
	tabindex="-1"
>
	{#each rows as m, i (m.id)}
		<button
			type="button"
			class="assign-option"
			role="option"
			aria-selected="false"
			tabindex={i === activeIndex ? 0 : -1}
			bind:this={rowEls[i]}
			onfocus={() => (activeIndex = i)}
			onclick={() => onSelect(m)}
		>
			<span class="mission-op-dot" style="background: {trafficColors[m.priority] ?? '#6b7280'}"></span>
			{m.title}
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
</style>
