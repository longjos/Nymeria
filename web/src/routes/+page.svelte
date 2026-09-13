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
	import NextStopPill from '$lib/components/NextStopPill.svelte';
	import type { MeasureBand, DistanceOrigin } from '$lib/components/NextStopPill.svelte';
	import { STOP_CATEGORIES } from '$lib/routeDistance';
	import { stations, stationList, initStationStore, wsClient, connectWS } from '$lib/stores/stations';
	import { initMessageStore, conversationList } from '$lib/stores/messages';
	import { initTransportStore } from '$lib/stores/transports';
	import { gpsStatus, gpsFollow, gpsAgeMs, gpsHasFix, initGpsStore } from '$lib/stores/gps';
	import { showToast } from '$lib/stores/toast';
	import { annotationList, initAnnotationStore } from '$lib/stores/annotations';
	import { api } from '$lib/api';
	import {
		initNetControlStore,
		activeNet, operatorsWithPosition, missionsWithPosition, assignmentLines,
		opsView, hoveredMissionId, hoveredCheckInId,
		netLocationAnnotations
	} from '$lib/stores/netcontrol';
	import { initTacticalStore } from '$lib/stores/tactical';
	import { initBulletinStore } from '$lib/stores/bulletins';
	import { initWeatherStore, weatherStations, selectedWeatherStation } from '$lib/stores/weather';
	import { dfStations } from '$lib/stores/df';
	import { loadW3WStatus } from '$lib/stores/w3w';
	import { initPacketStore } from '$lib/stores/packets';
	import { initPathStore } from '$lib/stores/paths';
	import { isLoggedIn, needsSetup, isApproved, isPending, isDenied, initSession, handleSessionEvent, currentUser, loadPendingRequests, canAdmin } from '$lib/stores/session';
	import { mapSettings, AGE_FILTER_MS, TRACK_DURATION_MS } from '$lib/stores/mapSettings';
	import { activeCheckIns } from '$lib/stores/netcontrol';
	import MapPalette from '$lib/components/MapPalette.svelte';
	import {
		selectedStation, panelMode, detailTab, searchOpen, sheetState,
		selectStation, closePanel, openStationList, openMessages, openConversation, openTransports, openActivity, openAnnotations, openNetControl, openBulletins, openICS309, openWeather, openTelemetry, openDF, openPackets, openSettings,
		togglePanel, commandPaletteOpen, toggleCommandPalette
	} from '$lib/stores/ui';
	import type { SheetState, DetailTab, PanelMode } from '$lib/stores/ui';
	import type { Annotation } from '$lib/types';

	let isDesktop = $state(true);
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

	// Any check-in change (status, note, roll call, position update) re-emits
	// $activeCheckIns, and a fresh Set here would change the identity of
	// stationsWithPosition below, forcing the Map to rebuild every marker even
	// though the callsign membership is usually unchanged. Return the cached Set
	// when the contents are identical; it is only ever read via .has(), so a
	// stable reference is indistinguishable from a fresh one.
	//
	// The roster is the checked-in operators PLUS the tracked devices linked to
	// them: an operator checked in as W4ABC may have their position arriving from
	// W4ABC-9. Tracked callsigns are stored verbatim (they may carry an SSID), so
	// membership is tested against both the station key and its bare callsign.
	//
	// Scoped to the currently open net by id: the netcontrol store keeps the
	// previous net's check-ins in $checkIns until the new net's data loads, so an
	// unscoped roster would filter the map to the *last* net's operators.
	let prevNetCallsigns: Set<string> | null = null;
	let netCallsigns = $derived.by(() => {
		const next = new Set<string>();
		const openNetId = $activeNet?.status === 'open' ? $activeNet.id : null;
		for (const ci of openNetId ? $activeCheckIns : []) {
			if (ci.netId !== openNetId) continue;
			next.add(ci.callsign);
			for (const ts of ci.trackedStations ?? []) next.add(ts.callsign);
		}
		if (prevNetCallsigns && prevNetCallsigns.size === next.size) {
			let same = true;
			for (const c of next) {
				if (!prevNetCallsigns.has(c)) { same = false; break; }
			}
			if (same) return prevNetCallsigns;
		}
		prevNetCallsigns = next;
		return next;
	});

	let allStationsWithPosition = $derived(
		$stationList.filter((s) => s.position)
	);

	// The roster filter only applies while a net is running with someone on it —
	// otherwise the persisted toggle would blank the map on a later, netless day.
	// Must test the status, not just for a net object: the netcontrol store keeps
	// the closed net in $activeNet after a net_updated close event, so
	// `$activeNet !== null` stays true and the map would stay filtered.
	let netIsOpen = $derived($activeNet?.status === 'open');

	// Availability counts what the filter would actually leave on the map, not
	// raw check-ins: a roster of voice-only operators with no position anywhere
	// passes a check-in count gate and then blanks the map.
	let rosterMappedCount = $derived.by(() => {
		if (!netIsOpen) return 0;
		let n = 0;
		for (const s of allStationsWithPosition) {
			const key = s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
			if (netCallsigns.has(s.callsign) || netCallsigns.has(key)) n++;
		}
		// Check-ins placed manually draw on their own (unfiltered) operator
		// layer, so they keep the map populated even with no roster station.
		const openNetId = $activeNet?.id ?? null;
		for (const ci of $operatorsWithPosition) {
			if (ci.netId === openNetId) n++;
		}
		return n;
	});
	let rosterFilterAvailable = $derived(netIsOpen && rosterMappedCount > 0);
	let rosterFilterActive = $derived($mapSettings.showRosterOnly && rosterFilterAvailable);

	let stationsWithPosition = $derived.by(() => {
		const cutoffMs = AGE_FILTER_MS[$mapSettings.stationAgeFilter];
		const rosterOnly = rosterFilterActive;
		if (cutoffMs === Infinity && !rosterOnly) return allStationsWithPosition;
		const now = Date.now();
		const sel = $selectedStation;
		return allStationsWithPosition.filter((s) => {
			const key = s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
			// Always show selected station — it would otherwise vanish out from
			// under the detail panel the user has open on it.
			if (key === sel) return true;
			// Always show roster stations (operators and their tracked devices)
			if (netCallsigns.has(s.callsign) || netCallsigns.has(key)) return true;
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
		const mq = window.matchMedia('(min-width: 769px)');
		isDesktop = mq.matches;
		const handler = (e: MediaQueryListEvent) => { isDesktop = e.matches; };
		mq.addEventListener('change', handler);
		return () => mq.removeEventListener('change', handler);
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
		/>
	</div>

	<!-- Map layers palette (bottom-left) -->
	<!-- rosterCount is the count the filter actually gates on, not the raw roster
	     size: a voice-only roster has check-ins but nothing on the map, and
	     netCallsigns.size there would enable a checkbox that changes nothing. -->
	<MapPalette
		filteredCount={stationsWithPosition.length}
		totalCount={allStationsWithPosition.length}
		hasActiveNet={netIsOpen}
		rosterCount={rosterMappedCount}
		nextStopEnabled={showNextStop}
		hasCourse={routeAnnotations.length > 0}
		onNextStopToggle={toggleNextStopPill}
	/>

	<!-- Live GPS follow toggle + status -->
	<GpsFollowControl oncenter={() => mapRef?.centerOnOwnPosition()} />
	<GpsStatusPill />

	<!-- Along-course distance readout (below the GPS status pill) -->
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

	<!-- Floating Ops View restore button -->
	{#if $opsView && $activeNet}
		<button class="ops-view-fab" onclick={handleGoToOpsView} title="Return to Ops View">
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<circle cx="8" cy="7" r="3" stroke="currentColor" stroke-width="1.5"/>
				<path d="M8 1C4.5 1 1.5 3.5 1 7c.5 3.5 3.5 6 7 6s6.5-2.5 7-6c-.5-3.5-3.5-6-7-6z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
				<path d="M8 13v2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
			</svg>
		</button>
	{/if}

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
			onBack={$panelMode === 'convo' ? openMessages : undefined}
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

	<!-- Mobile: Bottom Sheet -->
	{#if !isDesktop}
		<BottomSheet
			sheetLevel={$sheetState}
			onStateChange={handleSheetStateChange}
		>
			{#snippet peekContent()}
				<div class="sheet-peek-row">
					<button class="peek-status" onclick={openTransports} aria-label="Transport status — open transports">
						<ConnectionStatus />
					</button>
					<span class="station-count">{$stationList.length} stations</span>
				</div>
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
			{/if}
		</BottomSheet>
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
		inset: 0;
		z-index: var(--z-map);
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

	.ops-view-fab {
		position: absolute;
		top: 80px;
		left: 10px;
		z-index: var(--z-toolbar);
		width: 40px;
		height: 40px;
		border-radius: 8px;
		border: 2px solid #22c55e;
		background: var(--color-surface);
		background: rgba(var(--color-surface-rgb, 30, 30, 30), 0.9);
		color: #22c55e;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
		transition: background 0.15s, border-color 0.15s;
	}

	.ops-view-fab:hover {
		background: rgba(34, 197, 94, 0.15);
		border-color: #4ade80;
	}
</style>
