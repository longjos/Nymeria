<script lang="ts">
	import { APRS_PATH_PRESETS, presetIdFor } from '$lib/aprsPath';

	let {
		id,
		label,
		value = $bindable(''),
		compact = false,
		onchange
	}: {
		id: string;
		label?: string;
		value: string;
		compact?: boolean;
		onchange?: (value: string) => void;
	} = $props();

	let forceCustom = $state(false);

	let selected = $derived(forceCustom ? 'custom' : presetIdFor(value));
	let hint = $derived(
		selected === 'custom'
			? 'Comma-separated hops, e.g. WIDE1-1,WIDE2-1 or CALL-7'
			: (APRS_PATH_PRESETS.find((p) => p.id === selected)?.hint ?? '')
	);

	function onPresetChange(event: Event) {
		const id = (event.currentTarget as HTMLSelectElement).value;
		if (id === 'custom') {
			forceCustom = true;
			onchange?.(value);
			return;
		}
		forceCustom = false;
		const preset = APRS_PATH_PRESETS.find((p) => p.id === id);
		if (preset) {
			value = preset.value;
			onchange?.(value);
		}
	}

	function onCustomInput(event: Event) {
		value = (event.currentTarget as HTMLInputElement).value;
		if (presetIdFor(value) !== 'custom') {
			forceCustom = false;
		}
		onchange?.(value);
	}
</script>

<div class="path-select" class:compact>
	{#if compact}
		<div class="compact-row">
			<label class="compact-label" for={id}>via</label>
			<select {id} value={selected} onchange={onPresetChange} title={hint}>
				{#each APRS_PATH_PRESETS as preset}
					<option value={preset.id}>{preset.shortLabel}</option>
				{/each}
				<option value="custom">Custom…</option>
			</select>
			{#if selected === 'custom'}
				<input
					id="{id}-custom"
					class="compact-custom"
					type="text"
					value={value}
					oninput={onCustomInput}
					placeholder="WIDE1-1,WIDE2-1"
					spellcheck="false"
					autocomplete="off"
				/>
			{/if}
		</div>
	{:else}
		<div class="field-row">
			<label for={id}>{label}</label>
			<select {id} value={selected} onchange={onPresetChange}>
				{#each APRS_PATH_PRESETS as preset}
					<option value={preset.id}>{preset.label}</option>
				{/each}
				<option value="custom">Custom…</option>
			</select>
		</div>
		{#if selected === 'custom'}
			<div class="field-row">
				<label for="{id}-custom">Custom path</label>
				<input
					id="{id}-custom"
					type="text"
					value={value}
					oninput={onCustomInput}
					placeholder="WIDE1-1,WIDE2-1"
					spellcheck="false"
					autocomplete="off"
				/>
			</div>
		{/if}
		{#if hint}
			<p class="path-hint">{hint}</p>
		{/if}
	{/if}
</div>

<style>
	.path-select {
		margin-bottom: var(--space-sm);
	}

	.path-select.compact {
		margin-bottom: 0;
	}

	.field-row {
		margin-bottom: var(--space-sm);
	}

	.field-row label {
		display: block;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		margin-bottom: 2px;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.field-row input[type='text'],
	.field-row select {
		width: 100%;
		padding: 6px 10px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.82rem;
		font-family: inherit;
		outline: none;
	}

	.field-row select {
		appearance: none;
		cursor: pointer;
	}

	.field-row input:focus,
	.field-row select:focus {
		border-color: var(--color-accent);
	}

	.path-hint {
		margin: 2px 0 0;
		font-size: 0.7rem;
		color: var(--color-text-muted);
		line-height: 1.35;
	}

	.compact-row {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
	}

	.compact-label {
		flex-shrink: 0;
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	.compact select {
		max-width: 13.5rem;
		padding: 2px 6px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.7rem;
		font-family: inherit;
		outline: none;
		appearance: none;
		cursor: pointer;
	}

	.compact select:focus,
	.compact-custom:focus {
		border-color: var(--color-accent);
	}

	.compact-custom {
		flex: 1;
		min-width: 6rem;
		padding: 2px 6px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.7rem;
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		outline: none;
	}
</style>
