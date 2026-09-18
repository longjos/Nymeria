<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError } from '$lib/api';
	import { timeAgo } from '$lib/utils';
	import { openICS309, openSettings, missionDraftBackup } from '$lib/stores/ui';
	import { get } from 'svelte/store';
	import { canAdmin } from '$lib/stores/session';
	import type { Net, NetCheckIn, NetMission, NetEvent, NetNote, NetSummary, OperatorStatus, TrafficType, Annotation, NoteCategory, NoteSeverity, StationCategory, W3WSuggestion } from '$lib/types';
	import {
		activeNet, checkIns, missions, timeline, notes,
		sortedCheckIns, activeCheckIns,
		notesByCheckIn, notesByMission, pinnedNotes, pinnedCheckIns,
		netMetrics, categoryCounts,
		attentionItems,
		initNetControlStore, loadNetData, clearNetControl,
		opsView,
		hoveredMissionId, highlightedCheckIns,
		hoveredCheckInId,
		netAnnotations, annotationsByName, netLocationAnnotations,
		orderedCheckpoints
	} from '$lib/stores/netcontrol';
	import { annotationList } from '$lib/stores/annotations';
	import OperatorPicker from './OperatorPicker.svelte';
	import MissionPicker from './MissionPicker.svelte';
	import MissionOpChip from './MissionOpChip.svelte';
	import { categoryMeta, isTerminalStatus } from '$lib/annotationMeta';
	import { stationCategoryMeta } from '$lib/stationCategoryMeta';
	import { parseCommand, getModeIndicator, getAutocompleteContext, type ParsedCommand, type AutocompleteContext } from '$lib/commandParser';
	import { showToast } from '$lib/stores/toast';
	import { formatCoord } from '$lib/utils';
	import { nearestAnnotation, annotationCentroid, annotationPickPoint } from '$lib/geo';
	import {
		isFullAddress, looksLikePartial, formatWords, w3wLocationLabel,
		isW3WLocation, extractWords, w3wSuffix, cachedReverse, putReverse
	} from '$lib/w3w';
	import { w3wConfigured } from '$lib/stores/w3w';
	import {
		detectFormat, parsePlusCode, parseMGRS, isGeocodeError,
		formatMGRSGroups, formatPlusCode, formatMGRS,
		geocodeLocationLabel, extractGeocode,
		type GeocodeParse, type GeocodeResult, type GeocodeErrorCode, type GeocodeFormat, type RefPoint
	} from '$lib/geocodes';
	import { gpsStatus } from '$lib/stores/gps';
	import LocationManager from './LocationManager.svelte';
	import SituationBoard from './SituationBoard.svelte';
	import WxWatchAreaSheet from './WxWatchAreaSheet.svelte';
	import WxTierGlyph from './WxTierGlyph.svelte';
	import { wxActiveWarningIn, wxUnackedWarningsIn, wxIsNcs, openWxAlert, weatherCheckInIds, wxClock } from '$lib/stores/wxAlerts';
	import { clock, countdown } from '$lib/wxAlertTime';

	let {
		onFlyTo,
		onFlyToBounds,
		onSetOpsView,
		onGoToOpsView,
		onPlaceOperator,
		onPlaceAnnotation,
		annotationMapCoords = null,
		onMapCoordsConsumed,
		focusedAnnotationId = null,
		onFocusConsumed,
		onPlaceMissionLocation,
		missionMapCoords = null,
		onMissionMapCoordsConsumed,
		missionPickAnnotation = null,
		onMissionPickAnnotationConsumed,
		onClearMissionDraft,
		missionPickActive = false,
		onSetMissionDraftPoint,
		getMapCenter,
	}: {
		onFlyTo?: (lat: number, lon: number, zoom?: number) => void;
		onFlyToBounds?: (coords: Array<{ lat: number; lon: number }>) => void;
		onSetOpsView?: () => void;
		onGoToOpsView?: () => void;
		onPlaceOperator?: (ciId: string, callsign: string) => void;
		onPlaceAnnotation?: (id: string | null, name: string, mode: 'update' | 'form') => void;
		annotationMapCoords?: { lat: number; lon: number } | null;
		onMapCoordsConsumed?: () => void;
		focusedAnnotationId?: string | null;
		onFocusConsumed?: () => void;
		onPlaceMissionLocation?: (label: string) => void;
		missionMapCoords?: { lat: number; lon: number; label?: string } | null;
		onMissionMapCoordsConsumed?: () => void;
		/** A pick-mode click landed on an existing annotation — it IS the
		 * mission location (snap + link), not a dropped pin near it. */
		missionPickAnnotation?: { id: string; lat: number; lon: number } | null;
		onMissionPickAnnotationConsumed?: () => void;
		onClearMissionDraft?: () => void;
		missionPickActive?: boolean;
		/** Places the draggable draft pin on the map without routing through
		 * missionMapCoords — that path overwrites the location label with a
		 * nearby annotation's label, which a what3words resolve must never do
		 * to the words the NCS is about to read back on air. */
		onSetMissionDraftPoint?: (lat: number, lon: number) => void;
		/** Current map viewport centre, used to focus what3words autosuggest
		 * on the area the NCS is actually looking at. */
		getMapCenter?: () => { lat: number; lon: number; zoom: number } | null;
	} = $props();

	type Tab = 'situation' | 'roster' | 'missions' | 'locations' | 'timeline';
	let currentTab = $state<Tab>('situation');

	// A net-location marker click routes here (see +page.svelte
	// handleAnnotationClick) only while Net Control is already the open
	// panel; jump straight to the Locations tab so the reveal in
	// LocationManager has somewhere to scroll.
	$effect(() => {
		if (focusedAnnotationId) currentTab = 'locations';
	});

	// Metrics bar filter — clicking a metric filters the roster
	type MetricsFilter = null | 'available' | 'assigned' | 'missing' | 'stale';
	let metricsFilter = $state<MetricsFilter>(null);

	// Category filter — clicking a category chip filters the roster (AND with metricsFilter)
	let categoryFilter = $state<StationCategory | null>(null);

	// Station category metadata imported from shared module (stationCategoryMeta)

	// Create net form
	let showCreateForm = $state(false);
	let newNetName = $state('');
	let newNetType = $state('tactical');
	let newNetFreq = $state('');
	let newNetNotes = $state('');
	let creating = $state(false);

	// Command palette (enhanced quick-add)
	let quickAddInput = $state('');
	let quickAddRef = $state<HTMLInputElement>();
	let searchResults = $state<import('$lib/types').Station[]>([]);
	let searchTimeout: ReturnType<typeof setTimeout>;
	let cmdParsed = $state<ParsedCommand>({ type: 'unknown', raw: '' });
	let cmdModeLabel = $state('');
	let cmdAutocomplete = $state<AutocompleteContext | null>(null);
	let cmdAcIndex = $state(0);
	let cmdHistory = $state<string[]>(loadCmdHistory());
	let cmdHistoryPos = $state(-1);
	let cmdSavedInput = $state('');

	// Mission form
	let showMissionForm = $state(false);
	let newMissionTitle = $state('');
	let newMissionDesc = $state('');
	let newMissionPriority = $state('routine');
	// Check-in IDs, not callsigns: assignment is addressed by check-in, and the
	// mission card renders N operators, so the create form must too.
	let newMissionAssigneeIds = $state<string[]>([]);
	let titleEl = $state<HTMLInputElement>();

	// Mission location — one source of truth (design doc task #92 §3).
	// 'w3w'/'pluscode'/'mgrs' are distinct from 'map': the missionMapCoords
	// effect below overwrites the label with a nearby annotation's label,
	// which would destroy the exact words/code a resolve needs to preserve so
	// the NCS can read them back on air.
	type MissionLocSource = 'none' | 'annotation' | 'map' | 'typed' | 'coords' | 'w3w' | 'pluscode' | 'mgrs';
	let missionLocLabel = $state('');
	let missionLocLat = $state<number | null>(null);
	let missionLocLon = $state<number | null>(null);
	let missionLocSource = $state<MissionLocSource>('none');
	let missionLocNearId = $state<string | null>(null);
	let autoLinkedAnnId = $state<string | null>(null);

	// what3words — resolved words and the confirm-on-map safety gate.
	let missionLocWords = $state('');
	let missionLocNear = $state('');
	let missionLocConfirmed = $state(false);
	// Offline geocodes (#94) — normalized Plus Code / MGRS string, '' for
	// every other source. Shares missionLocConfirmed's gate with w3w.
	let missionLocCode = $state('');
	let w3wSuggestions = $state<W3WSuggestion[]>([]);
	let w3wLoading = $state(false);
	let w3wError = $state('');
	let w3wSearchedQuery = $state('');
	let w3wDebounce: ReturnType<typeof setTimeout> | null = null;
	let w3wSeq = 0;
	// Lazy reverse (coords -> ///words) shown under any chosen location.
	let reverseWords = $state('');
	let reverseCopied = $state<'plus' | 'mgrs' | 'w3w' | null>(null);

	let locOpen = $state(false);
	let locQuery = $state('');
	let locHighlight = $state(0);
	let locCoordLat = $state('');
	let locCoordLon = $state('');
	let locCoordError = $state('');
	let detailsOpen = $state(false);
	let missionSubmitting = $state(false);
	let formError = $state('');
	let srMessage = $state('');
	let pickingOnMap = $state(false);
	let locFieldTriggerEl = $state<HTMLButtonElement>();
	let locSearchEl = $state<HTMLInputElement>();
	let locPopoverEl = $state<HTMLDivElement>();
	const missionPriorities = ['routine', 'priority', 'welfare', 'emergency'] as const;
	const missionPriorityLabels: Record<(typeof missionPriorities)[number], string> = {
		routine: 'Routine',
		priority: 'Priority',
		welfare: 'Welfare',
		emergency: 'Emergency',
	};
	let priorityRefs: (HTMLButtonElement | undefined)[] = [];

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

	// Net lifecycle + overflow menus. Both popovers are position:fixed and
	// anchored from getBoundingClientRect(), because .sheet-content scrolls and
	// .side-panel clips — an absolutely-positioned menu would detach or vanish.
	let lifecycleOpen = $state(false);
	let moreOpen = $state(false);
	let showCloseDialog = $state(false);
	/** Replaces the panel content with WxWatchAreaSheet (NCS/admin, ⋯ menu). */
	let showWxWatchSheet = $state(false);
	let stateChipEl = $state<HTMLButtonElement | null>(null);
	let moreBtnEl = $state<HTMLButtonElement | null>(null);
	let popTop = $state(0);
	let popLeft = $state(0);
	let closeDialogEl = $state<HTMLDialogElement | null>(null);
	let closeCancelEl = $state<HTMLButtonElement | null>(null);
	/** Captured before the dialog mounts — showModal() moves focus, so
	    document.activeElement is no longer the invoker by the time effects run. */
	let closeDialogInvoker: HTMLElement | null = null;
	/** The active-net header — focus lands here once the net is gone. */
	let headerEl = $state<HTMLDivElement | null>(null);
	// The panel root outlives the active-net branch, so it is the only focus
	// target guaranteed to still exist after a net ends.
	let panelRootEl = $state<HTMLDivElement | null>(null);
	let closePending = $state(false);
	let closeError = $state<string | null>(null);
	/** After-action card. Component-local: NetControlPanel stays mounted when the
	    net closes, so the card renders. It is lost if the operator leaves and
	    comes back — the toast is the durable signal. */
	let closedSummary = $state<NetSummary | null>(null);
	/** Set once the dialog has decided how it closed, so focus returns correctly. */
	let closeDialogSucceeded = false;

	// Stakes shown in the close-net dialog. isTerminalStatus takes one argument.
	let openMissionCount = $derived($missions.filter((m) => m.status !== 'complete').length);
	let openLocationCount = $derived($netAnnotations.filter((a) => !isTerminalStatus(a.status)).length);

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
		ncs_transfer: '🔀',
		wx_alert: '🌩'
	};

	onMount(() => {
		// Restore a mission draft saved by openW3WSettings before Settings
		// unmounted this component (see missionDraftBackup) — scoped to the
		// net it was captured in so a draft never leaks onto a different
		// net's form. Consumed unconditionally: a mismatched or stray
		// backup (e.g. the NCS switched nets while in Settings) is dropped
		// rather than left to surprise a later mount.
		const backup = get(missionDraftBackup);
		if (backup) {
			if ($activeNet && backup.netId === $activeNet.id) {
				showMissionForm = true;
				newMissionTitle = backup.title;
				newMissionDesc = backup.desc;
				newMissionPriority = backup.priority;
				newMissionAssigneeIds = backup.assigneeIds ?? [];
				missionLocLabel = backup.locLabel;
				missionLocLat = backup.locLat;
				missionLocLon = backup.locLon;
				missionLocSource = backup.locSource as MissionLocSource;
				missionLocNearId = backup.locNearId;
				missionLocWords = backup.locWords;
				missionLocNear = backup.locNear;
				missionLocConfirmed = backup.locConfirmed;
				missionLocCode = backup.locCode;
				selectedAnnotationIds = backup.selectedAnnotationIds;
			}
			missionDraftBackup.set(null);
		}

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

	// --- Command palette helpers ---

	function loadCmdHistory(): string[] {
		try {
			const raw = sessionStorage.getItem('nymeria_cmd_history');
			return raw ? JSON.parse(raw) : [];
		} catch { return []; }
	}

	function saveCmdHistory(history: string[]) {
		try { sessionStorage.setItem('nymeria_cmd_history', JSON.stringify(history)); } catch { /* ignore */ }
	}

	function pushCmdHistory(cmd: string) {
		const trimmed = cmd.trim();
		if (!trimmed) return;
		cmdHistory = [trimmed, ...cmdHistory.filter(h => h !== trimmed)].slice(0, 20);
		saveCmdHistory(cmdHistory);
		cmdHistoryPos = -1;
	}

	function getCheckedInCallsigns(): string[] {
		return $checkIns.filter(ci => ci.status !== 'released').map(ci => ci.callsign);
	}

	function getMissionTitles(): string[] {
		return $missions.filter(m => m.status !== 'complete').map(m => m.title);
	}

	function findCheckInByCallsign(callsign: string): NetCheckIn | undefined {
		return $checkIns.find(ci => ci.callsign.toUpperCase() === callsign.toUpperCase() && ci.status !== 'released');
	}

	function findMissionByTitle(title: string): NetMission | undefined {
		const lower = title.toLowerCase();
		return $missions.find(m => m.title.toLowerCase() === lower && m.status !== 'complete')
			|| $missions.find(m => m.title.toLowerCase().includes(lower) && m.status !== 'complete');
	}

	function getAnnotationLocation(name: string): { label: string; lat: number; lon: number } | null {
		const ann = $annotationsByName.get(name.toLowerCase());
		if (!ann) return null;
		try {
			const geo = typeof ann.geometry === 'string' ? JSON.parse(ann.geometry) : ann.geometry;
			if (geo?.type === 'Point' && geo.coordinates) {
				return { label: ann.label, lat: geo.coordinates[1], lon: geo.coordinates[0] };
			}
		} catch { /* ignore */ }
		return null;
	}

	function updateParsedCommand() {
		const cis = getCheckedInCallsigns();
		cmdParsed = parseCommand(quickAddInput, cis);
		cmdModeLabel = getModeIndicator(cmdParsed);

		// Autocomplete context
		const missionTitles = getMissionTitles();
		const ctx = getAutocompleteContext(quickAddInput, cis, missionTitles);

		// Inject location annotations into location-phase suggestions
		if (ctx?.phase === 'location') {
			const partial = ctx.partial.toLowerCase();
			const annNames: string[] = [];
			for (const ann of $netAnnotations) {
				try {
					const geo = typeof ann.geometry === 'string' ? JSON.parse(ann.geometry) : ann.geometry;
					if (geo?.type === 'Point' && geo.coordinates && (geo.coordinates[0] || geo.coordinates[1])) {
						const name = ann.shortName || ann.label;
						if (!partial || name.toLowerCase().includes(partial)) {
							annNames.push(name);
						}
					}
				} catch { /* skip */ }
			}
			ctx.suggestions = annNames;
		}

		cmdAutocomplete = ctx;
		cmdAcIndex = 0;
	}

	function handleQuickAddInput() {
		updateParsedCommand();

		// Also do APRS station search for the first token (check-in autocomplete)
		clearTimeout(searchTimeout);
		const q = quickAddInput.trim().split(/\s+/)[0];
		if (q.length < 2 || quickAddInput.trim().includes(' ')) {
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
		updateParsedCommand();
		quickAddRef?.focus();
	}

	function selectAcSuggestion(suggestion: string) {
		const parts = quickAddInput.trimEnd().split(/\s+/);
		const ctx = cmdAutocomplete;
		if (!ctx) return;

		if (ctx.phase === 'callsign') {
			quickAddInput = suggestion + ' ';
		} else if (ctx.phase === 'action' || ctx.phase === 'category') {
			// Replace the last partial token
			if (ctx.partial) {
				parts[parts.length - 1] = suggestion;
			} else {
				parts.push(suggestion);
			}
			quickAddInput = parts.join(' ') + ' ';
		} else if (ctx.phase === 'mission_name' || ctx.phase === 'location') {
			// Replace everything after the keyword
			const keyword = ctx.phase === 'mission_name' ? 'assign' : 'loc';
			const kwIdx = parts.findIndex(p => p.toLowerCase() === keyword);
			if (kwIdx >= 0) {
				quickAddInput = parts.slice(0, kwIdx + 1).join(' ') + ' ' + suggestion + ' ';
			}
		}

		searchResults = [];
		updateParsedCommand();
		quickAddRef?.focus();
	}

	async function handleQuickAdd() {
		if (!$activeNet || !quickAddInput.trim()) return;

		const cis = getCheckedInCallsigns();
		const parsed = parseCommand(quickAddInput, cis);

		try {
			switch (parsed.type) {
				case 'checkin': {
					// Dedup: highlight existing operator instead of creating duplicate
					const existing = findCheckInByCallsign(parsed.callsign);
					if (existing) {
						highlightedCheckInId = existing.id;
						setTimeout(() => { highlightedCheckInId = null; }, 2000);
						pushCmdHistory(quickAddInput);
						quickAddInput = '';
						searchResults = [];
						cmdAutocomplete = null;
						cmdModeLabel = '';
						cmdParsed = { type: 'unknown', raw: '' };
						quickAddRef?.focus();
						return;
					}
					await api.checkIn($activeNet.id, parsed.callsign, parsed.traffic, parsed.category);
					const desc = [parsed.callsign, 'checked in', parsed.traffic, parsed.category].filter(Boolean).join(' ');
					showToast(desc, 'success');
					break;
				}
				case 'status': {
					const ci = findCheckInByCallsign(parsed.callsign);
					if (!ci) { showToast(`${parsed.callsign} not found in roster`, 'error'); break; }
					await api.updateCheckIn($activeNet.id, ci.id, { status: parsed.status as OperatorStatus });
					showToast(`${parsed.callsign} → ${parsed.status}`, 'success');
					break;
				}
				case 'checkout': {
					const ci = findCheckInByCallsign(parsed.callsign);
					if (!ci) { showToast(`${parsed.callsign} not found in roster`, 'error'); break; }
					await api.checkOut($activeNet.id, ci.id);
					showToast(`${parsed.callsign} checked out`, 'success');
					break;
				}
				case 'note': {
					if (!parsed.text) { showToast('Note text required', 'error'); break; }
					const ci = findCheckInByCallsign(parsed.callsign);
					const payload: { checkInId?: string; content: string; category: string; severity: string } = {
						content: parsed.text,
						category: 'general',
						severity: 'info',
					};
					if (ci) payload.checkInId = ci.id;
					await api.addNetNote($activeNet.id, payload);
					showToast(`Note added to ${parsed.callsign}`, 'success');
					break;
				}
				case 'mission_create': {
					await api.createMission($activeNet.id, { title: parsed.title, priority: 'routine' });
					showToast(`Mission created: ${parsed.title}`, 'success');
					break;
				}
				case 'mission_assign': {
					const ci = findCheckInByCallsign(parsed.callsign);
					if (!ci) { showToast(`${parsed.callsign} not found in roster`, 'error'); break; }
					const mission = findMissionByTitle(parsed.missionTitle);
					if (!mission) { showToast(`Mission "${parsed.missionTitle}" not found`, 'error'); break; }
					await api.assignMission($activeNet.id, ci.id, mission.id);
					showToast(`${parsed.callsign} assigned to ${mission.title}`, 'success');
					break;
				}
				case 'location': {
					const ci = findCheckInByCallsign(parsed.callsign);
					if (!ci) { showToast(`${parsed.callsign} not found in roster`, 'error'); break; }
					if (parsed.lat != null && parsed.lon != null) {
						await api.updateCheckIn($activeNet.id, ci.id, { lat: parsed.lat, lon: parsed.lon } as any);
						showToast(`${parsed.callsign} location set`, 'success');
					} else if (parsed.locationName) {
						const loc = getAnnotationLocation(parsed.locationName);
						if (loc) {
							await api.updateCheckIn($activeNet.id, ci.id, { location: loc.label, lat: loc.lat, lon: loc.lon } as any);
							showToast(`${parsed.callsign} → ${loc.label}`, 'success');
						} else {
							await api.updateCheckIn($activeNet.id, ci.id, { location: parsed.locationName } as any);
							showToast(`${parsed.callsign} location: ${parsed.locationName}`, 'success');
						}
					} else {
						showToast('Location required: name or lat lon', 'error');
						break;
					}
					break;
				}
				case 'checkpoint_passage': {
					const seqNum = parseInt(parsed.checkpointRef, 10);
					const cp = $orderedCheckpoints.find(c => c.meta.sequenceNumber === seqNum);
					if (!cp) { showToast(`Checkpoint #${seqNum} not found`, 'error'); break; }
					await api.logPassage($activeNet.id, cp.meta.annotationId, { label: parsed.label });
					showToast(`${parsed.label} passed CP${seqNum} (${cp.annotation.label})`, 'success');
					break;
				}
				case 'unknown':
					showToast('Unrecognized command', 'error');
					quickAddRef?.focus();
					return;
			}
			pushCmdHistory(quickAddInput);
			quickAddInput = '';
			searchResults = [];
			cmdAutocomplete = null;
			cmdModeLabel = '';
			cmdParsed = { type: 'unknown', raw: '' };
		} catch (e: any) {
			showToast(e?.message || 'Command failed', 'error');
		}

		quickAddRef?.focus();
	}

	function handleCmdKeydown(e: KeyboardEvent) {
		// Tab completion
		if (e.key === 'Tab' && cmdAutocomplete?.suggestions.length) {
			e.preventDefault();
			selectAcSuggestion(cmdAutocomplete.suggestions[cmdAcIndex]);
			return;
		}

		// Enter executes
		if (e.key === 'Enter') {
			// If autocomplete is showing and a suggestion is highlighted, complete it first
			if (cmdAutocomplete?.suggestions.length && cmdAcIndex >= 0) {
				// If user is mid-word on a callsign/action, complete; otherwise execute
				const ctx = cmdAutocomplete;
				if (ctx.partial && ctx.suggestions.length > 0 && ctx.phase !== 'note_text' && ctx.phase !== 'mission_title') {
					e.preventDefault();
					selectAcSuggestion(ctx.suggestions[cmdAcIndex]);
					return;
				}
			}
			e.preventDefault();
			handleQuickAdd();
			return;
		}

		// Arrow keys for autocomplete navigation
		if (e.key === 'ArrowDown') {
			if (cmdAutocomplete?.suggestions.length) {
				e.preventDefault();
				cmdAcIndex = Math.min(cmdAcIndex + 1, cmdAutocomplete.suggestions.length - 1);
				return;
			}
			// Command history (down = newer)
			if (cmdHistoryPos > 0) {
				e.preventDefault();
				cmdHistoryPos--;
				quickAddInput = cmdHistory[cmdHistoryPos];
				updateParsedCommand();
				return;
			} else if (cmdHistoryPos === 0) {
				e.preventDefault();
				cmdHistoryPos = -1;
				quickAddInput = cmdSavedInput;
				updateParsedCommand();
				return;
			}
			return;
		}

		if (e.key === 'ArrowUp') {
			if (cmdAutocomplete?.suggestions.length) {
				e.preventDefault();
				cmdAcIndex = Math.max(cmdAcIndex - 1, 0);
				return;
			}
			// Command history (up = older)
			if (cmdHistory.length > 0 && cmdHistoryPos < cmdHistory.length - 1) {
				e.preventDefault();
				if (cmdHistoryPos === -1) cmdSavedInput = quickAddInput;
				cmdHistoryPos++;
				quickAddInput = cmdHistory[cmdHistoryPos];
				updateParsedCommand();
				return;
			}
			return;
		}

		// Escape clears
		if (e.key === 'Escape') {
			e.preventDefault();
			quickAddInput = '';
			searchResults = [];
			cmdAutocomplete = null;
			cmdModeLabel = '';
			cmdParsed = { type: 'unknown', raw: '' };
			cmdHistoryPos = -1;
			quickAddRef?.focus();
			return;
		}
	}

	/** Dynamic hint text based on parse state */
	function getCmdHint(parsed: ParsedCommand, ac: AutocompleteContext | null): string {
		if (!quickAddInput.trim()) return '';
		if (parsed.type === 'unknown' && quickAddInput.trim().length > 0) {
			const first = quickAddInput.trim().split(/\s+/)[0].toLowerCase();
			if (first === 'mission') return 'Type mission title and press Enter';
			return '';
		}
		if (ac?.phase === 'action') return 'status \u00b7 note \u00b7 assign \u00b7 loc \u00b7 out';
		if (ac?.phase === 'note_text') return 'Type note text and press Enter';
		if (ac?.phase === 'mission_title') return 'Type mission title and press Enter';
		if (ac?.phase === 'mission_name') return 'Type or select mission name';
		if (ac?.phase === 'location') return 'Annotation name or lat lon';
		if (ac?.phase === 'category') return 'Station category';
		return '';
	}

	const cmdModeColors: Record<string, string> = {
		checkin: '#22c55e',
		status: '#3b82f6',
		checkout: '#6b7280',
		note: '#f59e0b',
		mission_create: '#8b5cf6',
		mission_assign: '#8b5cf6',
		location: '#06b6d4',
	};

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

	function requestCloseNet() {
		if (!$activeNet) return;
		closeError = null;
		closeDialogInvoker = document.activeElement as HTMLElement | null;
		closePopovers();
		showCloseDialog = true;
	}

	function finishCloseDialog(succeeded: boolean) {
		closeDialogSucceeded = succeeded;
		showCloseDialog = false;
	}

	async function confirmCloseNet() {
		if (!$activeNet || closePending) return;
		closePending = true;
		closeError = null;
		try {
			const { summary } = await api.closeNet($activeNet.id);
			closedSummary = summary;
			clearNetControl();
			showToast(
				`Net closed — ${summary.duration}, ${summary.totalCheckIns} check-ins, ${summary.totalMissions} missions.`,
				'info',
				6000
			);
			finishCloseDialog(true);
		} catch (e) {
			closeError = e instanceof ApiError ? e.message : 'Close net failed. The net is still open.';
			console.error('Close net failed:', e);
		} finally {
			closePending = false;
		}
	}

	// --- Popovers -------------------------------------------------------------

	/**
	 * Promote a fixed-position overlay into the top layer.
	 *
	 * Every shell this panel renders in is transformed — SidePanel is
	 * `translateX(0)` when open, BottomSheet carries an inline `translateY` —
	 * and any transform other than `none` makes that ancestor the containing
	 * block for `position: fixed` descendants. Coordinates from
	 * getBoundingClientRect() are viewport coordinates, so the menus landed
	 * hundreds of pixels away (measured: a menu anchored to a chip at x=856
	 * rendered at x=1609 in a 1280px viewport, entirely off-screen).
	 *
	 * The top layer has no containing-block ancestor, so `fixed` resolves
	 * against the viewport again. The attribute is set here rather than in the
	 * markup deliberately: `[popover]` is `display: none` until shown, so on an
	 * engine without the API the element must never receive the attribute at
	 * all, or it would vanish instead of merely being mispositioned.
	 */
	function topLayer(node: HTMLElement) {
		if (typeof node.showPopover !== 'function') return;
		try {
			node.setAttribute('popover', 'manual');
			node.showPopover();
		} catch {
			node.removeAttribute('popover');
		}
		return {
			destroy() {
				try {
					if (node.isConnected) node.hidePopover();
				} catch {
					/* already closed by the engine */
				}
			}
		};
	}

	function openPopover(anchor: HTMLElement | null, align: 'left' | 'right') {
		if (!anchor) return;
		const r = anchor.getBoundingClientRect();
		popTop = r.bottom + 6;
		popLeft = align === 'left' ? r.left : Math.max(8, r.right - 240);
	}

	function closePopovers() {
		lifecycleOpen = false;
		moreOpen = false;
	}

	// Dismiss an open popover on an outside press, or whenever the anchor could
	// have moved out from under it.
	$effect(() => {
		if (!lifecycleOpen && !moreOpen) return;
		const onDown = (e: PointerEvent) => {
			const t = e.target as HTMLElement;
			if (
				t.closest('.pop-menu') ||
				t === stateChipEl ||
				t === moreBtnEl ||
				t.closest('.net-state-chip') ||
				t.closest('.more-btn')
			) return;
			closePopovers();
		};
		const onScrollOrResize = () => closePopovers();
		window.addEventListener('pointerdown', onDown, true);
		window.addEventListener('scroll', onScrollOrResize, true);
		window.addEventListener('resize', onScrollOrResize);
		return () => {
			window.removeEventListener('pointerdown', onDown, true);
			window.removeEventListener('scroll', onScrollOrResize, true);
			window.removeEventListener('resize', onScrollOrResize);
		};
	});

	/**
	 * Open a <dialog> modally.
	 *
	 * showModal() is what buys the behaviour this panel used to hand-roll and
	 * got wrong: everything outside the dialog goes inert (the old loop walked
	 * document.body.children and skipped any node containing the dialog — which
	 * is the single SvelteKit app root, so nothing was ever made inert), Tab is
	 * trapped, background scroll is suppressed, and Escape is handled by the UA.
	 */
	function modalDialog(node: HTMLDialogElement) {
		node.showModal();
		return {
			destroy() {
				if (node.open) node.close();
			}
		};
	}

	/**
	 * The WAI-ARIA menu keyboard contract for a `.pop-menu`, which declares
	 * role="menu": focus lands on the first item when the menu opens, Up/Down
	 * move between items and wrap, Home/End jump to the ends, and only the
	 * active item is tabbable (roving tabindex) so Tab leaves the menu rather
	 * than walking it. Escape and outside-press dismissal stay with the panel's
	 * own handlers, which also return focus to the invoking control.
	 */
	function menuNav(node: HTMLElement) {
		const items = () =>
			Array.from(node.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])'));

		function activate(index: number) {
			const list = items();
			if (list.length === 0) return;
			const i = ((index % list.length) + list.length) % list.length;
			list.forEach((el, n) => el.setAttribute('tabindex', n === i ? '0' : '-1'));
			list[i].focus();
		}

		function onKeydown(e: KeyboardEvent) {
			const list = items();
			if (list.length === 0) return;
			const cur = list.indexOf(document.activeElement as HTMLElement);
			switch (e.key) {
				case 'ArrowDown':
					e.preventDefault();
					activate(cur + 1);
					break;
				case 'ArrowUp':
					// From outside the item list, Up enters at the end.
					e.preventDefault();
					activate(cur < 0 ? -1 : cur - 1);
					break;
				case 'Home':
					e.preventDefault();
					activate(0);
					break;
				case 'End':
					e.preventDefault();
					activate(list.length - 1);
					break;
			}
		}

		node.addEventListener('keydown', onKeydown);
		activate(0);
		return {
			destroy() {
				node.removeEventListener('keydown', onKeydown);
			}
		};
	}

	function handleWindowKeydown(e: KeyboardEvent) {
		// The close-net dialog is a native modal: the UA closes it on Escape and
		// traps Tab inside it, so there is nothing to do here.
		if (showCloseDialog) return;
		if ((lifecycleOpen || moreOpen) && e.key === 'Escape') {
			e.preventDefault();
			const back = lifecycleOpen ? stateChipEl : moreBtnEl;
			closePopovers();
			back?.focus();
		}
	}

	// Set initial focus and restore it on close. Inertness, the Tab trap, the
	// scroll lock and Escape all belong to showModal() now (see modalDialog).
	$effect(() => {
		if (!closeDialogEl) return;

		// Cancel, never the danger button, takes initial focus.
		closeCancelEl?.focus();

		return () => {
			// On success the invoking row, the chip and the whole active-net
			// header unmount with the net, so headerEl and the invoker are both
			// detached by now; without the panel-root fallback focus would drop
			// to <body> and a keyboard operator would lose their place.
			const target =
				closeDialogSucceeded && headerEl?.isConnected ? headerEl : closeDialogInvoker;
			if (target?.isConnected) target.focus();
			else panelRootEl?.focus();
			closeDialogSucceeded = false;
			closeDialogInvoker = null;
		};
	});

	// Open the form — focuses the title input once it mounts.
	$effect(() => {
		if (showMissionForm) {
			queueMicrotask(() => titleEl?.focus());
		}
	});

	// Consume map-clicked coordinates into the location field (mirrors
	// LocationManager's mapClickedCoords effect).
	$effect(() => {
		if (!missionMapCoords) return;
		const { lat, lon } = missionMapCoords;
		const near = nearestAnnotation(lat, lon, locOptions, 100);
		unlinkStaleAutoLink();
		missionLocLat = lat;
		missionLocLon = lon;
		missionLocSource = 'map';
		missionLocNearId = near?.annotation.id ?? null;
		// An explicit label (a station/operator/mission marker the pick
		// landed on) wins over a merely-nearby annotation guess; a plain
		// background click behaves exactly as before.
		missionLocLabel = missionMapCoords.label ?? near?.annotation.label ?? '';
		// Dragging (or re-clicking) away from a what3words square / geocode
		// cell means the NCS is overriding it — the words/code are gone
		// because the pin is no longer in that square, which is correct and
		// honest.
		missionLocWords = '';
		missionLocNear = '';
		missionLocConfirmed = false;
		missionLocCode = '';
		srMessage = missionMapCoords.label
			? `Location set to ${missionMapCoords.label}`
			: near ? `Location set near ${near.annotation.label}` : `Location set to ${formatCoord(lat, lon)}`;
		pickingOnMap = false;
		onMissionMapCoordsConsumed?.();
	});

	// A pick-mode click landed on an existing annotation layer — snap the
	// location to it and, when it's a net-scoped, non-terminal location
	// option, link it to the mission (mirrors selectLocationAnnotation's own
	// list-pick behaviour). An annotation from another net (or otherwise not
	// in locOptions) still becomes the location text + coordinates, just
	// without being silently attached to this net's mission.
	$effect(() => {
		const pick = missionPickAnnotation;
		if (!pick) return;
		const a = $annotationList.find((x) => x.id === pick.id);
		if (a) {
			showMissionForm = true; // defensive; the form is already open
			selectLocationAnnotation(a, { lat: pick.lat, lon: pick.lon });
		}
		pickingOnMap = false;
		onMissionPickAnnotationConsumed?.();
	});

	// The map side cancelled (Esc) without ever sending coordinates back —
	// drop the panel's "picking" state so the trigger reverts.
	$effect(() => {
		if (!missionPickActive && pickingOnMap) {
			pickingOnMap = false;
		}
	});

	// Removes whatever annotation was auto-linked by a *previous* location
	// selection before a new selection overwrites autoLinkedAnnId — otherwise
	// switching location twice leaves the first pick's annotation linked to
	// the mission forever, invisibly (it no longer matches the location
	// chip, so nothing in the UI hints it's still selected).
	function unlinkStaleAutoLink() {
		if (autoLinkedAnnId && selectedAnnotationIds.includes(autoLinkedAnnId)) {
			selectedAnnotationIds = selectedAnnotationIds.filter((id) => id !== autoLinkedAnnId);
		}
		autoLinkedAnnId = null;
	}

	// fallback (the exact point clicked, when there was one) is used only for
	// LineString/Polygon geometry — clicking a spot on a route/area places
	// the mission there rather than at the route's midpoint. Point geometry
	// always wins over the fallback (see annotationPickPoint).
	function selectLocationAnnotation(a: Annotation, fallback?: { lat: number; lon: number }) {
		unlinkStaleAutoLink();
		const p = annotationPickPoint(a, fallback);
		if (p) {
			missionLocLat = p.lat;
			missionLocLon = p.lon;
		}
		missionLocLabel = a.label;
		missionLocSource = 'annotation';
		missionLocNearId = a.id;
		// A resolved geocode/w3w selection preceding this pick must not leave
		// stale words/code behind once the location has moved to an
		// annotation — same honesty rule as the missionMapCoords effect.
		missionLocWords = '';
		missionLocNear = '';
		missionLocCode = '';
		missionLocConfirmed = false;
		// Only link annotations that are actually valid location options for
		// this net (net-scoped, non-terminal) — an annotation picked off the
		// map that belongs to another net (or is terminal) still becomes the
		// mission's location text + coordinates, but is never silently
		// attached to this net's mission.
		if (locOptions.some((o) => o.id === a.id)) {
			if (!selectedAnnotationIds.includes(a.id)) {
				selectedAnnotationIds = [...selectedAnnotationIds, a.id];
			}
			autoLinkedAnnId = a.id;
		}
		locOpen = false;
		locQuery = '';
		srMessage = `Location set to ${a.label}`;
		queueMicrotask(() => locFieldTriggerEl?.focus());
	}

	function useTypedLocation() {
		const q = locQuery.trim();
		if (!q) return;
		unlinkStaleAutoLink();
		missionLocLabel = q;
		missionLocLat = null;
		missionLocLon = null;
		missionLocSource = 'typed';
		missionLocNearId = null;
		locOpen = false;
		locQuery = '';
		srMessage = `Location set to ${q}`;
		queueMicrotask(() => locFieldTriggerEl?.focus());
	}

	function useTypedCoords() {
		const lat = parseFloat(locCoordLat);
		const lon = parseFloat(locCoordLon);
		if (isNaN(lat) || lat < -90 || lat > 90) {
			locCoordError = 'Latitude must be −90 to 90';
			return;
		}
		if (isNaN(lon) || lon < -180 || lon > 180) {
			locCoordError = 'Longitude must be −180 to 180';
			return;
		}
		locCoordError = '';
		unlinkStaleAutoLink();
		missionLocLat = lat;
		missionLocLon = lon;
		missionLocSource = 'coords';
		missionLocNearId = null;
		locOpen = false;
		locCoordLat = '';
		locCoordLon = '';
		srMessage = missionLocLabel ? `Location set to ${missionLocLabel}` : `Location set to ${formatCoord(lat, lon)}`;
		queueMicrotask(() => locFieldTriggerEl?.focus());
	}

	function clearLocation() {
		missionLocLabel = '';
		missionLocLat = null;
		missionLocLon = null;
		missionLocSource = 'none';
		missionLocNearId = null;
		missionLocWords = '';
		missionLocNear = '';
		missionLocConfirmed = false;
		missionLocCode = '';
		unlinkStaleAutoLink();
		onClearMissionDraft?.();
		srMessage = 'Location cleared.';
		queueMicrotask(() => locFieldTriggerEl?.focus());
	}

	function startMapPick() {
		locOpen = false;
		locQuery = '';
		pickingOnMap = true;
		srMessage = 'Picking location on the map. Press Escape to cancel.';
		onPlaceMissionLocation?.(newMissionTitle.trim() || 'Mission location');
	}

	function onLocTriggerClick() {
		if (pickingOnMap) return;
		openLocPopover();
	}

	function openLocPopover() {
		// Clicking the chip body is forgiving for an unconfirmed resolve
		// (w3w/Plus Code/MGRS) — it re-flies to the pin (the confirmation
		// check) without auto-confirming.
		if (locNeedsConfirm && missionLocLat != null && missionLocLon != null) {
			flyToLocationPin(missionLocLat, missionLocLon);
		}
		locOpen = true;
		locQuery = missionLocSource !== 'none' ? missionLocLabel : '';
		locHighlight = 0;
		resetW3WSuggestState();
		queueMicrotask(() => {
			locSearchEl?.focus();
			locSearchEl?.select();
		});
	}

	// Closing on blur must not fire while focus is only moving to another
	// control inside the same popover (e.g. into the Coordinates fields).
	function onLocSearchBlur(e: FocusEvent) {
		const next = e.relatedTarget as Node | null;
		if (next && locPopoverEl?.contains(next)) return;
		setTimeout(() => { locOpen = false; }, 150);
	}

	// --- what3words -------------------------------------------------------

	function isMobileViewport(): boolean {
		return typeof window !== 'undefined' && window.matchMedia('(max-width: 768px)').matches;
	}

	// The panel lives in the bottom sheet on mobile — flying straight to the
	// pin's coordinates would centre it right where the sheet covers it.
	// Nudge the fly-to target north so the pin lands in the upper third of
	// the visible map instead (design §6.11's "ship this first" 5-liner).
	function latOffsetForSheet(lat: number, zoom: number): number {
		const metersPerPixel = (156543.03392 * Math.cos((lat * Math.PI) / 180)) / Math.pow(2, zoom);
		const pixelShift = 140;
		return (metersPerPixel * pixelShift) / 111320;
	}

	function flyToLocationPin(lat: number, lon: number) {
		const zoom = 16;
		const flyLat = isMobileViewport() ? lat + latOffsetForSheet(lat, zoom) : lat;
		onFlyTo?.(flyLat, lon, zoom);
	}

	function copyForW3WError(e: unknown): string {
		const code = e instanceof ApiError ? (e.body?.code as string | undefined) : undefined;
		switch (code) {
			case 'bad_words':
			case 'bad_request':
				return 'That isn’t a valid three-word address — check the spelling.';
			case 'quota_exceeded':
			case 'rate_limited':
				return 'what3words quota reached. Try again later, or use coordinates.';
			case 'invalid_key':
				return 'what3words rejected the API key. Check it in Settings.';
			case 'not_configured':
				return 'what3words isn’t configured. Add an API key in Settings.';
			default:
				return 'Couldn’t reach what3words. Check the connection, or use coordinates.';
		}
	}

	function resetW3WSuggestState() {
		if (w3wDebounce) {
			clearTimeout(w3wDebounce);
			w3wDebounce = null;
		}
		w3wSeq++;
		w3wSuggestions = [];
		w3wError = '';
		w3wSearchedQuery = '';
		w3wLoading = false;
	}

	// Fires on every keystroke while the query is what3words-shaped. Local
	// regex gate first (§9's quota guard) — a syntactically invalid or
	// too-short partial never reaches the network.
	function scheduleW3WSuggest() {
		if (w3wDebounce) {
			clearTimeout(w3wDebounce);
			w3wDebounce = null;
		}
		w3wSeq++;
		w3wSuggestions = [];
		w3wError = '';
		w3wSearchedQuery = '';
		if (!$w3wConfigured) {
			w3wLoading = false;
			return;
		}
		const q = locQuery.trim();
		if (!looksLikePartial(q) && !isFullAddress(q)) {
			w3wLoading = false;
			return;
		}
		const dotIdx = q.indexOf('.');
		if (dotIdx === -1 || q.slice(dotIdx + 1).length < 2) {
			w3wLoading = false;
			return;
		}
		w3wLoading = true; // optimistic — covers the debounce window too
		const seq = w3wSeq;
		w3wDebounce = setTimeout(() => { void runW3WSuggest(q, seq); }, 250);
	}

	async function runW3WSuggest(q: string, seq: number) {
		try {
			const center = getMapCenter?.() ?? null;
			const centroid = center ? null : annotationCentroid(locOptions);
			const focus = center ? { lat: center.lat, lon: center.lon } : (centroid ?? undefined);
			const resp = await api.w3wSuggest(q, focus);
			if (seq !== w3wSeq) return; // stale — a later keystroke has already superseded this
			w3wSuggestions = resp.suggestions ?? [];
			w3wSearchedQuery = q;
		} catch (e) {
			if (seq !== w3wSeq) return;
			w3wSuggestions = [];
			w3wError = copyForW3WError(e);
		} finally {
			if (seq === w3wSeq) w3wLoading = false;
		}
	}

	// The heart of the feature: resolve → draft pin → fly-to → unconfirmed
	// chip. The map fly-to is the safety check, not a nicety — a mis-heard
	// word lands you in another country, and this is how the NCS catches it.
	//
	// Guarded by the same w3wSeq generation counter runW3WSuggest uses: two
	// resolves can race (e.g. two suggestion rows triggered before either
	// response returns, or Enter on one address followed by Enter on a
	// retyped one before the first replies) and network order is not call
	// order, so an older response must never clobber a newer selection —
	// that would silently move the pin without the NCS asking for it.
	async function resolveW3W(words: string) {
		w3wLoading = true;
		w3wError = '';
		const seq = ++w3wSeq;
		try {
			const r = await api.w3wResolve(words);
			if (seq !== w3wSeq) return; // stale — a newer resolve/query has superseded this
			unlinkStaleAutoLink();
			const near = nearestAnnotation(r.lat, r.lon, locOptions, 100);
			const nearText = near?.annotation.label ?? r.nearestPlace ?? '';
			missionLocLat = r.lat;
			missionLocLon = r.lon;
			missionLocWords = r.words;
			missionLocNear = nearText;
			missionLocSource = 'w3w';
			missionLocCode = ''; // not a geocode selection
			missionLocNearId = near?.annotation.id ?? null;
			missionLocLabel = w3wLocationLabel(r.words, nearText || undefined);
			missionLocConfirmed = false;
			putReverse(r.lat, r.lon, r.words); // seed the reverse cache — free
			onSetMissionDraftPoint?.(r.lat, r.lon);
			flyToLocationPin(r.lat, r.lon);
			locOpen = false;
			locQuery = '';
			resetW3WSuggestState();
			srMessage = `Location set to ${formatWords(r.words)}${nearText ? `, near ${nearText}` : ''}. Shown on the map — confirm before creating.`;
			queueMicrotask(() => locFieldTriggerEl?.focus());
		} catch (e) {
			if (seq !== w3wSeq) return;
			w3wError = copyForW3WError(e);
		} finally {
			if (seq === w3wSeq) w3wLoading = false;
		}
	}

	function confirmLocation() {
		if (missionLocLat == null || missionLocLon == null) return;
		flyToLocationPin(missionLocLat, missionLocLon);
		missionLocConfirmed = true;
		srMessage = `Confirmed ${locChipPrimary} on the map.`;
	}

	function openW3WSettings() {
		// Settings and Net Control are mutually exclusive panels — opening
		// Settings unmounts this component and, with it, every mission-draft
		// field below (plain $state, no other persistence). Snapshot the
		// draft so onMount can restore it when the NCS comes back instead of
		// losing their in-progress mission over an API key paste.
		if (showMissionForm && $activeNet) {
			missionDraftBackup.set({
				netId: $activeNet.id,
				title: newMissionTitle,
				desc: newMissionDesc,
				priority: newMissionPriority,
				assigneeIds: [...newMissionAssigneeIds],
				locLabel: missionLocLabel,
				locLat: missionLocLat,
				locLon: missionLocLon,
				locSource: missionLocSource,
				locNearId: missionLocNearId,
				locWords: missionLocWords,
				locNear: missionLocNear,
				locConfirmed: missionLocConfirmed,
				locCode: missionLocCode,
				selectedAnnotationIds: [...selectedAnnotationIds],
			});
		}
		openSettings('what3words');
	}

	/** Shared copy handler for the reverse-formats row (§4) — one clipboard
	 * path for Plus Code / MGRS / what3words, "Copied" feedback keyed by
	 * which token was copied. Clipboard may be unavailable (permissions,
	 * insecure context) — this is a convenience, not worth an error row. */
	async function copyToken(kind: 'plus' | 'mgrs' | 'w3w', text: string) {
		if (!text) return;
		try {
			await navigator.clipboard?.writeText(text);
			reverseCopied = kind;
			setTimeout(() => { if (reverseCopied === kind) reverseCopied = null; }, 1500);
		} catch {
			// silent — see comment above
		}
	}

	// --- Offline geocodes (Plus Code / MGRS) — #94 -------------------------

	// Reference point for recovering a short Plus Code, and for shortening
	// the Plus Code shown in the reverse row — same order both places so the
	// code an operator reads out matches the code they'd type back in.
	// Captured fresh on every call; never stored, so a resolved code is
	// always a fixed lat/lon regardless of where the map pans afterward.
	function currentRefPoint(): RefPoint | null {
		// 1. Map centre — what the NCS is actually looking at. Same source the
		//    w3w autosuggest focus already uses.
		const c = getMapCenter?.();
		if (c) return { lat: c.lat, lon: c.lon };
		// 2. Net location centroid — the event's own footprint.
		const centroid = annotationCentroid(locOptions);
		if (centroid) return centroid;
		// 3. Our own GPS fix, when it's a real, non-stale one.
		const status = get(gpsStatus);
		if (status.fix && status.fix.mode >= 2 && !status.stale) {
			return { lat: status.fix.lat, lon: status.fix.lon };
		}
		return null;
	}

	// resolveW3W's success branch with the await removed — offline codes
	// resolve synchronously, but everything downstream (draft pin, fly-to,
	// unconfirmed chip, srMessage) is identical so the NCS gets the same
	// safety check regardless of which format they typed.
	function useGeocode(g: GeocodeResult) {
		unlinkStaleAutoLink();
		const near = nearestAnnotation(g.lat, g.lon, locOptions, 100);
		const nearText = near?.annotation.label ?? '';
		missionLocLat = g.lat;
		missionLocLon = g.lon;
		missionLocCode = g.label;
		missionLocWords = ''; // not a w3w selection
		missionLocNear = nearText;
		missionLocSource = g.format;
		missionLocNearId = near?.annotation.id ?? null;
		missionLocLabel = geocodeLocationLabel(
			g.format === 'mgrs' ? formatMGRSGroups(g.label) : g.label,
			nearText || undefined
		);
		missionLocConfirmed = false;
		onSetMissionDraftPoint?.(g.lat, g.lon);
		flyToLocationPin(g.lat, g.lon);
		locOpen = false;
		locQuery = '';
		resetW3WSuggestState();
		const formatName = g.format === 'mgrs' ? 'MGRS ' : 'Plus Code ';
		srMessage = `Location set to ${formatName}${missionLocLabel}. Shown on the map — confirm before creating.`;
		queueMicrotask(() => locFieldTriggerEl?.focus());
	}

	/** Pull the "near X" fragment out of a "<code> · near X" stored location
	 * string — same separator as w3wSuffix, generalized for the mission-card
	 * geocode treatment since Plus Code/MGRS labels don't start with '///'. */
	function geocodeNearSuffix(location: string): string | null {
		const marker = ' · near ';
		const idx = location.indexOf(marker);
		if (idx === -1) return null;
		const suffix = location.slice(idx + marker.length).trim();
		return suffix || null;
	}

	// Lazily resolve the ///words for whatever location is currently chosen,
	// module cache first. Silent on failure — a reverse lookup is a
	// convenience; failing loudly would be noise during an incident.
	$effect(() => {
		const lat = missionLocLat;
		const lon = missionLocLon;
		reverseWords = '';
		if (!$w3wConfigured || lat == null || lon == null) return;
		// Already showing the words as the primary label — no duplicate line.
		if (missionLocSource === 'w3w') return;
		const cached = cachedReverse(lat, lon);
		if (cached) {
			reverseWords = cached;
			return;
		}
		let cancelled = false;
		api.w3wReverse(lat, lon).then((r) => {
			if (cancelled) return;
			putReverse(lat, lon, r.words);
			reverseWords = r.words;
		}).catch(() => { /* silent — see comment above */ });
		return () => { cancelled = true; };
	});

	// Plus Code / MGRS reverse tokens — synchronous, no effect, no loading
	// state: the whole point of #94 is that these never touch the network.
	// Self-suppressed against the current source, same rule as the w3w leg.
	let reversePlusCode = $derived.by(() => {
		if (missionLocSource === 'pluscode' || missionLocLat == null || missionLocLon == null) return null;
		return formatPlusCode(missionLocLat, missionLocLon, currentRefPoint());
	});
	let reverseMGRS = $derived.by(() => {
		if (missionLocSource === 'mgrs' || missionLocLat == null || missionLocLon == null) return '';
		return formatMGRS(missionLocLat, missionLocLon, 4);
	});

	function commitLocRow(i: number) {
		if (i === 0) {
			// Enter on a complete code/address commits it directly instead of
			// requiring an arrow-down into the suggestion list first.
			if (locGeocode && !isGeocodeError(locGeocode)) {
				useGeocode(locGeocode);
				return;
			}
			if ($w3wConfigured && locIsFullAddress) {
				resolveW3W(locQuery.trim());
				return;
			}
			startMapPick();
			return;
		}
		if (i === 1 && geocodeRowCount === 1 && locGeocode && !isGeocodeError(locGeocode)) {
			useGeocode(locGeocode);
			return;
		}
		if (i >= 1 + geocodeRowCount && i < 1 + geocodeRowCount + w3wRowCount) {
			resolveW3W(w3wSuggestions[i - 1 - geocodeRowCount].words);
			return;
		}
		const annIdx = i - 1 - geocodeRowCount - w3wRowCount;
		if (annIdx >= 0 && annIdx < locFiltered.length) {
			selectLocationAnnotation(locFiltered[annIdx]);
			return;
		}
		if (showFreeTextRow) useTypedLocation();
	}

	// Keep the highlighted row in range as the filtered list shrinks.
	$effect(() => {
		if (locHighlight > locRowCount - 1) locHighlight = Math.max(0, locRowCount - 1);
	});

	// Jump the highlight onto a freshly-detected geocode resolve row so Enter
	// is unambiguous — but only on the actual 0->1 transition (tracked below),
	// so it never fights an operator who has arrowed back to row 0 on purpose
	// while a code is still showing.
	let prevGeocodeRowCount = 0;
	$effect(() => {
		const cur = geocodeRowCount;
		if (cur === 1 && prevGeocodeRowCount === 0 && locHighlight === 0) locHighlight = 1;
		prevGeocodeRowCount = cur;
	});

	function onLocKeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			locHighlight = (locHighlight + 1) % locRowCount;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			locHighlight = (locHighlight - 1 + locRowCount) % locRowCount;
		} else if (e.key === 'Home') {
			e.preventDefault();
			locHighlight = 0;
		} else if (e.key === 'End') {
			e.preventDefault();
			locHighlight = locRowCount - 1;
		} else if (e.key === 'Enter') {
			e.preventDefault();
			commitLocRow(locHighlight);
		} else if (e.key === 'Tab') {
			locOpen = false;
		}
		// Escape is intentionally left unhandled here — it bubbles to
		// onFormKeydown, which owns the whole form's Escape priority chain.
	}

	function selectPriorityAt(i: number) {
		newMissionPriority = missionPriorities[i];
		queueMicrotask(() => priorityRefs[i]?.focus());
	}

	function onPriorityKeydown(e: KeyboardEvent, i: number) {
		if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
			e.preventDefault();
			selectPriorityAt((i + 1) % missionPriorities.length);
		} else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
			e.preventDefault();
			selectPriorityAt((i - 1 + missionPriorities.length) % missionPriorities.length);
		} else if (e.key === 'Enter') {
			e.preventDefault();
			handleCreateMission();
		}
		// Space is left to native button activation, which selects this chip.
	}

	function onFormKeydown(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		if (locOpen) {
			// This form owns Escape for the location popover: closing it here
			// must not also bubble to SidePanel.svelte's window-level listener
			// and collapse the whole Net Control panel along with the draft.
			e.stopPropagation();
			locOpen = false;
			locQuery = '';
			queueMicrotask(() => locFieldTriggerEl?.focus());
		} else if (pickingOnMap) {
			// Deliberately left unstopped: Map.svelte owns Escape for an
			// in-progress map pick via its own window-level listener, and
			// calls stopImmediatePropagation() itself once it cancels the
			// pick — that is what keeps SidePanel from also closing. If we
			// stopped propagation here instead, the event would never reach
			// Map.svelte at all whenever focus is inside this form (e.g. the
			// title field) while picking, leaving the pick stuck forever
			// with no other way to cancel it.
		} else {
			// Same reasoning as the locOpen branch: cancelling the mission
			// form itself must not also bubble up and close the panel.
			e.stopPropagation();
			cancelMissionForm();
		}
	}

	function resetMissionForm() {
		showMissionForm = false;
		newMissionTitle = '';
		newMissionDesc = '';
		newMissionPriority = 'routine';
		newMissionAssigneeIds = [];
		missionLocLabel = '';
		missionLocLat = null;
		missionLocLon = null;
		missionLocSource = 'none';
		missionLocNearId = null;
		missionLocWords = '';
		missionLocNear = '';
		missionLocConfirmed = false;
		missionLocCode = '';
		autoLinkedAnnId = null;
		selectedAnnotationIds = [];
		locQuery = '';
		locOpen = false;
		locCoordLat = '';
		locCoordLon = '';
		locCoordError = '';
		detailsOpen = false;
		formError = '';
		pickingOnMap = false;
		resetW3WSuggestState();
	}

	function toggleMissionAssignee(ciId: string) {
		newMissionAssigneeIds = newMissionAssigneeIds.includes(ciId)
			? newMissionAssigneeIds.filter((id) => id !== ciId)
			: [...newMissionAssigneeIds, ciId];
	}

	function cancelMissionForm() {
		resetMissionForm();
		onClearMissionDraft?.();
	}

	async function handleCreateMission() {
		if (!$activeNet) return;
		if (!newMissionTitle.trim()) {
			formError = 'Mission title is required';
			titleEl?.focus();
			return;
		}
		formError = '';
		missionSubmitting = true;
		try {
			const data: Partial<NetMission> & { assigneeIds?: string[] } = {
				title: newMissionTitle.trim(),
				description: newMissionDesc.trim(),
				priority: newMissionPriority,
				location: missionLocLabel.trim(),
				assigneeIds: newMissionAssigneeIds,
			};
			if (missionLocLat != null && missionLocLon != null) {
				data.lat = missionLocLat;
				data.lon = missionLocLon;
			}
			// handleCreateMission never blocks on w3w confirmation — an NCS
			// under pressure must never be gated by a ceremony button. Instead,
			// an unconfirmed w3w mission gets the words in the success toast
			// one more time, in a place the NCS will see. Capture this before
			// resetMissionForm() clears the location state.
			const w3wToastWords =
				missionLocSource === 'w3w' && !missionLocConfirmed ? missionLocWords : '';
			const mission = await api.createMission($activeNet.id, data);
			let failed = 0;
			for (const annId of selectedAnnotationIds) {
				try {
					await api.linkAnnotation(annId, mission.id);
				} catch (err) {
					failed++;
					console.error('Link annotation failed:', err);
				}
			}
			resetMissionForm();
			onClearMissionDraft?.();
			const assignedCount = mission.assignedOperators?.length ?? 0;
			const opPart =
				assignedCount === 0
					? ''
					: assignedCount === 1
						? ' — 1 operator assigned'
						: ` — ${assignedCount} operators assigned`;
			showToast(
				(w3wToastWords ? `Mission created at ${formatWords(w3wToastWords)}` : `Mission created: ${mission.title}`) + opPart,
				'success'
			);
			// A roster mismatch never fails the create — say so instead of
			// silently dropping the operator.
			const skipped = mission.skippedAssignees ?? [];
			if (skipped.length > 0) {
				showToast(
					`${skipped.length} operator${skipped.length === 1 ? '' : 's'} not in roster — mission created without them`,
					'error',
					5000
				);
			}
			if (failed > 0) {
				showToast(`Mission created — ${failed} annotation link(s) failed`, 'error', 5000);
			}
			if (recentlyChangedTimer) clearTimeout(recentlyChangedTimer);
			recentlyChangedMissionId = mission.id;
			recentlyChangedTimer = setTimeout(() => { recentlyChangedMissionId = null; }, 2000);
		} catch (e) {
			console.error('Create mission failed:', e);
			formError = 'Could not create mission — check the connection and try again';
			showToast('Create mission failed', 'error', 5000);
		} finally {
			missionSubmitting = false;
		}
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

	// Assignment is ONE operation, addressable from either object. The mission
	// card and the roster card both land here.
	async function assignOperator(missionId: string, ciId: string) {
		if (!$activeNet) return;
		try {
			await api.assignMissionOperator($activeNet.id, missionId, ciId);
			assigningMissionId = null;
			assigningCheckInId = null;
		} catch (e) {
			console.error('Assign failed:', e);
			showToast('Assign failed — check the connection', 'error', 5000);
		}
	}

	async function unassignOperator(missionId: string, ciId: string) {
		if (!$activeNet) return;
		try {
			await api.unassignMissionOperator($activeNet.id, missionId, ciId);
		} catch (e) {
			console.error('Unassign failed:', e);
			showToast('Unassign failed — check the connection', 'error', 5000);
		}
	}

	// Roster-side wrapper: the chip strip is keyed by (checkIn, mission).
	const handleUnassignMission = (ciId: string, missionId: string) => unassignOperator(missionId, ciId);

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

	// Mission location picker — net-scoped point annotations only (not
	// linkableAnnotations, which is not net-scoped and would leak other nets').
	let locOptions = $derived(
		$netLocationAnnotations.filter((a) => !isTerminalStatus(a.status))
	);
	let locFiltered = $derived.by(() => {
		const q = locQuery.trim().toLowerCase();
		if (!q) return locOptions;
		return locOptions.filter((a) =>
			a.label.toLowerCase().includes(q) ||
			(a.shortName && a.shortName.toLowerCase().includes(q))
		);
	});
	// --- offline geocode detection & rows (#94) — checked ahead of w3w's own
	// derived block below since detectFormat's precedence already puts w3w
	// first; pluscode/mgrs only ever come back once a w3w shape is ruled out.
	let locFormat = $derived(detectFormat(locQuery));
	let locGeocode = $derived.by((): GeocodeParse | null => {
		if (locFormat === 'pluscode') return parsePlusCode(locQuery, currentRefPoint());
		if (locFormat === 'mgrs') return parseMGRS(locQuery);
		return null;
	});
	let geocodeRowCount = $derived(locGeocode && !isGeocodeError(locGeocode) ? 1 : 0);
	let geocodeErrCode = $derived(
		locGeocode && isGeocodeError(locGeocode) ? locGeocode.error : null
	);

	// Copy for the inline error under the Plus Code / MGRS group header —
	// short-code-without-reference is the only one with an action in it,
	// because it's the only one that is actually fixable from here (pan the
	// map). The others are terminal and self-explanatory.
	function geocodeErrorCopy(code: GeocodeErrorCode, format: GeocodeFormat | null): string {
		if (code === 'needs_reference') {
			return 'Short Plus Code needs a nearby reference — pan the map there first, or type the full code.';
		}
		if (code === 'too_coarse') {
			return 'That Plus Code is too coarse to place a pin — include the characters after the +.';
		}
		return format === 'mgrs'
			? 'Not a valid MGRS grid reference.'
			: 'Not a valid Plus Code — check the characters after the +.';
	}

	// Announce the error once, on the transition into it — this only reruns
	// when geocodeErrCode's *value* actually changes, not on every keystroke
	// that leaves it unchanged (e.g. typing further into an already-invalid
	// code).
	$effect(() => {
		const code = geocodeErrCode;
		if (code) srMessage = geocodeErrorCopy(code, locFormat);
	});

	let showFreeTextRow = $derived(
		locQuery.trim().length > 0 &&
		geocodeRowCount === 0 &&
		!locFiltered.some((a) => a.label.toLowerCase() === locQuery.trim().toLowerCase())
	);

	// --- what3words detection & rows ---
	let locIsFullAddress = $derived(isFullAddress(locQuery));
	let locW3WShaped = $derived(locIsFullAddress || looksLikePartial(locQuery));
	// Stricter than locW3WShaped: also requires the ≥2-chars-after-first-dot
	// quota guard, so the "what3words" group (and any network call) only
	// appears once there's something worth searching for.
	let w3wGateOk = $derived.by(() => {
		if (!locW3WShaped) return false;
		const q = locQuery.trim();
		const dotIdx = q.indexOf('.');
		return dotIdx !== -1 && q.slice(dotIdx + 1).length >= 2;
	});
	let w3wRowCount = $derived(
		$w3wConfigured && w3wGateOk && !w3wLoading && !w3wError ? w3wSuggestions.length : 0
	);
	let w3wNoResults = $derived(
		$w3wConfigured && w3wGateOk && !w3wLoading && !w3wError &&
		w3wSuggestions.length === 0 && w3wSearchedQuery === locQuery.trim()
	);

	let locRowCount = $derived(
		1 + geocodeRowCount + w3wRowCount + locFiltered.length + (showFreeTextRow ? 1 : 0)
	);

	// Whether the current selection still needs the map fly-to acknowledged
	// before it's trustworthy — true for every resolve that skipped a human
	// picking a point directly (w3w, Plus Code, MGRS all share the gate).
	let locNeedsConfirm = $derived(
		missionLocSource === 'w3w' || missionLocSource === 'pluscode' || missionLocSource === 'mgrs'
	);

	// The annotation behind the current selection, when source is 'annotation'
	// (for its category icon/color) or 'map'/'w3w' with a near match (for its
	// label) — a what3words resolve that lands near a net location still
	// picks up that location's icon/colour without losing the words.
	let missionLocAnn = $derived(
		missionLocNearId ? ($netLocationAnnotations.find((a) => a.id === missionLocNearId) ?? null) : null
	);

	let locChipPrimary = $derived.by(() => {
		if (missionLocSource === 'w3w') return formatWords(missionLocWords);
		if (missionLocSource === 'mgrs') return formatMGRSGroups(missionLocCode);
		if (missionLocSource === 'pluscode') return missionLocCode;
		if (missionLocSource === 'map') return missionLocLabel || 'Dropped pin';
		if (missionLocSource === 'coords' && !missionLocLabel && missionLocLat != null && missionLocLon != null) {
			return formatCoord(missionLocLat, missionLocLon);
		}
		return missionLocLabel;
	});
	let locChipSecondary = $derived.by(() => {
		if (missionLocLat == null || missionLocLon == null) return '';
		const coords = formatCoord(missionLocLat, missionLocLon);
		if (missionLocSource === 'w3w') return missionLocNear ? `near ${missionLocNear} · ${coords}` : coords;
		if (missionLocSource === 'pluscode' || missionLocSource === 'mgrs') {
			return missionLocNear ? `near ${missionLocNear} · ${coords}` : coords;
		}
		if (missionLocSource === 'map') return missionLocLabel ? `near · ${coords}` : coords;
		if (missionLocSource === 'coords') return missionLocLabel ? coords : '';
		return coords;
	});

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
				if (timelineFilter === 'weather' && e.type !== 'wx_alert') return false;
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

<svelte:window onkeydown={handleWindowKeydown} />

<div class="net-panel" bind:this={panelRootEl} tabindex="-1">
	{#if showWxWatchSheet && $activeNet}
		<WxWatchAreaSheet netId={$activeNet.id} onClose={() => (showWxWatchSheet = false)} />
	{:else if !$activeNet}
		<!-- No active net -->
		<div class="panel-header">
			<span class="title">Net Control</span>
		</div>

		{#if closedSummary}
			<div class="summary-card">
				<span class="summary-kicker">Net ended</span>
				<span class="summary-name">{closedSummary.name}</span>
				<div class="summary-grid">
					<div class="summary-stat">
						<span class="summary-value">{closedSummary.duration}</span>
						<span class="summary-label">Duration</span>
					</div>
					<div class="summary-stat">
						<span class="summary-value">{closedSummary.totalCheckIns}</span>
						<span class="summary-label">Check-ins</span>
					</div>
					<div class="summary-stat">
						<span class="summary-value">{closedSummary.totalMissions}</span>
						<span class="summary-label">Missions</span>
					</div>
				</div>
				<div class="summary-actions">
					<button class="btn-secondary" onclick={() => openICS309(closedSummary?.netId)}>Open ICS-309 Log</button>
					<button class="btn-secondary" onclick={() => (closedSummary = null)}>Dismiss</button>
				</div>
			</div>
		{/if}

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
		<!-- Active net header. The net's lifecycle belongs to the status chip, not to
		     the action row: ending a net is not a sibling of logging a note. -->
		<div class="panel-header" bind:this={headerEl} tabindex="-1">
			<div class="header-info">
				<div class="header-title-row">
					<span class="title">{$activeNet.name}</span>
					<button
						class="net-state-chip state-{$activeNet.status}"
						bind:this={stateChipEl}
						onclick={(e) => {
							e.stopPropagation();
							moreOpen = false;
							if (!lifecycleOpen) openPopover(stateChipEl, 'left');
							lifecycleOpen = !lifecycleOpen;
						}}
						aria-haspopup="menu"
						aria-expanded={lifecycleOpen}
						aria-label="Net status: {$activeNet.status}{elapsed ? `, open ${elapsed}` : ''}. Net lifecycle actions."
					>
						<span class="chip-state">{$activeNet.status}</span>
						{#if elapsed}<span class="chip-elapsed">{elapsed}</span>{/if}
						<svg class="chip-caret" width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
							<path d="M2 4l3 3 3-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
						</svg>
					</button>
					{#if $wxActiveWarningIn}
						{@const wxWarn = $wxActiveWarningIn}
						<button
							class="wx-header-chip"
							onclick={() => openWxAlert(wxWarn.id)}
							aria-label="{wxWarn.event} in watch area, ends {clock(wxWarn.endsAt)}. Open alert."
						>
							<WxTierGlyph tier="warning" size={12} />
							{wxWarn.shortCode}
							<span class="wx-header-ttl">{countdown(wxWarn.endsAt, $wxClock).text}</span>
						</button>
					{/if}
				</div>
				{#if $activeNet.frequency}
					<span class="frequency">{$activeNet.frequency}</span>
				{/if}
				{#if $activeNet.missionBrief}
					<span class="mission-brief">{$activeNet.missionBrief}</span>
				{/if}
			</div>
			<!-- role="group", not "toolbar": a toolbar promises arrow-key
			     navigation with a roving tabindex, and these are plain
			     Tab-reachable buttons. The two menus below do implement the
			     menu contract (see menuNav). -->
			<div class="header-actions" role="group" aria-label="Net actions">
				<button class="action-btn labelled" onclick={handleRollCall} title="Roll Call" aria-label="Start roll call">
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
						<path d="M1 8h3l2-5 3 10 2-5h4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
					<span class="action-text">Roll Call</span>
				</button>

				<button class="action-btn labelled" onclick={() => openNoteComposer({})} title="Log Note" aria-label="Log a note to the net log">
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
						<path d="M12 2l2 2-8 8H4v-2l8-8z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
						<path d="M2 14h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
					<span class="action-text">Log Note</span>
				</button>

				<button
					class="action-btn more-btn"
					bind:this={moreBtnEl}
					title="More"
					onclick={(e) => {
						e.stopPropagation();
						lifecycleOpen = false;
						if (!moreOpen) openPopover(moreBtnEl, 'right');
						moreOpen = !moreOpen;
					}}
					aria-haspopup="menu"
					aria-expanded={moreOpen}
					aria-label="More net actions"
				>
					<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
						<circle cx="3" cy="8" r="1.4" fill="currentColor"/>
						<circle cx="8" cy="8" r="1.4" fill="currentColor"/>
						<circle cx="13" cy="8" r="1.4" fill="currentColor"/>
					</svg>
				</button>
			</div>
		</div>

		{#if lifecycleOpen && $activeNet}
			<div
				class="pop-menu"
				role="menu"
				aria-label="Net lifecycle"
				data-blocks-escape="true"
				style="top:{popTop}px; left:{popLeft}px"
				use:topLayer
				use:menuNav
			>
				<div class="pop-meta">
					<span class="pop-name">{$activeNet.name}</span>
					<span class="pop-sub">Opened {$activeNet.openedAt ? timeAgo($activeNet.openedAt) : '—'} · NCS {$activeNet.ncsCallsign}</span>
				</div>
				{#if $activeNet.status === 'open'}
					<button class="pop-row pop-row-danger" role="menuitem" onclick={requestCloseNet}>
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
							<path d="M8 2v6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
							<path d="M4.5 4.5a5 5 0 1 0 7 0" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
						</svg>
						<span>End net…</span>
					</button>
				{/if}
			</div>
		{/if}

		{#if moreOpen && $activeNet}
			<div
				class="pop-menu"
				role="menu"
				aria-label="More net actions"
				data-blocks-escape="true"
				style="top:{popTop}px; left:{popLeft}px"
				use:topLayer
				use:menuNav
			>
				<button
					class="pop-row"
					class:ops-view-set={$opsView}
					role="menuitem"
					onclick={() => { onSetOpsView?.(); closePopovers(); }}
				>
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
						<circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.5"/>
						<circle cx="8" cy="8" r="2" fill="currentColor"/>
						<path d="M8 1v3M8 12v3M1 8h3M12 8h3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
					<span>{$opsView ? 'Ops View saved — save again' : 'Save Ops View'}</span>
				</button>

				<button class="pop-row" role="menuitem" onclick={() => { openICS309($activeNet?.id); closePopovers(); }}>
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
						<path d="M4 2h8a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z" stroke="currentColor" stroke-width="1.5"/>
						<path d="M6 5h4M6 8h4M6 11h2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
					<span>ICS-309 Log</span>
				</button>

				<!-- Stays a real link: middle-click and copy-link matter when handing a
				     URL to an agency rep. -->
				<a
					class="pop-row"
					role="menuitem"
					href="/dashboard?net={$activeNet.id}"
					target="_blank"
					rel="noopener"
					aria-label="Agency View (opens in a new tab)"
					onclick={() => closePopovers()}
				>
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
						<rect x="2" y="3" width="12" height="8" rx="1" stroke="currentColor" stroke-width="1.5"/>
						<line x1="5" y1="13" x2="11" y2="13" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
						<line x1="8" y1="11" x2="8" y2="13" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
					<span>Agency View</span>
					<span class="pop-hint" aria-hidden="true">↗</span>
				</a>

				{#if $wxIsNcs}
					<button class="pop-row" role="menuitem" onclick={() => { showWxWatchSheet = true; closePopovers(); }}>
						<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
							<path d="M8 1L2 4v4c0 4 2.5 6.5 6 7 3.5-.5 6-3 6-7V4L8 1z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
						</svg>
						<span>Weather watch area…</span>
					</button>
				{/if}
			</div>
		{/if}

		<!-- Metrics bar -->
		{#if $activeCheckIns.length > 0 || $missions.length > 0}
			<!-- Also a group, not a toolbar — same reason as .header-actions. -->
			<div class="metrics-bar" role="group" aria-label="Net status metrics">
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
		<div class="tabs" role="tablist" aria-label="Net control sections">
			<button class="tab" role="tab" aria-selected={currentTab === 'situation'} class:active={currentTab === 'situation'} onclick={() => { currentTab = 'situation'; metricsFilter = null; }}>
				<span class="tab-label">SitBoard</span>
				{#if $attentionItems.length + $wxUnackedWarningsIn.length > 0}<span class="tab-count tab-count-alert" aria-label="{$attentionItems.length + $wxUnackedWarningsIn.length} items need attention">{$attentionItems.length + $wxUnackedWarningsIn.length}</span>{/if}
			</button>
			<button class="tab" role="tab" aria-selected={currentTab === 'roster'} class:active={currentTab === 'roster'} onclick={() => { currentTab = 'roster'; metricsFilter = null; }}>
				<span class="tab-label">Roster</span>
				<span class="tab-count">{$activeCheckIns.length}</span>
			</button>
			<button class="tab" role="tab" aria-selected={currentTab === 'missions'} class:active={currentTab === 'missions'} onclick={() => { currentTab = 'missions'; metricsFilter = null; }}>
				<span class="tab-label">Missions</span>
				{#if activeMissionCount > 0}<span class="tab-count">{activeMissionCount}</span>{/if}
			</button>
			<button class="tab" role="tab" aria-selected={currentTab === 'locations'} class:active={currentTab === 'locations'} onclick={() => { currentTab = 'locations'; metricsFilter = null; }}>
				<span class="tab-label">Locations</span>
				{#if $netAnnotations.length > 0}<span class="tab-count">{$netAnnotations.length}</span>{/if}
			</button>
			<button class="tab" role="tab" aria-selected={currentTab === 'timeline'} class:active={currentTab === 'timeline'} onclick={() => { currentTab = 'timeline'; metricsFilter = null; }}>
				<span class="tab-label">Timeline</span>
			</button>
		</div>

		<!-- Tab content -->
		<div class="tab-content">
			{#if currentTab === 'situation'}
				<SituationBoard
					onNavigateTab={(tab, filter) => {
						currentTab = tab;
						if (filter === 'missing') metricsFilter = 'missing';
						else if (filter === 'stale') metricsFilter = 'stale';
						else metricsFilter = null;
					}}
					onFlyTo={(lat, lon) => onFlyTo?.(lat, lon)}
				/>
			{:else if currentTab === 'roster'}
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
				<!-- Command Palette (Enhanced Quick Add) -->
				<div class="quick-add">
					<div class="quick-add-wrap">
						{#if cmdModeLabel}
							<span class="cmd-mode-badge" style="background: {cmdModeColors[cmdParsed.type] ?? '#6b7280'}">{cmdModeLabel}</span>
						{/if}
						<input
							bind:this={quickAddRef}
							type="text"
							bind:value={quickAddInput}
							oninput={handleQuickAddInput}
							onkeydown={handleCmdKeydown}
							placeholder="KD7BBC R medical  ·  KD7BBC assigned  ·  mission Search Grid A"
							class="quick-add-input"
						/>
						<button class="quick-add-btn" onclick={handleQuickAdd} disabled={!quickAddInput.trim()} aria-label="Submit command">
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M5 12h14M12 5l7 7-7 7"/></svg>
						</button>
					</div>
					<div class="quick-add-hint">
						{#if !quickAddInput.trim()}
							<span class="hint-key">R</span>outine
							<span class="hint-key">P</span>riority
							<span class="hint-key">W</span>elfare
							<span class="hint-key">E</span>mergency
							<span class="hint-sep">&middot;</span>
							<span class="hint-muted">status · note · assign · loc · out · mission</span>
						{:else}
							{@const hint = getCmdHint(cmdParsed, cmdAutocomplete)}
							{#if hint}
								<span class="hint-muted">{hint}</span>
							{/if}
							<span class="hint-kbd">Tab</span> complete
							<span class="hint-kbd">&uarr;&darr;</span> history
						{/if}
					</div>
					{#if cmdAutocomplete?.suggestions.length && quickAddInput.trim()}
						<div class="search-dropdown">
							{#each cmdAutocomplete.suggestions.slice(0, 8) as suggestion, i}
								<button
									class="search-item"
									class:search-item-active={i === cmdAcIndex}
									onmousedown={() => selectAcSuggestion(suggestion)}
									onmouseenter={() => { cmdAcIndex = i; }}
								>
									<span class="search-call">{suggestion}</span>
									{#if cmdAutocomplete?.phase === 'callsign'}
										{@const ci = findCheckInByCallsign(suggestion)}
										{#if ci}
											<span class="search-comment">{ci.status}{ci.tacticalCall ? ` · ${ci.tacticalCall}` : ''}</span>
										{/if}
									{/if}
								</button>
							{/each}
						</div>
					{:else if searchResults.length > 0}
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
							<div class="op-status-bar" class:wx-warning={$weatherCheckInIds.has(ci.id)} style="background: {statusColors[ci.status]}"></div>
							<div class="op-main">
								<div class="op-header">
									<span class="op-callsign">{ci.callsign}</span>
									{#if $weatherCheckInIds.has(ci.id)}
										<WxTierGlyph tier="warning" size={11} title="Inside an active NWS warning" />
									{/if}
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
								{#if ci.missionIds?.length > 0}
									<div class="op-mission-chips">
									{#each ci.missionIds as mid (mid)}
										{@const linkedMission = $missions.find((m) => m.id === mid)}
										{#if linkedMission}
											<!-- svelte-ignore a11y_no_static_element_interactions -->
											<div
													class="op-mission-chip"
													class:chip-highlighted={$hoveredMissionId === mid}
													style="--mission-color: {trafficColors[linkedMission.priority] ?? '#6b7280'}"
													onmouseenter={() => hoveredMissionId.set(mid)}
													onmouseleave={() => hoveredMissionId.set(null)}
												>
													<span class="mission-chip-dot"></span>
													<span class="mission-chip-title">{linkedMission.title}</span>
													<button class="mission-chip-remove" title="Unassign mission" aria-label="Unassign {linkedMission.title}" onclick={(e) => { e.stopPropagation(); handleUnassignMission(ci.id, mid); }}>✕</button>
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
								<button class="bar-btn" class:active={assigningCheckInId === ci.id} aria-expanded={assigningCheckInId === ci.id} title="Assign Mission" onclick={() => { assigningCheckInId = assigningCheckInId === ci.id ? null : ci.id; }}>Assign</button>
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
									<MissionPicker
										missions={$missions.filter((mm) => mm.status !== 'complete')}
										excludeIds={ci.missionIds ?? []}
										ariaLabel="Assign mission to {ci.callsign}"
										onSelect={(mm) => assignOperator(mm.id, ci.id)}
										onClose={() => { assigningCheckInId = null; }}
									/>
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
						<button class="btn-secondary btn-sm" onclick={() => (showMissionForm = true)}>+ Mission</button>
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
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div class="mission-form" onkeydown={onFormKeydown}>
						<input
							type="text"
							class="mission-title-input"
							bind:value={newMissionTitle}
							bind:this={titleEl}
							placeholder="Mission title"
							aria-label="Mission title"
							onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); handleCreateMission(); } }}
						/>

						<div class="mission-loc-field">
							{#if pickingOnMap}
								<button type="button" class="loc-trigger picking" bind:this={locFieldTriggerEl} onclick={onLocTriggerClick}>
									<svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M8 1C5.24 1 3 3.24 3 6c0 3.75 5 9 5 9s5-5.25 5-9c0-2.76-2.24-5-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z" fill="currentColor"/></svg>
									Picking on map… <kbd>Esc</kbd>
								</button>
							{:else if missionLocSource === 'none'}
								<button
									type="button"
									class="loc-trigger"
									bind:this={locFieldTriggerEl}
									aria-haspopup="listbox"
									aria-expanded={locOpen}
									onclick={onLocTriggerClick}
									onkeydown={(e) => { if (e.key === 'ArrowDown') { e.preventDefault(); openLocPopover(); } }}
								>
									<svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M8 1C5.24 1 3 3.24 3 6c0 3.75 5 9 5 9s5-5.25 5-9c0-2.76-2.24-5-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z" fill="currentColor"/></svg>
									Add location
									<svg class="chev" width="10" height="10" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
								</button>
							{:else}
								<div class="loc-chip" class:unconfirmed={locNeedsConfirm && !missionLocConfirmed} role="group" aria-label="Mission location">
									<button
										type="button"
										class="loc-chip-main"
										bind:this={locFieldTriggerEl}
										onclick={openLocPopover}
										aria-label={
											missionLocSource === 'w3w' ? `what3words location ${missionLocWords.split('.').join(' dot ')}` :
											missionLocSource === 'pluscode' ? `Plus Code ${missionLocCode}` :
											missionLocSource === 'mgrs' ? `MGRS grid reference ${formatMGRSGroups(missionLocCode)}` :
											undefined
										}
									>
										{#if missionLocAnn}
											<span class="loc-chip-icon" style="--loc-cat-color: {categoryMeta[missionLocAnn.category]?.defaultColor ?? '#6b7280'}">
												<svg width="16" height="16" viewBox="0 0 16 16"><path d={categoryMeta[missionLocAnn.category]?.icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
											</span>
										{:else}
											<span class="loc-chip-icon" style="--loc-cat-color: #6b7280">
												<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M8 1C5.24 1 3 3.24 3 6c0 3.75 5 9 5 9s5-5.25 5-9c0-2.76-2.24-5-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z" fill="currentColor"/></svg>
											</span>
										{/if}
										<span class="loc-chip-text">
											{#if missionLocSource === 'w3w'}
												<span class="loc-chip-label loc-chip-label-w3w" aria-hidden="true">
													<span class="w3w-slashes">///</span>{missionLocWords}
												</span>
											{:else if missionLocSource === 'pluscode' || missionLocSource === 'mgrs'}
												<span class="loc-chip-label loc-chip-label-code" aria-hidden="true">{locChipPrimary}</span>
											{:else}
												<span class="loc-chip-label">{locChipPrimary}</span>
											{/if}
											{#if locChipSecondary}<span class="loc-chip-coords">{locChipSecondary}</span>{/if}
										</span>
									</button>
									{#if locNeedsConfirm}
										{#if missionLocConfirmed}
											<span class="loc-chip-confirmed">✓ Confirmed</span>
										{:else}
											<button type="button" class="loc-chip-confirm" onclick={confirmLocation}>Confirm on map</button>
										{/if}
									{/if}
									<button type="button" class="loc-chip-map" title="Pick on map" aria-label="Pick on map" onclick={startMapPick}>
										<svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M8 1C5.24 1 3 3.24 3 6c0 3.75 5 9 5 9s5-5.25 5-9c0-2.76-2.24-5-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z" fill="currentColor"/></svg>
									</button>
									<button type="button" class="loc-chip-clear" title="Clear location" aria-label="Clear location" onclick={clearLocation}>✕</button>
								</div>
								{#if missionLocLat != null && missionLocLon != null}
									<div class="loc-reverse" aria-label="This location in other formats">
										{#if reverseMGRS}
											<div class="loc-reverse-token">
												<span class="loc-reverse-kind">MGRS</span>
												<span class="loc-reverse-value">{formatMGRSGroups(reverseMGRS)}</span>
												<button type="button" class="loc-reverse-copy" aria-label="Copy MGRS grid reference {formatMGRSGroups(reverseMGRS)}" onclick={() => copyToken('mgrs', formatMGRSGroups(reverseMGRS))}>
													{reverseCopied === 'mgrs' ? 'Copied' : '⧉'}
												</button>
											</div>
										{/if}
										{#if reversePlusCode}
											{@const rpc = reversePlusCode}
											<div class="loc-reverse-token">
												<span class="loc-reverse-kind">Plus</span>
												<span class="loc-reverse-value" title={rpc.short !== rpc.full ? rpc.full : undefined}>{rpc.short}</span>
												<button type="button" class="loc-reverse-copy" aria-label="Copy full Plus Code {rpc.full}" onclick={() => copyToken('plus', rpc.full)}>
													{reverseCopied === 'plus' ? 'Copied' : '⧉'}
												</button>
											</div>
										{/if}
										{#if $w3wConfigured && reverseWords && missionLocSource !== 'w3w'}
											<div class="loc-reverse-token">
												<span class="loc-reverse-kind">///</span>
												<span class="loc-reverse-value loc-reverse-value-w3w">{formatWords(reverseWords)}</span>
												<button type="button" class="loc-reverse-copy" aria-label="Copy three-word address {formatWords(reverseWords)}" onclick={() => copyToken('w3w', formatWords(reverseWords))}>
													{reverseCopied === 'w3w' ? 'Copied' : '⧉'}
												</button>
											</div>
										{/if}
									</div>
								{/if}
							{/if}

							{#if locOpen}
								<div class="loc-popover" bind:this={locPopoverEl}>
									<input
										type="text"
										class="loc-search"
										role="combobox"
										autocomplete="off"
										bind:value={locQuery}
										bind:this={locSearchEl}
										placeholder="Search locations or type a name"
										aria-expanded="true"
										aria-controls="mission-loc-list"
										aria-autocomplete="list"
										aria-activedescendant={'mission-loc-opt-' + locHighlight}
										oninput={() => { locHighlight = 0; scheduleW3WSuggest(); }}
										onkeydown={onLocKeydown}
										onblur={onLocSearchBlur}
										inputmode="text"
										autocapitalize="none"
										autocorrect="off"
										spellcheck="false"
									/>
									<div class="loc-list" role="listbox" id="mission-loc-list">
										<button
											type="button"
											id="mission-loc-opt-0"
											role="option"
											aria-selected={locHighlight === 0}
											class="loc-opt loc-opt-map"
											class:highlight={locHighlight === 0}
											onmousedown={() => startMapPick()}
										>
											📍 Choose on map
										</button>

										{#if geocodeRowCount === 1 && locGeocode && !isGeocodeError(locGeocode)}
											{@const g = locGeocode}
											<div class="loc-group-header" role="presentation">{g.format === 'mgrs' ? 'MGRS' : 'Plus Code'}</div>
											<button
												type="button"
												id="mission-loc-opt-1"
												role="option"
												aria-selected={locHighlight === 1}
												class="loc-opt loc-opt-geocode"
												class:highlight={locHighlight === 1}
												onmousedown={() => useGeocode(g)}
											>
												<span class="loc-opt-geocode-icon" aria-hidden="true">
													<svg width="14" height="14" viewBox="0 0 16 16" fill="none"><path d="M2 2h4v4H2V2zM10 2h4v4h-4V2zM2 10h4v4H2v-4zM10 10h4v4h-4v-4z" stroke="currentColor" stroke-width="1.3" fill="none" stroke-linejoin="round"/></svg>
												</span>
												<span class="loc-opt-geocode-text">
													<span class="loc-opt-geocode-line1">
														Use {g.format === 'mgrs' ? 'MGRS' : 'Plus Code'}
														<span class="loc-opt-geocode-code">{g.format === 'mgrs' ? formatMGRSGroups(g.label) : g.label}</span>
													</span>
													<span class="loc-opt-geocode-line2">
														{#if g.recoveredFrom}{g.recoveredFrom} · {/if}±{g.precisionM} m
													</span>
												</span>
												<span class="loc-opt-geocode-coords">{formatCoord(g.lat, g.lon)}</span>
											</button>
										{:else if geocodeErrCode}
											<div class="loc-group-header" role="presentation">{locFormat === 'mgrs' ? 'MGRS' : 'Plus Code'}</div>
											<p class="loc-status loc-status-error" role="presentation">{geocodeErrorCopy(geocodeErrCode, locFormat)}</p>
										{/if}

										{#if $w3wConfigured && w3wGateOk}
											<div class="loc-group-header" role="presentation">what3words</div>
											{#if w3wLoading}
												<p class="loc-w3w-status" role="presentation">Looking up three-word addresses…</p>
											{:else if w3wError}
												<p class="loc-w3w-status loc-w3w-error" role="presentation">{w3wError}</p>
											{:else if w3wSuggestions.length > 0}
												{#each w3wSuggestions as sug, i (sug.words)}
													{@const rowIdx = 1 + geocodeRowCount + i}
													<button
														type="button"
														id={'mission-loc-opt-' + rowIdx}
														role="option"
														aria-selected={locHighlight === rowIdx}
														class="loc-opt loc-opt-w3w"
														class:highlight={locHighlight === rowIdx}
														onmousedown={() => resolveW3W(sug.words)}
													>
														<span class="loc-opt-w3w-words">{formatWords(sug.words)}</span>
														<span class="loc-opt-w3w-meta">
															{sug.nearestPlace}{#if sug.nearestPlace && sug.distanceToFocusKm}<span> · </span>{/if}{#if sug.distanceToFocusKm}{sug.distanceToFocusKm.toFixed(1)} km{/if}
														</span>
													</button>
												{/each}
											{:else if w3wNoResults}
												<p class="loc-w3w-status" role="presentation">No what3words matches for “{locQuery.trim()}”</p>
											{/if}
										{:else if !$w3wConfigured && locW3WShaped}
											<div class="loc-w3w-hint" role="presentation">
												<p class="loc-w3w-hint-title">That looks like a what3words address.</p>
												{#if $canAdmin}
													<p>
														Add a free API key in Settings → what3words to turn three words into a map pin.
														<button type="button" class="loc-w3w-hint-link" onmousedown={(e) => { e.preventDefault(); openW3WSettings(); }}>Settings →</button>
													</p>
												{:else}
													<p>Ask an admin to add a what3words API key.</p>
												{/if}
											</div>
										{/if}

										{#each locFiltered as ann, i (ann.id)}
											{@const rowIdx = 1 + geocodeRowCount + w3wRowCount + i}
											<button
												type="button"
												id={'mission-loc-opt-' + rowIdx}
												role="option"
												aria-selected={locHighlight === rowIdx}
												class="loc-opt"
												class:highlight={locHighlight === rowIdx}
												onmousedown={() => selectLocationAnnotation(ann)}
											>
												<span class="loc-opt-icon" style="--loc-cat-color: {categoryMeta[ann.category]?.defaultColor ?? '#6b7280'}">
													<svg width="16" height="16" viewBox="0 0 16 16"><path d={categoryMeta[ann.category]?.icon} stroke="currentColor" stroke-width="1.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
												</span>
												<span class="loc-opt-label">{ann.label}</span>
												{#if ann.shortName}<span class="loc-opt-short">{ann.shortName}</span>{/if}
											</button>
										{/each}
										{#if showFreeTextRow}
											{@const freeIdx = 1 + geocodeRowCount + w3wRowCount + locFiltered.length}
											<button
												type="button"
												id={'mission-loc-opt-' + freeIdx}
												role="option"
												aria-selected={locHighlight === freeIdx}
												class="loc-opt loc-opt-free"
												class:highlight={locHighlight === freeIdx}
												onmousedown={() => useTypedLocation()}
											>
												Use “{locQuery.trim()}”
											</button>
										{/if}
										{#if locFiltered.length === 0 && !locQuery.trim()}
											<p class="loc-empty">No net locations yet — choose on map or type a name</p>
										{/if}
									</div>
									<details class="loc-coords">
										<summary>Coordinates</summary>
										<div class="form-row">
											<input type="text" bind:value={locCoordLat} placeholder="Lat" inputmode="decimal" aria-label="Latitude" />
											<input type="text" bind:value={locCoordLon} placeholder="Lon" inputmode="decimal" aria-label="Longitude" />
											<button type="button" class="btn-mini" onclick={useTypedCoords}>Use</button>
										</div>
										{#if locCoordError}<div class="form-error" role="alert">{locCoordError}</div>{/if}
									</details>
								</div>
							{/if}
						</div>

						<div class="mission-priority-group" role="radiogroup" aria-label="Priority">
							{#each missionPriorities as p, i}
								<button
									type="button"
									class="priority-chip priority-{p}"
									role="radio"
									aria-checked={newMissionPriority === p}
									tabindex={newMissionPriority === p ? 0 : -1}
									bind:this={priorityRefs[i]}
									onclick={() => (newMissionPriority = p)}
									onkeydown={(e) => onPriorityKeydown(e, i)}
								>
									{missionPriorityLabels[p]}
								</button>
							{/each}
						</div>

						<div class="mission-assign-field">
							<span class="mission-assign-label">
								Assign operators
								{#if newMissionAssigneeIds.length > 0}<span class="link-count">({newMissionAssigneeIds.length})</span>{/if}
							</span>
							{#if newMissionAssigneeIds.length > 0}
								<div class="mission-operators">
									{#each newMissionAssigneeIds as ciId (ciId)}
										{@const pickedCi = $activeCheckIns.find((c) => c.id === ciId)}
										{#if pickedCi}
											<MissionOpChip
												callsign={pickedCi.callsign}
												status={pickedCi.status}
												color={statusColors[pickedCi.status]}
												removable
												onRemove={() => toggleMissionAssignee(pickedCi.id)}
											/>
										{/if}
									{/each}
								</div>
							{/if}
							<OperatorPicker
								candidates={$activeCheckIns}
								selectedIds={newMissionAssigneeIds}
								mode="multi"
								ariaLabel="Assign operators to this mission"
								emptyLabel="No operators checked in"
								onSelect={(ci) => toggleMissionAssignee(ci.id)}
							/>
						</div>

						<button type="button" class="details-toggle" onclick={() => (detailsOpen = !detailsOpen)} aria-expanded={detailsOpen}>
							<span aria-hidden="true">{detailsOpen ? '▾' : '▸'}</span>
							Details & links
							{#if newMissionDesc.trim() || selectedAnnotationIds.length > 0}
								<span class="details-badge">
									{[newMissionDesc.trim() ? 'note' : null, selectedAnnotationIds.length > 0 ? `${selectedAnnotationIds.length} linked` : null].filter(Boolean).join(' · ')}
								</span>
							{/if}
						</button>

						{#if detailsOpen}
							<div class="mission-details">
								<textarea bind:value={newMissionDesc} rows="2" placeholder="Description (optional)"></textarea>
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
							</div>
						{/if}

						{#if formError}
							<div class="form-error" role="alert">{formError}</div>
						{/if}

						<div class="sr-live" aria-live="polite">{srMessage}</div>

						<div class="form-actions" class:submitting={missionSubmitting}>
							<button type="button" class="btn-secondary" disabled={missionSubmitting} onclick={cancelMissionForm}>Cancel</button>
							<button type="button" class="btn-primary" onclick={handleCreateMission} disabled={missionSubmitting || !newMissionTitle.trim()} aria-busy={missionSubmitting}>{missionSubmitting ? 'Creating…' : 'Create mission'}</button>
						</div>
					</div>
				{/if}

				<!-- Mission list -->
				<div class="mission-list">
					{#each filteredMissions as m (m.id)}
						{@const assignedOps = operatorsForMission(m.id)}
						{@const hasNoOperators = assignedOps.length === 0}
						<!-- Bound once: annotationsForMission() scans every annotation in the
						     app, and it was called twice per card (guard + each loop). Same
						     call, same result and ordering — purely fewer scans. -->
						{@const missionAnns = annotationsForMission(m.id)}
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
								{#if m.location || (m.lat != null && m.lon != null)}
									{#if isW3WLocation(m.location)}
										{@const words = extractWords(m.location)}
										{@const suffix = w3wSuffix(m.location)}
										<div class="mission-location" aria-label={words ? `what3words location ${words.split('.').join(' dot ')}` : undefined}>
											<span class="w3w-mark" aria-hidden="true">///</span><span class="w3w-words" aria-hidden="true">{words}</span>{#if suffix}<span class="w3w-near" aria-hidden="true"> · {suffix}</span>{/if}
										</div>
									{:else}
										{@const geo = extractGeocode(m.location)}
										{#if geo}
											{@const suffix = geocodeNearSuffix(m.location)}
											{@const geoLabel = geo.format === 'mgrs' ? formatMGRSGroups(geo.code) : geo.code}
											<div class="mission-location" aria-label={`${geo.format === 'mgrs' ? 'MGRS grid reference' : 'Plus Code'} ${geoLabel}`}>
												📍 <span class="geocode-code" aria-hidden="true">{geoLabel}</span>{#if suffix}<span class="w3w-near" aria-hidden="true"> · near {suffix}</span>{/if}
											</div>
										{:else}
											<div class="mission-location">📍 {m.location || formatCoord(m.lat ?? 0, m.lon ?? 0)}</div>
										{/if}
									{/if}
								{/if}

								<!-- Assigned operators -->
								<div class="mission-operators">
									{#each assignedOps as op (op.id)}
										<MissionOpChip
											callsign={op.callsign}
											status={op.status}
											color={statusColors[op.status]}
											removable={m.status !== 'complete'}
											onRemove={() => unassignOperator(m.id, op.id)}
										/>
									{/each}
									{#if hasNoOperators && m.status !== 'complete'}
										<span class="mission-unassigned">No operators assigned</span>
									{/if}
								</div>

								<!-- Linked annotations -->
								{#if missionAnns.length > 0}
									<div class="mission-annotations">
										{#each missionAnns as ann}
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
									<OperatorPicker
										candidates={$activeCheckIns}
										selectedIds={$activeCheckIns.filter((ci) => ci.missionIds?.includes(m.id)).map((ci) => ci.id)}
										mode="single"
										ariaLabel="Assign operator to {m.title}"
										onSelect={(ci) => assignOperator(m.id, ci.id)}
										onHover={(ci) => handleAssignPickerHover(m, ci)}
										onHoverEnd={() => hoveredCheckInId.set(null)}
										onClose={() => { assigningMissionId = null; }}
									/>
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
									<button class="bar-btn" class:active={assigningMissionId === m.id} aria-expanded={assigningMissionId === m.id} title="Assign operator" onclick={() => { assigningMissionId = assigningMissionId === m.id ? null : m.id; }}>+ Assign</button>
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
						{focusedAnnotationId}
						{onFocusConsumed}
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
					{#each [['all', 'All'], ['checkins', 'Check-ins'], ['assignments', 'Status'], ['missions', 'Missions'], ['notes', 'Notes'], ['rollcalls', 'Roll Calls'], ['weather', 'Weather']] as [value, label]}
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

	<!-- Close-net dialog. Inline by design: no new component file in this package.
	     Structure and geometry mirror BatchRemoveDialog. -->
	{#if showCloseDialog && $activeNet}
		<!-- A real <dialog> opened with showModal(): the UA supplies inertness,
		     the focus trap and Escape. data-blocks-escape keeps SidePanel's own
		     window-level Escape from closing the panel behind the dialog — a
		     native modal is implicitly aria-modal, so the attribute SidePanel
		     looks for first is not present. The dim is on ::backdrop, and the
		     dialog box itself is the only element, so a press that lands on the
		     dialog and nothing inside it is a backdrop press. -->
		<dialog
			class="cn-dialog"
			bind:this={closeDialogEl}
			use:modalDialog
			role="alertdialog"
			data-blocks-escape="true"
			aria-labelledby="close-net-title"
			aria-describedby="close-net-stakes"
			oncancel={(e) => { e.preventDefault(); if (!closePending) finishCloseDialog(false); }}
			onmousedown={(e) => { if (e.target === e.currentTarget && !closePending) finishCloseDialog(false); }}
		>
			<div class="cn-grab" aria-hidden="true"></div>

			<div class="cn-header">
				<span class="cn-title" id="close-net-title">End net</span>
				<button class="cn-x" onclick={() => finishCloseDialog(false)} disabled={closePending} aria-label="Close">&times;</button>
			</div>

			<div class="cn-body">
				<div class="cn-identity">
					<span class="cn-name">{$activeNet.name}</span>
					<span class="cn-sub">Open {elapsed || '—'} · NCS {$activeNet.ncsCallsign}</span>
				</div>

				<ul class="cn-stakes" id="close-net-stakes">
					{#if $netMetrics.totalIn > 0}
						<li class="cn-row">
							<span class="cn-icon" aria-hidden="true">!</span>
							<span>{$netMetrics.totalIn} operator{$netMetrics.totalIn === 1 ? ' is' : 's are'} still checked in. They will be checked out and the roster closes.</span>
						</li>
					{/if}
					{#if openMissionCount > 0}
						<li class="cn-row">
							<span class="cn-icon" aria-hidden="true">!</span>
							<span>{openMissionCount} mission{openMissionCount === 1 ? ' is' : 's are'} still open. They are recorded incomplete in the log.</span>
						</li>
					{/if}
					{#if openLocationCount > 0}
						<li class="cn-row">
							<span class="cn-icon" aria-hidden="true">!</span>
							<span>{openLocationCount} net location{openLocationCount === 1 ? '' : 's'} will be marked resolved or closed. This cannot be undone.</span>
						</li>
					{/if}
					<li class="cn-row">
						<span class="cn-icon" aria-hidden="true">▤</span>
						<span>The roster, the timeline and the ICS-309 log are kept. You can still export them after the net ends.</span>
					</li>
				</ul>

				<p class="cn-footnote">There is no “reopen net” in the app. Ending a net is final.</p>

				{#if closeError}<div class="cn-error" role="alert">{closeError}</div>{/if}
			</div>

			<div class="cn-footer">
				<button class="cn-btn" bind:this={closeCancelEl} onclick={() => finishCloseDialog(false)} disabled={closePending}>Keep net open</button>
				<button class="cn-btn cn-btn-danger" onclick={confirmCloseNet} disabled={closePending} aria-busy={closePending}>
					{closePending ? 'Ending…' : `End “${$activeNet.name}”`}
				</button>
			</div>
		</dialog>
	{/if}
</div>

<style>
	.net-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow-x: hidden;
		/* what3words display font — used by the location chip, the reverse
		   line, and the mission card's ///words. Promote to app.css if a
		   second component ever needs it. */
		--w3w-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
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
		min-width: 0;
	}

	.header-title-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.title {
		font-weight: 600;
		font-size: 0.95rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* The status chip owns the net's lifecycle: state, how long it has been in
	   that state, and the menu that changes it. */
	.net-state-chip {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		flex-shrink: 0;
		min-height: 34px;
		padding: 0 var(--space-sm);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		font-family: inherit;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text);
		cursor: pointer;
	}

	.net-state-chip.state-open { border-color: #22c55e; }
	.net-state-chip.state-open .chip-state { color: #22c55e; }
	.net-state-chip.state-closed .chip-state { color: var(--color-text-muted); }

	.chip-elapsed {
		font-family: monospace;
		font-weight: 600;
		text-transform: none;
		letter-spacing: 0;
		color: var(--color-text-muted);
	}

	.chip-caret {
		flex-shrink: 0;
		color: var(--color-text-muted);
	}

	.wx-header-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		flex-shrink: 0;
		height: 24px;
		padding: 0 8px;
		background: var(--color-wx-warning-soft);
		border: 1px solid var(--color-wx-warning);
		border-radius: var(--radius-full);
		font-size: 0.7rem;
		font-weight: 700;
		color: var(--color-wx-warning);
		cursor: pointer;
	}

	.wx-header-ttl {
		font-variant-numeric: tabular-nums;
		opacity: 0.85;
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
		gap: var(--space-sm);
		flex-shrink: 0;
		flex-wrap: wrap;
		justify-content: flex-end;
		align-items: center;
	}

	.action-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		min-width: 44px;
		height: 44px;
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

	/* The two frequent actions carry visible text: `title` is not a label on a
	   hoverless tablet. */
	.action-btn.labelled {
		width: auto;
		padding: 0 var(--space-sm);
		gap: var(--space-xs);
		color: var(--color-text);
	}

	.action-text {
		font-size: 0.8rem;
		font-weight: 600;
	}

	/* Popovers — one pattern, instantiated twice. position: fixed is required:
	   .sheet-content is overflow-y:auto and .side-panel is overflow:hidden. */
	.pop-menu {
		position: fixed;
		/* The UA stylesheet gives every [popover] inset:0 + margin:auto, which
		   would fight the anchored top/left set inline. */
		inset: auto;
		margin: 0;
		z-index: var(--z-overlay);
		min-width: 240px;
		max-width: min(280px, calc(100vw - 16px));
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg);
		padding: var(--space-xs);
		display: flex;
		flex-direction: column;
	}

	.pop-meta {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: var(--space-xs) var(--space-sm) var(--space-sm);
	}

	.pop-name {
		font-size: 0.85rem;
		font-weight: 600;
	}

	.pop-sub {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.pop-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		min-height: 44px;
		padding: 0 var(--space-sm);
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-family: inherit;
		font-size: 0.85rem;
		text-align: left;
		text-decoration: none;
		cursor: pointer;
	}

	.pop-row:hover,
	.pop-row:focus-visible {
		background: var(--color-primary);
	}

	.pop-row.ops-view-set { color: #22c55e; }

	.pop-row-danger { color: var(--color-error); }

	.pop-row-danger:hover,
	.pop-row-danger:focus-visible {
		background: rgba(231, 76, 60, 0.16);
	}

	.pop-hint {
		margin-left: auto;
		color: var(--color-text-muted);
	}

	/* Close-net dialog — same geometry as BatchRemoveDialog */
	.cn-dialog::backdrop {
		background: rgba(0, 0, 0, 0.6);
	}

	.cn-dialog {
		/* Reset the UA <dialog> box: 1em padding, a solid border and the
		   max-width/max-height that would otherwise fight the sizes below.
		   Centring is the UA's own inset:0 + margin:auto, left in place. */
		padding: 0;
		max-width: none;
		width: min(440px, 92vw);
		max-height: min(80vh, 620px);
		color: var(--color-text);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		overflow: hidden;
	}

	/* Qualified with [open]: an author `display` would beat the UA's
	   `dialog:not([open]) { display: none }` and render a closed dialog. */
	.cn-dialog[open] {
		display: flex;
		flex-direction: column;
	}

	.cn-grab {
		display: none;
	}

	.cn-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-md);
		border-bottom: 1px solid rgba(255, 255, 255, 0.06);
		flex-shrink: 0;
	}

	.cn-title {
		font-weight: 600;
		font-size: 0.9rem;
	}

	.cn-x {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.2rem;
		line-height: 1;
		cursor: pointer;
		padding: 4px 8px;
		border-radius: 4px;
	}

	.cn-x:hover:not(:disabled) {
		color: var(--color-text);
	}

	.cn-body {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.cn-identity {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.cn-name {
		font-size: 0.95rem;
		font-weight: 600;
	}

	.cn-sub {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.cn-stakes {
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		margin: 0;
		padding: 0;
	}

	.cn-row {
		display: flex;
		gap: var(--space-sm);
		font-size: 0.8rem;
		line-height: 1.4;
	}

	.cn-icon {
		flex-shrink: 0;
		width: 18px;
		text-align: center;
		color: var(--color-error);
		font-weight: 700;
	}

	.cn-footnote {
		font-size: 0.6875rem;
		color: var(--color-text-muted);
		margin: 0;
	}

	.cn-error {
		font-size: 0.75rem;
		line-height: 1.4;
		color: var(--color-error);
		padding: var(--space-sm);
		background: rgba(231, 76, 60, 0.1);
		border-radius: var(--radius-sm);
	}

	.cn-footer {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-sm);
		padding: var(--space-md);
		border-top: 1px solid rgba(255, 255, 255, 0.06);
		flex-shrink: 0;
	}

	.cn-btn {
		padding: 8px 14px;
		min-height: 44px;
		font-size: 0.8125rem;
		font-family: inherit;
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.12);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast), background var(--duration-fast);
	}

	.cn-btn:hover:not(:disabled) {
		border-color: rgba(255, 255, 255, 0.25);
		color: var(--color-text);
	}

	.cn-btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.cn-btn-danger {
		background: var(--color-error);
		border-color: var(--color-error);
		color: #fff;
		font-weight: 600;
	}

	.cn-btn-danger:hover:not(:disabled) {
		background: color-mix(in srgb, var(--color-error) 85%, black);
		border-color: color-mix(in srgb, var(--color-error) 85%, black);
		color: #fff;
	}

	/* After-action card: "Duration 3m · 0 check-ins" makes an accidental end
	   self-evident, where the panel used to just silently empty. */
	.summary-card {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		margin: var(--space-md);
		padding: var(--space-md);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
	}

	.summary-kicker {
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}

	.summary-name {
		font-size: 1rem;
		font-weight: 600;
	}

	.summary-grid {
		display: flex;
		gap: var(--space-lg);
	}

	.summary-stat {
		display: flex;
		flex-direction: column;
	}

	.summary-value {
		font-size: 1.1rem;
		font-weight: 700;
	}

	.summary-label {
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}

	.summary-actions {
		display: flex;
		gap: var(--space-sm);
		flex-wrap: wrap;
	}

	/* Tabs — one row, never wraps. Tabs share the width when it fits and
	   scroll horizontally when the panel is narrower. */
	.tabs {
		display: flex;
		align-items: stretch;
		gap: 2px;
		padding: 0 var(--space-xs);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
		overflow-x: auto;
		overflow-y: hidden;
		scrollbar-width: none;
		-webkit-overflow-scrolling: touch;
		scroll-snap-type: x proximity;
	}
	.tabs::-webkit-scrollbar { display: none; }

	.tab {
		flex: 1 0 auto;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		min-height: 44px;
		padding: 0 var(--space-sm);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		font-weight: 500;
		letter-spacing: 0.01em;
		white-space: nowrap;
		scroll-snap-align: start;
		cursor: pointer;
		transition: color var(--duration-fast), border-color var(--duration-fast);
	}

	.tab:hover { color: var(--color-text); }
	.tab:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
		border-radius: var(--radius-sm);
	}
	.tab.active {
		color: var(--color-text);
		border-bottom-color: var(--color-accent);
	}

	.tab-count {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 18px;
		height: 18px;
		padding: 0 5px;
		border-radius: var(--radius-full);
		background: var(--color-primary);
		color: var(--color-text-muted);
		font-size: 0.68rem;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		line-height: 1;
		transition: background var(--duration-fast), color var(--duration-fast);
	}
	.tab.active .tab-count {
		background: var(--color-accent);
		color: #fff;
	}

	.tab-count-alert,
	.tab.active .tab-count-alert {
		background: var(--color-error);
		color: #fff;
		animation: tab-alert-pulse 2s ease-in-out infinite;
	}
	@keyframes tab-alert-pulse {
		0%, 100% { box-shadow: 0 0 0 0 rgba(231, 76, 60, 0.55); }
		50% { box-shadow: 0 0 0 4px rgba(231, 76, 60, 0); }
	}
	@media (prefers-reduced-motion: reduce) {
		.tab-count-alert { animation: none; }
	}
	/* Phone widths: tighten so all five tabs fit without scrolling. */
	@media (max-width: 480px) {
		.tabs { gap: 0; padding: 0; }
		.tab { padding: 0 4px; gap: 3px; font-size: 0.75rem; }
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

	/* Command Palette (Quick Add) */
	.quick-add {
		position: relative;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.quick-add-wrap {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
	}

	.cmd-mode-badge {
		flex-shrink: 0;
		padding: 3px 8px;
		font-size: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #fff;
		border-radius: var(--radius-sm);
		white-space: nowrap;
		animation: cmd-badge-in 150ms ease-out;
	}

	@keyframes cmd-badge-in {
		from { opacity: 0; transform: scale(0.9); }
		to { opacity: 1; transform: scale(1); }
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
		min-width: 0;
	}

	.quick-add-input:focus {
		border-color: var(--color-accent);
	}

	.quick-add-input::placeholder {
		text-transform: none;
		color: var(--color-text-muted);
		font-size: 0.8rem;
	}

	.quick-add-hint {
		font-size: 0.6rem;
		color: var(--color-text-muted);
		padding: 2px var(--space-sm) 0;
		opacity: 0.7;
		letter-spacing: 0.01em;
		display: flex;
		align-items: center;
		gap: 4px;
		flex-wrap: wrap;
	}

	.hint-key {
		font-weight: 700;
		color: var(--color-accent);
	}

	.hint-sep {
		opacity: 0.4;
	}

	.hint-muted {
		color: var(--color-text-muted);
		opacity: 0.6;
	}

	.hint-kbd {
		display: inline-flex;
		align-items: center;
		padding: 0 4px;
		font-size: 0.55rem;
		font-family: inherit;
		color: var(--color-text-muted);
		background: var(--color-primary);
		border-radius: 3px;
		margin-left: 4px;
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
		display: flex;
		align-items: center;
		justify-content: center;
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
		max-height: 280px;
		overflow-y: auto;
	}

	.search-item {
		display: flex;
		flex-direction: column;
		gap: 2px;
		width: 100%;
		padding: 8px 12px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
		min-height: 36px;
		transition: background 80ms;
	}

	.search-item:hover,
	.search-item-active {
		background: var(--color-primary);
	}

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

	.op-status-bar.wx-warning {
		box-shadow: inset 0 -4px 0 var(--color-wx-warning);
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

	/* ---- Mission location field (task #92) ---- */
	.mission-loc-field { position: relative; display: flex; flex-direction: column; gap: var(--space-xs); }

	.loc-trigger {
		display: flex; align-items: center; gap: var(--space-sm);
		width: 100%; min-height: 44px; padding: 10px 12px;
		background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text-muted);
		font-size: 0.85rem; text-align: left; cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}
	.loc-trigger:hover { border-color: var(--color-text-muted); color: var(--color-text); }
	.loc-trigger .chev { margin-left: auto; opacity: 0.6; }
	.loc-trigger.picking {
		border-color: var(--color-accent); color: var(--color-accent);
		animation: locPulse 1.4s ease-in-out infinite;
	}
	@keyframes locPulse { 0%,100% { opacity: 1 } 50% { opacity: 0.55 } }

	/* ---- Selected chip ---- */
	.loc-chip {
		display: flex; align-items: center; gap: var(--space-sm);
		min-height: 44px; padding: 6px 8px 6px 10px;
		background: var(--color-bg);
		border: 1px solid var(--color-accent); border-radius: var(--radius-sm);
	}
	/* A what3words resolve hasn't been visually confirmed on the map yet —
	   the map fly-to is the safety check, not a nicety, so this stays
	   visually distinct until the NCS acknowledges it or creates the mission. */
	.loc-chip.unconfirmed {
		border-left: 2px solid var(--color-warning);
	}
	.loc-chip-main {
		display: flex; align-items: center; gap: var(--space-sm);
		flex: 1; min-width: 0; background: none; border: none; padding: 0;
		color: var(--color-text); text-align: left; cursor: pointer;
	}
	.loc-chip-icon { flex: 0 0 16px; color: var(--loc-cat-color, #6b7280); }
	.loc-chip-text { min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.loc-chip-label {
		font-size: 0.85rem; font-weight: 600; color: var(--color-text);
		white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
	}
	.loc-chip-label-w3w {
		font-family: var(--w3w-mono); font-size: 0.85rem; font-weight: 600;
		color: var(--color-text);
	}
	.w3w-slashes { color: var(--color-text-muted); }
	/* Plus Code / MGRS chip label (#94) — same treatment as the w3w mono
	   label so all three resolved-location sources read consistently. */
	.loc-chip-label-code {
		font-family: var(--w3w-mono); font-size: 0.85rem; font-weight: 600;
		letter-spacing: 0.02em; color: var(--color-text);
	}
	.loc-chip-coords {
		font-family: 'SF Mono','Fira Code',monospace; font-size: 0.7rem;
		color: var(--color-text-muted);
	}
	.loc-chip-confirm, .loc-chip-confirmed {
		flex: 0 0 auto; white-space: nowrap; font-size: 0.72rem; font-weight: 600;
	}
	.loc-chip-confirm {
		padding: 5px 10px; background: none; border: 1px solid var(--color-warning);
		border-radius: var(--radius-full); color: var(--color-warning); cursor: pointer;
		transition: background var(--duration-fast);
	}
	.loc-chip-confirm:hover { background: rgba(245, 158, 11, 0.12); }
	.loc-chip-confirmed { color: var(--color-success); padding: 5px 4px; }
	.loc-chip-map, .loc-chip-clear {
		flex: 0 0 auto; width: 32px; height: 32px;
		display: inline-flex; align-items: center; justify-content: center;
		background: none; border: none; border-radius: var(--radius-sm);
		color: var(--color-text-muted); cursor: pointer;
		transition: color var(--duration-fast), background var(--duration-fast);
	}
	.loc-chip-map:hover, .loc-chip-clear:hover { color: var(--color-text); background: rgba(255,255,255,0.06); }

	/* ---- Reverse row (coords -> Plus Code / MGRS / ///words) under any
	   chosen location (#94 §4) — up to three compact tokens, one per format,
	   self-suppressing the token matching the current source. ---- */
	.loc-reverse {
		display: flex; flex-wrap: wrap; align-items: center;
		gap: 4px var(--space-md); padding: 2px 2px 0 10px;
	}
	.loc-reverse-token { display: inline-flex; align-items: center; gap: 4px; min-width: 0; }
	.loc-reverse-kind {
		font-size: 0.62rem; letter-spacing: 0.06em; text-transform: uppercase;
		color: var(--color-text-muted); flex: none;
	}
	.loc-reverse-value {
		font-family: 'SF Mono','Fira Code',monospace; font-size: 0.72rem;
		color: var(--color-text-muted);
		overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
	}
	.loc-reverse-value-w3w { font-family: var(--w3w-mono); }
	.loc-reverse-copy {
		display: inline-flex; align-items: center; justify-content: center;
		min-width: 28px; min-height: 28px; padding: 2px 6px;
		background: none; border: none; border-radius: var(--radius-sm);
		color: var(--color-text-muted); cursor: pointer; font-size: 0.72rem;
		transition: color var(--duration-fast), background var(--duration-fast);
	}
	.loc-reverse-copy:hover { color: var(--color-text); background: rgba(255,255,255,0.06); }

	/* ---- Popover ---- */
	.loc-popover {
		position: absolute; top: 100%; left: 0; right: 0; margin-top: 2px;
		z-index: 10; display: flex; flex-direction: column;
		background: var(--color-bg); border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); box-shadow: var(--shadow-md);
		max-height: 260px; overflow: hidden;
	}
	.loc-search {
		border: none !important; border-bottom: 1px solid var(--color-primary) !important;
		border-radius: 0 !important; background: var(--color-surface) !important;
	}
	.loc-list { overflow-y: auto; flex: 1; }
	.loc-opt {
		display: flex; align-items: center; gap: var(--space-sm);
		width: 100%; min-height: 40px; padding: 8px 10px;
		background: none; border: none; color: var(--color-text);
		font-size: 0.8rem; text-align: left; cursor: pointer;
		transition: background var(--duration-fast);
	}
	.loc-opt:hover, .loc-opt.highlight { background: rgba(255,255,255,0.06); }
	.loc-opt-map { color: var(--color-accent); font-weight: 600; border-bottom: 1px solid var(--color-primary); }
	.loc-opt-icon { flex: 0 0 16px; color: var(--loc-cat-color, #6b7280); }
	.loc-opt-label { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.loc-opt-short {
		font-family: 'SF Mono','Fira Code',monospace; font-size: 0.7rem;
		color: var(--color-text-muted); padding: 1px 5px;
		background: rgba(255,255,255,0.06); border-radius: 3px;
	}
	.loc-empty { padding: var(--space-md); font-size: 0.78rem; color: var(--color-text-muted); text-align: center; }
	.loc-coords { border-top: 1px solid var(--color-primary); padding: var(--space-sm) 10px; }
	.loc-coords summary { font-size: 0.75rem; color: var(--color-text-muted); cursor: pointer; list-style: none; }
	.loc-coords summary::marker { content: ''; }

	/* ---- what3words group in the popover ---- */
	.loc-group-header {
		padding: 6px 10px 2px; font-size: 0.7rem; letter-spacing: 0.04em;
		text-transform: uppercase; color: var(--color-text-muted);
	}
	.loc-opt-w3w { flex-direction: column; align-items: flex-start; gap: 2px; min-height: 44px; }
	.loc-opt-w3w-words { font-family: var(--w3w-mono); font-size: 0.82rem; color: var(--color-text); }
	.loc-opt-w3w-meta { font-size: 0.72rem; color: var(--color-text-muted); }
	.loc-w3w-status {
		padding: 8px 10px; font-size: 0.78rem; color: var(--color-text-muted);
	}
	.loc-w3w-error { color: var(--color-warning); }

	/* ---- Offline geocode resolve row + error (#94) ---- */
	.loc-opt-geocode { align-items: center; gap: var(--space-sm); min-height: 44px; }
	.loc-opt-geocode-icon { flex: 0 0 14px; color: var(--color-accent); }
	.loc-opt-geocode-text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.loc-opt-geocode-line1 {
		font-size: 0.8rem; color: var(--color-text);
		white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
	}
	.loc-opt-geocode-code {
		font-family: 'SF Mono','Fira Code',monospace; font-weight: 600;
		letter-spacing: 0.02em; margin-left: 4px;
	}
	.loc-opt-geocode-line2 { font-size: 0.7rem; color: var(--color-text-muted); }
	.loc-opt-geocode-coords {
		flex: 0 0 auto; font-family: 'SF Mono','Fira Code',monospace; font-size: 0.68rem;
		color: var(--color-text-muted); font-variant-numeric: tabular-nums;
	}
	.loc-status {
		padding: 8px 10px; font-size: 0.78rem; color: var(--color-text-muted); line-height: 1.4;
	}
	.loc-status-error { color: var(--color-warning); }
	.loc-w3w-hint {
		padding: var(--space-sm) 10px; border-bottom: 1px solid var(--color-primary);
		font-size: 0.76rem; color: var(--color-text-muted); line-height: 1.4;
	}
	.loc-w3w-hint p { margin: 0 0 2px; }
	.loc-w3w-hint-title { color: var(--color-text); font-weight: 600; }
	.loc-w3w-hint-link {
		background: none; border: none; padding: 0; color: var(--color-accent);
		font-size: inherit; cursor: pointer; text-decoration: underline;
	}

	/* ---- Priority chips (same geometry as .annotation-chip) ---- */
	.mission-priority-group { display: flex; gap: 6px; }
	.priority-chip {
		flex: 1; min-height: 40px; padding: 6px 10px;
		background: none; border: 1px solid var(--color-primary);
		border-radius: var(--radius-full); color: var(--color-text-muted);
		font-size: 0.75rem; font-weight: 600; cursor: pointer;
		transition: all var(--duration-fast);
	}
	.priority-chip:hover { border-color: var(--color-text-muted); color: var(--color-text); }
	.priority-chip[aria-checked='true'] { color: var(--color-text); }
	.priority-chip.priority-routine[aria-checked='true']   { border-color: var(--color-text-muted); background: rgba(255,255,255,0.06); }
	.priority-chip.priority-priority[aria-checked='true']  { border-color: var(--color-warning); color: var(--color-warning); }
	.priority-chip.priority-welfare[aria-checked='true']   { border-color: var(--color-success); color: var(--color-success); }
	.priority-chip.priority-emergency[aria-checked='true'] { border-color: var(--color-error);   color: var(--color-error); background: rgba(233,69,96,0.08); }

	/* ---- Details disclosure ---- */
	.details-toggle {
		display: flex; align-items: center; gap: var(--space-sm);
		min-height: 36px; padding: 6px 2px;
		background: none; border: none; color: var(--color-text-muted);
		font-size: 0.75rem; cursor: pointer;
	}
	.details-toggle:hover { color: var(--color-text); }
	.details-badge { color: var(--color-accent); }
	.mission-details { display: flex; flex-direction: column; gap: var(--space-sm); }

	.btn-mini {
		flex: 0 0 auto; min-height: 32px; padding: 4px 10px;
		background: none; border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm); color: var(--color-text-muted);
		font-size: 0.75rem; cursor: pointer;
		transition: all var(--duration-fast);
	}
	.btn-mini:hover { border-color: var(--color-accent); color: var(--color-text); }

	/* ---- Feedback ---- */
	.form-error { font-size: 0.75rem; color: var(--color-error); }
	.sr-live { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.form-actions.submitting { pointer-events: none; }

	@media (max-width: 768px) {
		.loc-popover { max-height: min(320px, 40vh); }
		.loc-opt-w3w { min-height: 44px; }
		.loc-opt-w3w-meta { font-size: 0.72rem; }
		.mission-priority-group { flex-wrap: wrap; }
		.priority-chip { flex: 1 1 45%; }
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
	.mission-form textarea {
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

	.mission-assign-field {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
	}

	.mission-assign-label {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
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

	/* A w3w-sourced mission is instantly identifiable in a scrolling list —
	   the one place the brand accent appears on the mission card. */
	.w3w-mark { color: var(--color-accent); font-family: var(--w3w-mono); }
	.w3w-words { font-family: var(--w3w-mono); color: var(--color-text); }
	.w3w-near { color: var(--color-text-muted); font-size: 0.75rem; }
	/* Offline-geocode (Plus Code / MGRS) mission card treatment (#94) —
	   matching visual weight to the w3w treatment above, minus the accent
	   colour (that stays w3w's one place to appear). */
	.geocode-code {
		font-family: 'SF Mono','Fira Code',monospace; color: var(--color-text);
		letter-spacing: 0.02em;
	}

	/* Assigned operators on mission card */
	.mission-operators {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		margin-top: 4px;
	}

	/* Chip visuals live in MissionOpChip.svelte — the create form and the
	   mission card render the same component so they cannot drift. */

	.mission-unassigned {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	/* Shell for the annotation-link picker, the one picker left in this file.
	   OperatorPicker/MissionPicker carry their own copy — Svelte scoped styles
	   cannot be shared, and hoisting these to app.css would leak globals. */
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

	/* Narrow panel: stack the header so the action row left-aligns and never
	   sits under the mobile toolbar's FAB column. */
	@media (max-width: 768px) {
		.panel-header {
			flex-direction: column;
			align-items: stretch;
			gap: var(--space-sm);
		}

		.header-actions {
			justify-content: flex-start;
		}
	}

	@media (max-width: 640px) {
		/* Bottom sheet. The dialog is in the top layer, so `fixed` resolves
		   against the viewport even though every shell ancestor is transformed. */
		.cn-dialog {
			width: 100%;
			position: fixed;
			inset: auto 0 0 0;
			margin: 0;
			max-height: 80vh;
			overflow-y: auto;
			border-radius: var(--radius-lg) var(--radius-lg) 0 0;
			border-bottom: none;
			padding-bottom: max(var(--space-md), env(safe-area-inset-bottom));
		}

		.cn-grab {
			display: block;
			width: 36px;
			height: 4px;
			margin: var(--space-sm) auto 0;
			border-radius: var(--radius-full);
			background: rgba(255, 255, 255, 0.2);
		}

		/* Danger sits at the thumb; DOM order unchanged so Cancel keeps focus. */
		.cn-footer {
			flex-direction: column-reverse;
		}

		.cn-btn {
			width: 100%;
		}
	}

</style>
