<script lang="ts">
	import { api } from '$lib/api';
	import type { Annotation, AnnotationCategory, Net } from '$lib/types';
	import { netAnnotations, orderedCheckpoints } from '$lib/stores/netcontrol';
	import { categoryMeta } from '$lib/annotationMeta';
	import { eventTemplates, type LocationTemplate } from '$lib/data/locationTemplates';
	import { showToast } from '$lib/stores/toast';

	let {
		net,
		onFlyTo,
		onPlaceOnMap,
		mapClickedCoords = null,
		onMapCoordsConsumed,
	}: {
		net: Net;
		onFlyTo?: (lat: number, lon: number) => void;
		onPlaceOnMap?: (id: string | null, name: string, mode: 'update' | 'form') => void;
		mapClickedCoords?: { lat: number; lon: number } | null;
		onMapCoordsConsumed?: () => void;
	} = $props();

	// Point-only event categories for location management.
	const locationCategories: AnnotationCategory[] = [
		'checkpoint', 'aid', 'staging', 'shelter', 'hazard', 'parking', 'start', 'finish', 'general',
	];

	// Edit state
	let editingId = $state<string | null>(null);
	let editLabel = $state('');
	let editShortName = $state('');
	let editCategory = $state<AnnotationCategory>('general');
	let editDescription = $state('');
	let editLat = $state('');
	let editLon = $state('');

	// Add form state
	let showAddForm = $state(false);
	let newLabel = $state('');
	let newShortName = $state('');
	let newCategory = $state<AnnotationCategory>('general');
	let newDescription = $state('');
	let newLat = $state('');
	let newLon = $state('');

	// Template modal
	let showTemplateModal = $state(false);

	// Copy from previous
	let showCopyModal = $state(false);
	let previousNets = $state<Net[]>([]);
	let loadingNets = $state(false);

	// Import
	let fileInputRef = $state<HTMLInputElement>();
	let importing = $state(false);

	// Saving feedback
	let saving = $state(false);

	// Checkpoint meta editing
	let editSeqNum = $state('');
	let passageLabel = $state('lead');

	// Get checkpoint meta for an annotation.
	function getCheckpointSeq(annId: string): number | null {
		const cp = $orderedCheckpoints.find(c => c.meta.annotationId === annId);
		return cp?.meta.sequenceNumber ?? null;
	}

	// Auto-assign next sequence number for new checkpoint annotations.
	function nextCheckpointSeq(): number {
		if ($orderedCheckpoints.length === 0) return 1;
		return Math.max(...$orderedCheckpoints.map(c => c.meta.sequenceNumber)) + 1;
	}

	async function saveCheckpointMeta(annId: string, seqStr: string) {
		const seq = parseInt(seqStr, 10);
		if (isNaN(seq) || seq < 1) return;
		try {
			await api.updateCheckpointMeta(net.id, annId, { sequenceNumber: seq });
			showToast(`Checkpoint sequence set to #${seq}`, 'success');
		} catch (e: any) {
			showToast(e?.message || 'Failed to set checkpoint meta', 'error');
		}
	}

	async function handleLogPassage(annId: string) {
		try {
			await api.logPassage(net.id, annId, { label: passageLabel });
			showToast(`${passageLabel} passage logged`, 'success');
		} catch (e: any) {
			showToast(e?.message || 'Failed to log passage', 'error');
		}
	}

	// Consume map-clicked coordinates into the active form
	$effect(() => {
		if (mapClickedCoords) {
			if (editingId) {
				editLat = String(mapClickedCoords.lat.toFixed(6));
				editLon = String(mapClickedCoords.lon.toFixed(6));
			} else if (showAddForm) {
				newLat = String(mapClickedCoords.lat.toFixed(6));
				newLon = String(mapClickedCoords.lon.toFixed(6));
			}
			onMapCoordsConsumed?.();
		}
	});

	let sortedAnnotations = $derived(
		[...$netAnnotations].sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0))
	);

	function makeGeometry(lat: number, lon: number): string {
		return JSON.stringify({ type: 'Point', coordinates: [lon, lat] });
	}

	function extractCoords(a: Annotation): { lat: number; lon: number } | null {
		try {
			const geo = typeof a.geometry === 'string' ? JSON.parse(a.geometry) : a.geometry;
			if (geo?.type === 'Point' && geo.coordinates) {
				return { lat: geo.coordinates[1], lon: geo.coordinates[0] };
			}
		} catch { /* ignore */ }
		return null;
	}

	// --- Actions ---

	async function handleAdd() {
		if (!newLabel.trim()) return;
		saving = true;
		try {
			const lat = parseFloat(newLat);
			const lon = parseFloat(newLon);
			const hasCoords = !isNaN(lat) && !isNaN(lon) && (lat !== 0 || lon !== 0);
			const created = await api.createAnnotation({
				type: 'point',
				label: newLabel.trim(),
				shortName: newShortName.trim().toUpperCase(),
				category: newCategory,
				description: newDescription.trim(),
				geometry: hasCoords ? makeGeometry(lat, lon) : makeGeometry(0, 0),
				netId: net.id,
				sortOrder: $netAnnotations.length,
				priority: 'routine',
			});
			// Auto-set checkpoint meta for new checkpoint annotations.
			if (newCategory === 'checkpoint' && created?.id) {
				const seq = nextCheckpointSeq();
				await api.updateCheckpointMeta(net.id, created.id, { sequenceNumber: seq }).catch(() => {});
			}
			resetAddForm();
		} catch (e) {
			console.error('Create location annotation failed:', e);
		} finally {
			saving = false;
		}
	}

	function resetAddForm() {
		showAddForm = false;
		newLabel = '';
		newShortName = '';
		newCategory = 'general';
		newDescription = '';
		newLat = '';
		newLon = '';
	}

	function startEdit(a: Annotation) {
		editingId = a.id;
		editLabel = a.label;
		editShortName = a.shortName || '';
		editCategory = a.category;
		editDescription = a.description || '';
		const coords = extractCoords(a);
		editLat = coords?.lat ? String(coords.lat) : '';
		editLon = coords?.lon ? String(coords.lon) : '';
		// Populate checkpoint sequence if applicable.
		const seq = getCheckpointSeq(a.id);
		editSeqNum = seq != null ? String(seq) : String(nextCheckpointSeq());
	}

	async function saveEdit() {
		if (!editingId || !editLabel.trim()) return;
		saving = true;
		try {
			const lat = parseFloat(editLat);
			const lon = parseFloat(editLon);
			const hasCoords = !isNaN(lat) && !isNaN(lon) && (lat !== 0 || lon !== 0);
			await api.updateAnnotation(editingId, {
				label: editLabel.trim(),
				shortName: editShortName.trim().toUpperCase(),
				category: editCategory,
				description: editDescription.trim(),
				geometry: hasCoords ? makeGeometry(lat, lon) : makeGeometry(0, 0),
			});
			// Save checkpoint meta if category is checkpoint.
			if (editCategory === 'checkpoint' && editSeqNum) {
				await saveCheckpointMeta(editingId, editSeqNum);
			}
			editingId = null;
		} catch (e) {
			console.error('Update location annotation failed:', e);
		} finally {
			saving = false;
		}
	}

	function cancelEdit() {
		editingId = null;
	}

	async function handleDelete(a: Annotation) {
		if (!confirm(`Delete "${a.label}"?`)) return;
		try {
			await api.deleteAnnotation(a.id);
		} catch (e) {
			console.error('Delete location annotation failed:', e);
		}
	}

	async function handleMoveUp(a: Annotation, index: number) {
		if (index === 0) return;
		const prev = sortedAnnotations[index - 1];
		try {
			await Promise.all([
				api.updateAnnotation(a.id, { sortOrder: prev.sortOrder ?? 0 }),
				api.updateAnnotation(prev.id, { sortOrder: a.sortOrder ?? 0 }),
			]);
		} catch (e) {
			console.error('Reorder failed:', e);
		}
	}

	async function handleMoveDown(a: Annotation, index: number) {
		if (index >= sortedAnnotations.length - 1) return;
		const next = sortedAnnotations[index + 1];
		try {
			await Promise.all([
				api.updateAnnotation(a.id, { sortOrder: next.sortOrder ?? 0 }),
				api.updateAnnotation(next.id, { sortOrder: a.sortOrder ?? 0 }),
			]);
		} catch (e) {
			console.error('Reorder failed:', e);
		}
	}

	function handleFlyTo(a: Annotation) {
		const coords = extractCoords(a);
		if (coords && (coords.lat || coords.lon)) {
			onFlyTo?.(coords.lat, coords.lon);
		}
	}

	// --- Import ---

	function triggerImport() {
		fileInputRef?.click();
	}

	async function handleFileImport(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		importing = true;
		try {
			await api.importNetAnnotations(net.id, file);
		} catch (err) {
			console.error('Import failed:', err);
		} finally {
			importing = false;
			input.value = '';
		}
	}

	// --- Copy from previous ---

	async function openCopyModal() {
		showCopyModal = true;
		loadingNets = true;
		try {
			const nets = await api.nets();
			previousNets = nets.filter(n => n.id !== net.id);
		} catch {
			previousNets = [];
		} finally {
			loadingNets = false;
		}
	}

	async function handleCopy(sourceNetId: string) {
		try {
			await api.copyNetAnnotations(net.id, sourceNetId);
			showCopyModal = false;
		} catch (e) {
			console.error('Copy failed:', e);
		}
	}

	// --- Templates ---

	async function applyTemplate(locations: LocationTemplate[]) {
		showTemplateModal = false;
		saving = true;
		try {
			const base = $netAnnotations.length;
			for (let i = 0; i < locations.length; i++) {
				const loc = locations[i];
				await api.createAnnotation({
					type: 'point',
					label: loc.name,
					shortName: loc.shortName,
					category: loc.category,
					description: loc.description,
					geometry: makeGeometry(0, 0),
					netId: net.id,
					sortOrder: base + i,
					priority: 'routine',
				});
			}
		} catch (e) {
			console.error('Apply template failed:', e);
		} finally {
			saving = false;
		}
	}
</script>

<div class="loc-manager">
	<!-- Toolbar -->
	<div class="loc-toolbar">
		<button class="loc-btn loc-btn-primary" onclick={() => (showAddForm = !showAddForm)}>
			+ Add
		</button>
		<button class="loc-btn" onclick={triggerImport} disabled={importing}>
			{importing ? 'Importing...' : 'Import GPX/KML'}
		</button>
		<button class="loc-btn" onclick={openCopyModal}>
			Copy from...
		</button>
		<button class="loc-btn" onclick={() => (showTemplateModal = true)}>
			Template
		</button>
		<input
			bind:this={fileInputRef}
			type="file"
			accept=".gpx,.kml"
			class="hidden-input"
			onchange={handleFileImport}
		/>
	</div>

	<!-- Add form -->
	{#if showAddForm}
		<div class="loc-form">
			<div class="loc-form-row">
				<input type="text" bind:value={newLabel} placeholder="Location name" class="loc-input loc-input-name" />
				<input type="text" bind:value={newShortName} placeholder="Short" class="loc-input loc-input-short" maxlength="8" />
			</div>
			<div class="loc-form-row">
				<select bind:value={newCategory} class="loc-select">
					{#each locationCategories as cat}
						<option value={cat}>{categoryMeta[cat].label}</option>
					{/each}
				</select>
			</div>
			<textarea bind:value={newDescription} rows="2" placeholder="Description (optional)" class="loc-textarea"></textarea>
			<div class="loc-form-row loc-coord-row">
				<input type="text" bind:value={newLat} placeholder="Lat" inputmode="decimal" class="loc-input" />
				<input type="text" bind:value={newLon} placeholder="Lon" inputmode="decimal" class="loc-input" />
				<button
					class="loc-btn loc-btn-map"
					type="button"
					title="Place on map"
					onclick={() => onPlaceOnMap?.(null, newLabel.trim() || 'New location', 'form')}
				>
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M8 1C5.2 1 3 3.2 3 6c0 4 5 9 5 9s5-5 5-9c0-2.8-2.2-5-5-5z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
						<circle cx="8" cy="6" r="1.5" fill="currentColor"/>
					</svg>
					Map
				</button>
			</div>
			<div class="loc-form-actions">
				<button class="loc-btn" onclick={resetAddForm}>Cancel</button>
				<button class="loc-btn loc-btn-primary" onclick={handleAdd} disabled={!newLabel.trim() || saving}>
					{saving ? 'Saving...' : 'Add'}
				</button>
			</div>
		</div>
	{/if}

	<!-- Location list -->
	<div class="loc-list">
		{#each sortedAnnotations as ann, i (ann.id)}
			{@const coords = extractCoords(ann)}
			{@const hasCoords = coords && (coords.lat !== 0 || coords.lon !== 0)}
			{#if editingId === ann.id}
				<!-- Inline edit form -->
				<div class="loc-form loc-form-inline">
					<div class="loc-form-row">
						<input type="text" bind:value={editLabel} placeholder="Name" class="loc-input loc-input-name" />
						<input type="text" bind:value={editShortName} placeholder="Short" class="loc-input loc-input-short" maxlength="8" />
					</div>
					<div class="loc-form-row">
						<select bind:value={editCategory} class="loc-select">
							{#each locationCategories as cat}
								<option value={cat}>{categoryMeta[cat].label}</option>
							{/each}
						</select>
					</div>
					<textarea bind:value={editDescription} rows="2" placeholder="Description" class="loc-textarea"></textarea>
					{#if editCategory === 'checkpoint'}
						<div class="loc-form-row loc-cp-row">
							<label class="loc-cp-label">Seq #</label>
							<input type="number" bind:value={editSeqNum} min="1" class="loc-input loc-input-seq" />
						</div>
					{/if}
					<div class="loc-form-row loc-coord-row">
						<input type="text" bind:value={editLat} placeholder="Lat" inputmode="decimal" class="loc-input" />
						<input type="text" bind:value={editLon} placeholder="Lon" inputmode="decimal" class="loc-input" />
						<button
							class="loc-btn loc-btn-map"
							type="button"
							title="Place on map"
							onclick={() => onPlaceOnMap?.(editingId, editLabel.trim() || 'Location', 'form')}
						>
							<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
								<path d="M8 1C5.2 1 3 3.2 3 6c0 4 5 9 5 9s5-5 5-9c0-2.8-2.2-5-5-5z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
								<circle cx="8" cy="6" r="1.5" fill="currentColor"/>
							</svg>
							Map
						</button>
					</div>
					<div class="loc-form-actions">
						<button class="loc-btn" onclick={cancelEdit}>Cancel</button>
						<button class="loc-btn loc-btn-primary" onclick={saveEdit} disabled={!editLabel.trim() || saving}>Save</button>
					</div>
				</div>
			{:else}
				<!-- Location row -->
				<div class="loc-row">
					<div class="loc-order-btns">
						<button class="loc-order-btn" onclick={() => handleMoveUp(ann, i)} disabled={i === 0} title="Move up">&#9650;</button>
						<button class="loc-order-btn" onclick={() => handleMoveDown(ann, i)} disabled={i === sortedAnnotations.length - 1} title="Move down">&#9660;</button>
					</div>
					<div class="loc-info">
						<div class="loc-name-row">
							<span class="loc-name">{ann.label}</span>
							{#if ann.shortName}
								<span class="loc-short-badge">{ann.shortName}</span>
							{/if}
							{#if ann.category === 'checkpoint'}
								{@const seq = getCheckpointSeq(ann.id)}
								{#if seq != null}
									<span class="loc-seq-badge">#{seq}</span>
								{/if}
							{/if}
							<span class="loc-cat-badge" style="background: {categoryMeta[ann.category]?.defaultColor || '#6b7280'}">{categoryMeta[ann.category]?.label || ann.category}</span>
						</div>
						{#if hasCoords}
							<span class="loc-coords">{coords.lat.toFixed(4)}, {coords.lon.toFixed(4)}</span>
						{:else}
							<button class="loc-coords loc-coords-unset loc-coords-clickable" onclick={() => onPlaceOnMap?.(ann.id, ann.label, 'update')}>
								Click to place on map
							</button>
						{/if}
					</div>
					<div class="loc-actions">
						{#if ann.category === 'checkpoint' && getCheckpointSeq(ann.id) != null}
							<button class="loc-action-btn loc-action-passage" onclick={() => handleLogPassage(ann.id)} title="Log passage">
								<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
									<path d="M4 2v12M4 3h7l-2 3 2 3H4"/>
								</svg>
							</button>
						{/if}
						{#if hasCoords}
							<button class="loc-action-btn" onclick={() => handleFlyTo(ann)} title="Fly to">
								<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
									<circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/>
									<circle cx="8" cy="8" r="2" fill="currentColor"/>
								</svg>
							</button>
						{/if}
						<button class="loc-action-btn loc-action-map" onclick={() => onPlaceOnMap?.(ann.id, ann.label, 'update')} title="Set position on map">
							<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
								<path d="M8 1C5.2 1 3 3.2 3 6c0 4 5 9 5 9s5-5 5-9c0-2.8-2.2-5-5-5z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
								<circle cx="8" cy="6" r="1.5" fill="currentColor"/>
							</svg>
						</button>
						<button class="loc-action-btn" onclick={() => startEdit(ann)} title="Edit">
							<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
								<path d="M11.5 1.5l3 3L5 14H2v-3L11.5 1.5z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
							</svg>
						</button>
						<button class="loc-action-btn loc-action-danger" onclick={() => handleDelete(ann)} title="Delete">
							<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
								<path d="M2 4h12M5 4V2h6v2M6 7v5M10 7v5M3 4l1 10h8l1-10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
							</svg>
						</button>
					</div>
				</div>
			{/if}
		{/each}
		{#if sortedAnnotations.length === 0 && !showAddForm}
			<div class="loc-empty">
				No locations defined. Add locations or load a template.
			</div>
		{/if}
	</div>
</div>

<!-- Template modal -->
{#if showTemplateModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="loc-modal-backdrop" onmousedown={(e) => { if (e.target === e.currentTarget) showTemplateModal = false; }}>
		<div class="loc-modal" role="dialog" aria-label="Event templates">
			<div class="loc-modal-header">
				<span class="loc-modal-title">Load Template</span>
				<button class="loc-modal-close" onclick={() => (showTemplateModal = false)}>&times;</button>
			</div>
			<div class="loc-modal-body">
				{#each eventTemplates as tmpl}
					<div class="loc-template-card">
						<div class="loc-template-info">
							<span class="loc-template-name">{tmpl.name}</span>
							<span class="loc-template-desc">{tmpl.description}</span>
							<div class="loc-template-locations">
								{#each tmpl.locations as loc}
									<span class="loc-template-chip" style="background: {categoryMeta[loc.category]?.defaultColor || '#6b7280'}">{loc.shortName}</span>
								{/each}
							</div>
						</div>
						<button class="loc-btn loc-btn-primary" onclick={() => applyTemplate(tmpl.locations)}>
							Apply
						</button>
					</div>
				{/each}
			</div>
		</div>
	</div>
{/if}

<!-- Copy modal -->
{#if showCopyModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="loc-modal-backdrop" onmousedown={(e) => { if (e.target === e.currentTarget) showCopyModal = false; }}>
		<div class="loc-modal" role="dialog" aria-label="Copy from previous net">
			<div class="loc-modal-header">
				<span class="loc-modal-title">Copy Locations From</span>
				<button class="loc-modal-close" onclick={() => (showCopyModal = false)}>&times;</button>
			</div>
			<div class="loc-modal-body">
				{#if loadingNets}
					<div class="loc-modal-loading">Loading nets...</div>
				{:else if previousNets.length === 0}
					<div class="loc-modal-loading">No other nets found</div>
				{:else}
					{#each previousNets as pnet}
						<button class="loc-copy-row" onclick={() => handleCopy(pnet.id)}>
							<span class="loc-copy-name">{pnet.name}</span>
							<span class="loc-copy-status">{pnet.status}</span>
						</button>
					{/each}
				{/if}
			</div>
		</div>
	</div>
{/if}

<style>
	.loc-manager {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	/* Toolbar */
	.loc-toolbar {
		display: flex;
		gap: var(--space-xs);
		flex-wrap: wrap;
		padding: var(--space-xs) 0;
	}

	.loc-btn {
		padding: 5px 10px;
		font-size: 0.75rem;
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.12);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
		white-space: nowrap;
	}

	.loc-btn:hover:not(:disabled) {
		border-color: rgba(255, 255, 255, 0.25);
		color: var(--color-text);
	}

	.loc-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.loc-btn-primary {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: #fff;
	}

	.loc-btn-primary:hover:not(:disabled) {
		opacity: 0.9;
		border-color: var(--color-accent);
		color: #fff;
	}

	.hidden-input {
		display: none;
	}

	/* Form */
	.loc-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		padding: var(--space-sm);
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
	}

	.loc-form-inline {
		margin-bottom: var(--space-xs);
	}

	.loc-form-row {
		display: flex;
		gap: var(--space-xs);
	}

	.loc-input {
		flex: 1;
		padding: 6px 8px;
		font-size: 0.8rem;
		background: var(--color-bg);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-family: inherit;
		outline: none;
		transition: border-color var(--duration-fast);
	}

	.loc-input:focus {
		border-color: var(--color-accent);
	}

	.loc-input-name {
		flex: 2;
	}

	.loc-input-short {
		flex: 0 0 80px;
		text-transform: uppercase;
		font-family: 'SF Mono', 'Fira Code', monospace;
		font-size: 0.75rem;
	}

	.loc-select {
		flex: 1;
		padding: 6px 8px;
		font-size: 0.8rem;
		background: var(--color-bg);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-family: inherit;
		outline: none;
	}

	.loc-textarea {
		width: 100%;
		padding: 6px 8px;
		font-size: 0.8rem;
		background: var(--color-bg);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-family: inherit;
		resize: vertical;
		outline: none;
	}

	.loc-textarea:focus, .loc-select:focus {
		border-color: var(--color-accent);
	}

	.loc-form-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-xs);
	}

	/* List */
	.loc-list {
		display: flex;
		flex-direction: column;
	}

	.loc-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-xs) 0;
		border-bottom: 1px solid rgba(255, 255, 255, 0.04);
	}

	.loc-row:last-child {
		border-bottom: none;
	}

	.loc-order-btns {
		display: flex;
		flex-direction: column;
		gap: 1px;
		flex-shrink: 0;
	}

	.loc-order-btn {
		padding: 0;
		width: 18px;
		height: 14px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 8px;
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: 2px;
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
		line-height: 1;
	}

	.loc-order-btn:hover:not(:disabled) {
		border-color: rgba(255, 255, 255, 0.25);
		color: var(--color-text);
	}

	.loc-order-btn:disabled {
		opacity: 0.2;
		cursor: default;
	}

	.loc-info {
		flex: 1;
		min-width: 0;
	}

	.loc-name-row {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		flex-wrap: wrap;
	}

	.loc-name {
		font-size: 0.82rem;
		font-weight: 600;
		color: var(--color-text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.loc-short-badge {
		padding: 1px 5px;
		font-size: 0.65rem;
		font-family: 'SF Mono', 'Fira Code', monospace;
		font-weight: 600;
		background: rgba(255, 255, 255, 0.08);
		border-radius: 3px;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.loc-seq-badge {
		padding: 1px 5px;
		font-size: 0.65rem;
		font-family: 'SF Mono', 'Fira Code', monospace;
		font-weight: 700;
		background: rgba(42, 157, 143, 0.15);
		border-radius: 3px;
		color: #2a9d8f;
		flex-shrink: 0;
	}

	.loc-cp-row {
		align-items: center;
	}

	.loc-cp-label {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-weight: 600;
		white-space: nowrap;
	}

	.loc-input-seq {
		width: 60px;
	}

	.loc-action-passage {
		color: #2a9d8f;
	}

	.loc-action-passage:hover {
		background: rgba(42, 157, 143, 0.15);
	}

	.loc-cat-badge {
		padding: 1px 6px;
		font-size: 0.6rem;
		border-radius: var(--radius-full);
		color: #fff;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		flex-shrink: 0;
	}

	.loc-coords {
		font-size: 0.72rem;
		color: var(--color-text-muted);
		font-family: 'SF Mono', 'Fira Code', monospace;
	}

	.loc-coords-unset {
		font-style: italic;
		font-family: inherit;
		opacity: 0.5;
	}

	.loc-coords-clickable {
		background: none;
		border: none;
		padding: 0;
		cursor: pointer;
		color: #3b82f6;
		opacity: 0.8;
		font-size: 0.72rem;
		font-style: italic;
		transition: opacity var(--duration-fast);
	}

	.loc-coords-clickable:hover {
		opacity: 1;
		text-decoration: underline;
	}

	.loc-actions {
		display: flex;
		gap: 2px;
		flex-shrink: 0;
	}

	.loc-action-btn {
		padding: 4px;
		background: transparent;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		border-radius: 3px;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: color var(--duration-fast), background var(--duration-fast);
	}

	.loc-action-btn:hover {
		color: var(--color-text);
		background: rgba(255, 255, 255, 0.06);
	}

	.loc-action-danger:hover {
		color: #ef4444;
	}

	.loc-action-map:hover {
		color: #3b82f6;
	}

	.loc-coord-row {
		align-items: center;
	}

	.loc-btn-map {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 5px 8px;
		color: #3b82f6;
		border-color: rgba(59, 130, 246, 0.3);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.loc-btn-map:hover:not(:disabled) {
		border-color: #3b82f6;
		background: rgba(59, 130, 246, 0.1);
		color: #60a5fa;
	}

	.loc-empty {
		text-align: center;
		padding: var(--space-lg) var(--space-md);
		color: var(--color-text-muted);
		font-size: 0.82rem;
	}

	/* Modal */
	.loc-modal-backdrop {
		position: fixed;
		inset: 0;
		z-index: 600;
		background: rgba(0, 0, 0, 0.6);
		display: flex;
		justify-content: center;
		align-items: center;
		padding: var(--space-lg);
	}

	.loc-modal {
		width: min(480px, 90vw);
		max-height: min(70vh, 500px);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.loc-modal-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-md);
		border-bottom: 1px solid rgba(255, 255, 255, 0.06);
	}

	.loc-modal-title {
		font-weight: 600;
		font-size: 0.9rem;
	}

	.loc-modal-close {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.2rem;
		cursor: pointer;
		padding: 2px 6px;
		border-radius: 4px;
	}

	.loc-modal-close:hover {
		color: var(--color-text);
	}

	.loc-modal-body {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.loc-modal-loading {
		text-align: center;
		color: var(--color-text-muted);
		padding: var(--space-lg);
		font-size: 0.85rem;
	}

	/* Template cards */
	.loc-template-card {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		padding: var(--space-sm);
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: var(--radius-sm);
	}

	.loc-template-info {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.loc-template-name {
		font-weight: 600;
		font-size: 0.85rem;
	}

	.loc-template-desc {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.loc-template-locations {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
		margin-top: 2px;
	}

	.loc-template-chip {
		padding: 1px 5px;
		font-size: 0.6rem;
		font-family: 'SF Mono', 'Fira Code', monospace;
		border-radius: 3px;
		color: #fff;
	}

	/* Copy rows */
	.loc-copy-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		padding: var(--space-sm);
		background: none;
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		cursor: pointer;
		transition: background var(--duration-fast);
	}

	.loc-copy-row:hover {
		background: rgba(255, 255, 255, 0.05);
	}

	.loc-copy-name {
		font-weight: 600;
	}

	.loc-copy-status {
		font-size: 0.72rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
	}
</style>
