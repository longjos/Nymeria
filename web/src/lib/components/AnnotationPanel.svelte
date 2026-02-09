<script lang="ts">
	import { api } from '$lib/api';
	import { canPlot } from '$lib/stores/session';
	import { annotationList } from '$lib/stores/annotations';
	import { timeAgo } from '$lib/utils';
	import type { Annotation } from '$lib/types';

	let {
		onFlyToAnnotation,
		onStartDraw,
		onPreviewChange,
		onStartEdit,
		onStopEdit,
	}: {
		onFlyToAnnotation?: (ann: Annotation) => void;
		onStartDraw?: (mode: 'point' | 'line' | 'area') => void;
		onPreviewChange?: (geometry: string | null, color: string) => void;
		onStartEdit?: (id: string) => void;
		onStopEdit?: () => void;
	} = $props();

	let creating = $state(false);
	let formType = $state<'point' | 'line' | 'area'>('point');
	let formLabel = $state('');
	let formDescription = $state('');
	let formColor = $state('#e63946');
	let pendingGeometry = $state<string | null>(null);
	let waitingForDraw = $state(false);

	// Edit mode state
	let editingId = $state<string | null>(null);
	let editGeometry = $state<string | null>(null);

	const COLORS = ['#e63946', '#457b9d', '#2a9d8f', '#e9c46a', '#f4a261', '#264653', '#a8dadc', '#d62828'];

	const typeIcons: Record<string, string> = {
		point: 'M8 1a5 5 0 00-5 5c0 4 5 9 5 9s5-5 5-9a5 5 0 00-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z',
		line: 'M2 14L7 6l3 4 4-8',
		area: 'M3 13l3-10 4 4 4-4 2 10z'
	};

	export function setGeometry(geometry: string) {
		if (geometry) {
			pendingGeometry = geometry;
			waitingForDraw = false;
			onPreviewChange?.(geometry, formColor);
		} else {
			// Cancelled
			waitingForDraw = false;
		}
	}

	export function setEditGeometry(geometry: string) {
		editGeometry = geometry;
	}

	function handleStartEdit(ann: Annotation) {
		editingId = ann.id;
		editGeometry = null;
		onStartEdit?.(ann.id);
	}

	function handleCancelEdit() {
		editingId = null;
		editGeometry = null;
		onStopEdit?.();
	}

	async function handleSaveEdit() {
		if (!editingId || !editGeometry) return;
		await api.updateAnnotation(editingId, { geometry: editGeometry });
		editingId = null;
		editGeometry = null;
		onStopEdit?.();
	}

	function handleStartCreate() {
		creating = true;
		formType = 'point';
		formLabel = '';
		formDescription = '';
		formColor = '#e63946';
		pendingGeometry = null;
		waitingForDraw = false;
	}

	function handleCancelCreate() {
		creating = false;
		pendingGeometry = null;
		waitingForDraw = false;
		onPreviewChange?.(null, formColor);
	}

	function handleDraw() {
		waitingForDraw = true;
		onStartDraw?.(formType);
	}

	function handleRedraw() {
		pendingGeometry = null;
		onPreviewChange?.(null, formColor);
		waitingForDraw = true;
		onStartDraw?.(formType);
	}

	async function handleSave() {
		if (!formLabel.trim() || !pendingGeometry) return;

		const style = JSON.stringify({ color: formColor });
		await api.createAnnotation({
			type: formType,
			label: formLabel.trim(),
			description: formDescription.trim() || undefined,
			geometry: pendingGeometry,
			style,
		});

		creating = false;
		pendingGeometry = null;
		waitingForDraw = false;
		onPreviewChange?.(null, formColor);
	}

	async function handleDelete(id: string) {
		await api.deleteAnnotation(id);
	}

	function handleClick(ann: Annotation) {
		onFlyToAnnotation?.(ann);
	}

	function getCenter(ann: Annotation): { lat: number; lon: number } | null {
		try {
			const geom = JSON.parse(ann.geometry);
			if (geom.type === 'Point') {
				return { lat: geom.coordinates[1], lon: geom.coordinates[0] };
			} else if (geom.type === 'LineString') {
				const mid = Math.floor(geom.coordinates.length / 2);
				return { lat: geom.coordinates[mid][1], lon: geom.coordinates[mid][0] };
			} else if (geom.type === 'Polygon') {
				const coords = geom.coordinates[0];
				let latSum = 0, lonSum = 0;
				for (const c of coords) { latSum += c[1]; lonSum += c[0]; }
				return { lat: latSum / coords.length, lon: lonSum / coords.length };
			}
		} catch { /* skip */ }
		return null;
	}
</script>

<div class="annotation-panel">
	<div class="panel-header">
		<div class="header-text">
			<span class="title">Annotations</span>
			<span class="subtitle">Local map markups shared with your team. Not transmitted over APRS.</span>
		</div>
		{#if $canPlot && !creating}
			<button class="add-btn" onclick={handleStartCreate}>
				<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
					<path d="M8 2v12M2 8h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
				</svg>
				Add
			</button>
		{/if}
	</div>

	{#if creating}
		<div class="create-form">
			<div class="type-selector">
				{#each (['point', 'line', 'area'] as const) as t}
					<button
						class="type-btn"
						class:active={formType === t}
						onclick={() => { formType = t; pendingGeometry = null; waitingForDraw = false; }}
					>
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
							<path d={typeIcons[t]} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
						</svg>
						{t.charAt(0).toUpperCase() + t.slice(1)}
					</button>
				{/each}
			</div>

			<input
				type="text"
				class="form-input"
				placeholder="Label (required)"
				bind:value={formLabel}
			/>

			<input
				type="text"
				class="form-input"
				placeholder="Description (optional)"
				bind:value={formDescription}
			/>

			<div class="color-row">
				<span class="color-label">Color</span>
				<div class="color-swatches">
					{#each COLORS as c}
						<button
							class="swatch"
							class:selected={formColor === c}
							style="background: {c}"
							onclick={() => { formColor = c; if (pendingGeometry) onPreviewChange?.(pendingGeometry, c); }}
							aria-label="Color {c}"
						></button>
					{/each}
				</div>
			</div>

			<div class="draw-row">
				{#if pendingGeometry}
					<span class="draw-status done">Shape drawn — shown on map</span>
					<button class="redraw-btn" onclick={handleRedraw}>Redraw</button>
				{:else if waitingForDraw}
					<span class="draw-status waiting">Drawing on map...</span>
				{:else}
					<button class="draw-btn" onclick={handleDraw}>
						<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
							<path d="M12 2l2 2-8 8H4v-2l8-8z" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
						</svg>
						Draw on map
					</button>
				{/if}
			</div>

			<div class="form-actions">
				<button class="btn btn-save" onclick={handleSave} disabled={!formLabel.trim() || !pendingGeometry}>
					Save to map
				</button>
				<button class="btn btn-cancel" onclick={handleCancelCreate}>
					Cancel
				</button>
			</div>
			<p class="form-scope-hint">Visible to all connected users. Does not transmit over RF or APRS-IS.</p>
		</div>
	{/if}

	<div class="entries">
		{#if $annotationList.length === 0 && !creating}
			<div class="empty">
				<p>No annotations yet.</p>
				<p class="empty-hint">Annotations are local markers, lines, and areas that appear on every connected user's map. Use them to mark points of interest, search boundaries, or rally points.</p>
			</div>
		{:else}
			{#each $annotationList as ann (ann.id)}
				<div class="entry" role="button" tabindex="0" onclick={() => handleClick(ann)} onkeydown={(e) => e.key === 'Enter' && handleClick(ann)}>
					<div class="entry-icon">
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
							<path d={typeIcons[ann.type] ?? typeIcons.point} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
						</svg>
					</div>
					<div class="entry-info">
						<span class="entry-label">{ann.label}</span>
						{#if ann.description}
							<span class="entry-desc">{ann.description}</span>
						{/if}
						<span class="entry-meta">
							{#if ann.createdByName}
								{ann.createdByName} &middot;
							{/if}
							{timeAgo(ann.createdAt)}
						</span>
					</div>
					{#if $canPlot}
						{#if editingId === ann.id}
							<div class="entry-actions">
								<button
									class="action-btn save-edit-btn"
									title="Save changes"
									disabled={!editGeometry}
									onclick={(e) => { e.stopPropagation(); handleSaveEdit(); }}
								>
									<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
										<path d="M3 8l4 4 6-8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
									</svg>
								</button>
								<button
									class="action-btn cancel-edit-btn"
									title="Cancel editing"
									onclick={(e) => { e.stopPropagation(); handleCancelEdit(); }}
								>
									<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
										<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
									</svg>
								</button>
							</div>
						{:else}
							<button
								class="action-btn edit-btn"
								title="Edit vertices"
								onclick={(e) => { e.stopPropagation(); handleStartEdit(ann); }}
							>
								<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
									<path d="M12 2l2 2-8 8H4v-2l8-8z" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
								</svg>
							</button>
							<button
								class="action-btn delete-btn"
								title="Delete"
								onclick={(e) => { e.stopPropagation(); handleDelete(ann.id); }}
							>
								<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
									<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
								</svg>
							</button>
						{/if}
					{/if}
				</div>
			{/each}
		{/if}
	</div>
</div>

<style>
	.annotation-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.panel-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.header-text {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.title {
		font-weight: 600;
		font-size: 0.95rem;
	}

	.subtitle {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		line-height: 1.3;
	}

	.add-btn {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 0.25rem 0.6rem;
		background: none;
		border: 1px solid var(--color-accent);
		border-radius: var(--radius-sm);
		color: var(--color-accent);
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
		transition: background var(--duration-fast), color var(--duration-fast);
	}

	.add-btn:hover {
		background: var(--color-accent);
		color: white;
	}

	/* Create form */
	.create-form {
		padding: var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.type-selector {
		display: flex;
		gap: var(--space-xs);
	}

	.type-btn {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 4px;
		padding: 0.4rem;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.8rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.type-btn:hover {
		border-color: var(--color-text-muted);
		color: var(--color-text);
	}

	.type-btn.active {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.form-input {
		width: 100%;
		padding: 0.4rem 0.6rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		outline: none;
		transition: border-color var(--duration-fast);
	}

	.form-input:focus {
		border-color: var(--color-accent);
	}

	.form-input::placeholder {
		color: var(--color-text-muted);
	}

	.color-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.color-label {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.color-swatches {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
	}

	.swatch {
		width: 22px;
		height: 22px;
		border-radius: 50%;
		border: 2px solid transparent;
		cursor: pointer;
		transition: border-color var(--duration-fast), transform var(--duration-fast);
	}

	.swatch:hover {
		transform: scale(1.15);
	}

	.swatch.selected {
		border-color: var(--color-text);
	}

	.draw-row {
		display: flex;
		align-items: center;
	}

	.draw-btn {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 0.35rem 0.6rem;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
		cursor: pointer;
		transition: border-color var(--duration-fast);
	}

	.draw-btn:hover {
		border-color: var(--color-accent);
	}

	.draw-status {
		font-size: 0.8rem;
		padding: 0.35rem 0;
	}

	.draw-status.done {
		color: var(--color-success, #2a9d8f);
	}

	.draw-status.waiting {
		color: var(--color-accent);
	}

	.redraw-btn {
		margin-left: auto;
		padding: 0.2rem 0.5rem;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.redraw-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.form-actions {
		display: flex;
		gap: var(--space-xs);
	}

	.btn {
		padding: 0.4rem 1rem;
		border-radius: var(--radius-sm);
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
		border: none;
	}

	.btn-save {
		background: var(--color-accent);
		color: white;
	}

	.btn-save:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.btn-cancel {
		background: var(--color-primary);
		color: var(--color-text);
	}

	/* Entries */
	.entries {
		flex: 1;
		overflow-y: auto;
	}

	.entry {
		display: flex;
		align-items: flex-start;
		gap: 0.6rem;
		padding: 0.6rem var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		cursor: pointer;
		transition: background var(--duration-fast);
	}

	.entry:hover {
		background: var(--color-primary);
	}

	.entry-icon {
		flex-shrink: 0;
		margin-top: 2px;
		color: var(--color-text-muted);
	}

	.entry-info {
		display: flex;
		flex-direction: column;
		gap: 1px;
		min-width: 0;
		flex: 1;
	}

	.entry-label {
		font-size: 0.85rem;
		font-weight: 500;
	}

	.entry-desc {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.entry-meta {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		opacity: 0.7;
	}

	.entry-actions {
		display: flex;
		gap: 2px;
		flex-shrink: 0;
	}

	.action-btn {
		flex-shrink: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: color var(--duration-fast), background var(--duration-fast);
	}

	.edit-btn:hover {
		color: var(--color-accent);
		background: rgba(230, 57, 70, 0.1);
	}

	.delete-btn:hover {
		color: var(--color-accent);
		background: rgba(230, 57, 70, 0.1);
	}

	.save-edit-btn:hover {
		color: var(--color-success, #2a9d8f);
		background: rgba(42, 157, 143, 0.1);
	}

	.save-edit-btn:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	.cancel-edit-btn:hover {
		color: var(--color-accent);
		background: rgba(230, 57, 70, 0.1);
	}

	.form-scope-hint {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		line-height: 1.3;
		margin: 0;
	}

	.empty {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}

	.empty-hint {
		font-size: 0.75rem;
		line-height: 1.4;
		margin-top: 0.5rem;
		opacity: 0.7;
	}
</style>
