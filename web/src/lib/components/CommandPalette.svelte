<script lang="ts">
	import { api } from '$lib/api';
	import { timeAgo, formatCoord } from '$lib/utils';
	import { activeNet, checkIns, missions, notesByCheckIn } from '$lib/stores/netcontrol';
	import { currentUser } from '$lib/stores/session';
	import {
		paletteQuery, paletteFilter, paletteResults, recentCallsigns, emptyStateData,
		recordInteraction, noteCategoryMeta, severityMeta, statusColors, missionPriorityColors,
		trafficMeta, stationCategoryMeta,
		type PaletteResult, type PaletteFilter
	} from '$lib/stores/commandpalette';
	import type { NoteCategory, NoteSeverity, TrafficType, StationCategory, NetMission, NetNote } from '$lib/types';

	let {
		onFlyTo,
		onClose,
	}: {
		onFlyTo?: (lat: number, lon: number) => void;
		onClose: () => void;
	} = $props();

	// --- State machine ---
	type Mode = 'search' | 'context';
	let mode = $state<Mode>('search');
	let selected = $state<PaletteResult | null>(null);
	let highlightIndex = $state(0);

	// Note composer state
	let noteContent = $state('');
	let noteCategory = $state<NoteCategory>('general');
	let noteSeverity = $state<NoteSeverity>('info');
	let noteTarget = $state<'operator' | string>('operator'); // 'operator' or mission ID
	let noteSaving = $state(false);
	let noteSaved = $state(false);

	// Quick action dropdowns
	let showStatusDropdown = $state(false);
	let showMissionDropdown = $state(false);
	let showTrafficDropdown = $state(false);
	let showCategoryDropdown = $state(false);

	// Refs
	let searchInputRef = $state<HTMLInputElement>();
	let textareaRef = $state<HTMLTextAreaElement>();

	// Derived
	let results = $derived($paletteResults);
	let empty = $derived($emptyStateData);
	let net = $derived($activeNet);
	let allMissions = $derived($missions);
	let allCheckIns = $derived($checkIns);

	// Notes for selected operator (reactive — updates when notes store changes)
	let noteMap = $derived($notesByCheckIn);
	let selectedNotes = $derived.by(() => {
		if (!selected?.checkIn) return [];
		return (noteMap.get(selected.checkIn.id) ?? []).slice(0, 3);
	});

	// Missions for selected operator
	let selectedMissions = $derived.by(() => {
		if (!selected?.missionIds?.length) return [];
		return allMissions.filter((m) => selected!.missionIds.includes(m.id));
	});

	// --- Lifecycle ---
	$effect(() => {
		// Focus search input on mount
		if (searchInputRef) {
			searchInputRef.focus();
		}
	});

	// Reset highlight when results change
	$effect(() => {
		void results;
		highlightIndex = 0;
	});

	// --- Handlers ---

	function selectResult(result: PaletteResult) {
		selected = result;
		mode = 'context';
		recordInteraction(result.callsign);
		noteTarget = 'operator';
		noteContent = '';
		noteCategory = 'general';
		noteSeverity = 'info';
		noteSaved = false;
		showStatusDropdown = false;
		showMissionDropdown = false;
		showTrafficDropdown = false;
		showCategoryDropdown = false;
		// Focus textarea after DOM updates
		requestAnimationFrame(() => {
			textareaRef?.focus();
		});
	}

	function backToSearch() {
		mode = 'search';
		selected = null;
		showStatusDropdown = false;
		showMissionDropdown = false;
		showTrafficDropdown = false;
		showCategoryDropdown = false;
		requestAnimationFrame(() => {
			searchInputRef?.focus();
		});
	}

	function handleClose() {
		paletteQuery.set('');
		paletteFilter.set('all');
		onClose();
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) handleClose();
	}

	function handleSearchKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			handleClose();
			return;
		}
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			highlightIndex = Math.min(highlightIndex + 1, results.length - 1);
			return;
		}
		if (e.key === 'ArrowUp') {
			e.preventDefault();
			highlightIndex = Math.max(highlightIndex - 1, 0);
			return;
		}
		if (e.key === 'Enter' && results.length > 0) {
			e.preventDefault();
			selectResult(results[highlightIndex]);
			return;
		}
	}

	function handleContextKeydown(e: KeyboardEvent) {
		// Escape always closes
		if (e.key === 'Escape') {
			if (document.activeElement === textareaRef) {
				textareaRef?.blur();
				return;
			}
			handleClose();
			return;
		}

		// If textarea is focused, only handle Ctrl+Enter
		if (document.activeElement === textareaRef) {
			if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
				e.preventDefault();
				saveNote();
			}
			return;
		}

		// Shortcut keys when textarea is NOT focused
		if (e.key === 'Backspace') {
			backToSearch();
			return;
		}
		if (e.key === 'n' || e.key === 'N') {
			e.preventDefault();
			textareaRef?.focus();
			return;
		}
		if (e.key === 'f' || e.key === 'F') {
			e.preventDefault();
			flyToSelected();
			return;
		}
		if (e.key === 's' || e.key === 'S') {
			e.preventDefault();
			showStatusDropdown = !showStatusDropdown;
			showMissionDropdown = false;
			showTrafficDropdown = false;
			showCategoryDropdown = false;
			return;
		}
		if (e.key === 'm' || e.key === 'M') {
			e.preventDefault();
			showMissionDropdown = !showMissionDropdown;
			showStatusDropdown = false;
			showTrafficDropdown = false;
			showCategoryDropdown = false;
			return;
		}
		if (e.key === 't' || e.key === 'T') {
			e.preventDefault();
			showTrafficDropdown = !showTrafficDropdown;
			showStatusDropdown = false;
			showMissionDropdown = false;
			showCategoryDropdown = false;
			return;
		}
		if (e.key === 'c' || e.key === 'C') {
			e.preventDefault();
			showCategoryDropdown = !showCategoryDropdown;
			showStatusDropdown = false;
			showMissionDropdown = false;
			showTrafficDropdown = false;
			return;
		}
	}

	function flyToSelected() {
		if (selected?.lat != null && selected?.lon != null) {
			onFlyTo?.(selected.lat, selected.lon);
			handleClose();
		}
	}

	async function saveNote() {
		if (!noteContent.trim() || !net || !selected) return;
		noteSaving = true;
		try {
			const payload: {
				checkInId?: string;
				missionId?: string;
				content: string;
				category: string;
				severity: string;
			} = {
				content: noteContent.trim(),
				category: noteCategory,
				severity: noteSeverity,
			};

			if (noteTarget === 'operator' && selected.checkIn) {
				payload.checkInId = selected.checkIn.id;
			} else if (noteTarget !== 'operator') {
				payload.missionId = noteTarget;
				// Also attach to operator
				if (selected.checkIn) {
					payload.checkInId = selected.checkIn.id;
				}
			}

			await api.addNetNote(net.id, payload);
			noteContent = '';
			noteSaved = true;
			setTimeout(() => { noteSaved = false; }, 2000);
			textareaRef?.focus();
		} catch {
			// Error handled silently — user sees note didn't clear
		} finally {
			noteSaving = false;
		}
	}

	async function updateStatus(status: string) {
		if (!net || !selected?.checkIn) return;
		try {
			await api.updateCheckIn(net.id, selected.checkIn.id, { status: status as any });
			selected = { ...selected, status };
			showStatusDropdown = false;
		} catch { /* ignore */ }
	}

	async function assignMission(missionId: string) {
		if (!net || !selected?.checkIn) return;
		try {
			await api.assignMission(net.id, selected.checkIn.id, missionId);
			selected = {
				...selected,
				missionIds: [...(selected.missionIds ?? []), missionId],
			};
			showMissionDropdown = false;
		} catch { /* ignore */ }
	}

	async function updateTraffic(traffic: string) {
		if (!net || !selected?.checkIn) return;
		try {
			await api.updateCheckIn(net.id, selected.checkIn.id, { traffic: traffic as any });
			selected = { ...selected, traffic };
			showTrafficDropdown = false;
		} catch { /* ignore */ }
	}

	async function updateCategory(category: string) {
		if (!net || !selected?.checkIn) return;
		try {
			await api.updateCheckIn(net.id, selected.checkIn.id, { category: category as any });
			selected = { ...selected };
			showCategoryDropdown = false;
		} catch { /* ignore */ }
	}

	function handleRecentClick(callsign: string) {
		paletteQuery.set(callsign);
	}

	function setFilter(f: PaletteFilter) {
		paletteFilter.set(f);
	}

	const statuses = ['available', 'assigned', 'enroute', 'onscene', 'brb', 'missing', 'released'] as const;
	const trafficTypes = ['none', 'routine', 'priority', 'welfare', 'emergency'] as const;
	const categories = ['general', 'command', 'medical', 'sag', 'marshal', 'fixed', 'mobile', 'tactical'] as const;
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="cp-backdrop" onmousedown={handleBackdropClick} onkeydown={mode === 'search' ? handleSearchKeydown : handleContextKeydown}>
	<div class="cp-container" role="dialog" aria-label="Command palette">
		{#if mode === 'search'}
			<!-- SEARCH MODE -->
			<div class="cp-search-header">
				<svg class="cp-search-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
				</svg>
				<input
					bind:this={searchInputRef}
					bind:value={$paletteQuery}
					class="cp-search-input"
					placeholder="Search operators, callsigns, tactical names..."
					type="text"
					spellcheck="false"
					autocomplete="off"
				/>
				<kbd class="cp-kbd">ESC</kbd>
			</div>

			<!-- Filter buttons -->
			<div class="cp-filters">
				<button class="cp-filter-btn" class:active={$paletteFilter === 'all'} onclick={() => setFilter('all')}>All</button>
				<button class="cp-filter-btn" class:active={$paletteFilter === 'roster'} onclick={() => setFilter('roster')}>Roster</button>
				<button class="cp-filter-btn" class:active={$paletteFilter === 'tactical'} onclick={() => setFilter('tactical')}>Tactical</button>
			</div>

			<!-- Results or empty state -->
			<div class="cp-results">
				{#if results.length > 0}
					{#each results as result, i}
						<button
							class="cp-result-row"
							class:highlighted={i === highlightIndex}
							onmouseenter={() => { highlightIndex = i; }}
							onclick={() => selectResult(result)}
						>
							<span class="cp-status-dot" style="background: {statusColors[result.status] ?? '#555'}"></span>
							<span class="cp-result-callsign">{result.callsign}</span>
							{#if result.tacticalCall}
								<span class="cp-result-tactical">"{result.tacticalCall}"</span>
							{/if}
							{#if result.operatorName}
								<span class="cp-result-name">{result.operatorName}</span>
							{/if}
							{#if result.status && result.type === 'checkin'}
								<span class="cp-result-badge" style="background: {statusColors[result.status] ?? '#555'}">{result.status}</span>
							{/if}
							{#if result.checkIn?.category && result.checkIn.category !== 'general'}
								{@const rCat = stationCategoryMeta[result.checkIn.category]}
								{#if rCat}
									<span class="cp-result-badge" style="background: {rCat.color}">{rCat.short}</span>
								{/if}
							{/if}
							<span class="cp-result-time">{timeAgo(result.lastHeard)}</span>
						</button>
					{/each}
				{:else if !$paletteQuery.trim()}
					<!-- Empty state -->
					<div class="cp-empty">
						{#if empty.activeOperators.length > 0}
							<div class="cp-empty-section">
								<span class="cp-empty-label">Active operators</span>
								{#each empty.activeOperators as op}
									<button class="cp-result-row" onclick={() => {
										const r: PaletteResult = {
											type: 'checkin', id: op.id, callsign: op.callsign,
											tacticalCall: op.tacticalCall, operatorName: op.operatorName,
											status: op.status, traffic: op.traffic, lastHeard: op.lastHeard,
											lat: op.lat, lon: op.lon, missionIds: op.missionIds ?? [],
											trackedStations: op.trackedStations ?? [], score: 0, checkIn: op,
										};
										selectResult(r);
									}}>
										<span class="cp-status-dot" style="background: {statusColors[op.status] ?? '#555'}"></span>
										<span class="cp-result-callsign">{op.callsign}</span>
										{#if op.tacticalCall}
											<span class="cp-result-tactical">"{op.tacticalCall}"</span>
										{/if}
										<span class="cp-result-badge" style="background: {statusColors[op.status] ?? '#555'}">{op.status}</span>
										<span class="cp-result-time">{timeAgo(op.lastHeard)}</span>
									</button>
								{/each}
							</div>
						{/if}
						{#if empty.recentCallsigns.length > 0}
							<div class="cp-empty-section">
								<span class="cp-empty-label">Recent</span>
								<div class="cp-recents">
									{#each empty.recentCallsigns as cs}
										<button class="cp-recent-chip" onclick={() => handleRecentClick(cs)}>{cs}</button>
									{/each}
								</div>
							</div>
						{/if}
						{#if empty.pinnedNotes.length > 0}
							<div class="cp-empty-section">
								<span class="cp-empty-label">Pinned notes</span>
								{#each empty.pinnedNotes as pn}
									<div class="cp-pinned-note" style="border-left-color: {noteCategoryMeta[pn.category as NoteCategory]?.color ?? '#6b7280'}">
										<span class="cp-pinned-cat" style="background: {noteCategoryMeta[pn.category as NoteCategory]?.color ?? '#6b7280'}">{pn.category}</span>
										<span class="cp-pinned-text">{pn.content}</span>
									</div>
								{/each}
							</div>
						{/if}
						{#if empty.activeOperators.length === 0 && empty.recentCallsigns.length === 0}
							<div class="cp-no-results">No active net. Open a net to use the command palette.</div>
						{/if}
					</div>
				{:else}
					<div class="cp-no-results">No matches for "{$paletteQuery}"</div>
				{/if}
			</div>
		{:else if mode === 'context' && selected}
			<!-- CONTEXT + NOTES MODE -->
			<div class="cp-context">
				<!-- Back button + header -->
				<div class="cp-context-header">
					<button class="cp-back-btn" onclick={backToSearch} title="Back to search (Backspace)">
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M19 12H5M12 19l-7-7 7-7"/>
						</svg>
					</button>
					<span class="cp-status-dot cp-status-dot-lg" style="background: {statusColors[selected.status] ?? '#555'}"></span>
					<span class="cp-context-callsign">{selected.callsign}</span>
					{#if selected.tacticalCall}
						<span class="cp-context-tactical">"{selected.tacticalCall}"</span>
					{/if}
					{#if selected.status && selected.type === 'checkin'}
						<span class="cp-context-badge" style="background: {statusColors[selected.status] ?? '#555'}">{selected.status}</span>
					{/if}
					{#if selected.traffic && selected.traffic !== 'none' && selected.type === 'checkin'}
						<span class="cp-context-badge" style="background: {trafficMeta[selected.traffic]?.color ?? '#555'}">{selected.traffic}</span>
					{/if}
					{#if selected.checkIn?.category && selected.checkIn.category !== 'general'}
						{@const catM = stationCategoryMeta[selected.checkIn.category]}
						{#if catM}
							<span class="cp-context-badge" style="background: {catM.color}">{catM.short}</span>
						{/if}
					{/if}
					<span class="cp-context-time">{timeAgo(selected.lastHeard)}</span>
					<kbd class="cp-kbd cp-kbd-right">ESC</kbd>
				</div>

				<!-- Details row -->
				<div class="cp-details-row">
					{#if selected.lat != null && selected.lon != null}
						<span class="cp-detail-item">{formatCoord(selected.lat, selected.lon)}</span>
					{/if}
					{#if selectedMissions.length > 0}
						{#each selectedMissions as m}
							<span class="cp-mission-chip" style="background: {missionPriorityColors[m.priority] ?? '#6b7280'}">{m.title}</span>
						{/each}
					{/if}
					{#if selected.trackedStations.length > 0}
						<span class="cp-detail-item">{selected.trackedStations.length} tracked device{selected.trackedStations.length > 1 ? 's' : ''}</span>
					{/if}
				</div>

				<!-- Recent notes for this operator -->
				{#if selectedNotes.length > 0}
					<div class="cp-notes-history">
						<span class="cp-empty-label">Recent notes</span>
						{#each selectedNotes as n}
							<div class="cp-note-item" style="border-left-color: {noteCategoryMeta[n.category as NoteCategory]?.color ?? '#6b7280'}">
								<span class="cp-note-cat" style="background: {noteCategoryMeta[n.category as NoteCategory]?.color ?? '#6b7280'}">{noteCategoryMeta[n.category as NoteCategory]?.label ?? n.category}</span>
								{#if n.severity && n.severity !== 'info'}
									<span class="cp-note-sev" style="color: {severityMeta[n.severity as NoteSeverity]?.color ?? '#6b7280'}">{n.severity}</span>
								{/if}
								<span class="cp-note-time">{timeAgo(n.createdAt)}</span>
								<span class="cp-note-text">{n.content}</span>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Note composer -->
				<div class="cp-composer">
					<div class="cp-composer-header">
						<span class="cp-empty-label">Add note</span>
						{#if noteSaved}
							<span class="cp-saved-indicator">Saved</span>
						{/if}
					</div>

					<!-- Target selector -->
					{#if selected.type === 'checkin'}
						<div class="cp-target-row">
							<button
								class="cp-target-chip"
								class:active={noteTarget === 'operator'}
								onclick={() => { noteTarget = 'operator'; }}
							>Operator</button>
							{#each selectedMissions as m}
								<button
									class="cp-target-chip"
									class:active={noteTarget === m.id}
									onclick={() => { noteTarget = m.id; }}
									style="--chip-color: {missionPriorityColors[m.priority] ?? '#6b7280'}"
								>{m.title}</button>
							{/each}
						</div>
					{/if}

					<!-- Category chips -->
					<div class="cp-chip-row">
						{#each Object.entries(noteCategoryMeta) as [key, meta]}
							<button
								class="cp-cat-chip"
								class:active={noteCategory === key}
								style="--chip-bg: {meta.color}"
								onclick={() => { noteCategory = key as NoteCategory; }}
							>{meta.label}</button>
						{/each}
					</div>

					<!-- Severity chips -->
					<div class="cp-chip-row">
						{#each Object.entries(severityMeta) as [key, meta]}
							<button
								class="cp-sev-chip"
								class:active={noteSeverity === key}
								style="--chip-bg: {meta.color}"
								onclick={() => { noteSeverity = key as NoteSeverity; }}
							>{meta.label}</button>
						{/each}
					</div>

					<!-- Textarea -->
					<div class="cp-textarea-wrap">
						<textarea
							bind:this={textareaRef}
							bind:value={noteContent}
							class="cp-textarea"
							placeholder="Type note..."
							rows="3"
						></textarea>
						<div class="cp-textarea-footer">
							<kbd class="cp-kbd cp-kbd-sm">Ctrl+Enter</kbd>
							<button
								class="cp-save-btn"
								disabled={!noteContent.trim() || noteSaving}
								onclick={saveNote}
							>{noteSaving ? 'Saving...' : 'Save'}</button>
						</div>
					</div>
				</div>

				<!-- Quick actions bar -->
				<div class="cp-actions-bar">
					<div class="cp-action-group">
						<button class="cp-action-btn" onclick={() => { showStatusDropdown = !showStatusDropdown; showMissionDropdown = false; showTrafficDropdown = false; showCategoryDropdown = false; }} disabled={!selected.checkIn}>
							<kbd class="cp-kbd cp-kbd-sm">S</kbd> Status
						</button>
						<button class="cp-action-btn" onclick={() => { showTrafficDropdown = !showTrafficDropdown; showStatusDropdown = false; showMissionDropdown = false; showCategoryDropdown = false; }} disabled={!selected.checkIn}>
							<kbd class="cp-kbd cp-kbd-sm">T</kbd> Traffic
						</button>
						<button class="cp-action-btn" onclick={() => { showCategoryDropdown = !showCategoryDropdown; showStatusDropdown = false; showMissionDropdown = false; showTrafficDropdown = false; }} disabled={!selected.checkIn}>
							<kbd class="cp-kbd cp-kbd-sm">C</kbd> Category
						</button>
						<button class="cp-action-btn" onclick={() => { showMissionDropdown = !showMissionDropdown; showStatusDropdown = false; showTrafficDropdown = false; showCategoryDropdown = false; }} disabled={!selected.checkIn}>
							<kbd class="cp-kbd cp-kbd-sm">M</kbd> Mission
						</button>
						<button class="cp-action-btn" onclick={flyToSelected} disabled={selected.lat == null}>
							<kbd class="cp-kbd cp-kbd-sm">F</kbd> Fly To
						</button>
					</div>

					<!-- Status dropdown -->
					{#if showStatusDropdown}
						<div class="cp-dropdown">
							{#each statuses as s}
								<button
									class="cp-dropdown-item"
									class:active={selected.status === s}
									onclick={() => updateStatus(s)}
								>
									<span class="cp-status-dot" style="background: {statusColors[s]}"></span>
									{s}
								</button>
							{/each}
						</div>
					{/if}

					<!-- Mission dropdown -->
					{#if showMissionDropdown}
						<div class="cp-dropdown">
							{#each allMissions.filter(m => m.status !== 'complete') as m}
								<button
									class="cp-dropdown-item"
									class:active={selected.missionIds.includes(m.id)}
									onclick={() => assignMission(m.id)}
								>
									<span class="cp-mission-dot" style="background: {missionPriorityColors[m.priority] ?? '#6b7280'}"></span>
									{m.title}
									{#if selected.missionIds.includes(m.id)}
										<span class="cp-assigned-check">&#10003;</span>
									{/if}
								</button>
							{/each}
							{#if allMissions.filter(m => m.status !== 'complete').length === 0}
								<div class="cp-dropdown-empty">No active missions</div>
							{/if}
						</div>
					{/if}

					<!-- Traffic dropdown -->
					{#if showTrafficDropdown}
						<div class="cp-dropdown">
							{#each trafficTypes as t}
								<button
									class="cp-dropdown-item"
									class:active={selected.traffic === t || (!selected.traffic && t === 'none')}
									onclick={() => updateTraffic(t)}
								>
									<span class="cp-status-dot" style="background: {trafficMeta[t]?.color ?? '#555'}"></span>
									{trafficMeta[t]?.label ?? t}
									{#if selected.traffic === t || (!selected.traffic && t === 'none')}
										<span class="cp-assigned-check">&#10003;</span>
									{/if}
								</button>
							{/each}
						</div>
					{/if}

					<!-- Category dropdown -->
					{#if showCategoryDropdown}
						<div class="cp-dropdown">
							{#each categories as cat}
								{@const catM = stationCategoryMeta[cat]}
								<button
									class="cp-dropdown-item"
									class:active={(selected.checkIn?.category || 'general') === cat}
									onclick={() => updateCategory(cat)}
								>
									<span class="cp-status-dot" style="background: {catM?.color ?? '#555'}"></span>
									{catM?.label ?? cat}
									{#if (selected.checkIn?.category || 'general') === cat}
										<span class="cp-assigned-check">&#10003;</span>
									{/if}
								</button>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	/* Backdrop */
	.cp-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-command-palette, 500);
		background: rgba(0, 0, 0, 0.6);
		display: flex;
		justify-content: center;
		align-items: flex-start;
		padding-top: 15vh;
	}

	@media (max-width: 768px) {
		.cp-backdrop {
			padding-top: 0;
			align-items: stretch;
		}
	}

	/* Container */
	.cp-container {
		width: min(560px, 90vw);
		max-height: min(70vh, 600px);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		display: flex;
		flex-direction: column;
		overflow: hidden;
		animation: cp-slide-in var(--duration-fast) var(--ease-out);
	}

	@media (max-width: 768px) {
		.cp-container {
			width: 100%;
			max-height: 100%;
			border-radius: 0;
			border: none;
			border-bottom: 1px solid var(--color-primary);
		}
	}

	@keyframes cp-slide-in {
		from {
			opacity: 0;
			transform: translateY(-8px) scale(0.98);
		}
		to {
			opacity: 1;
			transform: translateY(0) scale(1);
		}
	}

	/* Search header */
	.cp-search-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-md);
		border-bottom: 1px solid rgba(255, 255, 255, 0.06);
	}

	.cp-search-icon {
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.cp-search-input {
		flex: 1;
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 1rem;
		font-family: inherit;
		outline: none;
	}

	.cp-search-input::placeholder {
		color: var(--color-text-muted);
		opacity: 0.6;
	}

	/* Kbd hints */
	.cp-kbd {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 2px 6px;
		font-size: 0.65rem;
		font-family: inherit;
		color: var(--color-text-muted);
		background: var(--color-primary);
		border-radius: 4px;
		flex-shrink: 0;
		line-height: 1;
	}

	.cp-kbd-right {
		margin-left: auto;
	}

	.cp-kbd-sm {
		font-size: 0.6rem;
		padding: 1px 4px;
	}

	/* Filters */
	.cp-filters {
		display: flex;
		gap: var(--space-xs);
		padding: var(--space-xs) var(--space-md);
		border-bottom: 1px solid rgba(255, 255, 255, 0.06);
	}

	.cp-filter-btn {
		padding: 4px 10px;
		font-size: 0.75rem;
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.cp-filter-btn.active {
		background: var(--color-primary);
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.cp-filter-btn:hover:not(.active) {
		border-color: rgba(255, 255, 255, 0.2);
		color: var(--color-text);
	}

	/* Results list */
	.cp-results {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-xs) 0;
	}

	.cp-result-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		padding: 8px var(--space-md);
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 0.85rem;
		cursor: pointer;
		transition: background var(--duration-fast);
		text-align: left;
		min-height: 44px;
	}

	.cp-result-row.highlighted,
	.cp-result-row:hover {
		background: rgba(255, 255, 255, 0.05);
	}

	.cp-status-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.cp-status-dot-lg {
		width: 10px;
		height: 10px;
	}

	.cp-result-callsign {
		font-family: 'SF Mono', 'Fira Code', monospace;
		font-weight: 600;
		white-space: nowrap;
	}

	.cp-result-tactical {
		color: var(--color-accent);
		font-size: 0.8rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.cp-result-name {
		color: var(--color-text-muted);
		font-size: 0.8rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.cp-result-badge {
		padding: 1px 6px;
		border-radius: var(--radius-full);
		font-size: 0.65rem;
		color: #fff;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		flex-shrink: 0;
	}

	.cp-result-time {
		margin-left: auto;
		color: var(--color-text-muted);
		font-size: 0.75rem;
		white-space: nowrap;
		flex-shrink: 0;
	}

	/* Empty state */
	.cp-empty {
		padding: var(--space-sm) var(--space-md);
	}

	.cp-empty-section {
		margin-bottom: var(--space-md);
	}

	.cp-empty-label {
		display: block;
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
		margin-bottom: var(--space-xs);
	}

	.cp-recents {
		display: flex;
		gap: var(--space-xs);
		flex-wrap: wrap;
	}

	.cp-recent-chip {
		padding: 3px 10px;
		font-size: 0.78rem;
		font-family: 'SF Mono', 'Fira Code', monospace;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-full);
		color: var(--color-text);
		cursor: pointer;
		transition: border-color var(--duration-fast);
	}

	.cp-recent-chip:hover {
		border-color: var(--color-accent);
	}

	.cp-pinned-note {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: 4px 8px;
		border-left: 3px solid #6b7280;
		margin-bottom: 4px;
	}

	.cp-pinned-cat {
		padding: 1px 5px;
		font-size: 0.65rem;
		border-radius: 3px;
		color: #fff;
		flex-shrink: 0;
	}

	.cp-pinned-text {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.cp-no-results {
		padding: var(--space-lg) var(--space-md);
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}

	/* === CONTEXT MODE === */
	.cp-context {
		display: flex;
		flex-direction: column;
		overflow-y: auto;
		max-height: min(70vh, 600px);
	}

	@media (max-width: 768px) {
		.cp-context {
			max-height: 100vh;
			max-height: 100dvh;
		}
	}

	.cp-context-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-md);
		border-bottom: 1px solid rgba(255, 255, 255, 0.06);
	}

	.cp-back-btn {
		background: none;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		padding: 4px;
		border-radius: 4px;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: color var(--duration-fast);
	}

	.cp-back-btn:hover {
		color: var(--color-text);
	}

	.cp-context-callsign {
		font-family: 'SF Mono', 'Fira Code', monospace;
		font-weight: 700;
		font-size: 1.05rem;
	}

	.cp-context-tactical {
		color: var(--color-accent);
		font-size: 0.9rem;
	}

	.cp-context-badge {
		padding: 2px 8px;
		border-radius: var(--radius-full);
		font-size: 0.7rem;
		color: #fff;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.cp-context-time {
		color: var(--color-text-muted);
		font-size: 0.75rem;
	}

	/* Details row */
	.cp-details-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-xs) var(--space-md);
		flex-wrap: wrap;
		border-bottom: 1px solid rgba(255, 255, 255, 0.04);
	}

	.cp-detail-item {
		font-size: 0.78rem;
		color: var(--color-text-muted);
	}

	.cp-mission-chip {
		padding: 1px 8px;
		font-size: 0.68rem;
		border-radius: var(--radius-full);
		color: #fff;
	}

	/* Notes history */
	.cp-notes-history {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid rgba(255, 255, 255, 0.04);
	}

	.cp-note-item {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 4px 8px;
		padding: 4px 0 4px 8px;
		border-left: 2px solid #6b7280;
		margin-bottom: 4px;
	}

	.cp-note-cat {
		padding: 1px 5px;
		font-size: 0.6rem;
		border-radius: 3px;
		color: #fff;
	}

	.cp-note-sev {
		font-size: 0.65rem;
		font-weight: 600;
	}

	.cp-note-time {
		font-size: 0.65rem;
		color: var(--color-text-muted);
	}

	.cp-note-text {
		width: 100%;
		font-size: 0.78rem;
		color: var(--color-text-muted);
	}

	/* Note composer */
	.cp-composer {
		padding: var(--space-sm) var(--space-md);
	}

	.cp-composer-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-bottom: var(--space-xs);
	}

	.cp-saved-indicator {
		font-size: 0.7rem;
		color: var(--color-success);
		animation: cp-fade-in var(--duration-fast);
	}

	@keyframes cp-fade-in {
		from { opacity: 0; }
		to { opacity: 1; }
	}

	.cp-target-row {
		display: flex;
		gap: var(--space-xs);
		margin-bottom: var(--space-xs);
		flex-wrap: wrap;
	}

	.cp-target-chip {
		padding: 3px 10px;
		font-size: 0.72rem;
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.cp-target-chip.active {
		background: var(--chip-color, var(--color-primary));
		border-color: var(--chip-color, var(--color-accent));
		color: #fff;
	}

	.cp-chip-row {
		display: flex;
		gap: 4px;
		margin-bottom: var(--space-xs);
		flex-wrap: wrap;
	}

	.cp-cat-chip,
	.cp-sev-chip {
		padding: 2px 8px;
		font-size: 0.68rem;
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.cp-cat-chip.active {
		background: var(--chip-bg);
		border-color: var(--chip-bg);
		color: #fff;
	}

	.cp-sev-chip.active {
		background: var(--chip-bg);
		border-color: var(--chip-bg);
		color: #fff;
	}

	.cp-cat-chip:hover:not(.active),
	.cp-sev-chip:hover:not(.active) {
		border-color: rgba(255, 255, 255, 0.2);
		color: var(--color-text);
	}

	/* Textarea */
	.cp-textarea-wrap {
		position: relative;
	}

	.cp-textarea {
		width: 100%;
		background: var(--color-bg);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		font-family: inherit;
		padding: var(--space-sm);
		resize: vertical;
		min-height: 64px;
		outline: none;
		transition: border-color var(--duration-fast);
	}

	.cp-textarea:focus {
		border-color: var(--color-accent);
	}

	.cp-textarea::placeholder {
		color: var(--color-text-muted);
		opacity: 0.5;
	}

	.cp-textarea-footer {
		display: flex;
		justify-content: flex-end;
		align-items: center;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
	}

	.cp-save-btn {
		padding: 4px 14px;
		font-size: 0.78rem;
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: #fff;
		cursor: pointer;
		transition: opacity var(--duration-fast);
	}

	.cp-save-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.cp-save-btn:not(:disabled):hover {
		opacity: 0.9;
	}

	/* Actions bar */
	.cp-actions-bar {
		position: relative;
		display: flex;
		flex-direction: column;
		padding: var(--space-xs) var(--space-md) var(--space-md);
		border-top: 1px solid rgba(255, 255, 255, 0.04);
	}

	.cp-action-group {
		display: flex;
		gap: var(--space-sm);
	}

	.cp-action-btn {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 5px 10px;
		font-size: 0.75rem;
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.cp-action-btn:not(:disabled):hover {
		border-color: rgba(255, 255, 255, 0.2);
		color: var(--color-text);
	}

	.cp-action-btn:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}

	/* Dropdowns */
	.cp-dropdown {
		position: absolute;
		bottom: 100%;
		left: var(--space-md);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		box-shadow: var(--shadow-md);
		min-width: 160px;
		max-height: 200px;
		overflow-y: auto;
		z-index: 10;
		margin-bottom: 4px;
	}

	.cp-dropdown-item {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		padding: 8px 12px;
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 0.8rem;
		cursor: pointer;
		text-align: left;
		transition: background var(--duration-fast);
	}

	.cp-dropdown-item:hover {
		background: rgba(255, 255, 255, 0.05);
	}

	.cp-dropdown-item.active {
		color: var(--color-accent);
	}

	.cp-mission-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.cp-assigned-check {
		margin-left: auto;
		color: var(--color-success);
	}

	.cp-dropdown-empty {
		padding: 12px;
		font-size: 0.78rem;
		color: var(--color-text-muted);
		text-align: center;
	}
</style>
