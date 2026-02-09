<script lang="ts">
	import { api } from '$lib/api';
	import { canPlot } from '$lib/stores/session';
	import { annotationList } from '$lib/stores/annotations';
	import { timeAgo } from '$lib/utils';
	import type { Annotation, AnnotationCategory, AnnotationPriority } from '$lib/types';
	import {
		categoryMeta, statusMeta, priorityMeta, allCategories,
		statusLabelToValue, statusValueToLabel, statusColor, isTerminalStatus
	} from '$lib/annotationMeta';

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
	let formCategory = $state<AnnotationCategory>('general');
	let formType = $state<'point' | 'line' | 'area'>('point');
	let formLabel = $state('');
	let formDescription = $state('');
	let formColor = $state('#e63946');
	let formPriority = $state<AnnotationPriority>('routine');
	let pendingGeometry = $state<string | null>(null);
	let waitingForDraw = $state(false);

	// Edit mode state
	let editingId = $state<string | null>(null);
	let editGeometry = $state<string | null>(null);

	// Filter state
	let filterCategory = $state<AnnotationCategory | ''>('');
	let filterExpanded = $state(false);

	// Status dropdown
	let statusDropdownId = $state<string | null>(null);

	const COLORS = ['#e63946', '#457b9d', '#2a9d8f', '#e9c46a', '#f4a261', '#264653', '#a8dadc', '#d62828'];

	// Filtered annotation list
	let filteredList = $derived(
		filterCategory
			? $annotationList.filter((a) => a.category === filterCategory)
			: $annotationList
	);

	export function setGeometry(geometry: string) {
		if (geometry) {
			pendingGeometry = geometry;
			waitingForDraw = false;
			onPreviewChange?.(geometry, formColor);
		} else {
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
		formCategory = 'general';
		formType = 'point';
		formLabel = '';
		formDescription = '';
		formColor = '#e63946';
		formPriority = 'routine';
		pendingGeometry = null;
		waitingForDraw = false;
	}

	function handleCancelCreate() {
		creating = false;
		pendingGeometry = null;
		waitingForDraw = false;
		onPreviewChange?.(null, formColor);
	}

	function handleSelectCategory(cat: AnnotationCategory) {
		formCategory = cat;
		const meta = categoryMeta[cat];
		formColor = meta.defaultColor;
		// Auto-select the first allowed geometry
		if (meta.allowedGeometry.length === 1) {
			formType = meta.allowedGeometry[0];
		} else if (!meta.allowedGeometry.includes(formType)) {
			formType = meta.allowedGeometry[0];
		}
		// Reset geometry if type changed
		pendingGeometry = null;
		waitingForDraw = false;
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
			category: formCategory,
			priority: formPriority,
		});

		creating = false;
		pendingGeometry = null;
		waitingForDraw = false;
		onPreviewChange?.(null, formColor);
	}

	async function handleDelete(id: string) {
		await api.deleteAnnotation(id);
	}

	async function handleStatusChange(ann: Annotation, newStatus: string) {
		statusDropdownId = null;
		await api.changeAnnotationStatus(ann.id, newStatus);
	}

	function toggleStatusDropdown(id: string, e: MouseEvent) {
		e.stopPropagation();
		statusDropdownId = statusDropdownId === id ? null : id;
	}

	// Inline color editing
	let colorEditId = $state<string | null>(null);

	function getAnnotationColor(ann: Annotation): string {
		if (ann.style) {
			try { return JSON.parse(ann.style).color ?? '#e63946'; } catch { /* skip */ }
		}
		return categoryMeta[ann.category || 'general']?.defaultColor || '#e63946';
	}

	function toggleColorEdit(id: string) {
		colorEditId = colorEditId === id ? null : id;
	}

	async function handleColorChange(ann: Annotation, color: string) {
		colorEditId = null;
		const style = JSON.stringify({ color });
		await api.updateAnnotation(ann.id, { style });
	}

	function handleClick(ann: Annotation) {
		onFlyToAnnotation?.(ann);
	}

	function getStatusesForCategory(cat: AnnotationCategory): { label: string; value: string; color: string }[] {
		const metas = statusMeta[cat] || statusMeta.general;
		return metas.map((m) => ({ label: m.label, value: statusLabelToValue(m.label), color: m.color }));
	}
</script>

<div class="annotation-panel">
	<div class="panel-header">
		<div class="header-text">
			<span class="title">Annotations</span>
			<span class="subtitle">Local map markups shared with your team. Not transmitted over APRS.</span>
		</div>
		<div class="header-actions">
			<button
				class="filter-btn"
				class:active={filterExpanded || filterCategory}
				onclick={() => filterExpanded = !filterExpanded}
				title="Filter annotations"
			>
				<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
					<path d="M1 3h14M3 8h10M5 13h6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
				</svg>
			</button>
			{#if $canPlot && !creating}
				<button class="add-btn" onclick={handleStartCreate}>
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M8 2v12M2 8h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
					Add
				</button>
			{/if}
		</div>
	</div>

	{#if filterExpanded}
		<div class="filter-bar">
			<button
				class="filter-chip"
				class:active={filterCategory === ''}
				onclick={() => filterCategory = ''}
			>All</button>
			{#each allCategories as cat}
				<button
					class="filter-chip"
					class:active={filterCategory === cat}
					onclick={() => filterCategory = filterCategory === cat ? '' : cat}
				>
					<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
						<path d={categoryMeta[cat].icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
					{categoryMeta[cat].label}
				</button>
			{/each}
		</div>
	{/if}

	{#if creating}
		<div class="create-form">
			<!-- Category selector -->
			<div class="category-grid">
				{#each allCategories as cat}
					<button
						class="cat-btn"
						class:active={formCategory === cat}
						onclick={() => handleSelectCategory(cat)}
					>
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
							<path d={categoryMeta[cat].icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
						</svg>
						<span>{categoryMeta[cat].label}</span>
					</button>
				{/each}
			</div>

			<!-- Geometry type (auto-restricted by category) -->
			{#if categoryMeta[formCategory].allowedGeometry.length > 1}
				<div class="type-selector">
					{#each categoryMeta[formCategory].allowedGeometry as t}
						<button
							class="type-btn"
							class:active={formType === t}
							onclick={() => { formType = t; pendingGeometry = null; waitingForDraw = false; }}
						>
							{t.charAt(0).toUpperCase() + t.slice(1)}
						</button>
					{/each}
				</div>
			{/if}

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

			<!-- Priority selector -->
			<div class="priority-row">
				<span class="field-label">Priority</span>
				<div class="priority-btns">
					{#each (['routine', 'priority', 'urgent', 'emergency'] as const) as p}
						<button
							class="priority-btn"
							class:active={formPriority === p}
							style="--pri-color: {priorityMeta[p].color}"
							onclick={() => formPriority = p}
						>{priorityMeta[p].label}</button>
					{/each}
				</div>
			</div>

			<div class="color-row">
				<span class="field-label">Color</span>
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
		{#if filteredList.length === 0 && !creating}
			<div class="empty">
				{#if filterCategory}
					<p>No {categoryMeta[filterCategory].label.toLowerCase()} annotations.</p>
				{:else}
					<p>No annotations yet.</p>
					<p class="empty-hint">Annotations are local markers, lines, and areas that appear on every connected user's map. Use them to mark points of interest, search boundaries, or rally points.</p>
				{/if}
			</div>
		{:else}
			{#each filteredList as ann (ann.id)}
				{@const cat = ann.category || 'general'}
				{@const meta = categoryMeta[cat]}
				{@const sColor = statusColor(cat, ann.status)}
				{@const pMeta = priorityMeta[ann.priority || 'routine']}
				<div
					class="entry"
					class:terminal={isTerminalStatus(ann.status)}
					style="--pri-accent: {pMeta.color}"
					role="button"
					tabindex="0"
					onclick={() => handleClick(ann)}
					onkeydown={(e) => e.key === 'Enter' && handleClick(ann)}
				>
					<!-- Category icon with status ring -->
					<div class="entry-icon" style="--icon-color: {getAnnotationColor(ann)}; --status-color: {sColor}">
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
							<path d={meta.icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
						</svg>
					</div>
					<div class="entry-info">
						<div class="entry-top">
							<span class="entry-label">{ann.label}</span>
							<span class="status-badge" style="background: {sColor}">
								{statusValueToLabel(cat, ann.status)}
							</span>
						</div>
						{#if ann.description}
							<span class="entry-desc">{ann.description}</span>
						{/if}
						<span class="entry-meta">
							<span class="cat-tag">{meta.label}</span>
							{#if ann.priority && ann.priority !== 'routine'}
								<span class="pri-tag" style="color: {pMeta.color}">{pMeta.label}</span>
							{/if}
							{#if ann.createdByName}
								&middot; {ann.createdByName}
							{/if}
							&middot; {timeAgo(ann.createdAt)}
						</span>
						{#if colorEditId === ann.id}
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div class="inline-swatches" onclick={(e) => e.stopPropagation()}>
								{#each COLORS as c}
									<button
										class="swatch"
										class:selected={getAnnotationColor(ann) === c}
										style="background: {c}"
										onclick={() => handleColorChange(ann, c)}
										aria-label="Color {c}"
									></button>
								{/each}
							</div>
						{/if}
						{#if statusDropdownId === ann.id}
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div class="status-dropdown" onclick={(e) => e.stopPropagation()}>
								{#each getStatusesForCategory(cat) as s}
									<button
										class="status-option"
										class:current={s.value === ann.status}
										onclick={() => handleStatusChange(ann, s.value)}
									>
										<span class="status-dot" style="background: {s.color}"></span>
										{s.label}
									</button>
								{/each}
							</div>
						{/if}
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
								class="action-btn status-btn"
								title="Change status"
								onclick={(e) => toggleStatusDropdown(ann.id, e)}
							>
								<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
									<circle cx="8" cy="8" r="5" stroke="currentColor" stroke-width="1.2" fill="none"/>
									<path d="M8 5v3l2 2" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
								</svg>
							</button>
							<button
								class="action-btn color-btn"
								title="Change color"
								onclick={(e) => { e.stopPropagation(); toggleColorEdit(ann.id); }}
							>
								<div class="color-dot-mini" style="background: {getAnnotationColor(ann)}"></div>
							</button>
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

	.header-actions {
		display: flex;
		gap: 6px;
		align-items: center;
		flex-shrink: 0;
	}

	.filter-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 30px;
		height: 30px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.filter-btn:hover, .filter-btn.active {
		border-color: var(--color-accent);
		color: var(--color-accent);
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

	/* Filter bar */
	.filter-bar {
		display: flex;
		gap: 4px;
		padding: 6px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		overflow-x: auto;
		flex-shrink: 0;
	}

	.filter-chip {
		display: flex;
		align-items: center;
		gap: 3px;
		padding: 3px 8px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: 12px;
		color: var(--color-text-muted);
		font-size: 0.7rem;
		white-space: nowrap;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast), background var(--duration-fast);
	}

	.filter-chip:hover {
		border-color: var(--color-text-muted);
		color: var(--color-text);
	}

	.filter-chip.active {
		border-color: var(--color-accent);
		color: var(--color-accent);
		background: rgba(230, 57, 70, 0.08);
	}

	/* Create form */
	.create-form {
		padding: var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.category-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 4px;
	}

	.cat-btn {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		padding: 6px 2px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.65rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.cat-btn:hover {
		border-color: var(--color-text-muted);
		color: var(--color-text);
	}

	.cat-btn.active {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.cat-btn span {
		line-height: 1;
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
		padding: 0.35rem;
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

	.priority-row, .color-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.field-label {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
		min-width: 48px;
	}

	.priority-btns {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
	}

	.priority-btn {
		padding: 2px 8px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: 10px;
		color: var(--color-text-muted);
		font-size: 0.7rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.priority-btn:hover {
		border-color: var(--pri-color);
		color: var(--pri-color);
	}

	.priority-btn.active {
		border-color: var(--pri-color);
		color: var(--pri-color);
		background: color-mix(in srgb, var(--pri-color) 10%, transparent);
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

	.form-scope-hint {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		line-height: 1.3;
		margin: 0;
	}

	/* Entries */
	.entries {
		flex: 1;
		overflow-y: auto;
	}

	.entry {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		padding: 0.6rem var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		border-left: 3px solid transparent;
		cursor: pointer;
		transition: background var(--duration-fast);
	}

	.entry:hover {
		background: var(--color-primary);
	}

	.entry.terminal {
		opacity: 0.55;
	}

	/* Priority accent on left border — only for non-routine */
	.entry:not(.terminal) {
		border-left-color: var(--pri-accent, transparent);
	}

	/* Category icon */
	.entry-icon {
		flex-shrink: 0;
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
		border: 2px solid var(--status-color, var(--color-primary));
		color: var(--icon-color, var(--color-text-muted));
		margin-top: 1px;
	}

	.entry-info {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
		flex: 1;
	}

	.entry-top {
		display: flex;
		align-items: center;
		gap: 6px;
	}

	.entry-label {
		font-size: 0.85rem;
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.status-badge {
		flex-shrink: 0;
		padding: 1px 6px;
		border-radius: 8px;
		font-size: 0.6rem;
		font-weight: 600;
		color: white;
		text-transform: uppercase;
		letter-spacing: 0.02em;
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
		display: flex;
		align-items: center;
		gap: 4px;
		flex-wrap: wrap;
	}

	.cat-tag {
		font-weight: 500;
		opacity: 1;
	}

	.pri-tag {
		font-weight: 600;
	}

	.inline-swatches {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
		padding-top: 4px;
	}

	/* Status dropdown */
	.status-dropdown {
		display: flex;
		flex-direction: column;
		gap: 1px;
		padding: 4px 0;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		margin-top: 4px;
	}

	.status-option {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 4px 10px;
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 0.75rem;
		cursor: pointer;
		text-align: left;
		transition: background var(--duration-fast);
	}

	.status-option:hover {
		background: var(--color-primary);
	}

	.status-option.current {
		font-weight: 600;
	}

	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	/* Action buttons */
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
		width: 26px;
		height: 26px;
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: color var(--duration-fast), background var(--duration-fast);
	}

	.status-btn:hover {
		color: var(--color-accent);
		background: rgba(230, 57, 70, 0.1);
	}

	.color-btn:hover {
		background: rgba(230, 57, 70, 0.1);
	}

	.color-dot-mini {
		width: 12px;
		height: 12px;
		border-radius: 50%;
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
