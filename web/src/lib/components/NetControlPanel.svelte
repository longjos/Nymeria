<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { timeAgo } from '$lib/utils';
	import type { Net, NetCheckIn, NetMission, NetEvent, OperatorStatus, TrafficType } from '$lib/types';
	import {
		activeNet, checkIns, missions, timeline,
		sortedCheckIns, activeCheckIns,
		initNetControlStore, loadNetData, clearNetControl
	} from '$lib/stores/netcontrol';

	type Tab = 'roster' | 'missions' | 'timeline';
	let currentTab = $state<Tab>('roster');

	// Create net form
	let showCreateForm = $state(false);
	let newNetName = $state('');
	let newNetType = $state('tactical');
	let newNetFreq = $state('');
	let newNetNotes = $state('');
	let creating = $state(false);

	// Quick add
	let quickAddInput = $state('');
	let quickAddRef = $state<HTMLInputElement>();
	let searchResults = $state<import('$lib/types').Station[]>([]);
	let searchTimeout: ReturnType<typeof setTimeout>;

	// Mission form
	let showMissionForm = $state(false);
	let newMissionTitle = $state('');
	let newMissionDesc = $state('');
	let newMissionPriority = $state('routine');
	let newMissionAssign = $state('');

	// Inline note
	let noteCheckInId = $state<string | null>(null);
	let noteContent = $state('');

	// Elapsed timer
	let elapsed = $state('');
	let timerInterval: ReturnType<typeof setInterval>;

	// Status colors
	const statusColors: Record<OperatorStatus, string> = {
		available: '#22c55e',
		assigned: '#3b82f6',
		enroute: '#8b5cf6',
		onscene: '#06b6d4',
		brb: '#f59e0b',
		missing: '#ef4444',
		released: '#6b7280'
	};

	const trafficLabels: Record<TrafficType, string> = {
		none: '',
		routine: 'R',
		priority: 'P',
		welfare: 'W',
		emergency: 'E'
	};

	const trafficColors: Record<string, string> = {
		routine: '#22c55e',
		priority: '#f59e0b',
		welfare: '#3b82f6',
		emergency: '#ef4444'
	};

	const eventIcons: Record<string, string> = {
		net_opened: '📡',
		net_closed: '🔒',
		checkin: '📥',
		checkout: '📤',
		status_change: '🔄',
		assignment: '📋',
		mission_created: '🎯',
		mission_updated: '✅',
		rollcall: '📢',
		note: '📝',
		ncs_transfer: '🔀'
	};

	onMount(() => {
		initNetControlStore();

		timerInterval = setInterval(() => {
			const net = $activeNet;
			if (net?.status === 'open' && net.openedAt) {
				const ms = Date.now() - new Date(net.openedAt).getTime();
				const h = Math.floor(ms / 3600000);
				const m = Math.floor((ms % 3600000) / 60000);
				const s = Math.floor((ms % 60000) / 1000);
				elapsed = h > 0 ? `${h}h ${m}m` : `${m}m ${s}s`;
			} else {
				elapsed = '';
			}
		}, 1000);

		return () => {
			clearInterval(timerInterval);
		};
	});

	function stalenessClass(lastHeard: string): string {
		const ms = Date.now() - new Date(lastHeard).getTime();
		const mins = ms / 60000;
		if (mins > 30) return 'overdue';
		if (mins > 20) return 'stale-amber';
		if (mins > 10) return 'stale-yellow';
		return '';
	}

	function stalenessLabel(lastHeard: string): string {
		const ms = Date.now() - new Date(lastHeard).getTime();
		const mins = Math.floor(ms / 60000);
		if (mins > 30) return `${mins}m OVERDUE`;
		return `${mins}m ago`;
	}

	// --- Actions ---

	async function handleCreateNet() {
		if (!newNetName.trim()) return;
		creating = true;
		try {
			const net = await api.createNet({
				name: newNetName.trim(),
				type: newNetType,
				frequency: newNetFreq.trim(),
				notes: newNetNotes.trim()
			});
			// Auto-open the net.
			await api.openNet(net.id);
			await loadNetData(net.id);
			showCreateForm = false;
			newNetName = '';
			newNetFreq = '';
			newNetNotes = '';
		} catch (e) {
			console.error('Failed to create net:', e);
		} finally {
			creating = false;
		}
	}

	async function handleQuickAdd() {
		if (!$activeNet || !quickAddInput.trim()) return;

		const parts = quickAddInput.trim().split(/\s+/);
		const callsign = parts[0];
		let traffic = '';

		if (parts.length > 1) {
			const shortcut = parts[1].toUpperCase();
			const map: Record<string, string> = { R: 'routine', P: 'priority', W: 'welfare', E: 'emergency' };
			traffic = map[shortcut] || '';
		}

		try {
			await api.checkIn($activeNet.id, callsign, traffic);
			quickAddInput = '';
			searchResults = [];
		} catch (e) {
			console.error('Check-in failed:', e);
		}

		quickAddRef?.focus();
	}

	function handleQuickAddInput() {
		clearTimeout(searchTimeout);
		const q = quickAddInput.trim().split(/\s+/)[0];
		if (q.length < 2) {
			searchResults = [];
			return;
		}
		searchTimeout = setTimeout(async () => {
			try {
				searchResults = await api.searchOperators(q);
			} catch {
				searchResults = [];
			}
		}, 200);
	}

	function selectSearchResult(callsign: string) {
		quickAddInput = callsign + ' ';
		searchResults = [];
		quickAddRef?.focus();
	}

	async function handleStatusChange(ci: NetCheckIn, newStatus: OperatorStatus) {
		if (!$activeNet) return;
		try {
			await api.updateCheckIn($activeNet.id, ci.id, { status: newStatus });
		} catch (e) {
			console.error('Status update failed:', e);
		}
	}

	async function handleCheckOut(ci: NetCheckIn) {
		if (!$activeNet) return;
		try {
			await api.checkOut($activeNet.id, ci.id);
		} catch (e) {
			console.error('Check-out failed:', e);
		}
	}

	async function handleRollCall() {
		if (!$activeNet) return;
		try {
			await api.initiateRollCall($activeNet.id);
		} catch (e) {
			console.error('Roll call failed:', e);
		}
	}

	async function handleRollCallResponse(ci: NetCheckIn) {
		if (!$activeNet) return;
		try {
			await api.recordRollCallResponse($activeNet.id, ci.id);
		} catch (e) {
			console.error('Roll call response failed:', e);
		}
	}

	async function handleCloseNet() {
		if (!$activeNet || !confirm('Close this net?')) return;
		try {
			await api.closeNet($activeNet.id);
			clearNetControl();
		} catch (e) {
			console.error('Close net failed:', e);
		}
	}

	async function handleCreateMission() {
		if (!$activeNet || !newMissionTitle.trim()) return;
		try {
			await api.createMission($activeNet.id, {
				title: newMissionTitle.trim(),
				description: newMissionDesc.trim(),
				priority: newMissionPriority,
				assignedTo: newMissionAssign
			});
			showMissionForm = false;
			newMissionTitle = '';
			newMissionDesc = '';
			newMissionPriority = 'routine';
			newMissionAssign = '';
		} catch (e) {
			console.error('Create mission failed:', e);
		}
	}

	async function handleMissionStatusChange(m: NetMission, status: string) {
		if (!$activeNet) return;
		try {
			await api.updateMission($activeNet.id, m.id, { status: status as any });
		} catch (e) {
			console.error('Mission update failed:', e);
		}
	}

	async function handleAddNote(checkInId?: string) {
		if (!$activeNet || !noteContent.trim()) return;
		try {
			await api.addNetNote($activeNet.id, {
				checkInId,
				content: noteContent.trim()
			});
			noteContent = '';
			noteCheckInId = null;
		} catch (e) {
			console.error('Add note failed:', e);
		}
	}

	let timelineFilter = $state('all');
	let filteredTimeline = $derived(
		$timeline.filter((e) => {
			if (timelineFilter === 'all') return true;
			if (timelineFilter === 'checkins') return e.type === 'checkin' || e.type === 'checkout';
			if (timelineFilter === 'assignments') return e.type === 'assignment' || e.type === 'status_change';
			if (timelineFilter === 'missions') return e.type === 'mission_created' || e.type === 'mission_updated';
			if (timelineFilter === 'rollcalls') return e.type === 'rollcall';
			return true;
		}).reverse()
	);
</script>

<div class="net-panel">
	{#if !$activeNet}
		<!-- No active net -->
		<div class="panel-header">
			<span class="title">Net Control</span>
		</div>

		{#if !showCreateForm}
			<div class="empty-state">
				<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
					<path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" stroke-linecap="round" stroke-linejoin="round"/>
				</svg>
				<p>No active net</p>
				<button class="btn-primary" onclick={() => (showCreateForm = true)}>Create Net</button>
			</div>
		{:else}
			<div class="create-form">
				<div class="form-group">
					<label for="net-name">Net Name</label>
					<input id="net-name" type="text" bind:value={newNetName} placeholder="Emergency Net" />
				</div>
				<div class="form-row">
					<div class="form-group">
						<label for="net-type">Type</label>
						<select id="net-type" bind:value={newNetType}>
							<option value="tactical">Tactical</option>
							<option value="resource">Resource</option>
							<option value="traffic">Traffic</option>
							<option value="training">Training</option>
						</select>
					</div>
					<div class="form-group">
						<label for="net-freq">Frequency</label>
						<input id="net-freq" type="text" bind:value={newNetFreq} placeholder="146.520 MHz" />
					</div>
				</div>
				<div class="form-group">
					<label for="net-notes">Notes</label>
					<textarea id="net-notes" bind:value={newNetNotes} rows="2" placeholder="Optional notes..."></textarea>
				</div>
				<div class="form-actions">
					<button class="btn-secondary" onclick={() => (showCreateForm = false)}>Cancel</button>
					<button class="btn-primary" onclick={handleCreateNet} disabled={creating || !newNetName.trim()}>
						{creating ? 'Creating...' : 'Create & Open'}
					</button>
				</div>
			</div>
		{/if}
	{:else}
		<!-- Active net header -->
		<div class="panel-header">
			<div class="header-info">
				<div class="header-title-row">
					<span class="title">{$activeNet.name}</span>
					<span class="status-badge status-{$activeNet.status}">{$activeNet.status}</span>
				</div>
				{#if elapsed}
					<span class="elapsed">{elapsed}</span>
				{/if}
				{#if $activeNet.frequency}
					<span class="frequency">{$activeNet.frequency}</span>
				{/if}
			</div>
			<div class="header-actions">
				<button class="action-btn" onclick={handleRollCall} title="Roll Call">
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M1 8h3l2-5 3 10 2-5h4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				<button class="action-btn danger" onclick={handleCloseNet} title="Close Net">
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
				</button>
			</div>
		</div>

		<!-- Tabs -->
		<div class="tabs">
			<button class="tab" class:active={currentTab === 'roster'} onclick={() => (currentTab = 'roster')}>
				Roster <span class="tab-count">{$activeCheckIns.length}</span>
			</button>
			<button class="tab" class:active={currentTab === 'missions'} onclick={() => (currentTab = 'missions')}>
				Missions <span class="tab-count">{$missions.length}</span>
			</button>
			<button class="tab" class:active={currentTab === 'timeline'} onclick={() => (currentTab = 'timeline')}>
				Timeline
			</button>
		</div>

		<!-- Tab content -->
		<div class="tab-content">
			{#if currentTab === 'roster'}
				<!-- Quick Add Bar -->
				<div class="quick-add">
					<div class="quick-add-wrap">
						<input
							bind:this={quickAddRef}
							type="text"
							bind:value={quickAddInput}
							oninput={handleQuickAddInput}
							onkeydown={(e) => { if (e.key === 'Enter') handleQuickAdd(); }}
							placeholder="Callsign + R/P/W/E"
							class="quick-add-input"
						/>
						<button class="quick-add-btn" onclick={handleQuickAdd} disabled={!quickAddInput.trim()}>+</button>
					</div>
					{#if searchResults.length > 0}
						<div class="search-dropdown">
							{#each searchResults.slice(0, 5) as st}
								{@const key = st.ssid > 0 ? `${st.callsign}-${st.ssid}` : st.callsign}
								<button class="search-item" onmousedown={() => selectSearchResult(key)}>
									<span class="search-call">{key}</span>
									{#if st.comment}
										<span class="search-comment">{st.comment}</span>
									{/if}
								</button>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Roster -->
				<div class="roster">
					{#each $sortedCheckIns as ci (ci.id)}
						<div class="operator-card" class:released={ci.status === 'released'}>
							<div class="op-status-bar" style="background: {statusColors[ci.status]}"></div>
							<div class="op-main">
								<div class="op-header">
									<span class="op-callsign">{ci.callsign}</span>
									{#if ci.tacticalCall}
										<span class="op-tactical">"{ci.tacticalCall}"</span>
									{/if}
									{#if ci.traffic && ci.traffic !== 'none'}
										<span class="traffic-badge" style="background: {trafficColors[ci.traffic]}">{trafficLabels[ci.traffic as TrafficType]}</span>
									{/if}
									<span class="op-age {stalenessClass(ci.lastHeard)}">{stalenessLabel(ci.lastHeard)}</span>
								</div>
								{#if ci.operatorName || ci.location}
									<div class="op-detail">
										{#if ci.operatorName}
											<span>{ci.operatorName}</span>
										{/if}
										{#if ci.location}
											<span class="op-location">{ci.location}</span>
										{/if}
									</div>
								{/if}
								{#if ci.assignment}
									<div class="op-assignment">📋 {ci.assignment}</div>
								{/if}
								{#if ci.missedRollCalls > 0}
									<div class="missed-badge">Missed {ci.missedRollCalls} roll call{ci.missedRollCalls > 1 ? 's' : ''}</div>
								{/if}

								<!-- Inline note form -->
								{#if noteCheckInId === ci.id}
									<div class="inline-note">
										<input
											type="text"
											bind:value={noteContent}
											placeholder="Quick note..."
											onkeydown={(e) => { if (e.key === 'Enter') handleAddNote(ci.id); if (e.key === 'Escape') { noteCheckInId = null; noteContent = ''; } }}
										/>
										<button class="note-send" onclick={() => handleAddNote(ci.id)}>Save</button>
									</div>
								{/if}
							</div>
							<div class="op-actions">
								<select
									class="status-select"
									value={ci.status}
									onchange={(e) => handleStatusChange(ci, (e.target as HTMLSelectElement).value as OperatorStatus)}
								>
									<option value="available">Available</option>
									<option value="assigned">Assigned</option>
									<option value="enroute">En Route</option>
									<option value="onscene">On Scene</option>
									<option value="brb">BRB</option>
									<option value="missing">Missing</option>
								</select>
								<div class="op-btn-row">
									{#if ci.missedRollCalls > 0}
										<button class="op-btn" title="Roll Call Response" onclick={() => handleRollCallResponse(ci)}>✓</button>
									{/if}
									<button class="op-btn" title="Note" onclick={() => { noteCheckInId = noteCheckInId === ci.id ? null : ci.id; noteContent = ''; }}>📝</button>
									<button class="op-btn" title="Check Out" onclick={() => handleCheckOut(ci)}>✕</button>
								</div>
							</div>
						</div>
					{/each}
					{#if $sortedCheckIns.length === 0}
						<p class="empty">No operators checked in. Use the input above to add.</p>
					{/if}
				</div>

			{:else if currentTab === 'missions'}
				<!-- Mission form toggle -->
				{#if !showMissionForm}
					<div class="mission-add">
						<button class="btn-secondary" onclick={() => (showMissionForm = true)}>+ New Mission</button>
					</div>
				{:else}
					<div class="mission-form">
						<input type="text" bind:value={newMissionTitle} placeholder="Mission title" />
						<textarea bind:value={newMissionDesc} rows="2" placeholder="Description (optional)"></textarea>
						<div class="form-row">
							<select bind:value={newMissionPriority}>
								<option value="routine">Routine</option>
								<option value="priority">Priority</option>
								<option value="welfare">Welfare</option>
								<option value="emergency">Emergency</option>
							</select>
							<select bind:value={newMissionAssign}>
								<option value="">Unassigned</option>
								{#each $activeCheckIns as ci}
									<option value={ci.callsign}>{ci.callsign}</option>
								{/each}
							</select>
						</div>
						<div class="form-actions">
							<button class="btn-secondary" onclick={() => (showMissionForm = false)}>Cancel</button>
							<button class="btn-primary" onclick={handleCreateMission} disabled={!newMissionTitle.trim()}>Create</button>
						</div>
					</div>
				{/if}

				<!-- Mission list -->
				<div class="mission-list">
					{#each $missions as m (m.id)}
						<div class="mission-card" class:complete={m.status === 'complete'}>
							<div class="mission-header">
								<span class="mission-title">{m.title}</span>
								<span class="priority-badge priority-{m.priority}">{m.priority}</span>
							</div>
							{#if m.description}
								<p class="mission-desc">{m.description}</p>
							{/if}
							<div class="mission-footer">
								{#if m.assignedTo}
									<span class="mission-assigned">→ {m.assignedTo}</span>
								{/if}
								<span class="mission-status-badge">{m.status}</span>
								{#if m.status === 'open'}
									<button class="op-btn" onclick={() => handleMissionStatusChange(m, 'active')}>Start</button>
								{:else if m.status === 'active'}
									<button class="op-btn" onclick={() => handleMissionStatusChange(m, 'complete')}>Complete</button>
								{/if}
							</div>
						</div>
					{/each}
					{#if $missions.length === 0}
						<p class="empty">No missions. Create one above.</p>
					{/if}
				</div>

			{:else if currentTab === 'timeline'}
				<!-- Timeline filters -->
				<div class="timeline-filters">
					{#each [['all', 'All'], ['checkins', 'Check-ins'], ['assignments', 'Status'], ['missions', 'Missions'], ['rollcalls', 'Roll Calls']] as [value, label]}
						<button
							class="filter-chip"
							class:active={timelineFilter === value}
							onclick={() => (timelineFilter = value)}
						>{label}</button>
					{/each}
				</div>

				<!-- Timeline feed -->
				<div class="timeline-feed">
					{#each filteredTimeline as evt (evt.id)}
						<div class="timeline-entry">
							<span class="tl-icon">{eventIcons[evt.type] ?? '•'}</span>
							<div class="tl-content">
								<span class="tl-summary">{evt.summary}</span>
								<span class="tl-time">{timeAgo(evt.createdAt)}</span>
							</div>
						</div>
					{/each}
					{#if filteredTimeline.length === 0}
						<p class="empty">No timeline events yet.</p>
					{/if}
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.net-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.panel-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.header-info {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.header-title-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.title {
		font-weight: 600;
		font-size: 0.95rem;
	}

	.status-badge {
		font-size: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 1px 6px;
		border-radius: var(--radius-sm);
	}

	.status-open { background: #22c55e; color: #000; }
	.status-draft { background: var(--color-primary); color: var(--color-text-muted); }
	.status-closed { background: #6b7280; color: #fff; }

	.elapsed {
		font-family: monospace;
		font-size: 0.75rem;
		color: #22c55e;
	}

	.frequency {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.header-actions {
		display: flex;
		gap: var(--space-xs);
	}

	.action-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.action-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.action-btn.danger:hover {
		border-color: #ef4444;
		color: #ef4444;
	}

	/* Tabs */
	.tabs {
		display: flex;
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.tab {
		flex: 1;
		padding: var(--space-sm);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.tab:hover { color: var(--color-text); }
	.tab.active {
		color: var(--color-accent);
		border-bottom-color: var(--color-accent);
	}

	.tab-count {
		font-size: 0.65rem;
		background: var(--color-primary);
		padding: 1px 5px;
		border-radius: 8px;
		margin-left: 3px;
	}

	.tab-content {
		flex: 1;
		overflow-y: auto;
	}

	/* Quick Add */
	.quick-add {
		position: relative;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.quick-add-wrap {
		display: flex;
		gap: var(--space-xs);
	}

	.quick-add-input {
		flex: 1;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-family: monospace;
		font-size: 0.85rem;
		padding: 6px 8px;
		outline: none;
		text-transform: uppercase;
	}

	.quick-add-input:focus {
		border-color: var(--color-accent);
	}

	.quick-add-input::placeholder {
		text-transform: none;
		color: var(--color-text-muted);
	}

	.quick-add-btn {
		width: 32px;
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: white;
		font-size: 1.1rem;
		cursor: pointer;
	}

	.quick-add-btn:disabled {
		opacity: 0.4;
		cursor: default;
	}

	.search-dropdown {
		position: absolute;
		left: var(--space-md);
		right: var(--space-md);
		top: calc(100% - 1px);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 0 0 var(--radius-sm) var(--radius-sm);
		z-index: 10;
		max-height: 200px;
		overflow-y: auto;
	}

	.search-item {
		display: flex;
		flex-direction: column;
		gap: 1px;
		width: 100%;
		padding: 6px 10px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
	}

	.search-item:hover { background: var(--color-primary); }
	.search-item:last-child { border-bottom: none; }

	.search-call {
		font-family: monospace;
		font-weight: 600;
		font-size: 0.8rem;
	}

	.search-comment {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	/* Roster */
	.roster {
		padding: 0;
	}

	.operator-card {
		display: flex;
		border-bottom: 1px solid var(--color-primary);
		transition: opacity var(--duration-fast);
	}

	.operator-card.released {
		opacity: 0.5;
	}

	.op-status-bar {
		width: 4px;
		flex-shrink: 0;
	}

	.op-main {
		flex: 1;
		padding: var(--space-sm) var(--space-sm) var(--space-sm) var(--space-sm);
		min-width: 0;
	}

	.op-header {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		flex-wrap: wrap;
	}

	.op-callsign {
		font-family: monospace;
		font-weight: 700;
		font-size: 0.85rem;
	}

	.op-tactical {
		font-size: 0.75rem;
		color: var(--color-accent);
	}

	.traffic-badge {
		font-size: 0.55rem;
		font-weight: 700;
		color: #000;
		padding: 1px 5px;
		border-radius: 3px;
	}

	.op-age {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		margin-left: auto;
	}

	.op-age.stale-yellow { color: #f59e0b; }
	.op-age.stale-amber { color: #f97316; }
	.op-age.overdue { color: #ef4444; font-weight: 700; }

	.op-detail {
		display: flex;
		gap: var(--space-sm);
		font-size: 0.7rem;
		color: var(--color-text-muted);
		margin-top: 2px;
	}

	.op-location { font-style: italic; }

	.op-assignment {
		font-size: 0.7rem;
		color: var(--color-accent);
		margin-top: 2px;
	}

	.missed-badge {
		font-size: 0.65rem;
		color: #ef4444;
		font-weight: 600;
		margin-top: 2px;
	}

	.inline-note {
		display: flex;
		gap: var(--space-xs);
		margin-top: var(--space-xs);
	}

	.inline-note input {
		flex: 1;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.75rem;
		padding: 3px 6px;
		outline: none;
	}

	.note-send {
		background: var(--color-primary);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.7rem;
		padding: 3px 8px;
		cursor: pointer;
	}

	.op-actions {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: var(--space-xs);
		padding: var(--space-sm) var(--space-sm);
		flex-shrink: 0;
	}

	.status-select {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.65rem;
		padding: 2px 4px;
		cursor: pointer;
	}

	.op-btn-row {
		display: flex;
		gap: 2px;
	}

	.op-btn {
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.65rem;
		padding: 2px 6px;
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.op-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	/* Missions */
	.mission-add {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.mission-form {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.mission-form input,
	.mission-form textarea,
	.mission-form select {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
		padding: 6px 8px;
		outline: none;
	}

	.mission-form textarea {
		resize: vertical;
	}

	.mission-list {
		padding: 0;
	}

	.mission-card {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.mission-card.complete {
		opacity: 0.5;
	}

	.mission-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.mission-title {
		font-weight: 600;
		font-size: 0.85rem;
	}

	.priority-badge {
		font-size: 0.55rem;
		font-weight: 700;
		text-transform: uppercase;
		padding: 1px 5px;
		border-radius: 3px;
	}

	.priority-routine { background: #22c55e; color: #000; }
	.priority-priority { background: #f59e0b; color: #000; }
	.priority-welfare { background: #3b82f6; color: #fff; }
	.priority-emergency { background: #ef4444; color: #fff; }

	.mission-desc {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		margin: 2px 0;
	}

	.mission-footer {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
	}

	.mission-assigned {
		font-family: monospace;
		font-size: 0.7rem;
		color: var(--color-accent);
	}

	.mission-status-badge {
		font-size: 0.6rem;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	/* Timeline */
	.timeline-filters {
		display: flex;
		gap: var(--space-xs);
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-wrap: wrap;
	}

	.filter-chip {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		font-size: 0.65rem;
		padding: 2px 8px;
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.filter-chip.active {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: white;
	}

	.timeline-feed {
		padding: 0;
	}

	.timeline-entry {
		display: flex;
		gap: var(--space-sm);
		padding: 6px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.tl-icon {
		font-size: 0.8rem;
		flex-shrink: 0;
		width: 20px;
		text-align: center;
	}

	.tl-content {
		flex: 1;
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: var(--space-sm);
		min-width: 0;
	}

	.tl-summary {
		font-size: 0.75rem;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.tl-time {
		font-size: 0.6rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	/* Empty state */
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-md);
		padding: var(--space-2xl) var(--space-md);
		color: var(--color-text-muted);
	}

	.empty-state p {
		font-size: 0.9rem;
	}

	.empty {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}

	/* Forms */
	.create-form {
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.form-group label {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		font-weight: 500;
	}

	.form-group input,
	.form-group select,
	.form-group textarea {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		padding: 6px 8px;
		outline: none;
	}

	.form-group input:focus,
	.form-group select:focus,
	.form-group textarea:focus {
		border-color: var(--color-accent);
	}

	.form-group textarea {
		resize: vertical;
	}

	.form-row {
		display: flex;
		gap: var(--space-sm);
	}

	.form-row .form-group {
		flex: 1;
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
	}

	.btn-primary {
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: white;
		font-size: 0.8rem;
		font-weight: 600;
		padding: 6px 16px;
		cursor: pointer;
		transition: opacity var(--duration-fast);
	}

	.btn-primary:hover { opacity: 0.9; }
	.btn-primary:disabled { opacity: 0.4; cursor: default; }

	.btn-secondary {
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.8rem;
		padding: 6px 16px;
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.btn-secondary:hover {
		border-color: var(--color-text-muted);
		color: var(--color-text);
	}
</style>
