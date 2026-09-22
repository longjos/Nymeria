<script lang="ts">
	// The traffic-priority ladder is agency-configurable per net (governing
	// fact 5) — this is the ONLY place a custom ladder is authored. Rank is
	// derived from row order and never typed directly, so it can never gap
	// or repeat (internal/netprofile.ValidateTiers's exact requirement).
	import type { PriorityTier } from '$lib/types';

	let {
		tiers, shipped, onChange
	}: {
		tiers: PriorityTier[];
		shipped: PriorityTier[];
		onChange: (tiers: PriorityTier[]) => void;
	} = $props();

	const MAX_TIERS = 8;
	const ID_PATTERN = /^[a-z][a-z0-9-]{0,23}$/;

	let useShipped = $derived(tiers.length === 0);

	function toggleShipped(use: boolean): void {
		if (use) {
			onChange([]);
		} else {
			onChange(shipped.length > 0 ? shipped.map((t, i) => ({ ...t, rank: i + 1, examples: [...t.examples] })) : [emptyTier(1)]);
		}
	}

	function emptyTier(rank: number): PriorityTier {
		return { id: '', label: '', rank, description: '', examples: [] };
	}

	function startFromShipped(): void {
		onChange(shipped.map((t, i) => ({ ...t, rank: i + 1, examples: [...t.examples] })));
	}

	function updateRow(idx: number, patch: Partial<PriorityTier>): void {
		onChange(tiers.map((t, i) => (i === idx ? { ...t, ...patch } : t)));
	}

	function addRow(): void {
		if (tiers.length >= MAX_TIERS) return;
		onChange([...tiers, emptyTier(tiers.length + 1)]);
	}

	function removeRow(idx: number): void {
		onChange(tiers.filter((_, i) => i !== idx).map((t, i) => ({ ...t, rank: i + 1 })));
	}

	function move(idx: number, dir: -1 | 1): void {
		const to = idx + dir;
		if (to < 0 || to >= tiers.length) return;
		const copy = [...tiers];
		[copy[idx], copy[to]] = [copy[to], copy[idx]];
		onChange(copy.map((t, i) => ({ ...t, rank: i + 1 })));
	}

	function addExample(idx: number, text: string): void {
		const t = tiers[idx];
		if (!t || text.trim() === '') return;
		updateRow(idx, { examples: [...t.examples, text.trim()] });
	}

	function removeExample(idx: number, exIdx: number): void {
		const t = tiers[idx];
		if (!t) return;
		updateRow(idx, { examples: t.examples.filter((_, i) => i !== exIdx) });
	}

	// Keyed by row index. A remove/reorder can leave a stale orphaned entry
	// behind, which is harmless — it is never read by a row that did not
	// create it.
	let exampleDrafts = $state<Record<number, string>>({});

	function idError(id: string, idx: number): string | null {
		if (id === '') return 'required';
		if (!ID_PATTERN.test(id)) return 'lowercase slug';
		if (tiers.some((t, i) => i !== idx && t.id === id)) return 'duplicate';
		return null;
	}
</script>

<div class="ple">
	<label class="ple-toggle">
		<input type="radio" name="ple-mode" checked={useShipped} onchange={() => toggleShipped(true)} />
		Use the profile's shipped ladder
	</label>
	<label class="ple-toggle">
		<input type="radio" name="ple-mode" checked={!useShipped} onchange={() => toggleShipped(false)} />
		Custom
	</label>

	{#if useShipped}
		<div class="ple-shipped-preview">
			{#each shipped as t (t.id)}
				<span class="ple-shipped-chip">{t.rank}. {t.label}</span>
			{/each}
		</div>
	{:else}
		<div class="ple-rows">
			{#each tiers as tier, idx (idx)}
				{@const err = idError(tier.id, idx)}
				<div class="ple-row">
					<div class="ple-row-main">
						<span class="ple-rank" title={idx === 0 ? 'most urgent' : undefined}>{tier.rank}</span>
						<div class="ple-fields">
							<input class="ple-id" type="text" value={tier.id} oninput={(e) => updateRow(idx, { id: (e.target as HTMLInputElement).value.toLowerCase() })} placeholder="id-slug" aria-invalid={!!err} />
							<input class="ple-label" type="text" value={tier.label} oninput={(e) => updateRow(idx, { label: (e.target as HTMLInputElement).value })} placeholder="Label" />
						</div>
						<div class="ple-move">
							<button type="button" class="ple-move-btn" onclick={() => move(idx, -1)} disabled={idx === 0} aria-label="Move {tier.label || 'tier'} up">&uarr;</button>
							<button type="button" class="ple-move-btn" onclick={() => move(idx, 1)} disabled={idx === tiers.length - 1} aria-label="Move {tier.label || 'tier'} down">&darr;</button>
						</div>
						<button type="button" class="ple-remove" onclick={() => removeRow(idx)} aria-label="Remove {tier.label || 'tier'}">&times;</button>
					</div>
					{#if idx === 0}<p class="ple-caption">Most urgent</p>{/if}
					{#if err}<p class="ple-error">{err}</p>{/if}
					<textarea class="ple-desc" rows="1" value={tier.description} oninput={(e) => updateRow(idx, { description: (e.target as HTMLTextAreaElement).value })} placeholder="One sentence of intent"></textarea>
					<div class="ple-examples">
						{#each tier.examples as ex, exIdx (exIdx)}
							<span class="ple-example-chip">{ex}<button type="button" onclick={() => removeExample(idx, exIdx)} aria-label="Remove example {ex}">&times;</button></span>
						{/each}
						<input
							class="ple-example-input"
							type="text"
							placeholder="+ example"
							value={exampleDrafts[idx] ?? ''}
							oninput={(e) => (exampleDrafts[idx] = (e.target as HTMLInputElement).value)}
							onkeydown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault();
									addExample(idx, exampleDrafts[idx] ?? '');
									exampleDrafts[idx] = '';
								}
							}}
						/>
					</div>
				</div>
			{/each}
		</div>
		<div class="ple-actions">
			<button type="button" class="ple-btn" onclick={addRow} disabled={tiers.length >= MAX_TIERS}>+ Add tier</button>
			<button type="button" class="ple-btn" onclick={startFromShipped}>Start from shipped</button>
		</div>
	{/if}
</div>

<style>
	.ple { display: flex; flex-direction: column; gap: var(--space-sm); }
	.ple-toggle { display: flex; align-items: center; gap: 6px; font-size: 0.82rem; }

	.ple-shipped-preview { display: flex; flex-wrap: wrap; gap: 6px; }
	.ple-shipped-chip {
		font-size: 0.75rem; padding: 2px 8px; border-radius: var(--radius-full);
		background: var(--color-bg); color: var(--color-text-muted);
	}

	.ple-rows { display: flex; flex-direction: column; gap: var(--space-sm); }
	.ple-row {
		border: 1px solid var(--color-primary); border-radius: var(--radius-sm); padding: var(--space-sm);
		display: flex; flex-direction: column; gap: var(--space-xs);
	}
	.ple-row-main { display: flex; align-items: center; gap: var(--space-sm); }
	.ple-rank {
		width: 22px; height: 22px; border-radius: 50%; background: var(--color-bg);
		display: flex; align-items: center; justify-content: center; font-size: 0.75rem; font-weight: 700; flex-shrink: 0;
	}
	.ple-fields { flex: 1; display: flex; gap: 6px; min-width: 0; }
	.ple-id { width: 110px; }
	.ple-label { flex: 1; min-width: 0; }
	.ple-fields input {
		min-height: 40px; background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); font: inherit; font-size: 0.82rem; padding: 0 var(--space-sm);
	}
	.ple-fields input[aria-invalid="true"] { border-color: var(--color-error); }
	.ple-move { display: flex; gap: var(--space-2xs); }
	.ple-move-btn, .ple-remove {
		width: 44px; height: 44px; background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); cursor: pointer; font-size: 0.9rem;
	}
	.ple-move-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.ple-remove { color: var(--color-error-text); border-color: var(--color-error); }
	.ple-caption { font-size: 0.7rem; color: var(--color-text-muted); margin: 0 0 0 30px; }
	.ple-error { font-size: 0.72rem; color: var(--color-error-text); margin: 0 0 0 30px; }
	.ple-desc {
		width: 100%; background: var(--color-bg); border: 1px solid var(--color-primary); border-radius: var(--radius-sm);
		color: var(--color-text); font: inherit; font-size: 0.8rem; padding: var(--space-sm); resize: vertical;
	}
	.ple-examples { display: flex; flex-wrap: wrap; gap: var(--space-xs); align-items: center; }
	.ple-example-chip {
		display: inline-flex; align-items: center; gap: var(--space-xs); font-size: 0.72rem; padding: 2px 6px;
		border-radius: var(--radius-full); background: var(--color-bg); color: var(--color-text-muted);
	}
	.ple-example-chip button { background: none; border: none; color: inherit; cursor: pointer; font-size: 0.8rem; line-height: 1; }
	.ple-example-input {
		min-height: 28px; background: none; border: 1px dashed var(--color-primary); border-radius: var(--radius-full);
		color: var(--color-text); font: inherit; font-size: 0.72rem; padding: 0 8px; width: 110px;
	}

	.ple-actions { display: flex; gap: var(--space-sm); }
	.ple-btn {
		min-height: 40px; padding: 0 var(--space-sm); background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text); font-weight: 600; cursor: pointer; font-size: 0.8rem;
	}
	.ple-btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
