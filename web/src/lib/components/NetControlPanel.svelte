<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { timeAgo } from '$lib/utils';
	import { openICS309 } from '$lib/stores/ui';
	import type { Net, NetCheckIn, NetMission, NetEvent, NetNote, OperatorStatus, TrafficType, Annotation, NoteCategory, NoteSeverity, StationCategory } from '$lib/types';
	import {
		activeNet, checkIns, missions, timeline, notes,
		sortedCheckIns, activeCheckIns,
		notesByCheckIn, notesByMission, pinnedNotes, pinnedCheckIns,
		netMetrics, categoryCounts,
		initNetControlStore, loadNetData, clearNetControl,
		opsView,
		hoveredMissionId, highlightedCheckIns,
		hoveredCheckInId,
		netAnnotations, annotationsByName
	} from '$lib/stores/netcontrol';
	import { annotationList } from '$lib/stores/annotations';
	import { categoryMeta, isTerminalStatus } from '$lib/annotationMeta';
	import LocationManager from './LocationManager.svelte';

	let {
		onFlyTo,
		onFlyToBounds,
		onSetOpsView,
		onGoToOpsView,
		onPlaceOperator,
		onPlaceAnnotation,
		annotationMapCoords = null,
		onMapCoordsConsumed,
	}: {
		onFlyTo?: (lat: number, lon: number) => void;
		onFlyToBounds?: (coords: Array<{ lat: number; lon: number }>) => void;
		onSetOpsView?: () => void;
		onGoToOpsView?: () => void;
		onPlaceOperator?: (ciId: string, callsign: string) => void;
		onPlaceAnnotation?: (id: string | null, name: string, mode: 'update' | 'form') => void;
		annotationMapCoords?: { lat: number; lon: number } | null;
		onMapCoordsConsumed?: () => void;
	} = $props();

	type Tab = 'roster' | 'missions' | 'locations' | 'timeline';
	let currentTab = $state<Tab>('roster');

	// Metrics bar filter — clicking a metric filters the roster
	type MetricsFilter = null | 'available' | 'assigned' | 'missing' | 'stale';
	let metricsFilter = $state<MetricsFilter>(null);

	// Category filter — clicking a category chip filters the roster (AND with metricsFilter)
	let categoryFilter = $state<StationCategory | null>(null);

	// Station category metadata (colors + short labels)
	const stationCategoryMeta: Record<StationCategory, { label: string; short: string; color: string }> = {
		general:  { label: 'General',  short: 'GEN', color: '#6b7280' },
		command:  { label: 'Command',  short: 'CMD', color: '#eab308' },
		medical:  { label: 'Medical',  short: 'MED', color: '#ef4444' },
		sag:      { label: 'SAG',      short: 'SAG', color: '#f97316' },
		marshal:  { label: 'Marshal',  short: 'MAR', color: '#3b82f6' },
		fixed:    { label: 'Fixed',    short: 'FIX', color: '#14b8a6' },
		mobile:   { label: 'Mobile',   short: 'MOB', color: '#8b5cf6' },
		tactical: { label: 'Tactical', short: 'TAC', color: '#6366f1' },
	};

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
	let newMissionLocation = $state('');
	let newMissionLat = $state('');
	let newMissionLon = $state('');

	// Mission location autocomplete
	let missionLocSuggestions = $state<import('$lib/types').Annotation[]>([]);
	let showMissionLocDropdown = $state(false);

	// Mission brief in create form
	let newNetMissionBrief = $state('');

	// Mission assignment from roster
	let assigningCheckInId = $state<string | null>(null);

	// Mission-side operator assignment
	let assigningMissionId = $state<string | null>(null);

	// Annotation linking in mission creation form
	let selectedAnnotationIds = $state<string[]>([]);

	// Annotation linking on mission cards
	let linkingAnnotationMissionId = $state<string | null>(null);

	// Mission filter
	let missionFilter = $state<'all' | 'active' | 'complete'>('all');
	let recentlyChangedMissionId = $state<string | null>(null);
	let recentlyChangedTimer: ReturnType<typeof setTimeout> | null = null;

	// Highlighted check-in (dedup flash)
	let highlightedCheckInId = $state<string | null>(null);

	// Tracked devices
	let expandedDeviceId = $state<string | null>(null);
	let addDeviceCallsign = $state('');

	// Note composer state
	let noteCheckInId = $state<string | null>(null);
	let noteMissionId = $state<string | null>(null);
	let noteContent = $state('');
	let noteCategory = $state<NoteCategory>('general');
	let noteSeverity = $state<NoteSeverity>('info');
	let showNetWideComposer = $state(false);

	// Expanded note history on cards
	let expandedNotesCheckInId = $state<string | null>(null);
	let expandedNotesMissionId = $state<string | null>(null);

	// Overflow menu
	let overflowOpenId = $state<string | null>(null);

	// Note category metadata
	const noteCategoryMeta: Record<NoteCategory, { label: string; color: string; icon: string }> = {
		general: { label: 'Gen', color: '#6b7280', icon: 'M12 19l9 2-9-18-9 18 9-2zm0 0v-8' },
		medical: { label: 'Med', color: '#ef4444', icon: 'M12 2v20M2 12h20' },
		logistical: { label: 'Log', color: '#3b82f6', icon: 'M1 3h22v18H1zM1 9h22' },
		tactical: { label: 'Tac', color: '#8b5cf6', icon: 'M12 2l3 7h7l-5.5 4 2 7L12 16l-6.5 4 2-7L2 9h7z' },
		weather: { label: 'Wx', color: '#06b6d4', icon: 'M3 15a4 4 0 014-4 4 4 0 017.87 3H16a3 3 0 010 6H7' },
		resource: { label: 'Res', color: '#f59e0b', icon: 'M17 10V6a2 2 0 00-2-2H9a2 2 0 00-2 2v4M3 10h18v10H3z' },
		hazard: { label: 'Haz', color: '#f97316', icon: 'M12 2L2 20h20L12 2zM12 10v4M12 17h.01' },
		comms: { label: 'Com', color: '#6b7280', icon: 'M8.5 2A5.5 5.5 0 003 7.5v3A5.5 5.5 0 008.5 16H10v5l5-5h2.5A5.5 5.5 0 0023 10.5v-3A5.5 5.5 0 0017.5 2z' },
	};

	const severityMeta: Record<NoteSeverity, { label: string; color: string }> = {
		info: { label: 'Info', color: '#6b7280' },
		routine: { label: 'Routine', color: '#22c55e' },
		priority: { label: 'Priority', color: '#f59e0b' },
		urgent: { label: 'Urgent', color: '#ef4444' },
	};

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
				notes: newNetNotes.trim(),
				missionBrief: newNetMissionBrief.trim()
			});
			// Auto-open the net.
			await api.openNet(net.id);
			await loadNetData(net.id);
			showCreateForm = false;
			newNetName = '';
			newNetFreq = '';
			newNetNotes = '';
			newNetMissionBrief = '';
		} catch (e) {
			console.error('Failed to create net:', e);
		} finally {
			creating = false;
		}
	}

	const trafficShortcuts: Record<string, string> = { R: 'routine', P: 'priority', W: 'welfare', E: 'emergency' };
	const categoryNames = new Set(Object.keys(stationCategoryMeta));

	async function handleQuickAdd() {
		if (!$activeNet || !quickAddInput.trim()) return;

		const parts = quickAddInput.trim().split(/\s+/);
		const callsign = parts[0].toUpperCase();
		let traffic = '';
		let category = '';

		// Parse remaining tokens: check traffic shortcuts first, then category names
		for (let i = 1; i < parts.length; i++) {
			const token = parts[i].toUpperCase();
			const lower = parts[i].toLowerCase();
			if (!traffic && trafficShortcuts[token]) {
				traffic = trafficShortcuts[token];
			} else if (!category && categoryNames.has(lower)) {
				category = lower;
			}
		}

		// Dedup: highlight existing operator instead of creating duplicate
		const existing = $checkIns.find((ci) => ci.callsign.toUpperCase() === callsign && ci.status !== 'released');
		if (existing) {
			highlightedCheckInId = existing.id;
			setTimeout(() => { highlightedCheckInId = null; }, 2000);
			quickAddInput = '';
			searchResults = [];
			quickAddRef?.focus();
			return;
		}

		try {
			await api.checkIn($activeNet.id, callsign, traffic, category);
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

	async function handleTrafficChange(ci: NetCheckIn, newTraffic: TrafficType) {
		if (!$activeNet) return;
		try {
			await api.updateCheckIn($activeNet.id, ci.id, { traffic: newTraffic });
		} catch (e) {
			console.error('Traffic update failed:', e);
		}
	}

	async function handleCategoryChange(ci: NetCheckIn, newCategory: StationCategory) {
		if (!$activeNet) return;
		try {
			await api.updateCheckIn($activeNet.id, ci.id, { category: newCategory });
		} catch (e) {
			console.error('Category update failed:', e);
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

	async function handlePinStation(callsign: string) {
		if (!$activeNet) return;
		try {
			const n = await api.pinStation($activeNet.id, callsign);
			activeNet.set(n);
		} catch (e) {
			console.error('Pin failed:', e);
		}
	}

	async function handleUnpinStation(callsign: string) {
		if (!$activeNet) return;
		try {
			const n = await api.unpinStation($activeNet.id, callsign);
			activeNet.set(n);
		} catch (e) {
			console.error('Unpin failed:', e);
		}
	}

	let dragPinCallsign = $state<string | null>(null);

	function onPinDragStart(e: DragEvent, callsign: string) {
		dragPinCallsign = callsign;
		if (e.dataTransfer) {
			e.dataTransfer.effectAllowed = 'move';
			e.dataTransfer.setData('text/plain', callsign);
		}
	}

	function onPinDragOver(e: DragEvent) {
		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
	}

	async function onPinDrop(e: DragEvent, targetCallsign: string) {
		e.preventDefault();
		if (!$activeNet || !dragPinCallsign || dragPinCallsign === targetCallsign) return;

		const pins = [...($activeNet.pinnedStations || [])];
		const fromIdx = pins.indexOf(dragPinCallsign);
		const toIdx = pins.indexOf(targetCallsign);
		if (fromIdx === -1 || toIdx === -1) return;

		pins.splice(fromIdx, 1);
		pins.splice(toIdx, 0, dragPinCallsign);

		try {
			const n = await api.reorderPins($activeNet.id, pins);
			activeNet.set(n);
		} catch (e) {
			console.error('Reorder pins failed:', e);
		}
		dragPinCallsign = null;
	}

	function scrollToOperator(callsign: string) {
		const el = document.querySelector(`[data-callsign="${callsign}"]`);
		if (el) el.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
	}

	function isPinned(callsign: string): boolean {
		return $activeNet?.pinnedStations?.includes(callsign) ?? false;
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
			const data: Partial<NetMission> = {
				title: newMissionTitle.trim(),
				description: newMissionDesc.trim(),
				priority: newMissionPriority,
				assignedTo: newMissionAssign,
				location: newMissionLocation.trim()
			};
			const lat = parseFloat(newMissionLat);
			const lon = parseFloat(newMissionLon);
			if (!isNaN(lat) && !isNaN(lon)) {
				data.lat = lat;
				data.lon = lon;
			}
			const mission = await api.createMission($activeNet.id, data);
			// Link selected annotations to the new mission.
			for (const annId of selectedAnnotationIds) {
				try {
					await api.linkAnnotation(annId, mission.id);
				} catch (err) {
					console.error('Link annotation failed:', err);
				}
			}
			showMissionForm = false;
			newMissionTitle = '';
			newMissionDesc = '';
			newMissionPriority = 'routine';
			newMissionAssign = '';
			newMissionLocation = '';
			newMissionLat = '';
			newMissionLon = '';
			selectedAnnotationIds = [];
		} catch (e) {
			console.error('Create mission failed:', e);
		}
	}

	function handleMissionLocationInput() {
		const q = newMissionLocation.trim().toLowerCase();
		if (!q) {
			missionLocSuggestions = [];
			showMissionLocDropdown = false;
			return;
		}
		missionLocSuggestions = $netAnnotations.filter(a =>
			a.label.toLowerCase().includes(q) ||
			(a.shortName && a.shortName.toLowerCase().includes(q))
		);
		showMissionLocDropdown = missionLocSuggestions.length > 0;
	}

	function selectMissionAnnotation(a: import('$lib/types').Annotation) {
		newMissionLocation = a.label;
		// Extract lat/lon from GeoJSON Point geometry.
		try {
			const geo = typeof a.geometry === 'string' ? JSON.parse(a.geometry) : a.geometry;
			if (geo?.type === 'Point' && geo.coordinates) {
				newMissionLon = String(geo.coordinates[0]);
				newMissionLat = String(geo.coordinates[1]);
			}
		} catch { /* ignore parse errors */ }
		showMissionLocDropdown = false;
		missionLocSuggestions = [];
	}

	async function handleMissionStatusChange(m: NetMission, status: string) {
		if (!$activeNet) return;
		try {
			await api.updateMission($activeNet.id, m.id, { status: status as any });
			// Flash the card so the user can follow it after reorder.
			if (recentlyChangedTimer) clearTimeout(recentlyChangedTimer);
			recentlyChangedMissionId = m.id;
			recentlyChangedTimer = setTimeout(() => { recentlyChangedMissionId = null; }, 2000);
		} catch (e) {
			console.error('Mission update failed:', e);
		}
	}

	async function handleAssignMission(ciId: string, missionId: string) {
		if (!$activeNet) return;
		try {
			await api.assignMission($activeNet.id, ciId, missionId);
			assigningCheckInId = null;
		} catch (e) {
			console.error('Assign mission failed:', e);
		}
	}

	async function handleUnassignMission(ciId: string, missionId: string) {
		if (!$activeNet) return;
		try {
			await api.unassignMission($activeNet.id, ciId, missionId);
		} catch (e) {
			console.error('Unassign mission failed:', e);
		}
	}

	function toggleDeviceList(ciId: string) {
		expandedDeviceId = expandedDeviceId === ciId ? null : ciId;
		addDeviceCallsign = '';
	}

	async function handleAddTrackedStation(ciId: string) {
		if (!$activeNet || !addDeviceCallsign.trim()) return;
		try {
			await api.addTrackedStation($activeNet.id, ciId, addDeviceCallsign.trim().toUpperCase());
			addDeviceCallsign = '';
		} catch (e) {
			console.error('Add tracked station failed:', e);
		}
	}

	async function handleRemoveTrackedStation(ciId: string, callsign: string) {
		if (!$activeNet) return;
		try {
			await api.removeTrackedStation($activeNet.id, ciId, callsign);
		} catch (e) {
			console.error('Remove tracked station failed:', e);
		}
	}

	function openNoteComposer(opts: { checkInId?: string; missionId?: string }) {
		noteCheckInId = opts.checkInId ?? null;
		noteMissionId = opts.missionId ?? null;
		showNetWideComposer = !opts.checkInId && !opts.missionId;
		noteContent = '';
		noteCategory = 'general';
		noteSeverity = 'info';
	}

	function closeNoteComposer() {
		noteCheckInId = null;
		noteMissionId = null;
		showNetWideComposer = false;
		noteContent = '';
		noteCategory = 'general';
		noteSeverity = 'info';
	}

	async function handleAddNote() {
		if (!$activeNet || !noteContent.trim()) return;
		try {
			const note = await api.addNetNote($activeNet.id, {
				checkInId: noteCheckInId ?? undefined,
				missionId: noteMissionId ?? undefined,
				content: noteContent.trim(),
				category: noteCategory,
				severity: noteSeverity,
			});
			notes.update((list) => [...list, note]);
			closeNoteComposer();
		} catch (e) {
			console.error('Add note failed:', e);
		}
	}

	// Operators assigned to each mission
	function operatorsForMission(missionId: string): NetCheckIn[] {
		return $checkIns.filter((ci) => ci.missionIds?.includes(missionId) && ci.status !== 'released');
	}

	// Sort missions: emergency > priority > welfare > routine, then active > open > complete
	const priorityOrder: Record<string, number> = { emergency: 0, priority: 1, welfare: 2, routine: 3 };
	const missionStatusOrder: Record<string, number> = { active: 0, open: 1, complete: 2 };

	let sortedMissions = $derived(
		[...$missions].sort((a, b) => {
			const sa = missionStatusOrder[a.status] ?? 1;
			const sb = missionStatusOrder[b.status] ?? 1;
			if (sa !== sb) return sa - sb;
			const pa = priorityOrder[a.priority] ?? 3;
			const pb = priorityOrder[b.priority] ?? 3;
			return pa - pb;
		})
	);

	let filteredMissions = $derived(
		sortedMissions.filter((m) => {
			if (missionFilter === 'active') return m.status !== 'complete';
			if (missionFilter === 'complete') return m.status === 'complete';
			return true;
		})
	);

	// Annotations linked to a specific mission
	function annotationsForMission(missionId: string): Annotation[] {
		return $annotationList.filter((a) => a.missionIds?.includes(missionId));
	}

	// Annotations available for linking (non-terminal)
	let linkableAnnotations = $derived(
		$annotationList.filter((a) => !isTerminalStatus(a.status))
	);

	let activeMissionCount = $derived($missions.filter((m) => m.status !== 'complete').length);
	let completeMissionCount = $derived($missions.filter((m) => m.status === 'complete').length);

	// Roster filtered by metrics bar click + category filter (AND logic)
	const STALE_MS = 20 * 60 * 1000;
	let displayedCheckIns = $derived.by(() => {
		let list = $sortedCheckIns;

		// Apply category filter first
		if (categoryFilter) {
			list = list.filter((ci) => (ci.category || 'general') === categoryFilter);
		}

		if (!metricsFilter) return list;
		const now = Date.now();
		return list.filter((ci) => {
			switch (metricsFilter) {
				case 'available': return ci.status === 'available';
				case 'assigned': return ci.status === 'assigned' || ci.status === 'enroute' || ci.status === 'onscene';
				case 'missing': return ci.status === 'missing' || ci.traffic === 'emergency';
				case 'stale': return ci.status !== 'released' && ci.status !== 'missing' && (now - new Date(ci.lastHeard).getTime()) > STALE_MS;
				default: return true;
			}
		});
	});

	function handleMetricClick(filter: MetricsFilter) {
		if (metricsFilter === filter) {
			metricsFilter = null; // toggle off
		} else {
			metricsFilter = filter;
			currentTab = 'roster'; // switch to roster when filtering
		}
	}

	function missionElapsed(m: NetMission): string {
		const start = new Date(m.createdAt).getTime();
		const end = m.completedAt ? new Date(m.completedAt).getTime() : Date.now();
		const ms = end - start;
		const h = Math.floor(ms / 3600000);
		const min = Math.floor((ms % 3600000) / 60000);
		if (h > 0) return `${h}h ${min}m`;
		return `${min}m`;
	}

	function handleFlyToMission(m: NetMission) {
		const coords: Array<{ lat: number; lon: number }> = [];

		// Mission location
		if (m.lat != null && m.lon != null) {
			coords.push({ lat: m.lat, lon: m.lon });
		}

		// Assigned operator positions
		const ops = operatorsForMission(m.id);
		for (const op of ops) {
			if (op.lat != null && op.lon != null) {
				coords.push({ lat: op.lat, lon: op.lon });
			}
		}

		// Linked annotation coordinates
		const anns = annotationsForMission(m.id);
		for (const ann of anns) {
			if (!ann.geometry) continue;
			try {
				const geom = JSON.parse(ann.geometry);
				if (geom.type === 'Point') {
					coords.push({ lat: geom.coordinates[1], lon: geom.coordinates[0] });
				} else if (geom.type === 'LineString') {
					for (const c of geom.coordinates) {
						coords.push({ lat: c[1], lon: c[0] });
					}
				} else if (geom.type === 'Polygon') {
					for (const c of geom.coordinates[0]) {
						coords.push({ lat: c[1], lon: c[0] });
					}
				}
			} catch { /* skip malformed geometry */ }
		}

		if (coords.length > 0) {
			onFlyToBounds?.(coords);
		} else if (m.lat != null && m.lon != null) {
			onFlyTo?.(m.lat, m.lon);
		}
	}

	function handleAssignPickerHover(m: NetMission, ci: NetCheckIn) {
		hoveredCheckInId.set(ci.id);
		const coords: Array<{ lat: number; lon: number }> = [];

		// Hovered candidate operator
		if (ci.lat != null && ci.lon != null) {
			coords.push({ lat: ci.lat, lon: ci.lon });
		}

		// Mission location
		if (m.lat != null && m.lon != null) {
			coords.push({ lat: m.lat, lon: m.lon });
		}

		// Already-assigned operator positions
		const ops = operatorsForMission(m.id);
		for (const op of ops) {
			if (op.lat != null && op.lon != null) {
				coords.push({ lat: op.lat, lon: op.lon });
			}
		}

		// Linked annotation coordinates
		const anns = annotationsForMission(m.id);
		for (const ann of anns) {
			if (!ann.geometry) continue;
			try {
				const geom = JSON.parse(ann.geometry);
				if (geom.type === 'Point') {
					coords.push({ lat: geom.coordinates[1], lon: geom.coordinates[0] });
				} else if (geom.type === 'LineString') {
					for (const c of geom.coordinates) {
						coords.push({ lat: c[1], lon: c[0] });
					}
				} else if (geom.type === 'Polygon') {
					for (const c of geom.coordinates[0]) {
						coords.push({ lat: c[1], lon: c[0] });
					}
				}
			} catch { /* skip */ }
		}

		if (coords.length > 1) {
			onFlyToBounds?.(coords);
		} else if (coords.length === 1) {
			onFlyTo?.(coords[0].lat, coords[0].lon);
		}
	}

	async function handleAssignOperatorToMission(missionId: string, ciId: string) {
		if (!$activeNet) return;
		try {
			await api.assignMission($activeNet.id, ciId, missionId);
			assigningMissionId = null;
		} catch (e) {
			console.error('Assign operator to mission failed:', e);
		}
	}

	async function handleUnassignFromMission(ciId: string, missionId: string) {
		if (!$activeNet) return;
		try {
			await api.unassignMission($activeNet.id, ciId, missionId);
		} catch (e) {
			console.error('Unassign from mission failed:', e);
		}
	}

	async function handleLinkAnnotationToMission(missionId: string, annId: string) {
		try {
			await api.linkAnnotation(annId, missionId);
			linkingAnnotationMissionId = null;
		} catch (e) {
			console.error('Link annotation to mission failed:', e);
		}
	}

	async function handleTogglePin(noteId: string) {
		if (!$activeNet) return;
		try {
			const updated = await api.toggleNotePin($activeNet.id, noteId);
			notes.update((list) =>
				list.map((n) => (n.id === noteId ? updated : n))
			);
		} catch (e) {
			console.error('Toggle pin failed:', e);
		}
	}

	async function handleUnlinkAnnotationFromMission(annId: string, missionId: string) {
		try {
			await api.unlinkAnnotation(annId, missionId);
		} catch (e) {
			console.error('Unlink annotation from mission failed:', e);
		}
	}

	function toggleAnnotationSelector(annId: string) {
		const idx = selectedAnnotationIds.indexOf(annId);
		if (idx >= 0) {
			selectedAnnotationIds = selectedAnnotationIds.filter((id) => id !== annId);
		} else {
			selectedAnnotationIds = [...selectedAnnotationIds, annId];
		}
	}

	let timelineFilter = $state('all');
	let timelineCallsignFilter = $state('');
	let filteredTimeline = $derived(
		$timeline.filter((e) => {
			if (timelineFilter !== 'all') {
				if (timelineFilter === 'checkins' && e.type !== 'checkin' && e.type !== 'checkout') return false;
				if (timelineFilter === 'assignments' && e.type !== 'assignment' && e.type !== 'status_change') return false;
				if (timelineFilter === 'missions' && e.type !== 'mission_created' && e.type !== 'mission_updated') return false;
				if (timelineFilter === 'rollcalls' && e.type !== 'rollcall') return false;
				if (timelineFilter === 'notes' && e.type !== 'note') return false;
			}
			if (timelineCallsignFilter.trim()) {
				const q = timelineCallsignFilter.trim().toUpperCase();
				return e.callsign.toUpperCase().includes(q) || e.summary.toUpperCase().includes(q);
			}
			return true;
		}).reverse()
	);

	// Parse category from timeline note entries (format: "[CATEGORY] Note by ...")
	function parseNoteCategory(summary: string): NoteCategory | null {
		const match = summary.match(/^\[(\w+)\]/);
		if (match) {
			const cat = match[1].toLowerCase();
			if (cat in noteCategoryMeta) return cat as NoteCategory;
		}
		return null;
	}
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
					<label for="net-brief">Mission Brief</label>
					<textarea id="net-brief" bind:value={newNetMissionBrief} rows="2" placeholder="Overall mission objective..."></textarea>
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
				{#if $activeNet.missionBrief}
					<span class="mission-brief">{$activeNet.missionBrief}</span>
				{/if}
			</div>
			<div class="header-actions">
				<button
					class="action-btn"
					class:ops-view-set={$opsView}
					onclick={() => onSetOpsView?.()}
					title="Save Ops View"
				>
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
						<circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/>
						<circle cx="8" cy="8" r="2" fill="currentColor"/>
						<path d="M8 1v3M8 12v3M1 8h3M12 8h3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
				</button>
				<button class="action-btn" onclick={handleRollCall} title="Roll Call">
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
						<path d="M1 8h3l2-5 3 10 2-5h4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				<button class="action-btn" onclick={() => openNoteComposer({})} title="Log Note">
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
						<path d="M12 2l2 2-8 8H4v-2l8-8z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
						<path d="M2 14h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
				</button>
				<button class="action-btn" onclick={() => openICS309($activeNet?.id)} title="ICS-309 Log">
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
						<path d="M4 2h8a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z" stroke="currentColor" stroke-width="1.5"/>
						<path d="M6 5h4M6 8h4M6 11h2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
				</button>
				<button class="action-btn danger" onclick={handleCloseNet} title="Close Net">
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
						<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
				</button>
			</div>
		</div>

		<!-- Metrics bar -->
		{#if $activeCheckIns.length > 0 || $missions.length > 0}
			<div class="metrics-bar" role="toolbar" aria-label="Net status metrics">
				<button
					class="metric"
					class:active={metricsFilter === null && currentTab === 'roster'}
					onclick={() => { metricsFilter = null; currentTab = 'roster'; }}
					title="Total checked-in operators"
				>
					<span class="metric-value">{$netMetrics.totalIn}</span>
					<span class="metric-label">In</span>
				</button>
				<button
					class="metric metric-available"
					class:active={metricsFilter === 'available'}
					onclick={() => handleMetricClick('available')}
					title="Available for tasking"
				>
					<span class="metric-value">{$netMetrics.available}</span>
					<span class="metric-label">Avail</span>
				</button>
				<button
					class="metric metric-assigned"
					class:active={metricsFilter === 'assigned'}
					onclick={() => handleMetricClick('assigned')}
					title="Assigned / en route / on scene"
				>
					<span class="metric-value">{$netMetrics.assigned}</span>
					<span class="metric-label">Tasked</span>
				</button>
				<button
					class="metric metric-missing"
					class:active={metricsFilter === 'missing'}
					class:alert={$netMetrics.missing > 0}
					onclick={() => handleMetricClick('missing')}
					title="Missing or emergency traffic"
				>
					<span class="metric-value">{$netMetrics.missing}</span>
					<span class="metric-label">Alert</span>
				</button>
				{#if $netMetrics.stale > 0}
					<button
						class="metric metric-stale"
						class:active={metricsFilter === 'stale'}
						onclick={() => handleMetricClick('stale')}
						title="Not heard from in 20+ minutes"
					>
						<span class="metric-value">{$netMetrics.stale}</span>
						<span class="metric-label">Stale</span>
					</button>
				{/if}
				<span class="metric-divider"></span>
				<button
					class="metric metric-missions"
					onclick={() => { metricsFilter = null; currentTab = 'missions'; missionFilter = 'active'; }}
					title="Active missions"
				>
					<span class="metric-value">{$netMetrics.missionsActive}</span>
					<span class="metric-label">Active</span>
				</button>
				<button
					class="metric metric-done"
					onclick={() => { metricsFilter = null; currentTab = 'missions'; missionFilter = 'complete'; }}
					title="Completed missions"
				>
					<span class="metric-value">{$netMetrics.missionsDone}</span>
					<span class="metric-label">Done</span>
				</button>
				{#if metricsFilter}
					<button
						class="metric-clear"
						onclick={() => (metricsFilter = null)}
						title="Clear filter"
					>&times;</button>
				{/if}
			</div>
		{/if}

		<!-- Tabs -->
		<div class="tabs">
			<button class="tab" class:active={currentTab === 'roster'} onclick={() => { currentTab = 'roster'; metricsFilter = null; }}>
				Roster <span class="tab-count">{$activeCheckIns.length}</span>
			</button>
			<button class="tab" class:active={currentTab === 'missions'} onclick={() => { currentTab = 'missions'; metricsFilter = null; }}>
				Missions {#if activeMissionCount > 0}<span class="tab-count">{activeMissionCount}</span>{/if}
			</button>
			<button class="tab" class:active={currentTab === 'locations'} onclick={() => { currentTab = 'locations'; metricsFilter = null; }}>
				Locations {#if $netAnnotations.length > 0}<span class="tab-count">{$netAnnotations.length}</span>{/if}
			</button>
			<button class="tab" class:active={currentTab === 'timeline'} onclick={() => { currentTab = 'timeline'; metricsFilter = null; }}>
				Timeline
			</button>
		</div>

		<!-- Tab content -->
		<div class="tab-content">
			{#if currentTab === 'roster'}
				<!-- Roster header with export -->
				{#if $activeNet && $sortedCheckIns.length > 0}
					<div class="roster-header">
						<span class="roster-count">{#if metricsFilter || categoryFilter}{displayedCheckIns.length} of {$activeCheckIns.length}{:else}{$activeCheckIns.length} active{/if}</span>
						<a href={api.rosterExportUrl($activeNet.id)} class="export-link" download>CSV</a>
					</div>
				{/if}
				<!-- Category filter chips -->
				{#if $categoryCounts.size > 1 || ($categoryCounts.size === 1 && !$categoryCounts.has('general'))}
					<div class="category-chips">
						<button class="cat-chip" class:active={categoryFilter === null}
							onclick={() => { categoryFilter = null; }}>
							All
						</button>
						{#each [...$categoryCounts.entries()].sort((a, b) => a[0].localeCompare(b[0])) as [cat, count]}
							{@const meta = stationCategoryMeta[cat]}
							<button class="cat-chip" class:active={categoryFilter === cat}
								style="--cat-chip-color: {meta.color}"
								onclick={() => { categoryFilter = categoryFilter === cat ? null : cat; currentTab = 'roster'; }}>
								{meta.short} <span class="cat-chip-count">{count}</span>
							</button>
						{/each}
						{#if categoryFilter}
							<button class="cat-chip-clear" onclick={() => (categoryFilter = null)} title="Clear category filter">&times;</button>
						{/if}
					</div>
				{/if}
				<!-- Quick Add Bar -->
				<div class="quick-add">
					<div class="quick-add-wrap">
						<input
							bind:this={quickAddRef}
							type="text"
							bind:value={quickAddInput}
							oninput={handleQuickAddInput}
							onkeydown={(e) => { if (e.key === 'Enter') handleQuickAdd(); }}
							placeholder="KD7BBC R medical"
							class="quick-add-input"
							title="Format: CALLSIGN [traffic] [category]&#10;Traffic: R=Routine P=Priority W=Welfare E=Emergency&#10;Category: medical, sag, command, marshal, fixed, mobile, tactical"
						/>
						<button class="quick-add-btn" onclick={handleQuickAdd} disabled={!quickAddInput.trim()}>+</button>
					</div>
					<div class="quick-add-hint">
						<span class="hint-key">R</span>outine
						<span class="hint-key">P</span>riority
						<span class="hint-key">W</span>elfare
						<span class="hint-key">E</span>mergency
						+ category
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

				<!-- Pinned/urgent notes banner -->
				{#if $pinnedNotes.length > 0}
					<div class="pinned-banner">
						{#each $pinnedNotes as pn}
							{@const pnCat = noteCategoryMeta[pn.category as NoteCategory]}
							<div class="pinned-note" style="--note-border-color: {pnCat?.color ?? '#6b7280'}">
								<span class="pinned-icon">📌</span>
								<span class="pinned-cat" style="background: {pnCat?.color ?? '#6b7280'}">{pn.category}</span>
								<span class="pinned-text">{pn.content}</span>
								<span class="pinned-age">{timeAgo(pn.createdAt)}</span>
								<button class="pinned-remove" title="Unpin" onclick={() => handleTogglePin(pn.id)}>✕</button>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Pinned stations strip -->
				{#if $pinnedCheckIns.length > 0}
					<div class="pin-strip">
						{#each $pinnedCheckIns as pci (pci.callsign)}
							{@const staleMs = Date.now() - new Date(pci.lastHeard).getTime()}
							{@const isStale = staleMs > 20 * 60 * 1000}
							{@const isUrgent = pci.status === 'missing' || pci.traffic === 'emergency'}
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div
								class="pin-chip"
								role="button"
								tabindex="0"
								class:pin-stale={isStale && !isUrgent}
								class:pin-urgent={isUrgent}
								draggable="true"
								ondragstart={(e) => onPinDragStart(e, pci.callsign)}
								ondragover={onPinDragOver}
								ondrop={(e) => onPinDrop(e, pci.callsign)}
								onclick={() => scrollToOperator(pci.callsign)}
								onkeydown={(e) => { if (e.key === 'Enter') scrollToOperator(pci.callsign); }}
								title="{pci.callsign} — {pci.status} — click to scroll"
							>
								<span class="pin-dot" style="background: {statusColors[pci.status]}"></span>
								<span class="pin-call">{pci.callsign}</span>
								{#if pci.tacticalCall}
									<span class="pin-tactical">{pci.tacticalCall}</span>
								{/if}
								<span class="pin-age {stalenessClass(pci.lastHeard)}">{stalenessLabel(pci.lastHeard)}</span>
								<button class="pin-remove" title="Unpin" onclick={(e) => { e.stopPropagation(); handleUnpinStation(pci.callsign); }}>✕</button>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Roster -->
				<div class="roster">
					{#each displayedCheckIns as ci (ci.id)}
						<div class="operator-card" data-callsign={ci.callsign} class:released={ci.status === 'released'} class:highlighted={highlightedCheckInId === ci.id} class:mission-highlighted={$highlightedCheckIns.has(ci.id)}>
							<div class="op-status-bar" style="background: {statusColors[ci.status]}"></div>
							<div class="op-main">
								<div class="op-header">
									<span class="op-callsign">{ci.callsign}</span>
									{#if ci.tacticalCall}
										<span class="op-tactical">"{ci.tacticalCall}"</span>
									{/if}
									{#if ci.source === 'voice'}
										<span class="source-badge vox">VOX</span>
									{/if}
									{#if ci.category && ci.category !== 'general'}
										{@const catMeta = stationCategoryMeta[ci.category as StationCategory]}
										{#if catMeta}
											<span class="source-badge cat-badge" style="background: {catMeta.color}">{catMeta.short}</span>
										{/if}
									{/if}
									{#if ci.trackedStations?.length > 0}
										<button class="device-badge" onclick={() => toggleDeviceList(ci.id)}>{ci.trackedStations.length} dev</button>
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
								<div class="op-position">
									{#if ci.lat != null && ci.lon != null}
										<button class="pos-flyto" onclick={() => onFlyTo?.(ci.lat!, ci.lon!)}>
											📍 {ci.lat!.toFixed(4)}, {ci.lon!.toFixed(4)}
										</button>
										{#if ci.source === 'voice'}
											<button class="pos-place-btn" title="Update position"
												onclick={() => onPlaceOperator?.(ci.id, ci.callsign)}>⊕</button>
										{/if}
									{:else}
										<button class="pos-place-btn pos-no-position"
											onclick={() => onPlaceOperator?.(ci.id, ci.callsign)}>
											📍 Place on map
										</button>
									{/if}
								</div>
								{#if ci.assignment}
									<div class="op-assignment">📋 {ci.assignment}</div>
								{/if}
								{#if ci.missionIds?.length > 0}
									<div class="op-mission-chips">
									{#each ci.missionIds as mid}
										{@const linkedMission = $missions.find((m) => m.id === mid)}
										{#if linkedMission}
											<div
													class="op-mission-chip"
													class:chip-highlighted={$hoveredMissionId === mid}
													style="--mission-color: {trafficColors[linkedMission.priority] ?? '#6b7280'}"
													onmouseenter={() => hoveredMissionId.set(mid)}
													onmouseleave={() => hoveredMissionId.set(null)}
												>
													<span class="mission-chip-dot"></span>
													<span class="mission-chip-title">{linkedMission.title}</span>
													<button class="mission-chip-remove" title="Unassign mission" onclick={() => handleUnassignMission(ci.id, mid)}>✕</button>
												</div>
										{/if}
									{/each}
									</div>
								{/if}
								{#if ci.missedRollCalls > 0}
									<div class="missed-badge">
										<span>Missed {ci.missedRollCalls} roll call{ci.missedRollCalls > 1 ? 's' : ''}</span>
										<button class="rollcall-respond-btn" onclick={() => handleRollCallResponse(ci)} title="Mark as responded">Responded</button>
									</div>
								{/if}
								<!-- Inline note preview -->
								{#if ($notesByCheckIn.get(ci.id)?.length ?? 0) > 0}
									{@const ciNotes = $notesByCheckIn.get(ci.id)!}
									{@const latest = ciNotes[0]}
									{@const catMeta = noteCategoryMeta[latest.category as NoteCategory]}
									<button class="note-preview" onclick={() => { expandedNotesCheckInId = expandedNotesCheckInId === ci.id ? null : ci.id; }}>
										<span class="note-preview-cat" style="background: {catMeta?.color ?? '#6b7280'}">{latest.category}</span>
										<span class="note-preview-text">{latest.content}</span>
										{#if ciNotes.length > 1}
											<span class="note-preview-count">+{ciNotes.length - 1}</span>
										{/if}
										<span class="note-preview-age">{timeAgo(latest.createdAt)}</span>
									</button>
								{/if}
							</div>
							<div class="op-action-bar">
								<select
									class="bar-select"
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
								<select
									class="bar-select"
									value={ci.traffic || 'none'}
									onchange={(e) => handleTrafficChange(ci, (e.target as HTMLSelectElement).value as TrafficType)}
								>
									<option value="none">Traffic</option>
									<option value="routine">Routine</option>
									<option value="priority">Priority</option>
									<option value="welfare">Welfare</option>
									<option value="emergency">Emergency</option>
								</select>
								<select
									class="bar-select"
									value={ci.category || 'general'}
									onchange={(e) => handleCategoryChange(ci, (e.target as HTMLSelectElement).value as StationCategory)}
								>
									{#each Object.entries(stationCategoryMeta) as [key, meta]}
										<option value={key}>{meta.label}</option>
									{/each}
								</select>
								<span class="bar-spacer"></span>
								<button class="bar-btn" title="Assign Mission" onclick={() => { assigningCheckInId = assigningCheckInId === ci.id ? null : ci.id; }}>Assign</button>
								<button class="bar-btn" title="Note" onclick={() => { if (noteCheckInId === ci.id) { closeNoteComposer(); } else { openNoteComposer({ checkInId: ci.id }); } }}>Note</button>
								<div class="overflow-wrap">
									<button class="bar-btn overflow-trigger" onclick={() => { overflowOpenId = overflowOpenId === ci.id ? null : ci.id; }} title="More actions">
										<svg width="14" height="14" viewBox="0 0 16 16" fill="none"><circle cx="4" cy="8" r="1.5" fill="currentColor"/><circle cx="8" cy="8" r="1.5" fill="currentColor"/><circle cx="12" cy="8" r="1.5" fill="currentColor"/></svg>
									</button>
									{#if overflowOpenId === ci.id}
										<div class="overflow-menu">
											{#if ci.lat != null && ci.lon != null}
												<button class="overflow-item" onclick={() => { onFlyTo?.(ci.lat!, ci.lon!); overflowOpenId = null; }}>Fly to</button>
											{/if}
											<button class="overflow-item" class:active={expandedDeviceId === ci.id} onclick={() => { toggleDeviceList(ci.id); overflowOpenId = null; }}>Tracked devices</button>
											{#if isPinned(ci.callsign)}
												<button class="overflow-item" onclick={() => { handleUnpinStation(ci.callsign); overflowOpenId = null; }}>Unpin from strip</button>
											{:else}
												<button class="overflow-item" onclick={() => { handlePinStation(ci.callsign); overflowOpenId = null; }}>Pin to strip</button>
											{/if}
											{#if ci.missedRollCalls > 0}
												<button class="overflow-item" onclick={() => { handleRollCallResponse(ci); overflowOpenId = null; }}>Roll call response</button>
											{/if}
											<button class="overflow-item overflow-danger" onclick={() => { handleCheckOut(ci); overflowOpenId = null; }}>Check out</button>
										</div>
									{/if}
								</div>
							</div>

							<!-- Expandable sections (full card width) -->
							{#if expandedDeviceId === ci.id}
								<div class="card-expand">
									<div class="device-list">
										{#each ci.trackedStations || [] as dev}
											<div class="device-chip">
												<span class="device-call">{dev.callsign}</span>
												<span class="device-type">{dev.autoLinked ? 'SSID' : 'manual'}</span>
												<button class="device-remove" onclick={() => handleRemoveTrackedStation(ci.id, dev.callsign)} title="Remove">×</button>
											</div>
										{/each}
										<div class="device-add">
											<input
												type="text"
												bind:value={addDeviceCallsign}
												placeholder="Callsign"
												class="device-add-input"
												onkeydown={(e) => { if (e.key === 'Enter') handleAddTrackedStation(ci.id); if (e.key === 'Escape') { expandedDeviceId = null; } }}
											/>
											<button class="device-add-btn" onclick={() => handleAddTrackedStation(ci.id)} disabled={!addDeviceCallsign.trim()}>+</button>
										</div>
									</div>
								</div>
							{/if}

							{#if assigningCheckInId === ci.id}
								<div class="card-expand">
									<div class="assign-picker">
										{#each $missions.filter((m) => m.status !== 'complete' && !ci.missionIds?.includes(m.id)) as m}
											<button class="assign-option" onclick={() => handleAssignMission(ci.id, m.id)}>
												<span class="priority-dot" style="background: {trafficColors[m.priority] ?? '#6b7280'}"></span>
												{m.title}
											</button>
										{/each}
										{#if $missions.filter((m) => m.status !== 'complete' && !ci.missionIds?.includes(m.id)).length === 0}
											<span class="assign-empty">No available missions</span>
										{/if}
									</div>
								</div>
							{/if}

							{#if noteCheckInId === ci.id}
								<div class="card-expand">
									<div class="note-composer">
										<div class="note-cat-row">
											{#each Object.entries(noteCategoryMeta) as [key, meta]}
												<button
													class="note-cat-chip"
													class:active={noteCategory === key}
													style="--cat-color: {meta.color}"
													onclick={() => { noteCategory = key as NoteCategory; }}
												>{meta.label}</button>
											{/each}
										</div>
										<textarea
											class="note-textarea"
											bind:value={noteContent}
											placeholder="Note text..."
											rows="2"
											onkeydown={(e) => { if (e.key === 'Escape') closeNoteComposer(); }}
										></textarea>
										<div class="note-sev-row">
											<span class="note-sev-label">Severity:</span>
											{#each Object.entries(severityMeta) as [key, meta]}
												<button
													class="note-sev-dot"
													class:active={noteSeverity === key}
													style="--sev-color: {meta.color}"
													title={meta.label}
													onclick={() => { noteSeverity = key as NoteSeverity; }}
												></button>
											{/each}
											<button class="note-save-btn" onclick={handleAddNote} disabled={!noteContent.trim()}>Save</button>
										</div>
									</div>
								</div>
							{/if}

							<!-- Expanded note history -->
							{#if expandedNotesCheckInId === ci.id}
								{@const ciNotes = $notesByCheckIn.get(ci.id) || []}
								<div class="card-expand">
									<div class="note-history">
										{#each ciNotes as n}
											{@const nCat = noteCategoryMeta[n.category as NoteCategory]}
											<div class="note-history-item" style="--note-border-color: {nCat?.color ?? '#6b7280'}">
												<div class="note-history-header">
													<span class="note-history-cat" style="background: {nCat?.color ?? '#6b7280'}">{n.category}</span>
													{#if n.severity && n.severity !== 'info'}
														<span class="note-history-sev" style="color: {severityMeta[n.severity as NoteSeverity]?.color ?? '#6b7280'}">{n.severity}</span>
													{/if}
													<span class="note-history-author">{n.authorName}</span>
													<span class="note-history-time">{timeAgo(n.createdAt)}</span>
												</div>
												<p class="note-history-content">{n.content}</p>
											</div>
										{/each}
										{#if ciNotes.length === 0}
											<p class="empty" style="padding: 0.5rem;">No notes for this operator.</p>
										{/if}
									</div>
								</div>
							{/if}
						</div>
					{/each}
					{#if $sortedCheckIns.length === 0}
						<p class="empty">No operators checked in. Use the input above to add.</p>
					{:else if displayedCheckIns.length === 0 && (metricsFilter || categoryFilter)}
						<p class="empty">No operators match filter. <button class="link-btn" onclick={() => { metricsFilter = null; categoryFilter = null; }}>Clear filters</button></p>
					{/if}
				</div>

			{:else if currentTab === 'missions'}
				<!-- Mission toolbar -->
				<div class="mission-toolbar">
					{#if !showMissionForm}
						<button class="btn-secondary btn-sm" onclick={() => (showMissionForm = true)}>+ New</button>
					{/if}
					<div class="mission-filter-chips">
						<button class="filter-chip" class:active={missionFilter === 'all'} onclick={() => (missionFilter = 'all')}>All {$missions.length}</button>
						<button class="filter-chip" class:active={missionFilter === 'active'} onclick={() => (missionFilter = 'active')}>Active {activeMissionCount}</button>
						{#if completeMissionCount > 0}
							<button class="filter-chip" class:active={missionFilter === 'complete'} onclick={() => (missionFilter = 'complete')}>Done {completeMissionCount}</button>
						{/if}
					</div>
				</div>

				{#if showMissionForm}
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
						<div class="mission-loc-wrap">
							<input type="text" bind:value={newMissionLocation} placeholder="Location (e.g., Main & 5th St)" oninput={handleMissionLocationInput} onfocus={handleMissionLocationInput} onblur={() => { setTimeout(() => { showMissionLocDropdown = false; }, 150); }} />
							{#if showMissionLocDropdown && missionLocSuggestions.length > 0}
								<div class="mission-loc-dropdown">
									{#each missionLocSuggestions.slice(0, 6) as sug}
										<button class="mission-loc-item" onmousedown={() => selectMissionAnnotation(sug)}>
											<span class="mission-loc-name">{sug.label}</span>
											{#if sug.shortName}
												<span class="mission-loc-short">{sug.shortName}</span>
											{/if}
										</button>
									{/each}
								</div>
							{/if}
						</div>
						<div class="form-row">
							<input type="text" bind:value={newMissionLat} placeholder="Lat" inputmode="decimal" />
							<input type="text" bind:value={newMissionLon} placeholder="Lon" inputmode="decimal" />
						</div>
						{#if linkableAnnotations.length > 0}
							<div class="annotation-link-section">
								<span class="field-label-sm">Link Annotations {#if selectedAnnotationIds.length > 0}<span class="link-count">({selectedAnnotationIds.length})</span>{/if}</span>
								<div class="annotation-chips-wrap">
									{#each linkableAnnotations as ann}
										{@const cat = ann.category || 'general'}
										{@const selected = selectedAnnotationIds.includes(ann.id)}
										<button
											class="annotation-chip"
											class:selected
											onclick={() => toggleAnnotationSelector(ann.id)}
										>
											<svg width="10" height="10" viewBox="0 0 16 16" fill="none">
												<path d={categoryMeta[cat].icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
											</svg>
											{ann.label}
										</button>
									{/each}
								</div>
							</div>
						{/if}
						<div class="form-actions">
							<button class="btn-secondary" onclick={() => { showMissionForm = false; selectedAnnotationIds = []; }}>Cancel</button>
							<button class="btn-primary" onclick={handleCreateMission} disabled={!newMissionTitle.trim()}>Create</button>
						</div>
					</div>
				{/if}

				<!-- Mission list -->
				<div class="mission-list">
					{#each filteredMissions as m (m.id)}
						{@const assignedOps = operatorsForMission(m.id)}
						{@const hasNoOperators = assignedOps.length === 0 && !m.assignedTo}
						<div
							class="mission-card priority-{m.priority}"
							class:complete={m.status === 'complete'}
							class:just-moved={recentlyChangedMissionId === m.id}
							role="article"
							onmouseenter={() => hoveredMissionId.set(m.id)}
							onmouseleave={() => hoveredMissionId.set(null)}
							onfocus={() => hoveredMissionId.set(m.id)}
							onblur={() => hoveredMissionId.set(null)}
						>
							<div class="mission-body">
								<div class="mission-header">
									<span class="mission-title">{m.title}</span>
									<span class="priority-badge priority-{m.priority}">{m.priority}</span>
									<span class="mission-age">{missionElapsed(m)}</span>
									<button
										class="fly-to-btn"
										onclick={() => handleFlyToMission(m)}
										title="Fit map to mission area"
									>
										<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
											<circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/>
											<circle cx="8" cy="8" r="2" fill="currentColor"/>
											<path d="M8 1v3M8 12v3M1 8h3M12 8h3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
										</svg>
									</button>
								</div>
								{#if m.description}
									<p class="mission-desc">{m.description}</p>
								{/if}
								{#if m.location}
									<div class="mission-location">📍 {m.location}</div>
								{/if}

								<!-- Assigned operators -->
								<div class="mission-operators">
									{#if assignedOps.length > 0}
										{#each assignedOps as op}
											<div class="mission-op-chip">
												<span class="mission-op-dot" style="background: {statusColors[op.status]}"></span>
												<span class="mission-op-call">{op.callsign}</span>
												<span class="mission-op-status">{op.status}</span>
												{#if m.status !== 'complete'}
													<button class="mission-op-remove" title="Unassign" onclick={() => handleUnassignFromMission(op.id, m.id)}>✕</button>
												{/if}
											</div>
										{/each}
									{:else if m.assignedTo}
										<span class="mission-assigned-text">→ {m.assignedTo}</span>
									{/if}
									{#if hasNoOperators && m.status !== 'complete'}
										<span class="mission-unassigned">No operators assigned</span>
									{/if}
								</div>

								<!-- Linked annotations -->
								{#if annotationsForMission(m.id).length > 0}
									<div class="mission-annotations">
										{#each annotationsForMission(m.id) as ann}
											{@const annCat = ann.category || 'general'}
											<div class="mission-ann-chip">
												<svg width="10" height="10" viewBox="0 0 16 16" fill="none">
													<path d={categoryMeta[annCat].icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
												</svg>
												<span class="mission-ann-label">{ann.label}</span>
												{#if m.status !== 'complete'}
													<button class="mission-ann-remove" title="Unlink" onclick={() => handleUnlinkAnnotationFromMission(ann.id, m.id)}>×</button>
												{/if}
											</div>
										{/each}
									</div>
								{/if}

								<!-- Annotation link picker -->
								{#if linkingAnnotationMissionId === m.id}
									{@const availableAnns = linkableAnnotations.filter((a) => !a.missionIds?.includes(m.id))}
									<div class="assign-picker">
										{#each availableAnns as ann}
											{@const annCat = ann.category || 'general'}
											<button class="assign-option" onclick={() => handleLinkAnnotationToMission(m.id, ann.id)}>
												<svg width="10" height="10" viewBox="0 0 16 16" fill="none">
													<path d={categoryMeta[annCat].icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
												</svg>
												{ann.label}
											</button>
										{/each}
										{#if availableAnns.length === 0}
											<span class="assign-empty">No available annotations</span>
										{/if}
									</div>
								{/if}

								<!-- Assign operator picker (from mission side) -->
								{#if assigningMissionId === m.id}
									<div class="assign-picker">
										{#each $activeCheckIns.filter((ci) => !ci.missionIds?.includes(m.id)) as ci}
											<button
												class="assign-option"
												onclick={() => handleAssignOperatorToMission(m.id, ci.id)}
												onmouseenter={() => handleAssignPickerHover(m, ci)}
												onmouseleave={() => hoveredCheckInId.set(null)}
											>
												<span class="mission-op-dot" style="background: {statusColors[ci.status]}"></span>
												{ci.callsign}
												{#if ci.tacticalCall}
													<span class="assign-option-tactical">"{ci.tacticalCall}"</span>
												{/if}
												{#if ci.lat != null}<span class="assign-option-pos">GPS</span>{/if}
											</button>
										{/each}
										{#if $activeCheckIns.filter((ci) => !ci.missionIds?.includes(m.id)).length === 0}
											<span class="assign-empty">No available operators</span>
										{/if}
									</div>
								{/if}

								<!-- Inline note preview on mission card -->
								{#if ($notesByMission.get(m.id)?.length ?? 0) > 0}
									{@const mNotes = $notesByMission.get(m.id)!}
									{@const mLatest = mNotes[0]}
									{@const mNoteCat = noteCategoryMeta[mLatest.category as NoteCategory]}
									<button class="note-preview" onclick={() => { expandedNotesMissionId = expandedNotesMissionId === m.id ? null : m.id; }}>
										<span class="note-preview-cat" style="background: {mNoteCat?.color ?? '#6b7280'}">{mLatest.category}</span>
										<span class="note-preview-text">{mLatest.content}</span>
										{#if mNotes.length > 1}
											<span class="note-preview-count">+{mNotes.length - 1}</span>
										{/if}
										<span class="note-preview-age">{timeAgo(mLatest.createdAt)}</span>
									</button>
								{/if}

								<!-- Expanded mission note history -->
								{#if expandedNotesMissionId === m.id}
									{@const mNotesAll = $notesByMission.get(m.id) || []}
									<div class="note-history">
										{#each mNotesAll as n}
											{@const nCat = noteCategoryMeta[n.category as NoteCategory]}
											<div class="note-history-item" style="--note-border-color: {nCat?.color ?? '#6b7280'}">
												<div class="note-history-header">
													<span class="note-history-cat" style="background: {nCat?.color ?? '#6b7280'}">{n.category}</span>
													{#if n.severity && n.severity !== 'info'}
														<span class="note-history-sev" style="color: {severityMeta[n.severity as NoteSeverity]?.color ?? '#6b7280'}">{n.severity}</span>
													{/if}
													<span class="note-history-author">{n.authorName}</span>
													<span class="note-history-time">{timeAgo(n.createdAt)}</span>
												</div>
												<p class="note-history-content">{n.content}</p>
											</div>
										{/each}
										{#if mNotesAll.length === 0}
											<p class="empty" style="padding: 0.5rem;">No notes for this mission.</p>
										{/if}
									</div>
								{/if}

								<!-- Mission note composer -->
								{#if noteMissionId === m.id}
									<div class="note-composer">
										<div class="note-cat-row">
											{#each Object.entries(noteCategoryMeta) as [key, meta]}
												<button
													class="note-cat-chip"
													class:active={noteCategory === key}
													style="--cat-color: {meta.color}"
													onclick={() => { noteCategory = key as NoteCategory; }}
												>{meta.label}</button>
											{/each}
										</div>
										<textarea
											class="note-textarea"
											bind:value={noteContent}
											placeholder="Mission note..."
											rows="2"
											onkeydown={(e) => { if (e.key === 'Escape') closeNoteComposer(); }}
										></textarea>
										<div class="note-sev-row">
											<span class="note-sev-label">Severity:</span>
											{#each Object.entries(severityMeta) as [key, meta]}
												<button
													class="note-sev-dot"
													class:active={noteSeverity === key}
													style="--sev-color: {meta.color}"
													title={meta.label}
													onclick={() => { noteSeverity = key as NoteSeverity; }}
												></button>
											{/each}
											<button class="note-save-btn" onclick={handleAddNote} disabled={!noteContent.trim()}>Save</button>
										</div>
									</div>
								{/if}

							</div>
							<div class="mission-action-bar">
								<span class="mission-status-badge mission-status-{m.status}">{m.status}</span>
								<span class="bar-spacer"></span>
								{#if m.status !== 'complete'}
									<button class="bar-btn" title="Link annotation" onclick={() => { linkingAnnotationMissionId = linkingAnnotationMissionId === m.id ? null : m.id; }}>+ Ann</button>
									<button class="bar-btn" title="Assign operator" onclick={() => { assigningMissionId = assigningMissionId === m.id ? null : m.id; }}>+ Assign</button>
									<button class="bar-btn" title="Add note" onclick={() => { if (noteMissionId === m.id) { closeNoteComposer(); } else { openNoteComposer({ missionId: m.id }); } }}>+ Note</button>
								{/if}
								{#if m.status === 'open'}
									<button class="bar-btn" onclick={() => handleMissionStatusChange(m, 'active')}>Start</button>
								{:else if m.status === 'active'}
									<button class="bar-btn" onclick={() => handleMissionStatusChange(m, 'complete')}>Complete</button>
								{/if}
							</div>
						</div>
					{/each}
					{#if filteredMissions.length === 0}
						{#if $missions.length === 0}
							<p class="empty">No missions. Create one above.</p>
						{:else}
							<p class="empty">No {missionFilter} missions.</p>
						{/if}
					{/if}
				</div>

			{:else if currentTab === 'locations'}
				{#if $activeNet}
					<LocationManager
						net={$activeNet}
						onFlyTo={(lat, lon) => onFlyTo?.(lat, lon)}
						onPlaceOnMap={onPlaceAnnotation}
						mapClickedCoords={annotationMapCoords}
						{onMapCoordsConsumed}
					/>
				{:else}
					<p class="empty">Open a net to manage locations.</p>
				{/if}

			{:else if currentTab === 'timeline'}
				<!-- Net-wide note composer -->
				{#if showNetWideComposer}
					<div class="note-composer note-composer-top">
						<div class="note-cat-row">
							{#each Object.entries(noteCategoryMeta) as [key, meta]}
								<button
									class="note-cat-chip"
									class:active={noteCategory === key}
									style="--cat-color: {meta.color}"
									onclick={() => { noteCategory = key as NoteCategory; }}
								>{meta.label}</button>
							{/each}
						</div>
						<textarea
							class="note-textarea"
							bind:value={noteContent}
							placeholder="Net-wide note..."
							rows="2"
							onkeydown={(e) => { if (e.key === 'Escape') closeNoteComposer(); }}
						></textarea>
						<div class="note-sev-row">
							<span class="note-sev-label">Severity:</span>
							{#each Object.entries(severityMeta) as [key, meta]}
								<button
									class="note-sev-dot"
									class:active={noteSeverity === key}
									style="--sev-color: {meta.color}"
									title={meta.label}
									onclick={() => { noteSeverity = key as NoteSeverity; }}
								></button>
							{/each}
							<button class="note-save-btn" onclick={handleAddNote} disabled={!noteContent.trim()}>Save</button>
						</div>
					</div>
				{/if}

				<!-- Timeline filters -->
				<div class="timeline-filters">
					{#each [['all', 'All'], ['checkins', 'Check-ins'], ['assignments', 'Status'], ['missions', 'Missions'], ['notes', 'Notes'], ['rollcalls', 'Roll Calls']] as [value, label]}
						<button
							class="filter-chip"
							class:active={timelineFilter === value}
							onclick={() => (timelineFilter = value)}
						>{label}</button>
					{/each}
					<input
						type="text"
						class="timeline-search"
						bind:value={timelineCallsignFilter}
						placeholder="Filter by callsign..."
					/>
				</div>

				<!-- Timeline feed -->
				<div class="timeline-feed">
					{#each filteredTimeline as evt (evt.id)}
						{@const noteCat = evt.type === 'note' ? parseNoteCategory(evt.summary) : null}
						{#if evt.type === 'note' && noteCat}
							<div class="timeline-entry timeline-note" style="--note-border-color: {noteCategoryMeta[noteCat]?.color ?? '#6b7280'}">
								<span class="tl-icon">{eventIcons[evt.type] ?? '•'}</span>
								<div class="tl-content">
									<div class="tl-note-header">
										<span class="tl-note-cat" style="background: {noteCategoryMeta[noteCat]?.color ?? '#6b7280'}">{noteCat}</span>
									</div>
									<span class="tl-summary">{evt.summary.replace(/^\[\w+\]\s*/, '')}</span>
									<span class="tl-time">{timeAgo(evt.createdAt)}</span>
								</div>
							</div>
						{:else}
							<div class="timeline-entry">
								<span class="tl-icon">{eventIcons[evt.type] ?? '•'}</span>
								<div class="tl-content">
									<span class="tl-summary">{evt.summary}</span>
									<span class="tl-time">{timeAgo(evt.createdAt)}</span>
								</div>
							</div>
						{/if}
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
		overflow-x: hidden;
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
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 2px 8px;
		border-radius: var(--radius-sm);
	}

	.status-open { background: #22c55e; color: #000; }
	.status-draft { background: var(--color-primary); color: var(--color-text-muted); }
	.status-closed { background: #6b7280; color: #fff; }

	.elapsed {
		font-family: monospace;
		font-size: 0.8rem;
		color: #22c55e;
	}

	.frequency {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.mission-brief {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-style: italic;
		max-width: 200px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.header-actions {
		display: flex;
		gap: 6px;
		flex-shrink: 0;
		flex-wrap: wrap;
		justify-content: flex-end;
	}

	.action-btn {
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
		transition: all var(--duration-fast);
	}

	.action-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.action-btn.danger {
		border-color: rgba(239, 68, 68, 0.4);
	}

	.action-btn.danger:hover {
		border-color: #ef4444;
		color: #ef4444;
	}

	.action-btn.ops-view-set {
		border-color: #22c55e;
		color: #22c55e;
	}

	.action-btn.ops-view-set:hover {
		background: rgba(34, 197, 94, 0.1);
	}

	/* Tabs */
	.tabs {
		display: flex;
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.tab {
		flex: 1;
		padding: 12px var(--space-sm);
		background: none;
		border: none;
		border-bottom: 3px solid transparent;
		color: var(--color-text-muted);
		font-size: 0.85rem;
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.tab:hover { color: var(--color-text); }
	.tab.active {
		color: var(--color-accent);
		border-bottom-color: var(--color-accent);
	}

	.tab-count {
		font-size: 0.7rem;
		background: var(--color-primary);
		padding: 2px 6px;
		border-radius: 8px;
		margin-left: 4px;
	}

	.tab-content {
		flex: 1;
		overflow-y: auto;
		overflow-x: hidden;
	}

	/* Metrics bar */
	.metrics-bar {
		display: flex;
		align-items: center;
		gap: 2px;
		padding: 6px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		background: rgba(15, 52, 96, 0.3);
		flex-shrink: 0;
		flex-wrap: wrap;
	}

	.metric {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 4px 10px;
		background: none;
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: all var(--duration-fast);
		min-width: 44px;
		line-height: 1;
	}

	.metric:hover {
		background: rgba(255, 255, 255, 0.05);
		border-color: var(--color-primary);
		color: var(--color-text);
	}

	.metric.active {
		background: rgba(233, 69, 96, 0.1);
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.metric-value {
		font-family: monospace;
		font-size: 1.1rem;
		font-weight: 700;
		line-height: 1.2;
	}

	.metric-label {
		font-size: 0.6rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		opacity: 0.7;
	}

	.metric-available .metric-value { color: #22c55e; }
	.metric-assigned .metric-value { color: #3b82f6; }
	.metric-missing .metric-value { color: #6b7280; }
	.metric-missing.alert .metric-value { color: #ef4444; }
	.metric-missing.alert {
		animation: pulse-alert 2s ease-in-out infinite;
	}
	.metric-stale .metric-value { color: #f59e0b; }
	.metric-missions .metric-value { color: #8b5cf6; }
	.metric-done .metric-value { color: #6b7280; }

	.metric-divider {
		width: 1px;
		height: 28px;
		background: var(--color-primary);
		margin: 0 4px;
		flex-shrink: 0;
	}

	.metric-clear {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 22px;
		height: 22px;
		background: none;
		border: 1px solid rgba(239, 68, 68, 0.4);
		border-radius: var(--radius-sm);
		color: #ef4444;
		cursor: pointer;
		font-size: 0.9rem;
		line-height: 1;
		margin-left: auto;
		transition: all var(--duration-fast);
	}

	.metric-clear:hover {
		background: rgba(239, 68, 68, 0.15);
		border-color: #ef4444;
	}

	@keyframes pulse-alert {
		0%, 100% { background: transparent; }
		50% { background: rgba(239, 68, 68, 0.1); }
	}

	.link-btn {
		background: none;
		border: none;
		color: var(--color-accent);
		cursor: pointer;
		font-size: inherit;
		text-decoration: underline;
		padding: 0;
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
		font-size: 1rem;
		padding: 10px 12px;
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

	.quick-add-hint {
		font-size: 0.6rem;
		color: var(--color-text-muted);
		padding: 2px var(--space-sm) 0;
		opacity: 0.7;
		letter-spacing: 0.01em;
	}

	.hint-key {
		font-weight: 700;
		color: var(--color-accent);
	}

	.quick-add-btn {
		width: 44px;
		height: 44px;
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: white;
		font-size: 1.2rem;
		cursor: pointer;
		flex-shrink: 0;
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
		gap: 2px;
		width: 100%;
		padding: 10px 12px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
		min-height: 44px;
	}

	.search-item:hover { background: var(--color-primary); }
	.search-item:last-child { border-bottom: none; }

	.search-call {
		font-family: monospace;
		font-weight: 600;
		font-size: 0.85rem;
	}

	.search-comment {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	/* Roster header */
	.roster-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 6px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.export-link {
		color: var(--color-accent);
		text-decoration: none;
		font-weight: 600;
		font-size: 0.75rem;
	}

	.export-link:hover {
		text-decoration: underline;
	}

	/* Roster */
	.roster {
		padding: 0;
	}

	.operator-card {
		display: flex;
		flex-wrap: wrap;
		border-bottom: 1px solid var(--color-primary);
		transition: opacity var(--duration-fast), background var(--duration-fast);
	}

	.card-expand {
		flex-basis: 100%;
		padding: 0 var(--space-sm) var(--space-sm) calc(4px + var(--space-sm));
		min-width: 0;
		overflow: hidden;
		box-sizing: border-box;
	}

	.operator-card.released {
		opacity: 0.5;
	}

	.operator-card.highlighted {
		animation: highlight-flash 2s ease-out;
	}

	@keyframes highlight-flash {
		0% { background: rgba(59, 130, 246, 0.3); }
		100% { background: transparent; }
	}

	.operator-card.mission-highlighted {
		background: rgba(59, 130, 246, 0.08);
	}

	.op-status-bar {
		width: 4px;
		flex-shrink: 0;
	}

	.op-main {
		flex: 1;
		padding: 8px var(--space-sm) 4px var(--space-sm);
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
		font-size: 1rem;
	}

	.op-tactical {
		font-size: 0.8rem;
		color: var(--color-accent);
	}

	.source-badge {
		font-size: 0.65rem;
		font-weight: 700;
		padding: 2px 6px;
		border-radius: 3px;
		letter-spacing: 0.03em;
	}

	.source-badge.vox {
		background: #6b7280;
		color: #fff;
	}

	.traffic-badge {
		font-size: 0.7rem;
		font-weight: 700;
		color: #000;
		padding: 2px 8px;
		border-radius: 3px;
	}

	.op-age {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		margin-left: auto;
	}

	.op-age.stale-yellow { color: #f59e0b; }
	.op-age.stale-amber { color: #f97316; }
	.op-age.overdue { color: #ef4444; font-weight: 700; }

	.op-detail {
		display: flex;
		gap: var(--space-sm);
		font-size: 0.8rem;
		color: var(--color-text-muted);
		margin-top: 3px;
	}

	.op-location { font-style: italic; }

	.op-position {
		display: flex;
		align-items: center;
		gap: 4px;
		margin-top: 2px;
	}

	.pos-flyto {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 0.72rem;
		font-family: monospace;
		cursor: pointer;
		padding: 2px 4px;
		border-radius: var(--radius-sm);
		transition: color var(--duration-fast);
	}

	.pos-flyto:hover {
		color: var(--color-accent);
	}

	.pos-place-btn {
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.72rem;
		cursor: pointer;
		padding: 2px 6px;
		min-height: 24px;
		transition: all var(--duration-fast);
	}

	.pos-place-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.pos-no-position {
		color: var(--color-accent);
		border-color: var(--color-accent);
	}

	.op-assignment {
		font-size: 0.8rem;
		color: var(--color-accent);
		margin-top: 3px;
	}

	/* Roster mission chips */
	.op-mission-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin-top: 6px;
	}

	.op-mission-chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		background: var(--color-bg);
		border: 1px solid var(--mission-color);
		border-radius: var(--radius-sm);
		padding: 4px 10px;
		font-size: 0.75rem;
		color: var(--color-text);
		min-height: 32px;
		transition: background var(--duration-fast), box-shadow var(--duration-fast);
		cursor: default;
	}

	.op-mission-chip.chip-highlighted {
		box-shadow: 0 0 0 2px var(--mission-color);
		background: rgba(255, 255, 255, 0.06);
	}

	.mission-chip-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--mission-color);
		flex-shrink: 0;
	}

	.mission-chip-title {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 140px;
	}

	.mission-chip-remove {
		background: none;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		font-size: 0.85rem;
		padding: 2px 4px;
		line-height: 1;
		min-width: 28px;
		min-height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: color var(--duration-fast);
	}

	.mission-chip-remove:hover {
		color: #ef4444;
	}

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

	.assign-option:last-child {
		border-bottom: none;
	}

	.assign-empty {
		padding: 10px 12px;
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	.priority-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.missed-badge {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.75rem;
		color: #ef4444;
		font-weight: 600;
		margin-top: 3px;
	}

	.rollcall-respond-btn {
		font-size: 0.65rem;
		padding: 2px 8px;
		background: rgba(34, 197, 94, 0.15);
		border: 1px solid rgba(34, 197, 94, 0.4);
		border-radius: var(--radius-sm);
		color: #22c55e;
		cursor: pointer;
		font-weight: 600;
		transition: all var(--duration-fast);
	}

	.rollcall-respond-btn:hover {
		background: rgba(34, 197, 94, 0.25);
		border-color: #22c55e;
	}

	.device-badge {
		font-size: 0.65rem;
		font-weight: 700;
		padding: 3px 8px;
		border-radius: 3px;
		background: var(--color-primary);
		color: var(--color-text-muted);
		border: 1px solid var(--color-primary);
		cursor: pointer;
		letter-spacing: 0.03em;
		transition: all var(--duration-fast);
	}

	.device-badge:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.device-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		min-width: 0;
		overflow: hidden;
	}

	.device-chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 0.8rem;
		min-height: 36px;
	}

	.device-call {
		font-family: monospace;
		font-weight: 600;
	}

	.device-type {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		padding: 2px 6px;
		border: 1px solid var(--color-primary);
		border-radius: 3px;
	}

	.device-remove {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1rem;
		cursor: pointer;
		padding: 8px;
		line-height: 1;
		min-width: 36px;
		min-height: 36px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.device-remove:hover {
		color: #ef4444;
	}

	.device-add {
		display: flex;
		gap: var(--space-xs);
		margin-top: 4px;
	}

	.device-add-input {
		flex: 1;
		min-width: 0;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-family: monospace;
		font-size: 0.85rem;
		padding: 8px 10px;
		outline: none;
		text-transform: uppercase;
		box-sizing: border-box;
	}

	.device-add-input:focus {
		border-color: var(--color-accent);
	}

	.device-add-input::placeholder {
		text-transform: none;
		color: var(--color-text-muted);
	}

	.device-add-btn {
		width: 36px;
		height: 36px;
		background: var(--color-primary);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 1rem;
		cursor: pointer;
	}

	.device-add-btn:disabled {
		opacity: 0.4;
		cursor: default;
	}

	/* Note composer */
	.note-composer {
		padding: var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		min-width: 0;
		overflow: hidden;
	}

	.note-composer-top {
		margin: 0;
		border-radius: 0;
		border-left: none;
		border-right: none;
		border-top: none;
	}

	.note-cat-row {
		display: flex;
		gap: 4px;
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
		scrollbar-width: none;
	}

	.note-cat-row::-webkit-scrollbar { display: none; }

	.note-cat-chip {
		flex-shrink: 0;
		min-height: 36px;
		padding: 6px 12px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.note-cat-chip.active {
		background: var(--cat-color);
		border-color: var(--cat-color);
		color: #fff;
	}

	.note-textarea {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		padding: 8px 10px;
		outline: none;
		resize: vertical;
		font-family: inherit;
		width: 100%;
		box-sizing: border-box;
	}

	.note-textarea:focus {
		border-color: var(--color-accent);
	}

	.note-sev-row {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}

	.note-sev-label {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.note-sev-dot {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		border: 2px solid var(--sev-color);
		background: none;
		cursor: pointer;
		transition: all var(--duration-fast);
		padding: 0;
	}

	.note-sev-dot.active {
		background: var(--sev-color);
	}

	.note-save-btn {
		margin-left: auto;
		min-height: 36px;
		padding: 6px 16px;
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: white;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.note-save-btn:disabled {
		opacity: 0.4;
		cursor: default;
	}

	.op-action-bar {
		flex-basis: 100%;
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 0 var(--space-sm) 8px calc(4px + var(--space-sm));
		min-width: 0;
	}

	.bar-select {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.72rem;
		padding: 4px 6px;
		cursor: pointer;
		min-height: 32px;
	}

	.bar-btn {
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.72rem;
		padding: 4px 8px;
		cursor: pointer;
		min-height: 32px;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: all var(--duration-fast);
		white-space: nowrap;
	}

	.bar-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.bar-btn.active {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.bar-spacer {
		flex: 1;
	}

	/* Overflow menu */
	.overflow-wrap {
		position: relative;
	}

	.overflow-trigger {
		padding: 6px 8px;
	}

	.overflow-menu {
		position: absolute;
		right: 0;
		top: 100%;
		min-width: 160px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		z-index: 20;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
	}

	.overflow-item {
		display: block;
		width: 100%;
		padding: 10px 14px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		font-size: 0.8rem;
		text-align: left;
		cursor: pointer;
		min-height: 44px;
	}

	.overflow-item:hover {
		background: var(--color-primary);
	}

	.overflow-item:last-child {
		border-bottom: none;
	}

	.overflow-item.overflow-danger {
		color: #ef4444;
	}

	.overflow-item.overflow-danger:hover {
		background: rgba(239, 68, 68, 0.1);
	}

	/* Old op-btn kept for mission card unassign buttons */
	.op-btn {
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		padding: 4px 8px;
		cursor: pointer;
		transition: all var(--duration-fast);
		min-width: 32px;
		min-height: 32px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.op-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.op-btn.active {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	/* Mission location autocomplete */
	.mission-loc-wrap {
		position: relative;
	}

	.mission-loc-dropdown {
		position: absolute;
		top: 100%;
		left: 0;
		right: 0;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		box-shadow: var(--shadow-md);
		z-index: 10;
		max-height: 180px;
		overflow-y: auto;
		margin-top: 2px;
	}

	.mission-loc-item {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		padding: 6px 10px;
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 0.8rem;
		cursor: pointer;
		text-align: left;
		transition: background var(--duration-fast);
	}

	.mission-loc-item:hover {
		background: rgba(255, 255, 255, 0.06);
	}

	.mission-loc-name {
		flex: 1;
	}

	.mission-loc-short {
		font-family: 'SF Mono', 'Fira Code', monospace;
		font-size: 0.7rem;
		color: var(--color-text-muted);
		padding: 1px 5px;
		background: rgba(255, 255, 255, 0.06);
		border-radius: 3px;
	}

	/* Missions */
	.mission-toolbar {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.mission-filter-chips {
		display: flex;
		gap: 6px;
		margin-left: auto;
	}

	.btn-sm {
		font-size: 0.8rem;
		padding: 6px 14px;
		min-height: 36px;
	}

	.mission-form {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.mission-form input,
	.mission-form textarea,
	.mission-form select {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		padding: 10px 12px;
		outline: none;
		min-height: 44px;
	}

	.mission-form textarea {
		resize: vertical;
	}

	.mission-list {
		padding: 6px 6px 0;
		display: flex;
		flex-direction: column;
	}

	.mission-card {
		display: flex;
		flex-direction: column;
		margin-bottom: 6px;
		border-radius: 6px;
		border: 1px solid rgba(255, 255, 255, 0.06);
		transition: background var(--duration-fast), border-color var(--duration-fast), box-shadow var(--duration-fast);
		border-left: 4px solid transparent;
	}

	.mission-card:hover {
		border-color: rgba(255, 255, 255, 0.15);
		box-shadow: 0 1px 6px rgba(0, 0, 0, 0.25);
	}

	/* Priority tint backgrounds */
	.mission-card.priority-emergency {
		background: linear-gradient(to right, rgba(239, 68, 68, 0.12), rgba(239, 68, 68, 0.04));
		border-left-color: #ef4444;
	}

	.mission-card.priority-emergency:hover {
		background: linear-gradient(to right, rgba(239, 68, 68, 0.2), rgba(239, 68, 68, 0.08));
	}

	.mission-card.priority-priority {
		background: linear-gradient(to right, rgba(245, 158, 11, 0.1), rgba(245, 158, 11, 0.03));
		border-left-color: #f59e0b;
	}

	.mission-card.priority-priority:hover {
		background: linear-gradient(to right, rgba(245, 158, 11, 0.18), rgba(245, 158, 11, 0.07));
	}

	.mission-card.priority-welfare {
		background: linear-gradient(to right, rgba(59, 130, 246, 0.08), rgba(59, 130, 246, 0.02));
		border-left-color: #3b82f6;
	}

	.mission-card.priority-welfare:hover {
		background: linear-gradient(to right, rgba(59, 130, 246, 0.16), rgba(59, 130, 246, 0.06));
	}

	.mission-card.priority-routine {
		background: linear-gradient(to right, rgba(34, 197, 94, 0.06), rgba(34, 197, 94, 0.02));
		border-left-color: #22c55e;
	}

	.mission-card.priority-routine:hover {
		background: linear-gradient(to right, rgba(34, 197, 94, 0.14), rgba(34, 197, 94, 0.06));
	}

	.mission-card.complete {
		opacity: 0.45;
	}

	.mission-card.just-moved {
		animation: card-flash 2s ease-out;
	}

	@keyframes card-flash {
		0% { box-shadow: 0 0 0 2px rgba(250, 204, 21, 0.8), 0 0 12px rgba(250, 204, 21, 0.4); }
		30% { box-shadow: 0 0 0 2px rgba(250, 204, 21, 0.5), 0 0 8px rgba(250, 204, 21, 0.2); }
		100% { box-shadow: none; }
	}

	.mission-body {
		flex: 1;
		padding: 8px var(--space-sm) 4px var(--space-sm);
		min-width: 0;
	}

	/* Fly-to button in mission header */
	.fly-to-btn {
		background: none;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		padding: 4px;
		display: flex;
		align-items: center;
		flex-shrink: 0;
		transition: color var(--duration-fast);
		min-width: 28px;
		min-height: 28px;
		justify-content: center;
	}

	.fly-to-btn:hover {
		color: var(--color-accent);
	}

	.mission-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.mission-title {
		font-weight: 600;
		font-size: 0.9rem;
	}

	.mission-age {
		font-size: 0.7rem;
		font-family: monospace;
		color: var(--color-text-muted);
		margin-left: auto;
		flex-shrink: 0;
	}

	.priority-badge {
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		padding: 2px 8px;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.priority-badge.priority-routine { background: #22c55e; color: #000; }
	.priority-badge.priority-priority { background: #f59e0b; color: #000; }
	.priority-badge.priority-welfare { background: #3b82f6; color: #fff; }
	.priority-badge.priority-emergency { background: #ef4444; color: #fff; }

	.mission-desc {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		margin: 3px 0;
	}

	.mission-location {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		margin-top: 3px;
	}

	/* Assigned operators on mission card */
	.mission-operators {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		margin-top: 4px;
	}

	.mission-op-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 4px 8px;
		font-size: 0.72rem;
		min-height: 28px;
	}

	.mission-op-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.mission-op-call {
		font-family: monospace;
		font-weight: 600;
	}

	.mission-op-status {
		color: var(--color-text-muted);
		font-size: 0.65rem;
		text-transform: uppercase;
	}

	.mission-op-remove {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		padding: 2px 6px;
		cursor: pointer;
		line-height: 1;
		min-width: 28px;
		min-height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.mission-op-remove:hover {
		color: #ef4444;
	}

	.mission-assigned-text {
		font-family: monospace;
		font-size: 0.8rem;
		color: var(--color-accent);
	}

	.mission-unassigned {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-style: italic;
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

	.mission-action-bar {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 4px var(--space-sm) 8px var(--space-sm);
	}

	.mission-status-badge {
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		padding: 2px 8px;
		border-radius: 3px;
	}

	.mission-status-open {
		color: var(--color-text-muted);
	}

	.mission-status-active {
		color: #22c55e;
	}

	.mission-status-complete {
		color: #6b7280;
	}

	/* Annotation linking in mission form */
	.annotation-link-section {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.field-label-sm {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-weight: 500;
	}

	.link-count {
		color: var(--color-accent);
	}

	.annotation-chips-wrap {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		max-height: 120px;
		overflow-y: auto;
	}

	.annotation-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 6px 12px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		cursor: pointer;
		transition: all var(--duration-fast);
		min-height: 36px;
	}

	.annotation-chip:hover {
		border-color: var(--color-text-muted);
		color: var(--color-text);
	}

	.annotation-chip.selected {
		border-color: var(--color-accent);
		color: var(--color-accent);
		background: rgba(230, 57, 70, 0.08);
	}

	/* Annotation chips on mission cards */
	.mission-annotations {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin-top: var(--space-sm);
	}

	.mission-ann-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 6px 10px;
		font-size: 0.75rem;
		color: var(--color-text-muted);
		min-height: 36px;
	}

	.mission-ann-label {
		max-width: 120px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.mission-ann-remove {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 0.85rem;
		padding: 4px 8px;
		cursor: pointer;
		line-height: 1;
		min-width: 32px;
		min-height: 32px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.mission-ann-remove:hover {
		color: #ef4444;
	}

	/* Timeline */
	.timeline-filters {
		display: flex;
		gap: 6px;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-wrap: wrap;
	}

	.filter-chip {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		padding: 6px 12px;
		cursor: pointer;
		transition: all var(--duration-fast);
		min-height: 32px;
	}

	.filter-chip.active {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: white;
	}

	.timeline-search {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.75rem;
		font-family: monospace;
		padding: 6px 10px;
		outline: none;
		width: 110px;
		text-transform: uppercase;
		margin-left: auto;
		min-height: 32px;
	}

	.timeline-search:focus {
		border-color: var(--color-accent);
	}

	.timeline-search::placeholder {
		text-transform: none;
		color: var(--color-text-muted);
	}

	.timeline-feed {
		padding: 0;
	}

	.timeline-entry {
		display: flex;
		gap: var(--space-sm);
		padding: 10px 16px;
		border-bottom: 1px solid var(--color-primary);
	}

	.timeline-entry.timeline-note {
		background: rgba(255, 255, 255, 0.02);
		border-left: 3px solid var(--note-border-color, #6b7280);
	}

	.tl-icon {
		font-size: 0.85rem;
		flex-shrink: 0;
		width: 22px;
		text-align: center;
	}

	.tl-content {
		flex: 1;
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		align-items: flex-start;
		gap: var(--space-xs);
		min-width: 0;
	}

	.tl-summary {
		font-size: 0.85rem;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.tl-time {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.tl-note-header {
		width: 100%;
		margin-bottom: 2px;
	}

	.tl-note-cat {
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		padding: 2px 8px;
		border-radius: 3px;
		color: #fff;
		letter-spacing: 0.04em;
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
		gap: 4px;
	}

	.form-group label {
		font-size: 0.75rem;
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
		padding: 10px 12px;
		outline: none;
		min-height: 44px;
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
		font-size: 0.85rem;
		font-weight: 600;
		padding: 10px 20px;
		cursor: pointer;
		transition: opacity var(--duration-fast);
		min-height: 44px;
	}

	.btn-primary:hover { opacity: 0.9; }
	.btn-primary:disabled { opacity: 0.4; cursor: default; }

	.btn-secondary {
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.85rem;
		padding: 10px 20px;
		cursor: pointer;
		transition: all var(--duration-fast);
		min-height: 44px;
	}

	.btn-secondary:hover {
		border-color: var(--color-text-muted);
		color: var(--color-text);
	}

	/* --- Note preview (inline on cards) --- */
	.note-preview {
		display: flex;
		align-items: center;
		gap: 6px;
		width: 100%;
		padding: 6px 8px;
		margin-top: 4px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		cursor: pointer;
		text-align: left;
		color: var(--color-text);
		font-size: 0.8rem;
		transition: background var(--duration-fast);
		min-height: 32px;
	}

	.note-preview:hover {
		background: rgba(255, 255, 255, 0.06);
	}

	.note-preview-cat {
		font-size: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		padding: 1px 5px;
		border-radius: 3px;
		color: #fff;
		flex-shrink: 0;
		letter-spacing: 0.03em;
	}

	.note-preview-text {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text-muted);
		font-size: 0.8rem;
	}

	.note-preview-count {
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--color-accent);
		flex-shrink: 0;
	}

	.note-preview-age {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	/* --- Note history (expanded on cards) --- */
	.note-history {
		display: flex;
		flex-direction: column;
		gap: 4px;
		max-height: 200px;
		overflow-y: auto;
	}

	.note-history-item {
		padding: 8px 10px;
		background: rgba(255, 255, 255, 0.02);
		border-left: 3px solid var(--note-border-color, #6b7280);
		border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
	}

	.note-history-header {
		display: flex;
		align-items: center;
		gap: 6px;
		margin-bottom: 3px;
	}

	.note-history-cat {
		font-size: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		padding: 1px 5px;
		border-radius: 3px;
		color: #fff;
		letter-spacing: 0.03em;
	}

	.note-history-sev {
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
	}

	.note-history-author {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.note-history-time {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		margin-left: auto;
	}

	.note-history-content {
		font-size: 0.8rem;
		color: var(--color-text);
		line-height: 1.4;
		word-break: break-word;
	}

	/* --- Pinned notes banner --- */
	.pinned-banner {
		border-bottom: 1px solid var(--color-primary);
		background: rgba(239, 68, 68, 0.04);
	}

	.pinned-note {
		display: flex;
		align-items: flex-start;
		gap: 6px;
		padding: 8px var(--space-md);
		border-left: 3px solid var(--note-border-color, #ef4444);
		border-bottom: 1px solid var(--color-primary);
	}

	.pinned-note:last-child {
		border-bottom: none;
	}

	.pinned-icon {
		font-size: 0.8rem;
		flex-shrink: 0;
	}

	.pinned-cat {
		font-size: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		padding: 1px 5px;
		border-radius: 3px;
		color: #fff;
		flex-shrink: 0;
		letter-spacing: 0.03em;
	}

	.pinned-text {
		flex: 1;
		min-width: 0;
		font-size: 0.8rem;
		color: var(--color-text);
		line-height: 1.3;
		word-break: break-word;
	}

	.pinned-age {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
		margin-top: 1px;
	}

	.pinned-remove {
		flex-shrink: 0;
		background: none;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		font-size: 0.75rem;
		padding: 0 2px;
		line-height: 1;
		opacity: 0.5;
		transition: opacity 0.15s, color 0.15s;
	}

	.pinned-remove:hover {
		opacity: 1;
		color: var(--color-text);
	}

	/* --- Pinned stations strip --- */
	.pin-strip {
		display: flex;
		gap: 6px;
		padding: 6px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		background: rgba(59, 130, 246, 0.04);
		overflow-x: auto;
		flex-shrink: 0;
		scrollbar-width: thin;
	}

	.pin-chip {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 4px 8px;
		background: rgba(255, 255, 255, 0.06);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-sm);
		cursor: pointer;
		font-size: 0.75rem;
		white-space: nowrap;
		transition: background 0.15s, border-color 0.15s, opacity 0.15s;
		flex-shrink: 0;
		color: var(--color-text);
	}

	.pin-chip:hover {
		background: rgba(255, 255, 255, 0.1);
		border-color: rgba(255, 255, 255, 0.2);
	}

	.pin-chip.pin-stale {
		border-color: #f59e0b;
		opacity: 0.75;
	}

	.pin-chip.pin-urgent {
		border-color: #ef4444;
		animation: pin-pulse 1.5s ease-in-out infinite;
	}

	@keyframes pin-pulse {
		0%, 100% { box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.4); }
		50% { box-shadow: 0 0 0 4px rgba(239, 68, 68, 0); }
	}

	.pin-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.pin-call {
		font-weight: 700;
		font-size: 0.75rem;
		letter-spacing: 0.02em;
	}

	.pin-tactical {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	.pin-age {
		font-size: 0.6rem;
		color: var(--color-text-muted);
	}

	.pin-remove {
		background: none;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		font-size: 0.65rem;
		padding: 0 1px;
		line-height: 1;
		opacity: 0;
		transition: opacity 0.15s, color 0.15s;
	}

	.pin-chip:hover .pin-remove {
		opacity: 0.7;
	}

	.pin-remove:hover {
		opacity: 1 !important;
		color: #ef4444;
	}

	/* --- Category filter chips --- */
	.category-chips {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 4px var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		background: rgba(15, 52, 96, 0.15);
		flex-wrap: wrap;
		flex-shrink: 0;
	}

	.cat-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 2px 8px;
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		background: rgba(255, 255, 255, 0.05);
		color: var(--color-text-muted);
		font-size: 0.7rem;
		font-weight: 600;
		letter-spacing: 0.03em;
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.cat-chip:hover {
		background: rgba(255, 255, 255, 0.1);
		color: var(--color-text);
	}

	.cat-chip.active {
		background: color-mix(in srgb, var(--cat-chip-color, var(--color-accent)) 20%, transparent);
		border-color: var(--cat-chip-color, var(--color-accent));
		color: var(--color-text);
	}

	.cat-chip-count {
		font-size: 0.6rem;
		opacity: 0.7;
	}

	.cat-chip-clear {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 18px;
		height: 18px;
		padding: 0;
		border: none;
		border-radius: 50%;
		background: rgba(255, 255, 255, 0.1);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.cat-chip-clear:hover {
		background: var(--color-accent);
		color: var(--color-text);
	}

	/* Category badge on operator card */
	.cat-badge {
		color: #fff;
	}

</style>
