<script lang="ts">
	import { api } from '$lib/api';
	import { canPlot, canOperate } from '$lib/stores/session';
	import { beaconPath } from '$lib/stores/paths';
	import { formatPathDisplay } from '$lib/aprsPath';
	import PathHint from './PathHint.svelte';
	import { annotationList, operations, activeOperationId } from '$lib/stores/annotations';
	import { missions as netMissions, activeNet } from '$lib/stores/netcontrol';
	import { timeAgo } from '$lib/utils';
	import type { Annotation, AnnotationCategory, AnnotationPriority, AnnotationTemplate } from '$lib/types';
	import {
		categoryMeta, statusMeta, priorityMeta, allCategories,
		statusLabelToValue, statusValueToLabel, statusColor, isTerminalStatus,
		geometryMeta, canTransmitViaAPRS, categoryCanTransmitViaAPRS,
	} from '$lib/annotationMeta';

	let {
		onFlyToAnnotation,
		onStartDraw,
		onPreviewChange,
		onStartEdit,
		onStopEdit,
		focusedAnnotationId = null,
		onFocusConsumed,
	}: {
		onFlyToAnnotation?: (ann: Annotation) => void;
		onStartDraw?: (mode: 'point' | 'line' | 'area') => void;
		onPreviewChange?: (geometry: string | null, color: string) => void;
		onStartEdit?: (id: string) => void;
		onStopEdit?: () => void;
		focusedAnnotationId?: string | null;
		onFocusConsumed?: () => void;
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

	// Template state
	let templates = $state<AnnotationTemplate[]>([]);
	let showTemplates = $state(false);

	// Filter state
	let filterCategory = $state<AnnotationCategory | ''>('');
	let filterExpanded = $state(false);

	// Focused annotation highlight
	let highlightedId = $state<string | null>(null);
	let highlightTimer: ReturnType<typeof setTimeout> | null = null;

	$effect(() => {
		if (!focusedAnnotationId) return;
		// Use a microtask to allow the DOM to render the entries first
		const id = focusedAnnotationId;
		onFocusConsumed?.();
		requestAnimationFrame(() => {
			const el = document.querySelector(`[data-annotation-id="${id}"]`) as HTMLElement | null;
			if (el) {
				el.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
				highlightedId = id;
				if (highlightTimer) clearTimeout(highlightTimer);
				highlightTimer = setTimeout(() => {
					highlightedId = null;
				}, 2000);
			}
		});
	});

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
		showTemplates = false;
		// Load templates if needed.
		if (templates.length === 0) {
			api.annotationTemplates().then((t) => templates = t).catch(() => {});
		}
	}

	function handleSelectTemplate(tmpl: AnnotationTemplate) {
		formCategory = tmpl.category;
		formType = tmpl.type;
		formLabel = tmpl.name;
		formDescription = tmpl.description;
		formPriority = tmpl.defaultPriority;
		formColor = categoryMeta[tmpl.category]?.defaultColor || '#e63946';
		showTemplates = false;
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

	async function handlePromoteToMission(ann: Annotation, e: MouseEvent) {
		e.stopPropagation();
		try {
			await api.promoteAnnotation(ann.id);
		} catch (err: any) {
			console.error('Promote failed:', err.message);
		}
	}

	async function handleUnlink(ann: Annotation, e: MouseEvent) {
		e.stopPropagation();
		try {
			await api.unlinkAnnotation(ann.id);
		} catch (err: any) {
			console.error('Unlink failed:', err.message);
		}
	}

	async function handleUnlinkSpecific(ann: Annotation, missionId: string, e: MouseEvent) {
		e.stopPropagation();
		try {
			await api.unlinkAnnotation(ann.id, missionId);
		} catch (err: any) {
			console.error('Unlink specific failed:', err.message);
		}
	}

	async function handleAddToMission(ann: Annotation, missionId: string, e: MouseEvent) {
		e.stopPropagation();
		addMissionPickerId = null;
		try {
			await api.linkAnnotation(ann.id, missionId);
		} catch (err: any) {
			console.error('Add to mission failed:', err.message);
		}
	}

	function toggleAddMissionPicker(annId: string, e: MouseEvent) {
		e.stopPropagation();
		addMissionPickerId = addMissionPickerId === annId ? null : annId;
	}

	async function handleTransmit(ann: Annotation, e: MouseEvent) {
		e.stopPropagation();
		try {
			await api.transmitAnnotation(ann.id);
		} catch (err: any) {
			console.error('Transmit failed:', err.message);
		}
	}

	async function handleStopTransmit(ann: Annotation, e: MouseEvent) {
		e.stopPropagation();
		try {
			await api.stopTransmitAnnotation(ann.id);
		} catch (err: any) {
			console.error('Stop transmit failed:', err.message);
		}
	}

	function toggleStatusDropdown(id: string, e: MouseEvent) {
		e.stopPropagation();
		statusDropdownId = statusDropdownId === id ? null : id;
	}

	// Add-to-mission picker
	let addMissionPickerId = $state<string | null>(null);

	const trafficColors: Record<string, string> = {
		routine: '#22c55e',
		priority: '#f59e0b',
		welfare: '#3b82f6',
		emergency: '#ef4444'
	};

	// File import
	let importInput = $state<HTMLInputElement>(null!);
	let importing = $state(false);

	async function handleImportFile(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		importing = true;
		try {
			await api.importAnnotations(file);
		} catch (err: any) {
			console.error('Import failed:', err.message);
		} finally {
			importing = false;
			input.value = '';
		}
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
			<span class="subtitle">Local map markups shared with your team.</span>
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
				<button
					class="import-btn"
					title="Import GPX/KML file"
					disabled={importing}
					onclick={() => importInput.click()}
				>
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M2 10v3h12v-3M8 2v8M5 7l3 3 3-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
					{importing ? 'Importing...' : 'Import'}
				</button>
				<input
					type="file"
					accept=".gpx,.kml"
					style="display:none"
					bind:this={importInput}
					onchange={handleImportFile}
				/>
				<button class="add-btn" onclick={handleStartCreate}>
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M8 2v12M2 8h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
					Add
				</button>
			{/if}
		</div>
	</div>

	<div class="tx-legend" title="APRS objects and items are named points. Lines and areas stay on Nymeria maps.">
		<span class="tx-legend-item">
			<span class="tx-legend-type">Point</span>
			<span class="aprs-tx-badge">APRS</span>
		</span>
		<span class="tx-legend-item">
			<span class="tx-legend-type">Line</span>
			<span class="aprs-local-badge">Local</span>
		</span>
		<span class="tx-legend-item">
			<span class="tx-legend-type">Area</span>
			<span class="aprs-local-badge">Local</span>
		</span>
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
			<!-- Template picker toggle -->
			{#if templates.length > 0}
				<button class="template-toggle" onclick={() => showTemplates = !showTemplates}>
					<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
						<path d="M4 2h8v12H4zM7 5h2M7 8h2M7 11h2" stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round"/>
					</svg>
					{showTemplates ? 'Hide templates' : 'Use a template'}
					<svg width="10" height="10" viewBox="0 0 16 16" fill="none" class:rotated={showTemplates}>
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
			{/if}

			{#if showTemplates}
				<div class="template-packs">
					{#each ['event', 'sar', 'disaster'] as pack}
						{@const packTemplates = templates.filter((t) => t.pack === pack)}
						{#if packTemplates.length > 0}
							<div class="template-pack">
								<span class="pack-label">{pack.charAt(0).toUpperCase() + pack.slice(1)}</span>
								{#each packTemplates as tmpl}
									<button class="template-item" onclick={() => handleSelectTemplate(tmpl)}>
										<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
											<path d={categoryMeta[tmpl.category]?.icon || ''} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
										</svg>
										{tmpl.name}
									</button>
								{/each}
							</div>
						{/if}
					{/each}
				</div>
			{/if}

			<!-- Category selector -->
			<div class="category-grid">
				{#each allCategories as cat}
					<button
						class="cat-btn"
						class:active={formCategory === cat}
						onclick={() => handleSelectCategory(cat)}
						title={categoryCanTransmitViaAPRS(cat)
							? `${categoryMeta[cat].label} — point annotations can be transmitted as APRS objects`
							: `${categoryMeta[cat].label} — local only, cannot be sent over APRS`}
					>
						{#if categoryCanTransmitViaAPRS(cat)}
							<span class="cat-aprs" aria-hidden="true">APRS</span>
						{/if}
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
							<path d={categoryMeta[cat].icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
						</svg>
						<span>{categoryMeta[cat].label}</span>
					</button>
				{/each}
			</div>

			<!-- Geometry type (auto-restricted by category) with APRS capability -->
			{#if categoryMeta[formCategory].allowedGeometry.length > 1}
				<div class="type-selector">
					{#each categoryMeta[formCategory].allowedGeometry as t}
						<button
							class="type-btn"
							class:active={formType === t}
							class:aprs-capable={canTransmitViaAPRS(t)}
							title={geometryMeta[t].aprsNote}
							onclick={() => { formType = t; pendingGeometry = null; waitingForDraw = false; }}
						>
							<span class="type-btn-label">{geometryMeta[t].label}</span>
							{#if canTransmitViaAPRS(t)}
								<span class="aprs-tx-badge">APRS</span>
							{:else}
								<span class="aprs-local-badge">Local</span>
							{/if}
						</button>
					{/each}
				</div>
			{:else}
				<div class="type-locked" class:aprs-capable={canTransmitViaAPRS(formType)}>
					<span class="type-locked-label">{geometryMeta[formType].label}</span>
					{#if canTransmitViaAPRS(formType)}
						<span class="aprs-tx-badge">APRS</span>
						<span class="type-locked-note">Can be transmitted as an APRS object</span>
					{:else}
						<span class="aprs-local-badge">Local</span>
						<span class="type-locked-note">Cannot be sent over APRS</span>
					{/if}
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
			{#if canTransmitViaAPRS(formType)}
				<p class="form-scope-hint">Visible to all connected users. After saving, an operator can transmit this as an APRS object (name truncated to 9 characters).</p>
			{:else}
				<p class="form-scope-hint">Visible to all connected users. {geometryMeta[formType].label}s stay on Nymeria maps — APRS has no standard encoding for them.</p>
			{/if}
		</div>
	{/if}

	<div class="entries">
		{#if filteredList.length === 0 && !creating}
			<div class="empty">
				{#if filterCategory}
					<p>No {categoryMeta[filterCategory].label.toLowerCase()} annotations.</p>
				{:else}
					<p>No annotations yet.</p>
					<p class="empty-hint">Annotations are local markers, lines, and areas that appear on every connected user's map. Point annotations can later be transmitted as APRS objects; lines and areas stay local.</p>
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
					class:focused={highlightedId === ann.id}
					style="--pri-accent: {pMeta.color}"
					role="button"
					tabindex="0"
					data-annotation-id={ann.id}
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
							<span class="geo-tag">{geometryMeta[ann.type]?.label ?? ann.type}</span>
							{#if canTransmitViaAPRS(ann.type)}
								<span
									class="aprs-tx-badge"
									class:live={ann.transmitting}
									title={ann.transmitting
										? 'Currently transmitting as an APRS object'
										: geometryMeta.point.aprsNote}
								>APRS</span>
							{/if}
							{#if ann.priority && ann.priority !== 'routine'}
								<span class="pri-tag" style="color: {pMeta.color}">{pMeta.label}</span>
							{/if}
							{#if ann.createdByName}
								&middot; {ann.createdByName}
							{/if}
							&middot; {timeAgo(ann.createdAt)}
						</span>
						{#if ann.missionIds?.length > 0}
							<div class="mission-chips">
								{#each ann.missionIds as mid}
									{@const linkedMission = $netMissions.find((m) => m.id === mid)}
									{#if linkedMission}
										<span class="mission-chip">
											<span class="mission-chip-dot" style="background: {trafficColors[linkedMission.priority] ?? '#6b7280'}"></span>
											<span class="mission-chip-title">{linkedMission.title}</span>
											{#if $canPlot}
												<button class="mission-chip-remove" onclick={(e) => handleUnlinkSpecific(ann, mid, e)}>×</button>
											{/if}
										</span>
									{:else}
										<span class="mission-chip mission-chip-unknown">
											<span class="mission-chip-title">Mission</span>
											{#if $canPlot}
												<button class="mission-chip-remove" onclick={(e) => handleUnlinkSpecific(ann, mid, e)}>×</button>
											{/if}
										</span>
									{/if}
								{/each}
								{#if $canPlot && $activeNet && !isTerminalStatus(ann.status)}
									{@const availableMissions = $netMissions.filter((m) => m.status !== 'complete' && !ann.missionIds?.includes(m.id))}
									{#if availableMissions.length > 0}
										<button class="add-mission-btn" onclick={(e) => toggleAddMissionPicker(ann.id, e)}>+ Mission</button>
									{/if}
								{/if}
							</div>
						{:else}
							<div class="mission-chips">
								{#if $canPlot && ann.type === 'point' && !isTerminalStatus(ann.status)}
									<button class="promote-btn" onclick={(e) => handlePromoteToMission(ann, e)}>
										<svg width="10" height="10" viewBox="0 0 16 16" fill="none"><path d="M4 2h8v12H4zM7 5h2M7 8h2" stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round"/></svg>
										Create Mission
									</button>
								{/if}
								{#if $canPlot && $activeNet && !isTerminalStatus(ann.status)}
									{@const availableMissions = $netMissions.filter((m) => m.status !== 'complete')}
									{#if availableMissions.length > 0}
										<button class="add-mission-btn" onclick={(e) => toggleAddMissionPicker(ann.id, e)}>+ Add to mission</button>
									{/if}
								{/if}
							</div>
						{/if}
						{#if addMissionPickerId === ann.id}
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<!-- svelte-ignore a11y_click_events_have_key_events -->
							<div class="add-mission-picker" onclick={(e) => e.stopPropagation()}>
								{#each $netMissions.filter((m) => m.status !== 'complete' && !ann.missionIds?.includes(m.id)) as m}
									<button class="add-mission-option" onclick={(e) => handleAddToMission(ann, m.id, e)}>
										<span class="mission-chip-dot" style="background: {trafficColors[m.priority] ?? '#6b7280'}"></span>
										<span class="add-mission-title">{m.title}</span>
										<span class="add-mission-status">{m.status}</span>
									</button>
								{/each}
								{#if $netMissions.filter((m) => m.status !== 'complete' && !ann.missionIds?.includes(m.id)).length === 0}
									<span class="add-mission-empty">No available missions</span>
								{/if}
							</div>
						{/if}
						{#if $canOperate && ann.type === 'point' && !isTerminalStatus(ann.status)}
							<div class="transmit-row">
								{#if ann.transmitting}
									<button
										class="transmit-btn active"
										title="Stop APRS object transmission via {formatPathDisplay($beaconPath)}"
										onclick={(e) => handleStopTransmit(ann, e)}
									>
										<svg width="10" height="10" viewBox="0 0 16 16" fill="none"><path d="M8 2v12M4 6l4-4 4 4M3 10a5 5 0 0 0 10 0" stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round"/></svg>
										Stop TX
									</button>
								{:else}
									<button
										class="transmit-btn"
										title="Transmit as APRS object via {formatPathDisplay($beaconPath)}"
										onclick={(e) => handleTransmit(ann, e)}
									>
										<svg width="10" height="10" viewBox="0 0 16 16" fill="none"><path d="M8 2v12M4 6l4-4 4 4M3 10a5 5 0 0 0 10 0" stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round"/></svg>
										Transmit
									</button>
								{/if}
								<PathHint kind="beacon" />
							</div>
						{/if}
						{#if colorEditId === ann.id}
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<!-- svelte-ignore a11y_click_events_have_key_events -->
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
							<!-- svelte-ignore a11y_click_events_have_key_events -->
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
		gap: 8px;
		align-items: center;
		flex-shrink: 0;
	}

	.tx-legend {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 10px 14px;
		padding: 6px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		background: rgba(15, 52, 96, 0.25);
	}

	.tx-legend-item {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}

	.tx-legend-type {
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--color-text-muted);
		letter-spacing: 0.02em;
	}

	.filter-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
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

	.import-btn {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 8px 12px;
		min-height: 36px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.85rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.import-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.import-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.add-btn {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 8px 12px;
		min-height: 36px;
		background: none;
		border: 1px solid var(--color-accent);
		border-radius: var(--radius-sm);
		color: var(--color-accent);
		font-size: 0.85rem;
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
		gap: 6px;
		padding: 8px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		overflow-x: auto;
		flex-shrink: 0;
	}

	.filter-chip {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 6px 12px;
		min-height: 32px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: 16px;
		color: var(--color-text-muted);
		font-size: 0.75rem;
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
		gap: 6px;
	}

	.cat-btn {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 4px;
		padding: 10px 4px;
		min-height: 44px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.7rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.cat-aprs {
		position: absolute;
		top: 3px;
		right: 3px;
		padding: 0 3px;
		border-radius: 2px;
		font-size: 0.5rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		line-height: 1.4;
		color: #2a9d8f;
		background: rgba(42, 157, 143, 0.16);
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
		gap: 6px;
		padding: 10px 8px;
		min-height: 44px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.85rem;
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

	.type-btn.aprs-capable.active {
		border-color: #2a9d8f;
	}

	.type-locked {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 6px 8px;
		padding: 8px 10px;
		min-height: 40px;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		background: rgba(15, 52, 96, 0.35);
	}

	.type-locked.aprs-capable {
		border-color: rgba(42, 157, 143, 0.45);
	}

	.type-locked-label {
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.type-locked-note {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		line-height: 1.3;
	}

	.aprs-tx-badge,
	.aprs-local-badge {
		display: inline-flex;
		align-items: center;
		padding: 1px 5px;
		border-radius: 3px;
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		line-height: 1.4;
		flex-shrink: 0;
	}

	.aprs-tx-badge {
		color: #2a9d8f;
		background: rgba(42, 157, 143, 0.15);
		border: 1px solid rgba(42, 157, 143, 0.4);
	}

	.aprs-tx-badge.live {
		color: #e63946;
		background: rgba(230, 57, 70, 0.15);
		border-color: rgba(230, 57, 70, 0.5);
		animation: pulse-tx 1.5s ease-in-out infinite;
	}

	.aprs-local-badge {
		color: var(--color-text-muted);
		background: rgba(108, 117, 125, 0.15);
		border: 1px solid rgba(108, 117, 125, 0.35);
	}

	.geo-tag {
		text-transform: lowercase;
	}

	.form-input {
		width: 100%;
		padding: 10px 12px;
		min-height: 44px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 1rem;
		outline: none;
		transition: border-color var(--duration-fast);
		box-sizing: border-box;
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
		gap: 6px;
		flex-wrap: wrap;
	}

	.priority-btn {
		padding: 6px 12px;
		min-height: 36px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: 18px;
		color: var(--color-text-muted);
		font-size: 0.8rem;
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
		gap: 8px;
		flex-wrap: wrap;
	}

	.swatch {
		width: 32px;
		height: 32px;
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
		gap: 6px;
		padding: 10px 14px;
		min-height: 44px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		cursor: pointer;
		transition: border-color var(--duration-fast);
	}

	.draw-btn:hover {
		border-color: var(--color-accent);
	}

	.draw-status {
		font-size: 0.85rem;
		padding: 10px 0;
	}

	.draw-status.done {
		color: var(--color-success, #2a9d8f);
	}

	.draw-status.waiting {
		color: var(--color-accent);
	}

	.redraw-btn {
		margin-left: auto;
		padding: 8px 12px;
		min-height: 36px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.8rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.redraw-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.form-actions {
		display: flex;
		gap: 8px;
	}

	.btn {
		padding: 10px 16px;
		min-height: 44px;
		border-radius: var(--radius-sm);
		font-size: 0.9rem;
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
		gap: 10px;
		padding: 10px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		border-left: 3px solid transparent;
		cursor: pointer;
		transition: background var(--duration-fast);
	}

	.entry:hover {
		background: var(--color-primary);
	}

	.entry.focused {
		animation: annotation-focus-flash 2s ease-out;
	}

	@keyframes annotation-focus-flash {
		0% {
			background: rgba(230, 57, 70, 0.25);
			box-shadow: inset 0 0 0 2px var(--color-accent);
		}
		60% {
			background: rgba(230, 57, 70, 0.10);
			box-shadow: inset 0 0 0 1px var(--color-accent);
		}
		100% {
			background: transparent;
			box-shadow: none;
		}
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
		width: 32px;
		height: 32px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
		border: 2px solid var(--status-color, var(--color-primary));
		color: var(--icon-color, var(--color-text-muted));
		margin-top: 2px;
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
		font-size: 0.95rem;
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.status-badge {
		flex-shrink: 0;
		padding: 2px 8px;
		border-radius: 8px;
		font-size: 0.65rem;
		font-weight: 600;
		color: white;
		text-transform: uppercase;
		letter-spacing: 0.02em;
	}

	.entry-desc {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.entry-meta {
		font-size: 0.75rem;
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

	/* Mission chips */
	.mission-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
		padding: 4px 0;
	}

	.mission-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 4px 8px;
		min-height: 32px;
		font-size: 0.75rem;
		color: var(--color-accent);
	}

	.mission-chip-unknown {
		color: var(--color-text-muted);
	}

	.mission-chip-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.mission-chip-title {
		max-width: 120px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.mission-chip-remove {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 0.85rem;
		padding: 4px 6px;
		min-width: 28px;
		min-height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		line-height: 1;
		margin: -4px -6px -4px 0;
	}

	.mission-chip-remove:hover {
		color: #ef4444;
	}

	.add-mission-btn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 6px 10px;
		min-height: 32px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		cursor: pointer;
		transition: color var(--duration-fast), border-color var(--duration-fast);
	}

	.add-mission-btn:hover {
		color: var(--color-accent);
		border-color: var(--color-accent);
	}

	.add-mission-picker {
		display: flex;
		flex-direction: column;
		gap: 1px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		margin-top: 4px;
		max-height: 120px;
		overflow-y: auto;
	}

	.add-mission-option {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 12px;
		min-height: 44px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		font-size: 0.8rem;
		text-align: left;
		cursor: pointer;
		transition: background var(--duration-fast);
	}

	.add-mission-option:hover {
		background: var(--color-primary);
	}

	.add-mission-option:last-child {
		border-bottom: none;
	}

	.add-mission-title {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.add-mission-status {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		flex-shrink: 0;
	}

	.add-mission-empty {
		padding: 10px 12px;
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	.promote-btn {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 6px 10px;
		min-height: 32px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: 4px;
		color: var(--color-text-muted);
		font-size: 0.75rem;
		cursor: pointer;
		transition: color var(--duration-fast), border-color var(--duration-fast);
		margin-top: 2px;
	}

	.promote-btn:hover {
		color: var(--color-accent);
		border-color: var(--color-accent);
	}

	.transmit-row {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		margin-top: 4px;
	}

	.transmit-btn {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 6px 10px;
		min-height: 32px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: 4px;
		color: var(--color-text-muted);
		font-size: 0.75rem;
		cursor: pointer;
		white-space: nowrap;
	}

	.transmit-btn:hover {
		color: var(--color-accent);
		border-color: var(--color-accent);
	}

	.transmit-btn.active {
		color: #e63946;
		border-color: #e63946;
		animation: pulse-tx 1.5s ease-in-out infinite;
	}

	@keyframes pulse-tx {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.5; }
	}

	.inline-swatches {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		padding-top: 6px;
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
		gap: 8px;
		padding: 10px 12px;
		min-height: 44px;
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 0.8rem;
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
		gap: 4px;
		flex-shrink: 0;
	}

	.action-btn {
		flex-shrink: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
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
		width: 14px;
		height: 14px;
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

	/* Template picker */
	.template-toggle {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 8px 12px;
		min-height: 36px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.8rem;
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.template-toggle:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.template-toggle svg.rotated {
		transform: rotate(180deg);
	}

	.template-packs {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 6px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
	}

	.template-pack {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.pack-label {
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 2px 6px;
	}

	.template-item {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 12px;
		min-height: 44px;
		background: none;
		border: none;
		border-radius: 4px;
		color: var(--color-text);
		font-size: 0.8rem;
		text-align: left;
		cursor: pointer;
		transition: background var(--duration-fast);
	}

	.template-item:hover {
		background: var(--color-primary);
	}
</style>
