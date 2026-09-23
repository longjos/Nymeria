<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import { browser } from '$app/environment';
	import Map from '$lib/components/Map.svelte';
	import Toolbar from '$lib/components/Toolbar.svelte';
	import ActivityRail from '$lib/components/ActivityRail.svelte';
	import SidePanel from '$lib/components/SidePanel.svelte';
	import BottomSheet from '$lib/components/BottomSheet.svelte';
	import StationList from '$lib/components/StationList.svelte';
	import StationDetail from '$lib/components/StationDetail.svelte';
	import ConvoList from '$lib/components/ConvoList.svelte';
	import TransportPanel from '$lib/components/TransportPanel.svelte';
	import ActivityPanel from '$lib/components/ActivityPanel.svelte';
	import AnnotationPanel from '$lib/components/AnnotationPanel.svelte';
	import NetControlPanel from '$lib/components/NetControlPanel.svelte';
	import BulletinPanel from '$lib/components/BulletinPanel.svelte';
	import ICS309Panel from '$lib/components/ICS309Panel.svelte';
	import WeatherPanel from '$lib/components/WeatherPanel.svelte';
	import TelemetryPanel from '$lib/components/TelemetryPanel.svelte';
	import DFPanel from '$lib/components/DFPanel.svelte';
	import PacketInspector from '$lib/components/PacketInspector.svelte';
	import SettingsPanel from '$lib/components/SettingsPanel.svelte';
	import ConnectionStatus from '$lib/components/ConnectionStatus.svelte';
	import SearchOverlay from '$lib/components/SearchOverlay.svelte';
	import LoginOverlay from '$lib/components/LoginOverlay.svelte';
	import SetupWizard from '$lib/components/SetupWizard.svelte';
	import CommandPalette from '$lib/components/CommandPalette.svelte';
	import GpsFollowControl from '$lib/components/GpsFollowControl.svelte';
	import GpsStatusPill from '$lib/components/GpsStatusPill.svelte';
	import WxLinkPill from '$lib/components/WxLinkPill.svelte';
	import WxInterruptBanner from '$lib/components/WxInterruptBanner.svelte';
	import RideInterruptBanner from '$lib/components/RideInterruptBanner.svelte';
	import RideStrip from '$lib/components/RideStrip.svelte';
	import SagDock from '$lib/components/ride/SagDock.svelte';
	import SagCandidatePanel from '$lib/components/ride/SagCandidatePanel.svelte';
	import RidePeek from '$lib/components/RidePeek.svelte';
	import RideShortcutHelp from '$lib/components/RideShortcutHelp.svelte';
	import RidePendingList from '$lib/components/RidePendingList.svelte';
	import ShiftBriefing from '$lib/components/ShiftBriefing.svelte';
	import NextStopPill from '$lib/components/NextStopPill.svelte';
	import type { MeasureBand, DistanceOrigin } from '$lib/components/NextStopPill.svelte';
	import { STOP_CATEGORIES } from '$lib/routeDistance';
	import { stations, stationList, initStationStore, wsClient, connectWS } from '$lib/stores/stations';
	import { initMessageStore, conversationList } from '$lib/stores/messages';
	import { initTransportStore } from '$lib/stores/transports';
	import { gpsStatus, gpsFollow, gpsAgeMs, gpsHasFix, initGpsStore } from '$lib/stores/gps';
	import { showToast } from '$lib/stores/toast';
	import { annotationList, initAnnotationStore } from '$lib/stores/annotations';
	import { api, ApiError } from '$lib/api';
	import {
		initNetControlStore,
		activeNet, operatorsWithPosition, missionsWithPosition, assignmentLines,
		opsView, hoveredMissionId, hoveredCheckInId,
		netLocationAnnotations
	} from '$lib/stores/netcontrol';
	import { initTacticalStore } from '$lib/stores/tactical';
	import { initBulletinStore } from '$lib/stores/bulletins';
	import { initWeatherStore, weatherStations, selectedWeatherStation } from '$lib/stores/weather';
	import {
		initWxAlertStore, wxInAreaAlerts, wxNearbyAlerts, wxZoneGeometry, wxLinkStatus,
		wxFootprint, wxFootprintPreview, wxFocusAlertId, wxSelectedAlertId, openWxAlert, wxMinute,
		wxAlertsById
	} from '$lib/stores/wxAlerts';
	import { tierMeta } from '$lib/wxAlertMeta';
	import { countdown } from '$lib/wxAlertTime';
	import WxTierGlyph from '$lib/components/WxTierGlyph.svelte';
	import { dfStations } from '$lib/stores/df';
	import { loadW3WStatus } from '$lib/stores/w3w';
	import { initPacketStore } from '$lib/stores/packets';
	import { initPathStore } from '$lib/stores/paths';
	import { isLoggedIn, needsSetup, isApproved, isPending, isDenied, initSession, handleSessionEvent, currentUser, loadPendingRequests, canAdmin } from '$lib/stores/session';
	import { mapSettings, AGE_FILTER_MS, TRACK_DURATION_MS, toggleSagFocusPreset } from '$lib/stores/mapSettings';
	import {
		rosterCallsigns, netIsOpen, rosterMappedCount, rosterFilterActive, isRosterStation,
		rosterScopedStations
	} from '$lib/stores/rosterScope';
	import MapPalette from '$lib/components/MapPalette.svelte';
	import {
		selectedStation, panelMode, detailTab, searchOpen, sheetState,
		selectStation, closePanel, openStationList, openMessages, openConversation, openTransports, openActivity, openAnnotations, openNetControl, openBulletins, openICS309, openWeather, openTelemetry, openDF, openPackets, openSettings,
		togglePanel, commandPaletteOpen, toggleCommandPalette, rideShortcutHelpOpen, openSag
	} from '$lib/stores/ui';
	import type { SheetState, DetailTab, PanelMode } from '$lib/stores/ui';
	import type { Annotation } from '$lib/types';
	import { activeInterruptSource } from '$lib/stores/interrupts';
	import {
		rideMode, initRideStore, rideEmergency, rideLadder,
		ackEmergency, seedPalette, upsertSagRequest
	} from '$lib/stores/ride';
	import {
		sagDockActive, sagFocus, sagOverlayActive, sagPlaced, sagVehiclesOnMap,
		sagCandidates, sagLegLines, sagFocusedVehicleId, sagVehicleGeo,
		focusSagRequest, focusSagVehicle, startDispatchFocus, clearSagFocus
	} from '$lib/stores/sagMap';
	import { padSinglePoint } from '$lib/sagDockModel';

	/** The desktop SAG dock's width, and the tablet rail's. Shared with the
	 *  map-fit padding so a `flyToBounds` can never put a pickup behind it. */
	const SAG_DOCK_W = 260;
	const SAG_RAIL_W = 44;

	let isDesktop = $state(true);
	let isTablet = $state(false);
	/** The tablet rail's state. Desktop never collapses unless the operator
	 *  asks; the tablet starts collapsed. */
	let sagDockCollapsed = $state(false);
	/** Landscape phone (spec §9: max-height: 499px) — same query as
	 * `.desktop-only`/BottomSheet's SHORT_VH_BREAKPOINT. RidePeek is
	 * suppressed there entirely rather than squeezed into an even shorter sheet. */
	let isShortViewport = $state(false);
	let flyToTarget = $state<{ lat: number; lon: number; zoom?: number } | null>(null);
	let flyToBounds = $state<Array<{ lat: number; lon: number }> | null>(null);
	let sessionReady = $state(false);
	let drawingMode = $state<'point' | 'line' | 'area' | null>(null);
	let previewGeometry = $state<string | null>(null);
	let previewColor = $state('#e63946');
	let annotationPanelRef = $state<AnnotationPanel>();
	let mapRef = $state<Map>();
	let editingAnnotationId = $state<string | null>(null);
	let focusedAnnotationId = $state<string | null>(null);
	let placingOperator = $state<{ id: string; callsign: string } | null>(null);
	let placingAnnotation = $state<{ id: string | null; name: string; mode: 'update' | 'form' } | null>(null);
	let annotationMapCoords = $state<{ lat: number; lon: number } | null>(null);
	let placingMissionLocation = $state<{ label: string } | null>(null);
	let missionDraftPoint = $state<{ lat: number; lon: number } | null>(null);
	let missionMapCoords = $state<{ lat: number; lon: number; label?: string } | null>(null);
	let missionPickAnnotation = $state<{ id: string; lat: number; lon: number } | null>(null);
	let sheetBeforePick: SheetState | null = null;

	// --- Next Stop (along-course distance) ---------------------------------
	// null origin means "measure from my own GPS fix"; a long press or the
	// station Distance button replaces it with an explicit point.
	let distanceOrigin = $state<{ lat: number; lon: number; label: string; bearingDeg?: number } | null>(null);
	/** True only for a long-pressed bare coordinate — the draggable pin case. */
	let distancePinned = $state(false);
	let nextStopExpanded = $state(false);
	let measureBand = $state<MeasureBand | null>(null);
	let showNextStop = $state(true);

	// Roster membership, net-open state and the roster scope now live in
	// $lib/stores/rosterScope — the same scope the station list, inline search and
	// command palette read, so a filtered map can never disagree with them (#106).
	let allStationsWithPosition = $derived(
		$stationList.filter((s) => s.position)
	);

	let stationsWithPosition = $derived.by(() => {
		const cutoffMs = AGE_FILTER_MS[$mapSettings.stationAgeFilter];
		const rosterOnly = $rosterFilterActive;
		if (cutoffMs === Infinity && !rosterOnly) return allStationsWithPosition;
		const now = Date.now();
		const sel = $selectedStation;
		return allStationsWithPosition.filter((s) => {
			const key = s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
			// Always show selected station — it would otherwise vanish out from
			// under the detail panel the user has open on it.
			if (key === sel) return true;
			// Always show roster stations (operators and their tracked devices)
			if (isRosterStation($rosterCallsigns, s)) return true;
			// Roster-only hides everything else outright, age irrelevant
			if (rosterOnly) return false;
			const age = now - new Date(s.lastHeard).getTime();
			return age <= cutoffMs;
		});
	});

	let totalUnread = $derived(
		$conversationList.reduce((sum, c) => sum + c.unreadCount, 0)
	);

	let panelIsOpen = $derived($panelMode !== 'closed');

	let wsConnected = $state(false);

	// Connect WS when logged in (even for pending — needed for approval events)
	$effect(() => {
		if ($isLoggedIn && $currentUser && !wsConnected) {
			connectWS($currentUser.token);
			wsClient.on('access_approved', handleSessionEvent);
			wsClient.on('access_denied', handleSessionEvent);
			wsClient.on('access_request', handleSessionEvent);
			wsConnected = true;
		}
	});

	// Init data stores only when approved
	$effect(() => {
		if ($isApproved) {
			initStationStore();
			initPathStore();
			initMessageStore();
			initTransportStore();
			initGpsStore();
			initAnnotationStore();
			initNetControlStore();
			initTacticalStore();
			initBulletinStore();
			initWeatherStore();
			initWxAlertStore();
			initRideStore();
			initPacketStore();
			loadW3WStatus();
			if ($canAdmin) {
				loadPendingRequests();
			}
		}
	});

	// Synchronous on purpose: Svelte only honours the returned destroy callback
	// for a sync onMount, so the async version silently dropped it and leaked
	// the matchMedia listener on every unmount.
	onMount(() => {
		void (async () => {
			await initSession();
			sessionReady = true;
		})();

		if (!browser) return;
		try {
			showNextStop = localStorage.getItem('nymeria_next_stop_pill') !== '0';
		} catch {
			// default on
		}
		// P0-3: width alone put a landscape phone (e.g. 852x393) on the desktop
		// layout — SidePanel + ActivityRail have no design for a 393px-tall
		// viewport. Desktop now requires height too; keep this in sync with the
		// (min-width: 769px) and (min-height: 500px) breakpoints in app.css and
		// the SHORT_VH_BREAKPOINT in BottomSheet.svelte.
		const mq = window.matchMedia('(min-width: 769px) and (min-height: 500px)');
		isDesktop = mq.matches;
		const handler = (e: MediaQueryListEvent) => { isDesktop = e.matches; };
		mq.addEventListener('change', handler);

		// The spec's tablet band: desktop layout, but a touch device whose map
		// cannot spare 260px to a dock that is idle most of the ride.
		const tabletMq = window.matchMedia('(min-width: 769px) and (max-width: 1199px) and (min-height: 500px)');
		isTablet = tabletMq.matches;
		const tabletHandler = (e: MediaQueryListEvent) => {
			isTablet = e.matches;
			// Collapse on the way in, expand on the way out: the rail is the
			// tablet's resting state, the full dock is the desktop's.
			sagDockCollapsed = e.matches;
		};
		tabletMq.addEventListener('change', tabletHandler);
		sagDockCollapsed = tabletMq.matches;

		const shortMq = window.matchMedia('(max-height: 499px)');
		isShortViewport = shortMq.matches;
		const shortHandler = (e: MediaQueryListEvent) => { isShortViewport = e.matches; };
		shortMq.addEventListener('change', shortHandler);

		return () => {
			mq.removeEventListener('change', handler);
			tabletMq.removeEventListener('change', tabletHandler);
			shortMq.removeEventListener('change', shortHandler);
		};
	});

	// Auto-enable overlay when its panel opens (convenience)
	$effect(() => {
		if ($panelMode === 'weather' && !$mapSettings.showWeatherOverlay) {
			mapSettings.update((s) => ({ ...s, showWeatherOverlay: true }));
		}
		if ($panelMode === 'df' && !$mapSettings.showDFOverlay) {
			mapSettings.update((s) => ({ ...s, showDFOverlay: true }));
		}
	});

	function handleStationClick(key: string) {
		const st = $stations.get(key);
		// Weather stations → open the weather panel detail instead
		if (st?.weather) {
			selectedWeatherStation.set(key);
			openWeather();
			return;
		}
		selectStation(key);
		if (st?.position) {
			flyToTarget = { lat: st.position.lat, lon: st.position.lon };
		}
	}

	function handleSearchSelect(key: string) {
		selectStation(key);
		searchOpen.set(false);
		const st = $stations.get(key);
		if (st?.position) {
			flyToTarget = { lat: st.position.lat, lon: st.position.lon };
		}
	}

	function handleFlyTo(lat: number, lon: number) {
		flyToTarget = { lat, lon };
	}

	function handleTabChange(tab: DetailTab) {
		detailTab.set(tab);
	}

	function handleSheetStateChange(s: SheetState) {
		sheetState.set(s);
	}

	function handleConvoSelect(callsign: string) {
		openConversation(callsign);
	}

	function handleAnnotationClick(id: string) {
		const ann = $annotationList.find((a) => a.id === id);
		const inActiveNet = !!ann && !!$activeNet && ann.netId === $activeNet.id;
		// Only stay in Net Control when the user is already there — never
		// hijack the default annotation flow.
		if (inActiveNet && $panelMode === 'netcontrol') {
			sheetState.set('half');
		} else {
			openAnnotations();
		}
		focusedAnnotationId = id;
	}

	function handleNetOperatorClick(_checkInId: string) {
		openNetControl();
	}

	function handleNetMissionClick(_missionId: string) {
		openNetControl();
	}

	/**
	 * Clicking a pickup pin goes STRAIGHT to dispatch focus rather than merely
	 * selecting the request (spec §6 step 1). There is exactly one reason an
	 * operator clicks a pickup on the map, and making them click twice to reach
	 * it would be asking them to confirm their own intention.
	 */
	function handleSagPickupClick(requestId: string) {
		startDispatchFocus(requestId);
	}

	/** A chit click selects the vehicle's request when it has one, so the
	 *  operator can see what it is carrying; otherwise it opens the roster. */
	/**
	 * `Place on map` — the dock's repair for a request the resolver could not
	 * place. The map is not just a display for this feature; it is the tool
	 * that supplies the coordinate the radio call never carried.
	 */
	let placingSagLocation = $state<{ requestId: string; which: 'pickup' | 'dropoff'; label: string } | null>(null);

	function handleSagPlaceRequest(requestId: string, which: 'pickup' | 'dropoff') {
		const p = $sagPlaced.find((x) => x.request.id === requestId);
		placingSagLocation = { requestId, which, label: p ? `SAG ${p.request.sequence}` : 'request' };
	}

	async function handleSagLocationPlaced(
		requestId: string,
		which: 'pickup' | 'dropoff',
		lat: number,
		lon: number
	) {
		const p = $sagPlaced.find((x) => x.request.id === requestId);
		placingSagLocation = null;
		const netId = $activeNet?.id;
		if (!p || !netId) return;
		const r = p.request;
		try {
			const updated = await api.updateSagRequest(netId, r.id, {
				pickup: which === 'pickup' ? { ...r.pickup, lat, lon } : r.pickup,
				dropoff: which === 'dropoff' ? { ...r.dropoff, lat, lon } : r.dropoff,
				reason: r.reason,
				priority: r.priority,
				notes: r.notes,
				requestedBy: r.requestedBy
			});
			upsertSagRequest(updated);
			showToast(`SAG ${updated.sequence} ${which} placed`, 'success');
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : `Could not place the ${which}`, 'error');
		}
	}

	/**
	 * A chit click selects the VEHICLE (spec §7, row 2), not the request it
	 * happens to be working. The operator is asking "what is this van doing",
	 * and the answer is the selection ring plus that van's own leg lines —
	 * which is why `SagFocus` carries a vehicle at all.
	 */
	function handleSagVehicleClick(checkInId: string) {
		focusSagVehicle(checkInId);
	}

	/** `Place on map` for a SAG unit that has never reported a position. It is
	 *  the same operator-placement pick mode the roster already uses — the dock
	 *  asks for picking, it never implements it. */
	function handleSagPlaceVehicle(checkInId: string) {
		const g = $sagVehicleGeo.find((v) => v.vehicle.checkInId === checkInId);
		handlePlaceOperator(checkInId, g?.vehicle.tacticalCall || g?.vehicle.callsign || 'unit');
	}

	function handleNetFlyTo(lat: number, lon: number, zoom?: number) {
		flyToTarget = { lat, lon, zoom: zoom ?? 15 };
	}

	// A resolved-geocode draft pin (what3words, Plus Code, or MGRS — #93/#94):
	// place it on the map without touching the existing missionMapCoords path
	// — that path (see handleMissionLocationPlaced) flows back into
	// NetControlPanel's own effect, which overwrites the location label with
	// a nearby annotation's label. A resolve must never lose the words/code
	// that way, so it only ever sets the marker.
	function handleSetMissionDraftPoint(lat: number, lon: number) {
		missionDraftPoint = { lat, lon };
	}

	function handleGetMapCenter(): { lat: number; lon: number; zoom: number } | null {
		return mapRef?.getViewport() ?? null;
	}

	function handleFlyToBounds(coords: Array<{ lat: number; lon: number }>) {
		flyToBounds = coords;
		setTimeout(() => { flyToBounds = null; }, 100);
	}

	/**
	 * The SAG dock's zoom-to-extent (sag-dock-design.md S5): a job, a vehicle,
	 * or all SAG. Goes through the same bounds fit + `sagFitPadding` as
	 * dispatch focus, so the targets land clear of the dock, rail or sheet. A
	 * lone point is padded to a small box so the map does not slam to max
	 * zoom. On a phone a full-height sheet would hide the answer, so it drops
	 * to `half` — the snap the fit padding assumes.
	 */
	function handleSagFit(pts: Array<{ lat: number; lon: number }>) {
		if (pts.length === 0) return;
		if (!isDesktop && get(sheetState) === 'full') sheetState.set('half');
		handleFlyToBounds(padSinglePoint(pts));
	}

	/**
	 * The insets the next map fit has to keep clear. Everything SAG puts on
	 * screen — the desktop dock, the tablet rail's overlay, the phone sheet —
	 * floats OVER the map rather than shrinking it, so a fit that only knows
	 * the map's box hides the pickup behind the very panel that asked for it.
	 * The sheet number is the same `half` snap the sheet itself uses.
	 */
	/** Width the SAG dock takes out of the map's left edge right now; 0 when
	 *  no dock is shown. The one number the fit padding and the dock's own
	 *  slot in the map HUD both read, so they cannot disagree. */
	let sagDockWidth = $derived(
		!(isDesktop && $sagDockActive)
			? 0
			: sagDockCollapsed && $sagFocus?.mode !== 'dispatch'
				? SAG_RAIL_W
				: SAG_DOCK_W
	);

	/**
	 * The map HUD's bottom edge, published as --map-hud-bottom on :root.
	 *
	 * The map's own "click the map to place …" hints sit top-centre. On a
	 * phone the HUD column spans most of that width, and the hints — inside
	 * the map layer, beneath the HUD — were hidden behind the GPS chip and the
	 * next-stop readout at exactly the moment the operator needed to read
	 * them. Map.svelte drops them below this edge on narrow screens.
	 */
	let hudEl = $state<HTMLDivElement>();
	$effect(() => {
		const el = hudEl;
		if (!el) return;
		const root = document.documentElement;
		const publish = () => root.style.setProperty('--map-hud-bottom', `${Math.round(el.getBoundingClientRect().bottom)}px`);
		const ro = new ResizeObserver(publish);
		ro.observe(el);
		publish();
		return () => {
			ro.disconnect();
			root.style.removeProperty('--map-hud-bottom');
		};
	});

	let sagFitPadding = $derived(
		isDesktop && $sagDockActive
			? { top: 0, right: 0, bottom: 0, left: sagDockWidth + 16 }
			: !isDesktop && $panelMode === 'sag'
				? { top: 0, right: 0, bottom: Math.round(window.innerHeight * 0.5), left: 0 }
				: null
	);

	/**
	 * Dispatch focus fits the map to the pickup plus the top three candidates
	 * (spec §6 step 2) — the whole reason the decision happens on a map rather
	 * than in a list, and worthless if the answer is off screen.
	 *
	 * Keyed on the request id so it fires ONCE per entry into dispatch focus.
	 * `sagCandidates` recomputes every second on the shared clock; refitting on
	 * every tick would be a map that will not hold still under an operator who
	 * is trying to read it.
	 */
	let lastDispatchFit = '';
	$effect(() => {
		const f = $sagFocus;
		const key = f?.mode === 'dispatch' ? (f.requestId ?? '') : '';
		if (key === lastDispatchFit) return;
		lastDispatchFit = key;
		if (!key) return;
		const p = $sagPlaced.find((x) => x.request.id === key);
		// An unplaceable pickup moves the map NOWHERE. A map that jumps to
		// somewhere it cannot justify is worse than a map that stays put.
		if (!p?.pickup.placed) return;
		const pts = [{ lat: p.pickup.lat, lon: p.pickup.lon }];
		for (const c of ($sagCandidates?.ranked ?? []).slice(0, 3)) {
			const g = $sagVehiclesOnMap.find((v) => v.vehicle.checkInId === c.id);
			if (g?.lat != null && g.lon != null) pts.push({ lat: g.lat, lon: g.lon });
		}
		handleFlyToBounds(pts);
	});

	/** On a phone there is no dock, so dispatch focus opens the sheet's `sag`
	 *  mode at `half` and the candidate list renders there (spec §11). */
	$effect(() => {
		if (isDesktop) return;
		if ($sagFocus?.mode !== 'dispatch') return;
		if (get(panelMode) !== 'sag') openSag();
		if (get(sheetState) === 'peek') sheetState.set('half');
	});

	async function handleSetOpsView() {
		const vp = mapRef?.getViewport();
		if (vp && $activeNet) {
			opsView.set(vp);
			try {
				await api.setOpsView($activeNet.id, vp.lat, vp.lon, vp.zoom);
			} catch { /* best-effort persist */ }
		}
	}

	function handleGoToOpsView() {
		const v = $opsView;
		if (v) {
			flyToTarget = { lat: v.lat, lon: v.lon, zoom: v.zoom };
		}
	}

	// The course lines and the marked stops on them. `general` is structurally
	// ineligible as a destination (STOP_CATEGORIES), which is what keeps an
	// imported turn cue from ever being announced as the next rest stop.
	let routeAnnotations = $derived($annotationList.filter((a) => a.category === 'route'));
	let stopAnnotations = $derived(
		$annotationList.filter((a) => (STOP_CATEGORIES as readonly string[]).includes(a.category))
	);

	// Active NWS alerts the map needs to draw — IN + NEAR only (FAR is never
	// broadcast/retained, decision 3/BUILD-PLAN §3.1 cuts the outside-footprint view).
	let wxMapAlerts = $derived([...$wxInAreaAlerts, ...$wxNearbyAlerts]);

	// Mobile bottom-sheet peek strip (P1-2): the single highest-priority
	// in-area alert. $wxInAreaAlerts is already sorted tier desc, severity
	// desc, endsAt asc (see wxAlertMeta.ts alertCompare) — so its first entry
	// already IS "warning before watch before advisory before statement;
	// among equals, the soonest to expire." Must stay in the BottomSheet's
	// PEEK_CONTENT_H budget below — grows/shrinks it via peekExtraH so the
	// nav rail is never pushed off-screen.
	const WX_PEEK_STRIP_H = 36;
	// Ride mode (spec §9): a 44px peek line, ABOVE the wx strip, phone only
	// (landscape phone — <500px tall — never mounts it: the strip's own
	// isDesktop gate already keeps RideStrip off there, and this mirrors it).
	const RIDE_PEEK_STRIP_H = 44;
	let showRidePeek = $derived(!isDesktop && !isShortViewport && $rideMode);
	let wxPeekAlert = $derived($wxInAreaAlerts[0] ?? null);
	let wxPeekCountdown = $derived(wxPeekAlert ? countdown(wxPeekAlert.endsAt, $wxMinute * 60000) : null);
	// Hidden entirely with no in-area alerts, when NWS alerts are off, or when the
	// strip's own alert is already the open detail below it (P0-2 finding 4 — the
	// strip only ever renders at 'peek' now, but the open detail can survive a
	// drag back down to peek, so the guard stays even though it's rarely hit).
	let showWxPeekStrip = $derived(
		$wxLinkStatus.state !== 'off' &&
			wxPeekAlert != null &&
			!($panelMode === 'weather' && $wxSelectedAlertId === wxPeekAlert?.id)
	);

	// P0-2: the sheet's peekContent used to render the same nav rail at every
	// sheet level, which is what pushed the actual panel content off the bottom
	// of the screen at 'half' and left 'full' mostly chrome. Above 'peek' the
	// user has already picked a destination, so the rail is replaced by this
	// ~44px context bar (title + close) instead — see peekContent below.
	const PANEL_TITLES: Record<PanelMode, string> = {
		closed: '',
		stations: 'Stations',
		detail: 'Station',
		messages: 'Messages',
		convo: 'Conversation',
		transports: 'Transports',
		activity: 'Activity',
		annotations: 'Annotations',
		netcontrol: 'Net Control',
		bulletins: 'Bulletins',
		ics309: 'ICS-309',
		weather: 'Weather',
		telemetry: 'Telemetry',
		df: 'Direction Finding',
		packets: 'Packets',
		settings: 'Settings',
		sag: 'SAG'
	};

	let selectedAlertEvent = $derived.by(() => {
		const id = $wxSelectedAlertId;
		if (!id) return null;
		return $wxAlertsById.get(id)?.effectiveEvent ?? null;
	});

	let panelTitle = $derived.by(() => {
		if (($panelMode === 'detail' || $panelMode === 'convo') && $selectedStation) return $selectedStation;
		if ($panelMode === 'weather' && selectedAlertEvent) return selectedAlertEvent;
		return PANEL_TITLES[$panelMode] ?? '';
	});

	// Small live-link dot in the context bar for the Weather panel only — the
	// context bar replaces the provenance strip's real estate at half/full, so
	// this is the one glanceable signal that survives.
	let wxContextDotColor = $derived.by(() => {
		switch ($wxLinkStatus.state) {
			case 'live': return 'var(--color-success)';
			case 'stale': return 'var(--color-warning)';
			case 'down': return 'var(--color-error)';
			default: return 'var(--color-text-muted)';
		}
	});

	/**
	 * Below this speed a GPS course-over-ground is noise, not a heading: NMEA
	 * RMC leaves it undefined at rest and receivers emit the last value, 0.0, or
	 * jitter. Feeding that to the distance engine flips "ahead" to the stop
	 * BEHIND the operator — observed on the real course while parked. Same gate,
	 * and same threshold, as courseForStation below.
	 */
	const MIN_COURSE_SPEED_KNOTS = 1;

	// Own GPS fix as an origin. A stale or old fix is still passed through —
	// the pill decides how to present it (amber + age, or "no fix" past 10 min).
	let gpsOrigin = $derived.by((): DistanceOrigin | null => {
		const fix = $gpsStatus.fix;
		if (!fix || fix.mode < 2) return null;
		return {
			lat: fix.lat,
			lon: fix.lon,
			label: 'my position',
			bearingDeg:
				fix.hasCourse && fix.speedKnots > MIN_COURSE_SPEED_KNOTS ? fix.course : undefined,
			ageMs: $gpsAgeMs ?? undefined,
			stale: $gpsStatus.stale || !$gpsHasFix,
			source: 'gps'
		};
	});

	let effectiveDistanceOrigin = $derived<DistanceOrigin | null>(
		distanceOrigin ? { ...distanceOrigin, source: 'pick' } : gpsOrigin
	);

	/**
	 * Long press / right-click on the map. Not a pick mode: it arms and commits
	 * in one gesture, so the bottom sheet is deliberately left alone.
	 */
	function handleDistanceOriginPicked(
		lat: number,
		lon: number,
		src: { kind: 'map' } | { kind: 'annotation' | 'station'; id: string; label: string }
	) {
		distanceOrigin = {
			lat,
			lon,
			label: src.kind === 'map' ? 'pin' : src.label,
			bearingDeg: src.kind === 'station' ? courseForStation(src.id) : undefined
		};
		distancePinned = src.kind === 'map';
		showNextStop = true;
	}

	/** Course of a moving station only — a parked station's last course is noise. */
	function courseForStation(key: string): number | undefined {
		const st = $stations.get(key);
		const pos = st?.position;
		if (!pos || pos.course == null) return undefined;
		return (pos.speed ?? 0) > 1 ? pos.course : undefined;
	}

	function handleDistanceOriginCleared() {
		distanceOrigin = null;
		distancePinned = false;
	}

	/** StationDetail's "Distance" button — the discoverable twin of long-press. */
	function handleStationDistance(key: string, lat: number, lon: number, label: string) {
		distanceOrigin = { lat, lon, label, bearingDeg: courseForStation(key) };
		distancePinned = false;
		showNextStop = true;
		nextStopExpanded = true;
	}

	function toggleNextStopPill() {
		showNextStop = !showNextStop;
		if (!showNextStop) {
			nextStopExpanded = false;
			measureBand = null;
		}
		try {
			localStorage.setItem('nymeria_next_stop_pill', showNextStop ? '1' : '0');
		} catch {
			// per-viewer convenience only — the session still honours the toggle
		}
	}

	function handleGpsFollowBreak() {
		if (get(gpsFollow)) {
			gpsFollow.set(false);
			showToast('Follow off', 'info', 2000);
		}
	}

	function handlePlaceOperator(ciId: string, callsign: string) {
		drawingMode = null;
		placingAnnotation = null;
		placingMissionLocation = null;
		placingOperator = { id: ciId, callsign };
	}

	async function handleOperatorPlaced(ciId: string, lat: number, lon: number) {
		placingOperator = null;
		if (!$activeNet) return;
		try {
			await api.updateCheckIn($activeNet.id, ciId, { lat, lon });
		} catch (e) {
			console.error('Failed to set operator position:', e);
		}
	}

	function handlePlaceCancelled() {
		placingOperator = null;
	}

	function handlePlaceAnnotation(id: string | null, name: string, mode: 'update' | 'form') {
		drawingMode = null;
		placingOperator = null;
		placingMissionLocation = null;
		placingAnnotation = { id, name, mode };
	}

	function handlePlaceMissionLocation(label: string) {
		drawingMode = null;
		placingOperator = null;
		placingAnnotation = null;
		placingMissionLocation = { label };
		if (!isDesktop) {
			sheetBeforePick = get(sheetState);
			sheetState.set('peek');
		}
	}

	function restoreSheetAfterPick() {
		if (sheetBeforePick && get(sheetState) === 'peek') sheetState.set(sheetBeforePick);
		sheetBeforePick = null;
	}

	function handleMissionLocationPlaced(lat: number, lon: number, label?: string) {
		missionDraftPoint = { lat, lon };
		missionMapCoords = { lat, lon, label };
		placingMissionLocation = null;
		restoreSheetAfterPick();
	}

	// The click landed on an existing annotation — the annotation IS the
	// location (snap + link), so there's no separate dropped pin.
	function handleMissionLocationAnnotationPicked(id: string, lat: number, lon: number) {
		missionPickAnnotation = { id, lat, lon };
		missionDraftPoint = null;
		placingMissionLocation = null;
		restoreSheetAfterPick();
	}

	function handleMissionPickAnnotationConsumed() {
		missionPickAnnotation = null;
	}

	function handleMissionLocationPlaceCancelled() {
		placingMissionLocation = null;
		restoreSheetAfterPick();
	}

	function handleMissionMapCoordsConsumed() {
		missionMapCoords = null;
	}

	function handleClearMissionDraft() {
		missionDraftPoint = null;
	}

	async function handleAnnotationPlaced(lat: number, lon: number) {
		const placing = placingAnnotation;
		placingAnnotation = null;

		if (placing?.mode === 'update' && placing.id) {
			try {
				const geometry = JSON.stringify({ type: 'Point', coordinates: [lon, lat] });
				await api.updateAnnotation(placing.id, { geometry });
			} catch (e) {
				console.error('Failed to set annotation position:', e);
			}
		} else {
			// Send coordinates to the form
			annotationMapCoords = { lat, lon };
		}
	}

	function handleAnnotationPlaceCancelled() {
		placingAnnotation = null;
	}

	function handleAnnotationFocusConsumed() {
		focusedAnnotationId = null;
	}

	function handleMapCoordsConsumed() {
		annotationMapCoords = null;
	}

	function handleFlyToAnnotation(ann: Annotation) {
		try {
			const geom = JSON.parse(ann.geometry);
			if (geom.type === 'Point') {
				flyToTarget = { lat: geom.coordinates[1], lon: geom.coordinates[0], zoom: 15 };
			} else if (geom.type === 'LineString') {
				const mid = Math.floor(geom.coordinates.length / 2);
				flyToTarget = { lat: geom.coordinates[mid][1], lon: geom.coordinates[mid][0], zoom: 14 };
			} else if (geom.type === 'Polygon') {
				const coords = geom.coordinates[0];
				let latSum = 0, lonSum = 0;
				for (const c of coords) { latSum += c[1]; lonSum += c[0]; }
				flyToTarget = { lat: latSum / coords.length, lon: lonSum / coords.length, zoom: 14 };
			}
		} catch { /* skip */ }
	}

	function handleStartDraw(mode: 'point' | 'line' | 'area') {
		placingOperator = null;
		placingAnnotation = null;
		placingMissionLocation = null;
		drawingMode = mode;
	}

	function handleDrawComplete(geometry: string) {
		drawingMode = null;
		annotationPanelRef?.setGeometry(geometry);
	}

	function handlePreviewChange(geometry: string | null, color: string) {
		previewGeometry = geometry;
		previewColor = color;
	}

	function handleStartEdit(id: string) {
		editingAnnotationId = id;
		// Fly to the annotation
		const ann = $annotationList.find((a) => a.id === id);
		if (ann) handleFlyToAnnotation(ann);
	}

	function handleStopEdit() {
		editingAnnotationId = null;
	}

	function handleGeometryEdit(geometry: string) {
		annotationPanelRef?.setEditGeometry(geometry);
	}

	function handlePreviewGeometryChange(geometry: string) {
		previewGeometry = geometry;
	}

	function handleRailToggle(mode: PanelMode) {
		togglePanel(mode);
	}

	/**
	 * Guards the ride strip's single-key accelerators (spec §5): inert
	 * outside ride mode, while typing, with a modifier held, while the
	 * command palette is already open, while any modal/overlay is up (the
	 * same DOM probe SidePanel.svelte already uses for Escape), or before the
	 * session is ready/approved.
	 */
	function rideShortcutsInert(e: KeyboardEvent): boolean {
		if (!$rideMode) return true;
		const tag = (e.target as HTMLElement)?.tagName;
		if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement)?.isContentEditable) return true;
		if (e.ctrlKey || e.metaKey || e.altKey) return true;
		if ($commandPaletteOpen) return true;
		if (document.querySelector('[aria-modal="true"], [data-blocks-escape="true"], .login-overlay, .setup-wizard')) return true;
		if (!sessionReady || $needsSetup || !$isApproved) return true;
		return false;
	}

	let ridePendingListOpen = $state(false);
	let rideBriefingOpen = $state(false);

	function handleGlobalKeydown(e: KeyboardEvent) {
		// Ctrl+K / Cmd+K → toggle command palette
		if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
			e.preventDefault();
			toggleCommandPalette();
			return;
		}
		// '/' when not in input/textarea → open command palette
		if (e.key === '/' && !$commandPaletteOpen) {
			const tag = (e.target as HTMLElement)?.tagName;
			if (tag !== 'INPUT' && tag !== 'TEXTAREA' && !(e.target as HTMLElement)?.isContentEditable) {
				e.preventDefault();
				commandPaletteOpen.set(true);
			}
			return;
		}

		if (rideShortcutsInert(e)) return;
		switch (e.key) {
			case 'q':
				e.preventDefault();
				seedPalette('lead ');
				break;
			case 'w':
				e.preventDefault();
				seedPalette('sweep ');
				break;
			case 'a': {
				e.preventDefault();
				const top = $rideEmergency;
				if (top) void ackEmergency(top.id);
				else showToast('Nothing to acknowledge', 'info', 2000);
				break;
			}
			case 'r':
				e.preventDefault();
				ridePendingListOpen = true;
				break;
			case 'x':
				e.preventDefault();
				rideBriefingOpen = true;
				break;
			case '?':
				e.preventDefault();
				rideShortcutHelpOpen.set(true);
				break;
			case 'e': {
				e.preventDefault();
				const rank1 = $rideLadder.find((t) => t.rank === 1);
				seedPalette('incident ', rank1?.id);
				break;
			}
			case 'n':
				e.preventDefault();
				seedPalette('incident ');
				break;
			case 's':
				e.preventDefault();
				seedPalette('sag ');
				break;
			case 'c':
				e.preventDefault();
				seedPalette('close ');
				break;
		}
	}

	function handlePaletteFlyTo(lat: number, lon: number) {
		flyToTarget = { lat, lon, zoom: 15 };
	}
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<svelte:head>
	<title>Nymeria - APRS Client</title>
</svelte:head>

<div class="app-container">
	<!-- Setup wizard takes priority over login -->
	{#if sessionReady && $needsSetup}
		<SetupWizard />
	{:else if sessionReady && (!$isLoggedIn || $isPending || $isDenied)}
		<LoginOverlay />
	{/if}

	<!-- Full-screen map -->
	<div class="map-layer">
		<Map
			bind:this={mapRef}
			stations={stationsWithPosition}
			annotations={$annotationList}
			selectedCallsign={$selectedStation ?? ''}
			onStationClick={handleStationClick}
			sagRequests={$sagPlaced}
			sagVehicles={$sagVehiclesOnMap}
			sagFocusId={$sagFocus?.requestId ?? null}
			sagFocusVehicleId={$sagFocusedVehicleId}
			sagDispatchMode={$sagFocus?.mode === 'dispatch'}
			sagCandidates={$sagCandidates}
			sagLegLines={$sagLegLines}
			showSag={$sagOverlayActive}
			onSagPickupClick={handleSagPickupClick}
			onSagVehicleClick={handleSagVehicleClick}
			{placingSagLocation}
			onSagLocationPlaced={handleSagLocationPlaced}
			onSagPlaceCancelled={() => (placingSagLocation = null)}
			onAnnotationClick={handleAnnotationClick}
			{flyToTarget}
			panelOpen={panelIsOpen}
			{drawingMode}
			onDrawComplete={handleDrawComplete}
			{previewGeometry}
			{previewColor}
			{editingAnnotationId}
			onGeometryEdit={handleGeometryEdit}
			onPreviewGeometryChange={handlePreviewGeometryChange}
			netOperators={$operatorsWithPosition}
			netMissions={$missionsWithPosition}
			netAssignmentLines={$assignmentLines}
			activeNetId={$activeNet?.id ?? null}
			onNetOperatorClick={handleNetOperatorClick}
			onNetMissionClick={handleNetMissionClick}
			{flyToBounds}
			fitPadding={sagFitPadding}
			highlightedMissionId={$hoveredMissionId}
			highlightedCheckInId={$hoveredCheckInId}
			weatherOverlay={$weatherStations}
			showWeatherOverlay={$mapSettings.showWeatherOverlay}
			dfOverlay={$dfStations}
			showDFOverlay={$mapSettings.showDFOverlay}
			showTracks={$mapSettings.showTracks}
			trackDurationMs={TRACK_DURATION_MS[$mapSettings.trackDuration]}
			showDRCones={$mapSettings.showDRCones}
			showCallsigns={$mapSettings.showCallsigns}
			{placingOperator}
			onOperatorPlaced={handleOperatorPlaced}
			onPlaceCancelled={handlePlaceCancelled}
			netLocationAnnotations={$netLocationAnnotations}
			showNetLocationAnnotations={$activeNet != null}
			placingAnnotation={placingAnnotation}
			onAnnotationPlaced={handleAnnotationPlaced}
			onAnnotationPlaceCancelled={handleAnnotationPlaceCancelled}
			{placingMissionLocation}
			{missionDraftPoint}
			onMissionLocationPlaced={handleMissionLocationPlaced}
			onMissionLocationAnnotationPicked={handleMissionLocationAnnotationPicked}
			onMissionLocationPlaceCancelled={handleMissionLocationPlaceCancelled}
			ownPosition={$gpsStatus.fix}
			ownPositionStale={$gpsStatus.stale}
			follow={$gpsFollow}
			onFollowBreak={handleGpsFollowBreak}
			onDistanceOriginPicked={handleDistanceOriginPicked}
			onDistanceOriginCleared={handleDistanceOriginCleared}
			distanceDraftPoint={distancePinned && distanceOrigin
				? { lat: distanceOrigin.lat, lon: distanceOrigin.lon }
				: null}
			measureBand={showNextStop ? measureBand : null}
			wxAlerts={wxMapAlerts}
			wxZoneGeometry={$wxZoneGeometry}
			wxLinkState={$wxLinkStatus.state}
			wxMapMode={$mapSettings.showWxAlerts}
			wxFootprintCentroid={$wxFootprint?.centroid ?? null}
			wxFootprintPreview={$wxFootprintPreview}
			wxFocusAlertId={$wxFocusAlertId}
			onWxAlertClick={openWxAlert}
			onWxFocusConsumed={() => wxFocusAlertId.set(null)}
		/>
	</div>

	<!-- The map HUD: ONE column down the map's left edge.
	     Row 1 is the map's tools (zoom, follow-me, ops view, layers); then
	     the status chips; then the next-stop readout; then, on a bike ride,
	     the SAG dock, which takes whatever height is left. Everything stacks
	     VERTICALLY in normal flow, so a chip appearing, the next-stop card
	     expanding or the dock growing moves its neighbours instead of landing
	     on them. (These used to be hand-placed at fixed `top:` offsets, and
	     the dock was a second column that shoved every control sideways.) -->
	<div
		bind:this={hudEl}
		class="map-hud"
		class:map-hud--dock={isDesktop && $sagDockActive}
		style:--sag-dock-w="{sagDockWidth}px"
	>
		<div class="map-hud-tools" role="group" aria-label="Map tools">
			<div class="map-hud-zoom" role="group" aria-label="Zoom">
				<button type="button" class="map-hud-btn" onclick={() => mapRef?.zoomIn()} aria-label="Zoom in" title="Zoom in">
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" aria-hidden="true"><path d="M8 3v10M3 8h10" /></svg>
				</button>
				<button type="button" class="map-hud-btn" onclick={() => mapRef?.zoomOut()} aria-label="Zoom out" title="Zoom out">
					<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" aria-hidden="true"><path d="M3 8h10" /></svg>
				</button>
			</div>

			<GpsFollowControl oncenter={() => mapRef?.centerOnOwnPosition()} />

			{#if $opsView && $activeNet}
				<button type="button" class="map-hud-btn ops-view-fab" onclick={handleGoToOpsView} aria-label="Return to Ops View" title="Return to Ops View">
					<svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden="true">
						<circle cx="8" cy="7" r="3" stroke="currentColor" stroke-width="1.5"/>
						<path d="M8 1C4.5 1 1.5 3.5 1 7c.5 3.5 3.5 6 7 6s6.5-2.5 7-6c-.5-3.5-3.5-6-7-6z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
						<path d="M8 13v2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
				</button>
			{/if}

			<!-- rosterCount is the count the filter actually gates on, not the raw roster
			     size: a voice-only roster has check-ins but nothing on the map, and
			     the raw roster size there would enable a checkbox that changes nothing. -->
			<MapPalette
				filteredCount={stationsWithPosition.length}
				totalCount={allStationsWithPosition.length}
				hasActiveNet={$netIsOpen}
				rosterCount={$rosterMappedCount}
				nextStopEnabled={showNextStop}
				hasCourse={routeAnnotations.length > 0}
				onNextStopToggle={toggleNextStopPill}
			/>
		</div>

		<div class="map-hud-status">
			<GpsStatusPill />
			<WxLinkPill />
		</div>

		<!-- Along-course distance readout -->
		{#if showNextStop}
			<NextStopPill
				origin={effectiveDistanceOrigin}
				{routeAnnotations}
				{stopAnnotations}
				activeNetId={$activeNet?.id ?? null}
				expanded={nextStopExpanded}
				onToggle={() => (nextStopExpanded = !nextStopExpanded)}
				onClear={handleDistanceOriginCleared}
				onMeasureChange={(b) => (measureBand = b)}
			/>
		{/if}

		<!-- Desktop: SAG dock (bike-ride nets with the overlay on), last in
		     the column so it takes the height the rows above leave. In
		     dispatch focus the candidate panel takes its place — same slot,
		     same width, so the operator's eye does not have to move. -->
		{#if isDesktop && $sagDockActive}
			<div
				class="sag-dock-slot"
				class:sag-dock-slot--rail={sagDockCollapsed && $sagFocus?.mode !== 'dispatch'}
			>
				{#if $sagFocus?.mode === 'dispatch'}
				<!-- Deliberately NOT gated on $sagCandidates: that store is null
				     when the pickup itself is unplaceable, and the panel is built
				     to work in exactly that case. Somebody is standing at the
				     roadside whether or not we can draw them. -->
				<SagCandidatePanel />
			{:else}
				<SagDock
					onPlaceRequest={handleSagPlaceRequest}
					onPlaceVehicle={handleSagPlaceVehicle}
					onSagFocusPreset={toggleSagFocusPreset}
					onFitPoints={handleSagFit}
					collapsed={sagDockCollapsed}
					onExpand={() => (sagDockCollapsed = false)}
					onCollapse={isTablet ? () => (sagDockCollapsed = true) : undefined}
				/>
			{/if}
			</div>
		{/if}
	</div>

	<!-- Desktop: Activity Rail (right edge) -->
	{#if isDesktop}
		<ActivityRail
			panelMode={$panelMode}
			unreadCount={totalUnread}
			onToggle={handleRailToggle}
			onSelectStation={handleSearchSelect}
			onCommandPalette={toggleCommandPalette}
		/>
	{/if}

	<!-- Desktop: Side Panel -->
	{#if isDesktop}
		<SidePanel
			open={panelIsOpen}
			onClose={closePanel}
			onBack={$panelMode === 'convo' ? openMessages : ($panelMode === 'weather' && $wxSelectedAlertId) ? () => wxSelectedAlertId.set(null) : undefined}
			backLabel={$panelMode === 'weather' ? 'Alerts' : 'Messages'}
			hideBackButton={$panelMode === 'weather'}
			onTransitionEnd={() => {}}
		>
			{#if $panelMode === 'stations'}
				<StationList
					onSelect={handleStationClick}
					selectedKey={$selectedStation}
				/>
			{:else if $panelMode === 'detail' && $selectedStation}
				<StationDetail
					stationKey={$selectedStation}
					activeTab={$detailTab}
					onTabChange={handleTabChange}
					onClose={closePanel}
					onFlyTo={handleFlyTo}
					onDistance={handleStationDistance}
				/>
			{:else if $panelMode === 'messages'}
				<ConvoList
					onSelectConvo={handleConvoSelect}
				/>
			{:else if $panelMode === 'convo' && $selectedStation}
				<StationDetail
					stationKey={$selectedStation}
					activeTab={$detailTab}
					onTabChange={handleTabChange}
					onClose={closePanel}
					onFlyTo={handleFlyTo}
					onDistance={handleStationDistance}
				/>
			{:else if $panelMode === 'transports'}
				<TransportPanel />
			{:else if $panelMode === 'activity'}
				<ActivityPanel />
			{:else if $panelMode === 'annotations'}
				<AnnotationPanel
					bind:this={annotationPanelRef}
					onFlyToAnnotation={handleFlyToAnnotation}
					onStartDraw={handleStartDraw}
					onPreviewChange={handlePreviewChange}
					onStartEdit={handleStartEdit}
					onStopEdit={handleStopEdit}
					{focusedAnnotationId}
					onFocusConsumed={handleAnnotationFocusConsumed}
				/>
			{:else if $panelMode === 'bulletins'}
				<BulletinPanel />
			{:else if $panelMode === 'netcontrol'}
				<NetControlPanel
					onFlyTo={handleNetFlyTo}
					onFlyToBounds={handleFlyToBounds}
					onSetOpsView={handleSetOpsView}
					onGoToOpsView={handleGoToOpsView}
					onPlaceOperator={handlePlaceOperator}
					onPlaceAnnotation={handlePlaceAnnotation}
					{annotationMapCoords}
					onMapCoordsConsumed={handleMapCoordsConsumed}
					{focusedAnnotationId}
					onFocusConsumed={handleAnnotationFocusConsumed}
					onPlaceMissionLocation={handlePlaceMissionLocation}
					{missionMapCoords}
					onMissionMapCoordsConsumed={handleMissionMapCoordsConsumed}
					{missionPickAnnotation}
					onMissionPickAnnotationConsumed={handleMissionPickAnnotationConsumed}
					onClearMissionDraft={handleClearMissionDraft}
					missionPickActive={placingMissionLocation != null}
					onSetMissionDraftPoint={handleSetMissionDraftPoint}
					getMapCenter={handleGetMapCenter}
					isDesktop={true}
				/>
			{:else if $panelMode === 'weather'}
				<WeatherPanel onFlyTo={handleFlyTo} />
			{:else if $panelMode === 'telemetry'}
				<TelemetryPanel onFlyTo={handleFlyTo} />
			{:else if $panelMode === 'df'}
				<DFPanel onFlyTo={handleFlyTo} onFlyToTarget={handleFlyTo} />
			{:else if $panelMode === 'packets'}
				<PacketInspector />
			{:else if $panelMode === 'ics309'}
				<ICS309Panel />
			{:else if $panelMode === 'settings'}
				<SettingsPanel />
			{/if}
		</SidePanel>
	{/if}

	<!-- Desktop: Ride status strip (bike-ride profile nets only) -->
	{#if isDesktop && $rideMode}
		<RideStrip
			{isDesktop}
			onFlyTo={handleNetFlyTo}
			onFlyToBounds={handleFlyToBounds}
			onHeightChange={() => requestAnimationFrame(() => mapRef?.invalidateSize())}
		/>
	{/if}

	<!-- Mobile: Bottom Sheet -->
	{#if !isDesktop}
		<BottomSheet
			sheetLevel={$sheetState}
			onStateChange={handleSheetStateChange}
			peekExtraH={(showRidePeek ? RIDE_PEEK_STRIP_H : 0) + (showWxPeekStrip ? WX_PEEK_STRIP_H : 0)}
		>
			{#snippet peekContent()}
				{#if $sheetState === 'peek'}
					<div class="sheet-peek-row">
						<button class="peek-status" onclick={openTransports} aria-label="Transport status — open transports">
							<ConnectionStatus />
						</button>
						<span class="station-count">{$rosterScopedStations.scoped ? `${$rosterScopedStations.stations.length} on roster` : `${$stationList.length} stations`}</span>
					</div>
					{#if showRidePeek}
						<RidePeek />
					{/if}
					{#if showWxPeekStrip && wxPeekAlert && wxPeekCountdown}
						<button
							class="wx-peek-strip"
							style="border-left-color: var({tierMeta[wxPeekAlert.tier].colorVar})"
							onclick={() => openWxAlert(wxPeekAlert!.id)}
							aria-label={`${wxPeekAlert.event}, ${tierMeta[wxPeekAlert.tier].ariaWord}, ${wxPeekCountdown.text === 'expired' ? 'expired' : `ends in ${wxPeekCountdown.text}`}. Open alert.`}
						>
							<span class="wx-peek-glyph" style="color: var({tierMeta[wxPeekAlert.tier].colorVar})">
								<WxTierGlyph tier={wxPeekAlert.tier} size={16} />
							</span>
							<span class="wx-peek-event">
								{wxPeekAlert.event}{#if $wxInAreaAlerts.length > 1}<span class="wx-peek-more"> · {$wxInAreaAlerts.length - 1} more</span>{/if}
							</span>
							<span class="wx-peek-countdown">{wxPeekCountdown.text}</span>
							<span class="wx-peek-chevron" aria-hidden="true">›</span>
						</button>
					{/if}
					<Toolbar
						unreadCount={totalUnread}
						activeMode={$panelMode}
						onSearchOpen={() => searchOpen.set(true)}
						onMessagesOpen={openMessages}
						onBulletinsOpen={openBulletins}
						onTransportsOpen={openTransports}
						onAnnotationsOpen={openAnnotations}
						onNetControlOpen={openNetControl}
						onWeatherOpen={openWeather}
						onDFOpen={openDF}
						onPacketsOpen={openPackets}
						onSettingsOpen={openSettings}
						onCommandPalette={toggleCommandPalette}
					/>
				{:else}
					<!-- P0-2: above 'peek' the user has already chosen a destination —
					     hand the vertical space back to the panel instead of the rail. -->
					<div class="sheet-context">
						<button class="sheet-context-close" onclick={closePanel} aria-label="Close panel">×</button>
						<span class="sheet-context-title">{panelTitle}</span>
						{#if $panelMode === 'weather'}
							<span class="sheet-context-dot" style="background: {wxContextDotColor}" aria-hidden="true"></span>
						{/if}
					</div>
				{/if}
			{/snippet}

			{#if $panelMode === 'closed' || $panelMode === 'stations'}
				<StationList
					onSelect={handleStationClick}
					selectedKey={$selectedStation}
				/>
			{:else if $panelMode === 'detail' && $selectedStation}
				<StationDetail
					stationKey={$selectedStation}
					activeTab={$detailTab}
					visible={$sheetState !== 'peek'}
					onTabChange={handleTabChange}
					onClose={closePanel}
					onFlyTo={handleFlyTo}
					onDistance={handleStationDistance}
				/>
			{:else if $panelMode === 'messages'}
				<ConvoList
					onSelectConvo={handleConvoSelect}
				/>
			{:else if $panelMode === 'convo' && $selectedStation}
				<StationDetail
					stationKey={$selectedStation}
					activeTab={$detailTab}
					visible={$sheetState !== 'peek'}
					onTabChange={handleTabChange}
					onClose={closePanel}
					onBack={openMessages}
					onFlyTo={handleFlyTo}
					onDistance={handleStationDistance}
				/>
			{:else if $panelMode === 'transports'}
				<TransportPanel />
			{:else if $panelMode === 'activity'}
				<ActivityPanel />
			{:else if $panelMode === 'annotations'}
				<AnnotationPanel
					bind:this={annotationPanelRef}
					onFlyToAnnotation={handleFlyToAnnotation}
					onStartDraw={handleStartDraw}
					onPreviewChange={handlePreviewChange}
					onStartEdit={handleStartEdit}
					onStopEdit={handleStopEdit}
					{focusedAnnotationId}
					onFocusConsumed={handleAnnotationFocusConsumed}
				/>
			{:else if $panelMode === 'bulletins'}
				<BulletinPanel />
			{:else if $panelMode === 'netcontrol'}
				<NetControlPanel
					onFlyTo={handleNetFlyTo}
					onFlyToBounds={handleFlyToBounds}
					onSetOpsView={handleSetOpsView}
					onGoToOpsView={handleGoToOpsView}
					onPlaceOperator={handlePlaceOperator}
					onPlaceAnnotation={handlePlaceAnnotation}
					{annotationMapCoords}
					onMapCoordsConsumed={handleMapCoordsConsumed}
					{focusedAnnotationId}
					onFocusConsumed={handleAnnotationFocusConsumed}
					onPlaceMissionLocation={handlePlaceMissionLocation}
					{missionMapCoords}
					onMissionMapCoordsConsumed={handleMissionMapCoordsConsumed}
					{missionPickAnnotation}
					onMissionPickAnnotationConsumed={handleMissionPickAnnotationConsumed}
					onClearMissionDraft={handleClearMissionDraft}
					missionPickActive={placingMissionLocation != null}
					onSetMissionDraftPoint={handleSetMissionDraftPoint}
					getMapCenter={handleGetMapCenter}
				/>
			{:else if $panelMode === 'weather'}
				<WeatherPanel onFlyTo={handleFlyTo} />
			{:else if $panelMode === 'telemetry'}
				<TelemetryPanel onFlyTo={handleFlyTo} />
			{:else if $panelMode === 'df'}
				<DFPanel onFlyTo={handleFlyTo} onFlyToTarget={handleFlyTo} />
			{:else if $panelMode === 'packets'}
				<PacketInspector />
			{:else if $panelMode === 'ics309'}
				<ICS309Panel />
			{:else if $panelMode === 'settings'}
				<SettingsPanel />
			{:else if $panelMode === 'sag'}
				<!-- No dock below 769px: the dock's content, and in dispatch
				     focus the candidate list, live here instead (spec §11). -->
				{#if $sagFocus?.mode === 'dispatch'}
					<SagCandidatePanel />
				{:else}
					<SagDock
						onPlaceRequest={handleSagPlaceRequest}
						onPlaceVehicle={handleSagPlaceVehicle}
						onSagFocusPreset={toggleSagFocusPreset}
						onFitPoints={handleSagFit}
					/>
				{/if}
			{/if}
		</BottomSheet>
	{/if}

	<!-- Interrupt channel — overlays sheet/panel, never inside them. Exactly
	     one full-width banner can exist at a time; ride EMERGENCY always wins
	     arbitration over a weather interrupt (spec §0/§4). -->
	{#if $activeInterruptSource === 'ride'}
		<RideInterruptBanner />
	{:else if $activeInterruptSource === 'wx'}
		<WxInterruptBanner />
	{/if}

	<!-- Mobile: Search Overlay -->
	{#if $searchOpen && !isDesktop}
		<SearchOverlay
			onClose={() => searchOpen.set(false)}
			onSelect={handleSearchSelect}
		/>
	{/if}

	<!-- Command Palette (Ctrl+K) -->
	{#if $commandPaletteOpen}
		<CommandPalette
			onFlyTo={handlePaletteFlyTo}
			onClose={() => commandPaletteOpen.set(false)}
		/>
	{/if}

	<!-- Ride mode global overlays (keyboard accelerators r / x / ?) -->
	{#if ridePendingListOpen}
		<RidePendingList onClose={() => (ridePendingListOpen = false)} />
	{/if}
	{#if rideBriefingOpen}
		<ShiftBriefing onClose={() => (rideBriefingOpen = false)} />
	{/if}
	{#if $rideShortcutHelpOpen}
		<RideShortcutHelp onClose={() => rideShortcutHelpOpen.set(false)} />
	{/if}
</div>

<style>
	.app-container {
		position: relative;
		width: 100vw;
		height: 100vh;
		height: 100dvh;
		overflow: hidden;
	}

	.map-layer {
		position: absolute;
		/* The ride strip is a bottom band, not an overlay — it must not cover
		   the course it describes. --ride-strip-h defaults to 0px outside
		   bike-ride mode, so this is a no-op everywhere else (spec §9). */
		inset: 0 0 var(--ride-strip-h, 0px) 0;
		z-index: var(--z-map);
	}

	/* The map HUD column (see the markup). Absolutely placed over the map's
	   top-left corner; its rows are in normal flow, so nothing in it is ever
	   positioned by hand again. The column itself passes pointer events
	   through — only its controls take them — so the gaps between rows and
	   the empty space beside short rows are still the map. */
	.map-hud {
		position: absolute;
		top: 10px;
		left: 10px;
		z-index: var(--z-toolbar);
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: var(--space-sm);
		max-width: calc(100% - 20px - var(--rail-width, 0px));
		min-height: 0;
		pointer-events: none;
	}

	.map-hud > :global(*:not(.sag-dock-slot)),
	.map-hud-tools > :global(*),
	.map-hud-status > :global(*) {
		pointer-events: auto;
	}

	/* With the dock the column runs to the ride strip, so the dock can take
	   the height left under the rows above it and scroll inside that. */
	.map-hud--dock {
		bottom: calc(var(--ride-strip-h, 0px) + var(--space-sm));
	}

	.map-hud-tools {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		pointer-events: none;
	}

	/* Zoom in/out as one joined control, like every map the operator has used. */
	.map-hud-zoom {
		display: flex;
		pointer-events: none;
	}

	.map-hud-zoom > :global(.map-hud-btn) {
		pointer-events: auto;
	}

	.map-hud-zoom > :global(.map-hud-btn:first-child) {
		border-top-right-radius: 0;
		border-bottom-right-radius: 0;
	}

	.map-hud-zoom > :global(.map-hud-btn:last-child) {
		border-top-left-radius: 0;
		border-bottom-left-radius: 0;
		border-left: none;
	}

	.map-hud-status {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-xs);
		pointer-events: none;
	}

	/* No GPS configured and the NWS link healthy: no empty row, no gap. */
	.map-hud-status:empty {
		display: none;
	}

	.ops-view-fab {
		border-color: var(--color-success);
		color: var(--color-success);
	}

	.sag-dock-slot {
		flex: 1 1 auto;
		min-height: 0;
		width: var(--sag-dock-w);
		display: flex;
		flex-direction: column;
		/* The slot is as tall as the space left; the dock inside is usually
		   shorter. Only the dock takes clicks — the space under it is map. */
		pointer-events: none;
	}

	.sag-dock-slot > :global(*) {
		pointer-events: auto;
	}

	/* Tablet: the 44px rail is content-height, never a full-height column. */
	.sag-dock-slot--rail {
		flex: 0 1 auto;
	}

	.sheet-peek-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding-bottom: var(--space-xs);
	}

	.peek-status {
		display: inline-flex;
		align-items: center;
		min-height: 32px;
		padding: 0;
		background: none;
		border: none;
		color: inherit;
		font: inherit;
		cursor: pointer;
	}

	.station-count {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	/* P0-2: replaces the status row / wx strip / nav rail above 'peek' — a single
	   draggable-height context bar (close · title · link dot) so the panel gets
	   the vertical space instead of chrome it no longer needs once the user has
	   already picked a destination. */
	.sheet-context {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		min-height: 44px;
	}

	.sheet-context-close {
		flex-shrink: 0;
		width: 44px;
		height: 44px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: none;
		border: none;
		color: var(--color-text);
		font-size: 1.25rem;
		line-height: 1;
		border-radius: var(--radius-sm);
		cursor: pointer;
	}

	.sheet-context-close:hover,
	.sheet-context-close:focus-visible {
		background: var(--color-surface);
	}

	.sheet-context-title {
		flex: 1;
		min-width: 0;
		font-size: 0.95rem;
		font-weight: 700;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sheet-context-dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: var(--radius-full);
		margin-right: var(--space-sm);
	}

	/* P1-2: soonest in-area alert + countdown. Only rendered at 'peek' now (see
	   peekContent) — above peek the context bar takes over. */
	.wx-peek-strip {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		width: 100%;
		min-height: 36px;
		box-sizing: border-box;
		padding: 0 var(--space-sm);
		background: var(--color-surface);
		border: none;
		border-left: 3px solid;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		text-align: left;
		cursor: pointer;
	}

	.wx-peek-strip:hover,
	.wx-peek-strip:focus-visible {
		background: var(--color-primary);
	}

	.wx-peek-glyph {
		flex-shrink: 0;
		display: flex;
		align-items: center;
	}

	.wx-peek-event {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 0.8rem;
	}

	.wx-peek-more {
		color: var(--color-text-muted);
		font-weight: 400;
	}

	.wx-peek-countdown {
		flex-shrink: 0;
		font-size: 0.9rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.wx-peek-chevron {
		flex-shrink: 0;
		color: var(--color-text-muted);
	}
</style>
