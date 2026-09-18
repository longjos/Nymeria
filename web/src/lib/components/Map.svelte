<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { Station, Annotation, NetCheckIn, NetMission } from '$lib/types';
	import { categoryMeta } from '$lib/annotationMeta';
	import { symbolInfo } from '$lib/symbols';
	import { createStationIcon } from '$lib/aprs-icons';
	import { createOwnPositionIcon } from '$lib/gps-icons';
	import { formatGpsAge } from '$lib/stores/gps';
	import { isMoving, computeDRCone, DR_UPDATE_INTERVAL_MS } from '$lib/deadReckoning';
	import { stationDisplayName } from '$lib/utils';
	import { getTacticalAlias } from '$lib/stores/tactical';
	import { weatherUnits } from '$lib/stores/weather';
	import { convertTemp, convertWindSpeed } from '$lib/units';
	import type { UnitSystem } from '$lib/units';
	import type { GpsFix, WxAlert, WxLinkState, WxMapMode, WxPolygonGeometry } from '$lib/types';
	import { get } from 'svelte/store';
	import L from 'leaflet';
	import type { MeasureBand } from './NextStopPill.svelte';
	import { readWxTokens, pathOptions, ringsOf, nearestVertex, chipHtml, drawOrder, fallbackLayers, pointInAlert, type WxTokens } from '$lib/wxAlertMap';
	import { tierRank, zoneLabel } from '$lib/wxAlertMeta';
	import { clock } from '$lib/wxAlertTime';
	import { ensureZoneGeometry } from '$lib/stores/wxAlerts';

	const DEFAULT_ANN_COLOR = '#e63946';

	// The interactive layer (if any) a pick/draw click landed on. 'map' means
	// the click hit the background (a tile, or an interactive:false layer) —
	// see layerClick / commitPick below, the single choke point every
	// pick/draw mode's click now goes through, layer or background alike.
	type PickSource =
		| { kind: 'map' }
		| { kind: 'annotation'; id: string }
		| { kind: 'station'; callsign: string; label: string }
		| { kind: 'operator'; id: string; label: string }
		| { kind: 'mission'; id: string; label: string };

	let {
		stations = [],
		annotations = [],
		selectedCallsign = '',
		onStationClick,
		onAnnotationClick,
		flyToTarget,
		panelOpen = false,
		drawingMode = null,
		onDrawComplete,
		previewGeometry = null,
		previewColor = DEFAULT_ANN_COLOR,
		editingAnnotationId = null,
		onGeometryEdit,
		onPreviewGeometryChange,
		netOperators = [],
		netMissions = [],
		netAssignmentLines = [],
		activeNetId = null,
		onNetOperatorClick,
		onNetMissionClick,
		flyToBounds = null,
		highlightedMissionId = null,
		highlightedCheckInId = null,
		weatherOverlay = [],
		showWeatherOverlay = false,
		dfOverlay = [],
		showDFOverlay = false,
		placingOperator = null,
		onOperatorPlaced,
		onPlaceCancelled,
		netLocationAnnotations = [],
		showNetLocationAnnotations = false,
		showTracks = true,
		trackDurationMs = Infinity,
		showDRCones = true,
		showCallsigns = false,
		placingAnnotation = null,
		onAnnotationPlaced,
		onAnnotationPlaceCancelled,
		placingMissionLocation = null,
		missionDraftPoint = null,
		onMissionLocationPlaced,
		onMissionLocationAnnotationPicked,
		onMissionLocationPlaceCancelled,
		ownPosition = null,
		ownPositionStale = false,
		follow = false,
		onFollowBreak,
		onDistanceOriginPicked,
		onDistanceOriginCleared,
		distanceDraftPoint = null,
		measureBand = null,
		wxAlerts = [],
		wxZoneGeometry = new Map(),
		wxLinkState = 'off',
		wxMapMode = 'watches',
		wxFootprintCentroid = null,
		wxFootprintPreview = null,
		wxFocusAlertId = null,
		onWxAlertClick,
		onWxFocusConsumed,
	}: {
		stations?: Station[];
		annotations?: Annotation[];
		selectedCallsign?: string;
		onStationClick?: (stationKey: string) => void;
		onAnnotationClick?: (id: string) => void;
		flyToTarget?: { lat: number; lon: number; zoom?: number } | null;
		panelOpen?: boolean;
		drawingMode?: 'point' | 'line' | 'area' | null;
		onDrawComplete?: (geometry: string) => void;
		previewGeometry?: string | null;
		previewColor?: string;
		editingAnnotationId?: string | null;
		onGeometryEdit?: (geometry: string) => void;
		onPreviewGeometryChange?: (geometry: string) => void;
		netOperators?: NetCheckIn[];
		netMissions?: NetMission[];
		netAssignmentLines?: Array<{ operator: NetCheckIn; mission: NetMission }>;
		activeNetId?: string | null;
		onNetOperatorClick?: (checkInId: string) => void;
		onNetMissionClick?: (missionId: string) => void;
		flyToBounds?: Array<{ lat: number; lon: number }> | null;
		highlightedMissionId?: string | null;
		highlightedCheckInId?: string | null;
		weatherOverlay?: Station[];
		showWeatherOverlay?: boolean;
		dfOverlay?: Station[];
		showDFOverlay?: boolean;
		placingOperator?: { id: string; callsign: string } | null;
		onOperatorPlaced?: (id: string, lat: number, lon: number) => void;
		onPlaceCancelled?: () => void;
		netLocationAnnotations?: Annotation[];
		showNetLocationAnnotations?: boolean;
		showTracks?: boolean;
		trackDurationMs?: number;
		showDRCones?: boolean;
		/** Draw permanent call-sign labels next to station markers. */
		showCallsigns?: boolean;
		placingAnnotation?: { id: string | null; name: string } | null;
		onAnnotationPlaced?: (lat: number, lon: number) => void;
		onAnnotationPlaceCancelled?: () => void;
		placingMissionLocation?: { label: string } | null;
		missionDraftPoint?: { lat: number; lon: number } | null;
		onMissionLocationPlaced?: (lat: number, lon: number, label?: string) => void;
		/** Fired instead of onMissionLocationPlaced when the click landed on an
		 * existing annotation layer — the annotation IS the location (snap +
		 * link), rather than a dropped pin near it. lat/lon are the clicked
		 * point, used only as a fallback for LineString/Polygon geometry. */
		onMissionLocationAnnotationPicked?: (annotationId: string, lat: number, lon: number) => void;
		onMissionLocationPlaceCancelled?: () => void;
		/** Live host GPS fix (own-position marker). null = no usable fix / GPS off. */
		ownPosition?: GpsFix | null;
		ownPositionStale?: boolean;
		/** When true, the map re-centers on every accepted own_position update. */
		follow?: boolean;
		/** Fired when the user manually drags/zooms (or a deliberate fly-to fires) while following. */
		onFollowBreak?: () => void;
		/**
		 * Long-press / right-click picked a "measure from here" origin. `src`
		 * carries what the gesture landed on so the caller can label the pill
		 * with a callsign or annotation name instead of a bare coordinate.
		 * This is deliberately NOT a pick mode: it arms and commits in one
		 * gesture, so it never joins anyPlaceMode and never steals a click.
		 */
		onDistanceOriginPicked?: (
			lat: number,
			lon: number,
			src: { kind: 'map' } | { kind: 'annotation' | 'station'; id: string; label: string }
		) => void;
		/** Escape with nothing else armed: drop the long-pressed origin. */
		onDistanceOriginCleared?: () => void;
		/** Draggable pin marking a long-pressed distance origin. */
		distanceDraftPoint?: { lat: number; lon: number } | null;
		/** What the Next Stop card measured — drawn only while the card is open. */
		measureBand?: MeasureBand | null;
		/** Active IN + NEAR NWS alerts (FAR is never broadcast/retained — no outside-footprint view, BUILD-PLAN §3.1). */
		wxAlerts?: WxAlert[];
		/** Cached zone polygons by UGC, filled lazily by stores/wxAlerts.ts ensureZoneGeometry(). */
		wxZoneGeometry?: Map<string, WxPolygonGeometry>;
		wxLinkState?: WxLinkState;
		wxMapMode?: WxMapMode;
		/** Net footprint centroid — chip placement fallback and the "no geometry at all" case. */
		wxFootprintCentroid?: { lat: number; lon: number } | null;
		/** Non-null for ~10 s after "Show on map" in the watch-area sheet. */
		wxFootprintPreview?: WxPolygonGeometry | null;
		/** "Show on map" target from a row/detail/banner — fitBounds then onWxFocusConsumed(). */
		wxFocusAlertId?: string | null;
		onWxAlertClick?: (id: string) => void;
		onWxFocusConsumed?: () => void;
	} = $props();

	let mapEl: HTMLDivElement;
	let map: L.Map;

	export function getViewport(): { lat: number; lon: number; zoom: number } | null {
		if (!map) return null;
		const c = map.getCenter();
		return { lat: c.lat, lon: c.lng, zoom: map.getZoom() };
	}

	/** Pans (without zooming) to the current own-position fix, if any. */
	export function centerOnOwnPosition(): void {
		if (!map || !ownPosition || ownPosition.mode < 2) return;
		programmaticMove = true;
		map.setView([ownPosition.lat, ownPosition.lon], map.getZoom(), { animate: true });
		map.once('moveend', () => { programmaticMove = false; });
	}
	let markers: Map<string, L.Marker> = new Map();
	// Render caches, keyed exactly like `markers`. These exist purely so
	// updateMarkers() can skip work whose output would be byte-identical to
	// what is already in the DOM — every inbound APRS packet re-runs the
	// station effect, and rebuilding N icons/tooltips/polylines for the one
	// station that actually moved is what saturates the main thread.
	// Every entry is deleted alongside its layer, so a removed-and-re-added
	// station is always rebuilt from scratch.
	const iconSigs: Map<string, string> = new Map();
	const tooltipState: Map<string, { name: string; permanent: boolean }> = new Map();
	const trackSigs: Map<string, string> = new Map();
	// key -> cached bbox of the drawn track, invalidated by the same inputs as
	// the polyline itself. See trackBBox().
	const trackBBoxes: Map<string, { sig: string; box: { minLat: number; maxLat: number; minLon: number; maxLon: number } | null }> = new Map();
	let trackLines: Map<string, L.Polyline> = new Map();
	let trackHighlights: Map<string, L.Polyline> = new Map();
	let drCones: Map<string, L.Polygon> = new Map();
	let drCenterLines: Map<string, L.Polyline> = new Map();
	let drTimer: ReturnType<typeof setInterval> | null = null;
	let annotationLayers: Map<string, L.Layer> = new Map();

	// Drawing state
	let drawVertices: L.LatLng[] = [];
	let drawMarkers: L.CircleMarker[] = [];
	let drawLine: L.Polyline | null = null;

	// Preview layer for unsaved annotation geometry
	let previewLayer: L.Layer | null = null;

	// Vertex editing state
	let vertexHandles: L.Marker[] = [];
	let editShape: L.Polyline | L.Polygon | L.CircleMarker | null = null;

	// Draft pin for the mission location picker
	let missionDraftMarker: L.Marker | null = null;

	// Draft pin for a long-pressed "measure from here" origin.
	let distanceDraftMarker: L.Marker | null = null;
	// Rubber band showing what the Next Stop card measured.
	let measureLayers: L.Layer[] = [];
	// Suppresses the native contextmenu that some browsers fire right after
	// our own touch-hold has already committed, so one long press is one pick.
	let longPressFiredAt = 0;
	let longPressTimer: ReturnType<typeof setTimeout> | null = null;
	let longPressStart: { x: number; y: number } | null = null;

	// Net overlay layers
	let netHalos: Map<string, L.CircleMarker | L.Marker> = new Map();
	let netMissionFlags: Map<string, L.Marker> = new Map();
	let netAssignLines: L.Polyline[] = [];
	// Highlight overlay layers for hovered mission
	let highlightOverlays: L.Layer[] = [];
	// Single operator highlight (assign picker hover)
	let operatorHighlight: L.Layer | null = null;
	// Weather overlay markers
	let wxMarkers: Map<string, L.Marker> = new Map();
	// NWS Alerts (internal/wxalert) — one L.LayerGroup + one chip marker per
	// alert id (rebuilt whenever the alert set/link state/map mode changes;
	// unlike stations this list is small, so a full clear+redraw is simplest
	// and cheap — see the effect below for why this deliberately does not
	// chase the spec's id+updatedAt diffing).
	let wxAlertRenderer: L.SVG | null = null;
	let wxAlertLayerGroups: Map<string, L.FeatureGroup> = new Map();
	let wxAlertChips: Map<string, L.Marker> = new Map();
	let wxFootprintPreviewLayer: L.Layer | null = null;
	let wxChipTickTimer: ReturnType<typeof setInterval> | null = null;
	// UGCs already requested this session — ensureZoneGeometry() is safe to
	// call again, but there is no reason to spam it every effect re-run
	// while the fetch is in flight.
	const wxRequestedZones = new Set<string>();
	// DF overlay layers
	let dfLines: Map<string, L.Polyline> = new Map();
	let dfRangeCircles: Map<string, L.Circle> = new Map();
	let dfTargetMarker: L.Marker | null = null;
	let dfTargetCircle: L.Circle | null = null;

	// Net location annotation layers
	let netLocMarkers: Map<string, L.Marker> = new Map();
	let netLocRouteLine: L.Polyline | null = null;

	// Own-position (live GPS) marker + accuracy ring
	let ownMarker: L.Marker | null = null;
	let ownAccuracyCircle: L.Circle | null = null;
	let ownIconSignature = '';
	// True while a follow-driven or fly-to pan/zoom is in flight, so the
	// dragstart/zoomstart listeners below don't mistake it for a user gesture.
	let programmaticMove = false;
	// Tracked as $state so the marker effect below re-runs (and does a single
	// catch-up rebuild) the moment the tab comes back. WebSocket handlers are
	// not throttled in background tabs, so without this a hidden tab keeps
	// rebuilding map layers nobody can see.
	let pageHidden = $state(typeof document !== 'undefined' ? document.hidden : false);

	// --- Viewport culling (#107) -------------------------------------------
	// The v0.11.0 pass took per-packet JS from 84ms to 6ms; what is left at high
	// marker counts is browser style/recalc/paint for one DOM node per marker,
	// which is why interactive panning stays ~flat per marker. Clustering and
	// canvas rendering both fix that by changing what the operator sees and how
	// they interact with it; culling is the only one of the three that is
	// invisible to them — a marker outside the viewport cannot be seen, so never
	// creating its node costs nothing on screen and removes most of the paint
	// cost exactly when panning hurts (zoomed in).
	const CULL_MIN_STATIONS = 200;
	/** Fraction of the viewport added on every side, so panning never pops markers in at the edge. */
	const CULL_PAD = 0.5;
	const VIEWPORT_SETTLE_MS = 120;
	// Bumped (coalesced) after the map settles so the station effect re-runs
	// against the new bounds. $state so the effect subscribes to it.
	let viewportEpoch = $state(0);
	let viewportTimer: ReturnType<typeof setTimeout> | null = null;

	function onViewportSettled() {
		// A hidden tab is not panning for anybody, and the unhide rebuild covers it.
		if (pageHidden) return;
		// One pinch-zoom fires both zoomend and moveend, and a held keyboard pan
		// fires a run of moveends — one rebuild per settle is enough.
		if (viewportTimer) clearTimeout(viewportTimer);
		viewportTimer = setTimeout(() => {
			viewportTimer = null;
			viewportEpoch++;
		}, VIEWPORT_SETTLE_MS);
	}

	const netStatusColors: Record<string, string> = {
		available: '#22c55e',
		assigned: '#3b82f6',
		enroute: '#8b5cf6',
		onscene: '#06b6d4',
		brb: '#f59e0b',
		missing: '#ef4444',
		released: '#6b7280'
	};

	const missionPriorityColors: Record<string, string> = {
		routine: '#22c55e',
		priority: '#f59e0b',
		welfare: '#3b82f6',
		emergency: '#ef4444'
	};

	const MAP_VIEW_KEY = 'nymeria_map_view';
	let saveViewTimer: ReturnType<typeof setTimeout> | null = null;

	/** Reads the persisted camera. Returns null if absent, unusable or unreadable. */
	function loadMapView(): { lat: number; lon: number; zoom: number } | null {
		try {
			const raw = localStorage.getItem(MAP_VIEW_KEY);
			if (!raw) return null;
			const v = JSON.parse(raw) as { lat?: unknown; lon?: unknown; zoom?: unknown };
			const lat = Number(v.lat), lon = Number(v.lon), zoom = Number(v.zoom);
			if (!Number.isFinite(lat) || !Number.isFinite(lon) || !Number.isFinite(zoom)) return null;
			if (lat < -90 || lat > 90 || lon < -180 || lon > 180 || zoom < 0 || zoom > 22) return null;
			return { lat, lon, zoom };
		} catch {
			// Private mode / blocked storage — fall back to the default view.
			return null;
		}
	}

	/** Coalesces a pan/zoom burst into one write. */
	function scheduleSaveMapView(): void {
		if (saveViewTimer) clearTimeout(saveViewTimer);
		saveViewTimer = setTimeout(() => {
			saveViewTimer = null;
			if (!map) return;
			try {
				const c = map.getCenter();
				localStorage.setItem(MAP_VIEW_KEY, JSON.stringify({ lat: c.lat, lon: c.lng, zoom: map.getZoom() }));
			} catch {
				// Ignore — losing the remembered view is not worth an error.
			}
		}, 400);
	}

	onMount(() => {
		const saved = loadMapView();
		map = L.map(mapEl, {
			zoomControl: true,
			attributionControl: true,
		}).setView(saved ? [saved.lat, saved.lon] : [39.8283, -98.5795], saved ? saved.zoom : 4);

		// Remember where the operator was looking. A hard refresh in the field
		// should not throw away the pan and zoom they just set up.
		map.on('moveend zoomend', scheduleSaveMapView);

		L.tileLayer('/tiles/{z}/{x}/{y}.png', {
			attribution: '&copy; OpenStreetMap contributors',
			maxZoom: 19,
		}).addTo(map);

		// NWS Alerts pane: z-index from --z-wx-pane (350) sits below Leaflet's
		// own overlayPane (400, annotations/route line) and markerPane (600),
		// so alert polygons draw under the route and every marker. A dedicated
		// SVG renderer is required (not the default shared one) because the
		// hatch fill references a <pattern> defined in the sibling .wx-defs
		// <svg> below, which only a real SVG (not Canvas) can paint.
		const wxPane = map.createPane('wxAlertPane');
		wxPane.style.zIndex = readWxTokens().zPane;
		wxAlertRenderer = L.svg({ pane: 'wxAlertPane' });
		wxChipTickTimer = setInterval(() => refreshWxChipLabels(), 60_000);
		// Chip visibility (hide < z8) is zoom-driven only — no need to rebuild
		// the whole alert layer set for it.
		map.on('zoomend', applyWxChipVisibility);

		// Fix Leaflet icon path issue with bundlers
		delete (L.Icon.Default.prototype as Record<string, unknown>)._getIconUrl;
		L.Icon.Default.mergeOptions({
			iconRetinaUrl: undefined,
			iconUrl: undefined,
			shadowUrl: undefined,
		});

		updateMarkers();
		updateAnnotations();
		updateDRCones();
		drTimer = setInterval(updateDRCones, DR_UPDATE_INTERVAL_MS);

		// Any real user drag/zoom (not one we drove ourselves) breaks follow.
		map.on('dragstart', () => { if (!programmaticMove) onFollowBreak?.(); });
		map.on('zoomstart', () => { if (!programmaticMove) onFollowBreak?.(); });

		// Viewport culling needs a rebuild whenever the visible bounds change.
		// 'move' fires continuously during a held drag; 'moveend' only on
		// release. Without the continuous one, a slow one-handed pan past the
		// 50% pad exposes empty map until the finger lifts — which is exactly
		// the gesture this is meant to help. onViewportSettled coalesces, so
		// the extra events cost one timer reset each.
		map.on('move', onViewportSettled);
		map.on('moveend', onViewportSettled);
		map.on('zoomend', onViewportSettled);

		// Long press / right-click = "measure from here". Bound on the DOM
		// container rather than via map.on('contextmenu') because L.Marker does
		// not bubble its mouse events to the map, so a Leaflet-level handler
		// would never fire over a station marker — and picking a station is
		// half the point of the gesture.
		mapEl.addEventListener('contextmenu', onMapContextMenu);
		mapEl.addEventListener('pointerdown', onMapPointerDown);
		mapEl.addEventListener('pointermove', onMapPointerMove);
		mapEl.addEventListener('pointerup', cancelLongPress);
		mapEl.addEventListener('pointercancel', cancelLongPress);

		document.addEventListener('visibilitychange', onVisibilityChange);
	});

	// --- "measure from here" long press ------------------------------------

	/** Pixel radius within which a long press counts as landing ON a feature. */
	const PICK_SNAP_PX = 22;

	/**
	 * What the gesture landed on. Resolved by proximity in screen space rather
	 * than by layer hit-testing: a thumb on a phone is far bigger than a
	 * circleMarker, and this also sidesteps Leaflet's marker/path event-bubbling
	 * split entirely.
	 */
	function identifyAt(latlng: L.LatLng):
		| { kind: 'map' }
		| { kind: 'annotation' | 'station'; id: string; label: string } {
		if (!map) return { kind: 'map' };
		const p = map.latLngToLayerPoint(latlng);
		let best: { kind: 'annotation' | 'station'; id: string; label: string } | null = null;
		let bestD = PICK_SNAP_PX;

		for (const st of stations) {
			if (!st.position) continue;
			const q = map.latLngToLayerPoint([st.position.lat, st.position.lon]);
			const d = p.distanceTo(q);
			if (d < bestD) {
				bestD = d;
				const key = stationKey(st);
				best = { kind: 'station', id: key, label: get(getTacticalAlias)(key) || key };
			}
		}
		for (const ann of annotations) {
			if (ann.type !== 'point') continue;
			try {
				const geom = JSON.parse(ann.geometry);
				if (geom?.type !== 'Point') continue;
				// GeoJSON is [lon, lat]; Leaflet wants [lat, lon].
				const q = map.latLngToLayerPoint([geom.coordinates[1], geom.coordinates[0]]);
				const d = p.distanceTo(q);
				if (d < bestD) {
					bestD = d;
					best = { kind: 'annotation', id: ann.id, label: ann.shortName || ann.label };
				}
			} catch {
				// Unparseable geometry is simply not pickable.
			}
		}
		return best ?? { kind: 'map' };
	}

	function commitDistanceOrigin(latlng: L.LatLng) {
		onDistanceOriginPicked?.(latlng.lat, latlng.lng, identifyAt(latlng));
	}

	function onMapContextMenu(e: MouseEvent) {
		// Always suppress the browser menu over the map — intended.
		e.preventDefault();
		if (!map) return;
		// An armed draw/place mode owns the gesture; measuring must not commit
		// an origin under the same press that commits the pick.
		if (anyPlaceMode) return;
		// Our own touch-hold already fired for this press on browsers that also
		// synthesize a contextmenu afterwards.
		if (Date.now() - longPressFiredAt < 1000) return;
		commitDistanceOrigin(map.mouseEventToLatLng(e));
	}

	function onMapPointerDown(e: PointerEvent) {
		cancelLongPress();
		if (e.pointerType === 'mouse') return; // right-click covers desktop
		if (!map) return;
		// Careful aiming for a draw/place tap takes well over 600 ms; arming the
		// long press here would fire "measure from here" mid-gesture.
		if (anyPlaceMode) return;
		longPressStart = { x: e.clientX, y: e.clientY };
		const ev = e;
		longPressTimer = setTimeout(() => {
			longPressTimer = null;
			longPressStart = null;
			longPressFiredAt = Date.now();
			commitDistanceOrigin(map.mouseEventToLatLng(ev as unknown as MouseEvent));
		}, 600);
	}

	function onMapPointerMove(e: PointerEvent) {
		// A pan is not a long press: 10 px of travel cancels it.
		if (!longPressStart) return;
		if (Math.abs(e.clientX - longPressStart.x) > 10 || Math.abs(e.clientY - longPressStart.y) > 10) {
			cancelLongPress();
		}
	}

	function cancelLongPress() {
		if (longPressTimer) clearTimeout(longPressTimer);
		longPressTimer = null;
		longPressStart = null;
	}

	function onVisibilityChange() {
		pageHidden = document.hidden;
	}

	onDestroy(() => {
		if (drTimer) clearInterval(drTimer);
		if (viewportTimer) clearTimeout(viewportTimer);
		if (saveViewTimer) clearTimeout(saveViewTimer);
		if (wxChipTickTimer) clearInterval(wxChipTickTimer);
		for (const [, g] of wxAlertLayerGroups) g.remove();
		for (const [, m] of wxAlertChips) m.remove();
		wxFootprintPreviewLayer?.remove();
		if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibilityChange);
		for (const [, layer] of drCones) layer.remove();
		for (const [, layer] of drCenterLines) layer.remove();
		for (const [, hl] of trackHighlights) hl.remove();
		for (const layer of highlightOverlays) layer.remove();
		highlightOverlays = [];
		operatorHighlight?.remove();
		missionDraftMarker?.remove();
		distanceDraftMarker?.remove();
		for (const l of measureLayers) l.remove();
		measureLayers = [];
		cancelLongPress();
		mapEl?.removeEventListener('contextmenu', onMapContextMenu);
		mapEl?.removeEventListener('pointerdown', onMapPointerDown);
		mapEl?.removeEventListener('pointermove', onMapPointerMove);
		mapEl?.removeEventListener('pointerup', cancelLongPress);
		mapEl?.removeEventListener('pointercancel', cancelLongPress);
		ownMarker?.remove();
		ownAccuracyCircle?.remove();
		map?.remove();
	});

	$effect(() => {
		// Touch reactive props so toggling triggers re-render
		const _tracks = showTracks;
		const _trackDur = trackDurationMs;
		const _dr = showDRCones;
		const _labels = showCallsigns;
		// Viewport changes decide which stations survive culling in updateMarkers().
		const _viewport = viewportEpoch;
		// Read unconditionally so the effect always re-subscribes to it: while
		// hidden the effect stops touching `stations`, and unhiding is the only
		// thing that can bring it back.
		const hidden = pageHidden;
		if (map && !hidden) {
			updateMarkers();
			updateDRCones();
		}
	});

	$effect(() => {
		if (map) updateAnnotations();
	});

	// Fly to target when it changes — a deliberate navigation elsewhere
	// (search, station select, ...), so it breaks GPS follow if it's on.
	$effect(() => {
		if (map && flyToTarget) {
			programmaticMove = true;
			map.flyTo([flyToTarget.lat, flyToTarget.lon], flyToTarget.zoom ?? 14);
			map.once('moveend', () => { programmaticMove = false; });
			onFollowBreak?.();
		}
	});

	// Fly to bounds (multi-point) when they change — same rationale as above.
	$effect(() => {
		if (map && flyToBounds && flyToBounds.length > 0) {
			const bounds = L.latLngBounds(flyToBounds.map(p => [p.lat, p.lon] as L.LatLngExpression));
			programmaticMove = true;
			map.flyToBounds(bounds, { padding: [50, 50], maxZoom: 16 });
			map.once('moveend', () => { programmaticMove = false; });
			onFollowBreak?.();
		}
	});

	// GPS follow: pan (never zoom) to the live fix on every accepted update.
	$effect(() => {
		if (map && follow && ownPosition && ownPosition.mode >= 2) {
			programmaticMove = true;
			map.panTo([ownPosition.lat, ownPosition.lon], { animate: true, duration: 0.4 });
			map.once('moveend', () => { programmaticMove = false; });
		}
	});

	// Own-position marker + accuracy ring. Updated in place (setLatLng /
	// setRadius) so a 1/s fix stream never respawns DOM nodes; the divIcon
	// itself is only rebuilt when its visual signature actually changes.
	$effect(() => {
		if (!map) return;
		const fix = ownPosition;
		const stale = ownPositionStale;

		if (!fix || fix.mode < 2) {
			// No fix — remove the whole layer. A stale wrong position is worse
			// than none, so we don't fall back to drawing the last known point.
			ownMarker?.remove();
			ownMarker = null;
			ownAccuracyCircle?.remove();
			ownAccuracyCircle = null;
			ownIconSignature = '';
			return;
		}

		const moving = fix.hasCourse && fix.speedKnots >= 1;
		const courseBucket = moving ? Math.round(fix.course / 5) * 5 : 0;
		const sig = `${fix.mode}|${stale}|${moving}|${courseBucket}`;

		if (!ownMarker) {
			ownMarker = L.marker([fix.lat, fix.lon], {
				icon: createOwnPositionIcon(fix, stale),
				interactive: false,
				keyboard: false,
				zIndexOffset: 1000,
			}).addTo(map);
			ownIconSignature = sig;
		} else {
			ownMarker.setLatLng([fix.lat, fix.lon]);
			if (sig !== ownIconSignature) {
				ownMarker.setIcon(createOwnPositionIcon(fix, stale));
				ownIconSignature = sig;
			}
		}

		if (stale) {
			const ageMs = Date.now() - new Date(fix.receivedAt).getTime();
			ownMarker.unbindTooltip();
			ownMarker.bindTooltip(`Last fix ${formatGpsAge(ageMs)} ago`, {
				direction: 'top',
				className: 'own-pos-tooltip',
			});
		} else {
			ownMarker.unbindTooltip();
		}

		if (fix.accuracy && fix.accuracy > 0 && fix.accuracy <= 500) {
			if (!ownAccuracyCircle) {
				ownAccuracyCircle = L.circle([fix.lat, fix.lon], {
					radius: fix.accuracy,
					color: '#38bdf8',
					weight: 1,
					opacity: 0.5,
					fillColor: '#38bdf8',
					fillOpacity: 0.10,
					interactive: false,
				}).addTo(map);
			} else {
				ownAccuracyCircle.setLatLng([fix.lat, fix.lon]);
				ownAccuracyCircle.setRadius(fix.accuracy);
			}
		} else {
			ownAccuracyCircle?.remove();
			ownAccuracyCircle = null;
		}
	});


	// Invalidate map size when panel opens/closes
	$effect(() => {
		const _open = panelOpen;
		if (map) {
			setTimeout(() => map.invalidateSize(), 400);
		}
	});

	// Any place/draw mode being active is the single source of truth for the
	// cursor + double-click-zoom toggle, so no individual mode effect below
	// needs to guard against the others still being active.
	let anyPlaceMode = $derived(
		!!drawingMode || !!placingOperator || !!placingAnnotation || !!placingMissionLocation
	);

	$effect(() => {
		if (!map) return;
		if (anyPlaceMode) {
			mapEl.style.cursor = 'crosshair';
			map.doubleClickZoom.disable();
		} else {
			mapEl.style.cursor = '';
			map.doubleClickZoom.enable();
		}
	});

	// Single click registration for every pick/draw mode. A click that lands
	// on an interactive layer never reaches here — layerClick (below)
	// intercepts it first and calls commitPick directly — so this only ever
	// fires for a background click (a tile, or an interactive:false layer).
	$effect(() => {
		if (!map) return;
		if (anyPlaceMode) {
			map.on('click', handleBackgroundPickClick);
			if (drawingMode) map.on('dblclick', handleDrawDblClick);
			else map.off('dblclick', handleDrawDblClick);
		} else {
			map.off('click', handleBackgroundPickClick);
			map.off('dblclick', handleDrawDblClick);
		}
	});

	// Drawing state (accumulated vertices/preview line) is scoped to
	// drawingMode specifically, independent of anyPlaceMode's transitions —
	// switching straight from drawing into another pick mode (the four
	// modes are mutually exclusive by construction, enforced in +page.svelte)
	// must still discard any in-progress, uncommitted vertices, even though
	// anyPlaceMode itself never goes false across that switch.
	$effect(() => {
		if (!drawingMode) clearDrawState();
	});

	// Draft pin for the mission location picker — independent of pick mode so
	// it stays draggable for nudging after the initial click commits it.
	$effect(() => {
		if (!map) return;
		if (missionDraftMarker) {
			missionDraftMarker.remove();
			missionDraftMarker = null;
		}
		if (!missionDraftPoint) return;
		const icon = L.divIcon({
			className: 'mission-draft-icon',
			html: '<div class="mission-draft-marker"><div class="mission-draft-pulse"></div><div class="mission-draft-pin"></div></div>',
			iconSize: [28, 34],
			iconAnchor: [14, 34],
		});
		const marker = L.marker([missionDraftPoint.lat, missionDraftPoint.lon], {
			icon,
			draggable: true,
			zIndexOffset: 1000,
			keyboard: false,
		}).addTo(map);
		marker.on('dragend', (e) => {
			const ll = (e.target as L.Marker).getLatLng();
			onMissionLocationPlaced?.(ll.lat, ll.lng);
		});
		marker.bindTooltip('Mission location — drag to adjust', { direction: 'top', className: 'annotation-tooltip' });
		missionDraftMarker = marker;
	});

	// Draft pin for a long-pressed distance origin. Same divIcon/marker recipe
	// as the mission draft pin, so the two read as one affordance; dragging it
	// re-fires the pick so the answer follows the pin live.
	$effect(() => {
		if (!map) return;
		if (distanceDraftMarker) {
			distanceDraftMarker.remove();
			distanceDraftMarker = null;
		}
		if (!distanceDraftPoint) return;
		const icon = L.divIcon({
			className: 'mission-draft-icon',
			html: '<div class="mission-draft-marker"><div class="mission-draft-pulse"></div><div class="mission-draft-pin"></div></div>',
			iconSize: [28, 34],
			iconAnchor: [14, 34],
		});
		const marker = L.marker([distanceDraftPoint.lat, distanceDraftPoint.lon], {
			icon,
			draggable: true,
			zIndexOffset: 1000,
			keyboard: false,
		}).addTo(map);
		marker.on('dragend', (e) => {
			const ll = (e.target as L.Marker).getLatLng();
			onDistanceOriginPicked?.(ll.lat, ll.lng, { kind: 'map' });
		});
		marker.bindTooltip('Measuring from here — drag to adjust', { direction: 'top', className: 'annotation-tooltip' });
		distanceDraftMarker = marker;
	});

	// The rubber band: draw exactly what was measured. Remove-then-recreate on
	// every change, the same discipline updateAnnotations() uses.
	$effect(() => {
		if (!map) return;
		for (const l of measureLayers) l.remove();
		measureLayers = [];
		const band = measureBand;
		if (!band) return;
		if (band.kind === 'road' && band.latlngs.length > 1) {
			measureLayers.push(
				L.polyline(band.latlngs as L.LatLngExpression[], {
					color: '#e94560',
					weight: 5,
					opacity: 0.9,
					dashArray: '10 8',
					interactive: false,
				}).addTo(map)
			);
		} else if (band.kind === 'direct' && band.from && band.to) {
			measureLayers.push(
				L.polyline([[band.from.lat, band.from.lon], [band.to.lat, band.to.lon]] as L.LatLngExpression[], {
					color: '#f59e0b',
					weight: 4,
					opacity: 0.9,
					dashArray: '4 8',
					interactive: false,
				}).addTo(map)
			);
		}
		// Amber tick from the operator to the point on the course the road
		// answer was actually measured from — visible proof of the snap.
		if (band.snap) {
			measureLayers.push(
				L.polyline(
					[[band.snap.from.lat, band.snap.from.lon], [band.snap.to.lat, band.snap.to.lon]] as L.LatLngExpression[],
					{ color: '#f59e0b', weight: 2, opacity: 0.9, dashArray: '3 4', interactive: false }
				).addTo(map)
			);
		}
	});

	// Preview layer for unsaved geometry
	$effect(() => {
		if (!map) return;

		// Remove old preview
		if (previewLayer) {
			(previewLayer as L.Layer & { remove: Function }).remove();
			previewLayer = null;
		}

		if (!previewGeometry) return;

		let geom: { type: string; coordinates: unknown };
		try {
			geom = JSON.parse(previewGeometry);
		} catch {
			return;
		}

		const color = previewColor;
		const previewStyle = {
			color,
			weight: 3,
			opacity: 0.7,
			dashArray: '8 6',
			fillColor: color,
			fillOpacity: 0.12,
		};

		if (geom.type === 'Point') {
			const coords = geom.coordinates as [number, number];
			previewLayer = L.circleMarker([coords[1], coords[0]], {
				radius: 10,
				...previewStyle,
			}).addTo(map);
		} else if (geom.type === 'LineString') {
			const coords = geom.coordinates as [number, number][];
			const latlngs = coords.map((c) => [c[1], c[0]] as L.LatLngExpression);
			previewLayer = L.polyline(latlngs, previewStyle).addTo(map);
		} else if (geom.type === 'Polygon') {
			const rings = geom.coordinates as [number, number][][];
			const latlngs = rings[0].map((c) => [c[1], c[0]] as L.LatLngExpression);
			previewLayer = L.polygon(latlngs, previewStyle).addTo(map);
		}

		if (previewLayer) {
			(previewLayer as L.Layer & { bindTooltip: Function }).bindTooltip('Unsaved', {
				permanent: true,
				direction: 'center',
				className: 'preview-tooltip',
			});
		}
	});

	// Vertex editing effect
	$effect(() => {
		cleanupVertexHandles();
		if (!map) return;

		// Pre-save: show handles on preview when not actively drawing
		if (previewGeometry && !drawingMode) {
			setupVertexHandles(previewGeometry, previewColor, (newGeom) => {
				onPreviewGeometryChange?.(newGeom);
			});
		}

		// Post-save: show handles on annotation being edited
		if (editingAnnotationId) {
			const ann = annotations.find((a) => a.id === editingAnnotationId);
			if (ann) {
				// Hide the saved annotation layer
				annotationLayers.get(ann.id)?.remove();
				const style = parseStyle(ann.style);
				const color = (style.color as string) || DEFAULT_ANN_COLOR;
				setupVertexHandles(ann.geometry, color, (newGeom) => {
					onGeometryEdit?.(newGeom);
				});
			}
		}
	});

	// Net operator halos
	$effect(() => {
		if (!map) return;
		const ops = netOperators;
		const _netId = activeNetId;
		const _labels = showCallsigns;

		// Clear old halos
		for (const [, layer] of netHalos) layer.remove();
		netHalos.clear();

		if (!_netId || !ops.length) return;

		for (const ci of ops) {
			if (ci.lat == null || ci.lon == null) continue;
			const color = netStatusColors[ci.status] || '#6b7280';
			const staleMs = Date.now() - new Date(ci.lastHeard).getTime();
			const opacity = staleMs > 20 * 60 * 1000 ? 0.4 : 1;

			let layer: L.CircleMarker | L.Marker;
			if (ci.source === 'voice') {
				// Voice-only: map pin marker
				const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="36" viewBox="0 0 24 36" style="opacity:${opacity}">` +
					`<path d="M12 0C5.4 0 0 5.4 0 12c0 9 12 24 12 24s12-15 12-24C24 5.4 18.6 0 12 0z" fill="${color}"/>` +
					`<circle cx="12" cy="12" r="5" fill="rgba(0,0,0,0.25)"/>` +
					`<circle cx="12" cy="12" r="4" fill="#fff"/>` +
					`</svg>`;
				const icon = L.divIcon({
					className: 'net-voice-marker',
					html: svg,
					iconSize: [24, 36],
					iconAnchor: [12, 36],
				});
				layer = L.marker([ci.lat, ci.lon], { icon, interactive: true }).addTo(map);
			} else {
				// APRS: circle halo
				layer = L.circleMarker([ci.lat, ci.lon], {
					radius: 12,
					weight: 3,
					color,
					fillColor: 'transparent',
					fillOpacity: 0,
					opacity,
				}).addTo(map);
			}

			// Voice-only pins are the station's only marker, so they honour the
			// call-sign label toggle. APRS operators already have a labelled
			// station marker under the halo — don't double up.
			(layer as L.Layer & { bindTooltip: Function }).bindTooltip(
				ci.tacticalCall ? `${ci.callsign} "${ci.tacticalCall}"` : ci.callsign,
				ci.source === 'voice'
					? stationTooltipOpts()
					: { permanent: false, direction: 'top', className: 'station-tooltip' }
			);
			(layer as L.Layer & { on: Function }).on('click', (e: L.LeafletMouseEvent) => {
				layerClick(
					e,
					{ kind: 'operator', id: ci.id, label: ci.callsign },
					layer.getLatLng?.() ?? null,
					() => onNetOperatorClick?.(ci.id)
				);
			});
			netHalos.set(ci.id, layer);
		}
	});

	// Net mission flags
	$effect(() => {
		if (!map) return;
		const ms = netMissions;
		const _netId = activeNetId;

		// Clear old flags
		for (const [, layer] of netMissionFlags) layer.remove();
		netMissionFlags.clear();

		if (!_netId || !ms.length) return;

		for (const m of ms) {
			if (m.lat == null || m.lon == null) continue;
			const color = missionPriorityColors[m.priority] || '#6b7280';
			const html = `<div style="width:0;height:0;border-left:8px solid ${color};border-top:6px solid transparent;border-bottom:6px solid transparent;filter:drop-shadow(0 1px 2px rgba(0,0,0,0.4));"></div>`;
			const icon = L.divIcon({
				className: 'net-mission-flag',
				html,
				iconSize: [8, 12],
				iconAnchor: [0, 6],
			});
			const marker = L.marker([m.lat, m.lon], { icon, interactive: true }).addTo(map);
			marker.bindTooltip(m.title, {
				permanent: false,
				direction: 'right',
				className: 'annotation-tooltip',
			});
			marker.on('click', (e: L.LeafletMouseEvent) => {
				layerClick(
					e,
					{ kind: 'mission', id: m.id, label: m.title },
					marker.getLatLng(),
					() => onNetMissionClick?.(m.id)
				);
			});
			netMissionFlags.set(m.id, marker);
		}
	});

	// Net assignment lines
	$effect(() => {
		if (!map) return;
		const lines = netAssignmentLines;
		const _netId = activeNetId;

		// Clear old lines
		for (const line of netAssignLines) line.remove();
		netAssignLines = [];

		if (!_netId || !lines.length) return;

		for (const { operator, mission } of lines) {
			if (operator.lat == null || operator.lon == null) continue;
			if (mission.lat == null || mission.lon == null) continue;
			const polyline = L.polyline(
				[[operator.lat, operator.lon], [mission.lat, mission.lon]],
				{
					color: '#3b82f6',
					weight: 2,
					opacity: 0.6,
					dashArray: '6 4',
				}
			).addTo(map);
			netAssignLines.push(polyline);
		}
	});

	// Highlight overlay: glow rings on annotations + operators for hovered mission
	$effect(() => {
		if (!map) return;
		const hovered = highlightedMissionId;

		// Clear previous overlays
		for (const layer of highlightOverlays) layer.remove();
		highlightOverlays = [];

		if (!hovered) return;

		// Highlight operator halos with pulsing ring
		for (const ci of netOperators) {
			if (ci.lat == null || ci.lon == null) continue;
			if (!ci.missionIds?.includes(hovered)) continue;
			const ring = L.circleMarker([ci.lat, ci.lon], {
				radius: 18,
				weight: 3,
				color: '#fff',
				fillColor: netStatusColors[ci.status] || '#6b7280',
				fillOpacity: 0.25,
				opacity: 0.8,
				className: 'highlight-pulse-ring',
			}).addTo(map);
			highlightOverlays.push(ring);
		}

		// Highlight mission flag
		for (const m of netMissions) {
			if (m.lat == null || m.lon == null) continue;
			if (m.id !== hovered) continue;
			const ring = L.circleMarker([m.lat, m.lon], {
				radius: 18,
				weight: 3,
				color: missionPriorityColors[m.priority] || '#6b7280',
				fillColor: missionPriorityColors[m.priority] || '#6b7280',
				fillOpacity: 0.2,
				opacity: 0.8,
				className: 'highlight-pulse-ring',
			}).addTo(map);
			highlightOverlays.push(ring);
		}

		// Highlight linked annotations
		for (const ann of annotations) {
			if (!ann.missionIds?.includes(hovered)) continue;
			let geom: { type: string; coordinates: unknown };
			try { geom = JSON.parse(ann.geometry); } catch { continue; }

			const annStyle = parseStyle(ann.style);
			const color = (annStyle.color as string) || DEFAULT_ANN_COLOR;

			if (geom.type === 'Point') {
				const coords = geom.coordinates as [number, number];
				const ring = L.circleMarker([coords[1], coords[0]], {
					radius: 16,
					weight: 3,
					color,
					fillColor: color,
					fillOpacity: 0.2,
					opacity: 0.8,
					className: 'highlight-pulse-ring',
				}).addTo(map);
				highlightOverlays.push(ring);
			} else if (geom.type === 'LineString') {
				const coords = geom.coordinates as [number, number][];
				const latlngs = coords.map(c => [c[1], c[0]] as L.LatLngExpression);
				const highlight = L.polyline(latlngs, {
					color,
					weight: 6,
					opacity: 0.5,
					className: 'highlight-pulse-ring',
				}).addTo(map);
				highlightOverlays.push(highlight);
			} else if (geom.type === 'Polygon') {
				const rings = geom.coordinates as [number, number][][];
				const latlngs = rings[0].map(c => [c[1], c[0]] as L.LatLngExpression);
				const highlight = L.polygon(latlngs, {
					color,
					weight: 4,
					opacity: 0.5,
					fillColor: color,
					fillOpacity: 0.15,
					className: 'highlight-pulse-ring',
				}).addTo(map);
				highlightOverlays.push(highlight);
			}
		}
	});

	// Highlight single operator when hovering in assign picker
	$effect(() => {
		if (!map) return;
		const ciId = highlightedCheckInId;

		operatorHighlight?.remove();
		operatorHighlight = null;

		if (!ciId) return;

		const ci = netOperators.find(op => op.id === ciId);
		if (!ci || ci.lat == null || ci.lon == null) return;

		const color = netStatusColors[ci.status] || '#6b7280';
		operatorHighlight = L.circleMarker([ci.lat, ci.lon], {
			radius: 20,
			weight: 3,
			color: '#fff',
			fillColor: color,
			fillOpacity: 0.3,
			opacity: 0.9,
			className: 'highlight-pulse-ring',
		}).addTo(map);
	});

	// Weather overlay markers
	$effect(() => {
		if (!map) return;
		const show = showWeatherOverlay;

		// Remove all existing weather markers when hidden or data changes
		for (const [, m] of wxMarkers) m.remove();
		wxMarkers.clear();

		// Bail out BEFORE reading `weatherOverlay`: that array gets a new
		// identity on every inbound packet, and reading it up front kept this
		// effect re-running on every packet even with the overlay switched off.
		if (!show) return;

		const wxStations = weatherOverlay;
		const units = get(weatherUnits) as UnitSystem;
		if (!wxStations.length) return;
		const wxTokensCache = readWxTokens();

		for (const s of wxStations) {
			if (!s.position || !s.weather) continue;
			const key = s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
			const temp = s.weather.temperature;
			const windDir = s.weather.windDir;
			const windSpeed = s.weather.windSpeed;

			// Staleness color: green → yellow-green → amber → grey
			const ageMs = Date.now() - new Date(s.lastHeard).getTime();
			const ageMin = ageMs / 60000;
			let staleColor: string;
			let staleOpacity: number;
			if (ageMin < 10) {
				staleColor = '#4ade80'; staleOpacity = 1;
			} else if (ageMin < 30) {
				staleColor = '#a3e635'; staleOpacity = 0.9;
			} else if (ageMin < 60) {
				staleColor = '#fbbf24'; staleOpacity = 0.75;
			} else {
				staleColor = '#6b7280'; staleOpacity = 0.5;
			}

			const tempStr = temp != null ? `${Math.round(convertTemp(temp, units))}°` : '—';
			const windArrow = windDir != null
				? `<span style="display:inline-block;transform:rotate(${windDir}deg);font-size:10px;">↑</span>`
				: '';
			const windStr = windSpeed != null ? `${Math.round(convertWindSpeed(windSpeed, units))}` : '';

			// §7.5: a station inside an active (polygon-drawable) NWS alert gets a
			// tier-colored left edge on its weather pill — redundant with the row
			// chip in WeatherStationCard, cheap to compute per marker.
			const wxHit = wxAlerts.find((a) => a.state === 'active' && pointInAlert(a, s.position!.lat, s.position!.lon));
			const wxBorder = wxHit ? `border-left:3px solid ${wxTierColorFor(wxHit, wxTokensCache)};` : '';

			const html = `<div class="wx-marker-pill" style="border-color:${staleColor};opacity:${staleOpacity};${wxBorder}">
				<span class="wx-temp">${tempStr}</span>
				${windArrow || windStr ? `<span class="wx-wind">${windArrow}${windStr}</span>` : ''}
				<span class="wx-stale-dot" style="background:${staleColor}"></span>
			</div>`;

			const icon = L.divIcon({
				className: 'wx-marker',
				html,
				iconSize: [60, 24],
				iconAnchor: [30, 12],
			});

			const marker = L.marker([s.position.lat, s.position.lon], { icon, interactive: false }).addTo(map);
			wxMarkers.set(key, marker);
		}
	});

	// DF overlay: bearing lines, range circles, and intersection target
	$effect(() => {
		if (!map) return;
		const show = showDFOverlay;

		// Clear existing DF layers
		for (const [, line] of dfLines) line.remove();
		dfLines.clear();
		for (const [, circle] of dfRangeCircles) circle.remove();
		dfRangeCircles.clear();
		dfTargetMarker?.remove();
		dfTargetMarker = null;
		dfTargetCircle?.remove();
		dfTargetCircle = null;

		// Same reasoning as the weather effect above: don't subscribe to the
		// per-packet `dfOverlay` array while the overlay is off.
		if (!show) return;

		const dfStations = dfOverlay;
		if (!dfStations.length) return;

		const DEFAULT_RANGE_MI = 50;
		const MI_TO_M = 1609.344;

		for (const s of dfStations) {
			if (!s.position || !s.df) continue;
			const key = s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
			const q = s.df.quality;
			const rangeMi = s.df.range > 0 ? s.df.range : DEFAULT_RANGE_MI;

			// Line color based on quality
			let lineColor: string;
			if (q >= 7) lineColor = '#22c55e';
			else if (q >= 4) lineColor = '#f59e0b';
			else lineColor = '#ef4444';

			// Line style based on quality
			let dashArray: string | undefined;
			if (q < 4) dashArray = '4 6';
			else if (q < 7) dashArray = '8 4';

			// Calculate bearing endpoint
			const toRad = Math.PI / 180;
			const brg = s.df.bearing * toRad;
			const lat1 = s.position.lat;
			const lon1 = s.position.lon;
			const cosLat = Math.cos(lat1 * toRad);
			// Approximate degrees per mile
			const dLat = rangeMi * (1 / 69.0);
			const dLon = rangeMi * (1 / (69.0 * cosLat));
			const lat2 = lat1 + dLat * Math.cos(brg);
			const lon2 = lon1 + dLon * Math.sin(brg);

			const lineOpts: L.PolylineOptions = {
				color: lineColor,
				weight: 2.5,
				opacity: Math.max(0.4, q / 9),
			};
			if (dashArray) lineOpts.dashArray = dashArray;

			const line = L.polyline(
				[[lat1, lon1], [lat2, lon2]],
				lineOpts
			).addTo(map);

			line.bindTooltip(`${key}: ${s.df.bearing.toFixed(0)}° Q${q}`, {
				permanent: false,
				direction: 'center',
				className: 'df-tooltip',
			});

			dfLines.set(key, line);

			// Range circle (subtle)
			if (s.df.range > 0) {
				const circle = L.circle([lat1, lon1], {
					radius: s.df.range * MI_TO_M,
					color: lineColor,
					weight: 1,
					opacity: 0.25,
					fillColor: lineColor,
					fillOpacity: 0.04,
					interactive: false,
				}).addTo(map);
				dfRangeCircles.set(key, circle);
			}
		}

		// Compute intersection target when 2+ DF stations
		if (dfStations.length >= 2) {
			const intersections: Array<{ lat: number; lon: number }> = [];

			for (let i = 0; i < dfStations.length; i++) {
				for (let j = i + 1; j < dfStations.length; j++) {
					const a = dfStations[i];
					const b = dfStations[j];
					if (!a.position || !b.position || !a.df || !b.df) continue;

					const pt = dfBearingIntersection(
						a.position.lat, a.position.lon, a.df.bearing,
						b.position.lat, b.position.lon, b.df.bearing
					);
					if (pt) intersections.push(pt);
				}
			}

			if (intersections.length > 0) {
				let latSum = 0, lonSum = 0;
				for (const p of intersections) {
					latSum += p.lat;
					lonSum += p.lon;
				}
				const cLat = latSum / intersections.length;
				const cLon = lonSum / intersections.length;

				// Spread for uncertainty circle
				let maxDist = 0;
				for (const p of intersections) {
					const d = dfHaversineKm(cLat, cLon, p.lat, p.lon);
					if (d > maxDist) maxDist = d;
				}

				// Target crosshair marker
				const targetHtml = `<div class="df-target-icon">
					<svg width="20" height="20" viewBox="0 0 20 20">
						<circle cx="10" cy="10" r="7" fill="none" stroke="#ef4444" stroke-width="2"/>
						<circle cx="10" cy="10" r="2" fill="#ef4444"/>
						<path d="M10 1v5M10 14v5M1 10h5M14 10h5" stroke="#ef4444" stroke-width="1.5"/>
					</svg>
				</div>`;

				const targetIcon = L.divIcon({
					className: 'df-target-marker',
					html: targetHtml,
					iconSize: [20, 20],
					iconAnchor: [10, 10],
				});

				dfTargetMarker = L.marker([cLat, cLon], { icon: targetIcon, interactive: false }).addTo(map);
				dfTargetMarker.bindTooltip(
					`Est. target: ${cLat.toFixed(4)}, ${cLon.toFixed(4)}`,
					{ permanent: false, direction: 'top', className: 'df-tooltip' }
				);

				// Uncertainty circle
				if (maxDist > 0.01) {
					dfTargetCircle = L.circle([cLat, cLon], {
						radius: maxDist * 1000, // km to meters
						color: '#ef4444',
						weight: 1.5,
						opacity: 0.4,
						fillColor: '#ef4444',
						fillOpacity: 0.06,
						dashArray: '6 4',
						interactive: false,
					}).addTo(map);
				}
			}
		}
	});

	// Net location annotation markers + route line
	$effect(() => {
		if (!map) return;
		const show = showNetLocationAnnotations;
		const anns = netLocationAnnotations;

		// Clear existing
		for (const [, m] of netLocMarkers) m.remove();
		netLocMarkers.clear();
		netLocRouteLine?.remove();
		netLocRouteLine = null;

		if (!show || !anns.length) return;

		const sorted = [...anns].sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0));
		const routePoints: L.LatLngExpression[] = [];

		for (const a of sorted) {
			let lat: number, lon: number;
			try {
				const geo = typeof a.geometry === 'string' ? JSON.parse(a.geometry) : a.geometry;
				if (geo?.type !== 'Point' || !geo.coordinates) continue;
				lon = geo.coordinates[0];
				lat = geo.coordinates[1];
			} catch { continue; }
			if (!lat && !lon) continue;

			const color = categoryMeta[a.category]?.defaultColor || '#6b7280';
			const label = a.shortName || a.label.slice(0, 6);

			const html = `<div class="loc-preset-marker" style="--loc-color: ${color}">
				<span class="loc-preset-label">${label}</span>
				<div class="loc-preset-pin"></div>
			</div>`;

			const icon = L.divIcon({
				className: 'loc-preset-icon',
				html,
				iconSize: [60, 32],
				iconAnchor: [30, 32],
			});

			const marker = L.marker([lat, lon], { icon, interactive: true }).addTo(map);
			marker.bindTooltip(a.label + (a.description ? `\n${a.description}` : ''), {
				permanent: false,
				direction: 'top',
				className: 'annotation-tooltip',
			});
			marker.on('click', (e: L.LeafletMouseEvent) => {
				layerClick(
					e,
					{ kind: 'annotation', id: a.id },
					marker.getLatLng(),
					() => onAnnotationClick?.(a.id)
				);
			});
			netLocMarkers.set(a.id, marker);
			routePoints.push([lat, lon]);
		}

		// Draw route line connecting markers in sort order
		if (routePoints.length >= 2) {
			netLocRouteLine = L.polyline(routePoints, {
				color: '#94a3b8',
				weight: 2,
				opacity: 0.5,
				dashArray: '8 6',
			}).addTo(map);
		}
	});

	function dfBearingIntersection(
		lat1: number, lon1: number, brg1: number,
		lat2: number, lon2: number, brg2: number
	): { lat: number; lon: number } | null {
		const toRad = Math.PI / 180;
		const b1 = brg1 * toRad;
		const b2 = brg2 * toRad;
		const dx1 = Math.sin(b1);
		const dy1 = Math.cos(b1);
		const dx2 = Math.sin(b2);
		const dy2 = Math.cos(b2);
		const det = dx1 * dy2 - dx2 * dy1;
		if (Math.abs(det) < 1e-10) return null;
		const cosLat = Math.cos(((lat1 + lat2) / 2) * toRad);
		const dLon = (lon2 - lon1) * cosLat;
		const dLat = lat2 - lat1;
		const t = (dLon * dy2 - dLat * dx2) / det;
		if (t < 0) return null;
		const lat = lat1 + t * dy1;
		const lon = lon1 + t * dx1 / cosLat;
		if (dfHaversineKm(lat1, lon1, lat, lon) > 500) return null;
		return { lat, lon };
	}

	function dfHaversineKm(lat1: number, lon1: number, lat2: number, lon2: number): number {
		const R = 6371;
		const toRad = Math.PI / 180;
		const dLat = (lat2 - lat1) * toRad;
		const dLon = (lon2 - lon1) * toRad;
		const a = Math.sin(dLat / 2) ** 2 +
			Math.cos(lat1 * toRad) * Math.cos(lat2 * toRad) * Math.sin(dLon / 2) ** 2;
		return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
	}

	function setupVertexHandles(geojsonStr: string, color: string, onChange: (geom: string) => void) {
		let geom: { type: string; coordinates: unknown };
		try {
			geom = JSON.parse(geojsonStr);
		} catch {
			return;
		}

		const handleIcon = L.divIcon({
			className: 'vertex-handle',
			iconSize: [12, 12],
			iconAnchor: [6, 6],
			html: `<div style="width:12px;height:12px;background:${color};border:2px solid white;border-radius:2px;box-shadow:0 1px 3px rgba(0,0,0,0.4);cursor:grab;"></div>`,
		});

		const editStyle = {
			color,
			weight: 3,
			opacity: 0.7,
			dashArray: '8 6',
			fillColor: color,
			fillOpacity: 0.12,
		};

		if (geom.type === 'Point') {
			const coords = geom.coordinates as [number, number];
			const marker = L.marker([coords[1], coords[0]], {
				draggable: true,
				icon: handleIcon,
			}).addTo(map);
			// Show a small circle around the point for context
			editShape = L.circleMarker([coords[1], coords[0]], {
				radius: 10,
				...editStyle,
			}).addTo(map);
			marker.on('drag', () => {
				const pos = marker.getLatLng();
				(editShape as L.CircleMarker).setLatLng(pos);
			});
			marker.on('dragend', () => {
				const pos = marker.getLatLng();
				onChange(JSON.stringify({
					type: 'Point',
					coordinates: [pos.lng, pos.lat],
				}));
			});
			vertexHandles.push(marker);
		} else if (geom.type === 'LineString') {
			const coords = geom.coordinates as [number, number][];
			const latlngs = coords.map((c) => L.latLng(c[1], c[0]));
			editShape = L.polyline(latlngs, editStyle).addTo(map);
			for (let i = 0; i < latlngs.length; i++) {
				const marker = L.marker(latlngs[i], {
					draggable: true,
					icon: handleIcon,
				}).addTo(map);
				marker.on('drag', () => {
					const positions = vertexHandles.map((h) => h.getLatLng());
					(editShape as L.Polyline).setLatLngs(positions);
				});
				marker.on('dragend', () => {
					const positions = vertexHandles.map((h) => h.getLatLng());
					onChange(JSON.stringify({
						type: 'LineString',
						coordinates: positions.map((p) => [p.lng, p.lat]),
					}));
				});
				vertexHandles.push(marker);
			}
		} else if (geom.type === 'Polygon') {
			const rings = geom.coordinates as [number, number][][];
			// Exclude the closing point (last == first)
			const outerRing = rings[0];
			const verts = outerRing[outerRing.length - 1][0] === outerRing[0][0] &&
				outerRing[outerRing.length - 1][1] === outerRing[0][1]
				? outerRing.slice(0, -1)
				: outerRing;
			const latlngs = verts.map((c) => L.latLng(c[1], c[0]));
			editShape = L.polygon(latlngs, editStyle).addTo(map);
			for (let i = 0; i < latlngs.length; i++) {
				const marker = L.marker(latlngs[i], {
					draggable: true,
					icon: handleIcon,
				}).addTo(map);
				marker.on('drag', () => {
					const positions = vertexHandles.map((h) => h.getLatLng());
					(editShape as L.Polygon).setLatLngs(positions);
				});
				marker.on('dragend', () => {
					const positions = vertexHandles.map((h) => h.getLatLng());
					const coords = positions.map((p) => [p.lng, p.lat]);
					coords.push(coords[0]); // close the ring
					onChange(JSON.stringify({
						type: 'Polygon',
						coordinates: [coords],
					}));
				});
				vertexHandles.push(marker);
			}
		}
	}

	function cleanupVertexHandles() {
		for (const h of vertexHandles) h.remove();
		vertexHandles = [];
		if (editShape) {
			(editShape as L.Layer & { remove: Function }).remove();
			editShape = null;
		}
	}

	// Escape key cancels drawing or placing
	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (placingMissionLocation) {
				// Consumed here — stop it from also reaching SidePanel's own
				// window-level Escape handler, which would otherwise close
				// the whole Net Control panel out from under the map pick.
				e.stopImmediatePropagation();
				onMissionLocationPlaceCancelled?.();
				return;
			}
			if (placingAnnotation) {
				onAnnotationPlaceCancelled?.();
				return;
			}
			if (placingOperator) {
				onPlaceCancelled?.();
				return;
			}
			if (drawingMode) {
				clearDrawState();
				onDrawComplete?.('');
				return;
			}
			// Last: only when nothing else is armed does Escape drop a
			// long-pressed distance origin. An expanded Next Stop card takes
			// Escape ahead of this in the capture phase, so closing the card
			// never also throws away the pin it was measuring from.
			if (distanceDraftPoint) {
				onDistanceOriginCleared?.();
			}
		}
	}

	// The only place a pick/draw click is committed — background and layer
	// alike. Priority order (mission -> annotation -> operator -> draw) is
	// defensive only: the four modes are already mutually exclusive, since
	// every mode-starter in +page.svelte nulls the other three.
	function commitPick(latlng: L.LatLng, src: PickSource): void {
		const { lat, lng: lon } = latlng;
		if (placingMissionLocation) {
			if (src.kind === 'annotation') {
				onMissionLocationAnnotationPicked?.(src.id, lat, lon);
			} else {
				onMissionLocationPlaced?.(lat, lon, src.kind === 'map' ? undefined : src.label);
			}
			return;
		}
		if (placingAnnotation) {
			onAnnotationPlaced?.(lat, lon);
			return;
		}
		if (placingOperator) {
			onOperatorPlaced?.(placingOperator.id, lat, lon);
			return;
		}
		if (drawingMode) {
			addDrawVertex(latlng);
			return;
		}
	}

	// The only wrapper every interactive layer's click handler goes through
	// while a pick/draw mode is active. Stops the click from also reaching
	// the map's own background click handler (see the single-registration
	// effect above) so a layer click is never a double-fire, regardless of
	// whether the layer is a bubbling L.Path or a non-bubbling L.Marker —
	// see the design notes on Leaflet's bubblingMouseEvents split (#96).
	// Outside a pick/draw mode, layerClick is a pure passthrough to the
	// layer's normal navigation callback.
	function layerClick(
		e: L.LeafletMouseEvent,
		src: PickSource,
		fallback: L.LatLng | null,
		nav: () => void
	): void {
		if (anyPlaceMode) {
			L.DomEvent.stopPropagation(e);
			commitPick(e.latlng ?? fallback ?? map.getCenter(), src);
			return;
		}
		nav();
	}

	function handleBackgroundPickClick(e: L.LeafletMouseEvent) {
		commitPick(e.latlng, { kind: 'map' });
	}

	function addDrawVertex(latlng: L.LatLng) {
		if (!drawingMode) return;

		if (drawingMode === 'point') {
			const geojson = JSON.stringify({
				type: 'Point',
				coordinates: [latlng.lng, latlng.lat]
			});
			clearDrawState();
			onDrawComplete?.(geojson);
			return;
		}

		// Line or Area — accumulate vertices
		drawVertices.push(latlng);
		const m = L.circleMarker(latlng, {
			radius: 5,
			fillColor: DEFAULT_ANN_COLOR,
			color: '#fff',
			weight: 2,
			fillOpacity: 1,
		}).addTo(map);
		drawMarkers.push(m);

		// Update preview line
		if (drawVertices.length > 1) {
			const latlngs = drawVertices.map((v) => [v.lat, v.lng] as L.LatLngExpression);
			if (drawingMode === 'area') {
				latlngs.push(latlngs[0]);
			}
			if (drawLine) {
				drawLine.setLatLngs(latlngs);
			} else {
				drawLine = L.polyline(latlngs, {
					color: DEFAULT_ANN_COLOR,
					weight: 2,
					dashArray: '6 4',
					opacity: 0.8,
				}).addTo(map);
			}
		}
	}

	function handleDrawDblClick(e: L.LeafletMouseEvent) {
		if (!drawingMode || drawingMode === 'point') return;
		// Prevent the last dblclick from also triggering a single click vertex
		L.DomEvent.stopPropagation(e);

		if (drawVertices.length < 2) return;

		let geojson: string;
		if (drawingMode === 'line') {
			geojson = JSON.stringify({
				type: 'LineString',
				coordinates: drawVertices.map((v) => [v.lng, v.lat])
			});
		} else {
			// area (polygon) — close the ring
			const coords = drawVertices.map((v) => [v.lng, v.lat]);
			coords.push(coords[0]);
			geojson = JSON.stringify({
				type: 'Polygon',
				coordinates: [coords]
			});
		}

		clearDrawState();
		onDrawComplete?.(geojson);
	}

	function clearDrawState() {
		for (const m of drawMarkers) m.remove();
		drawMarkers = [];
		drawVertices = [];
		if (drawLine) {
			drawLine.remove();
			drawLine = null;
		}
	}

	function highlightTrack(key: string, color: string) {
		const baseLine = trackLines.get(key);
		if (!baseLine || trackHighlights.has(key)) return;
		const hl = L.polyline(baseLine.getLatLngs() as L.LatLng[], {
			color,
			weight: 5,
			opacity: 0.85,
			interactive: false,
		}).addTo(map);
		hl.bringToBack();
		trackHighlights.set(key, hl);
	}

	function unhighlightTrack(key: string) {
		// Don't remove highlight if station is currently selected
		if (key === selectedCallsign) return;
		const hl = trackHighlights.get(key);
		if (hl) {
			hl.remove();
			trackHighlights.delete(key);
		}
	}

	function stationKey(s: Station): string {
		return s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
	}

	function parseStyle(styleStr?: string): Record<string, unknown> {
		if (!styleStr) return {};
		try {
			return JSON.parse(styleStr);
		} catch {
			return {};
		}
	}

	function updateAnnotations() {
		if (!map) return;

		const currentIds = new Set<string>();

		for (const ann of annotations) {
			currentIds.add(ann.id);

			// Remove existing layer if annotation was updated (re-render)
			if (annotationLayers.has(ann.id)) {
				(annotationLayers.get(ann.id) as L.Layer & { remove: Function }).remove();
				annotationLayers.delete(ann.id);
			}

			// Skip rendering if this annotation is being vertex-edited
			if (ann.id === editingAnnotationId) continue;

			let geom: { type: string; coordinates: unknown };
			try {
				geom = JSON.parse(ann.geometry);
			} catch {
				continue;
			}

			const style = parseStyle(ann.style);
			const color = (style.color as string) || DEFAULT_ANN_COLOR;
			const opacity = (style.opacity as number) || 0.8;
			const weight = (style.weight as number) || 2;
			const fillColor = (style.fillColor as string) || color;
			const fillOpacity = (style.fillOpacity as number) || 0.25;

			let layer: L.Layer | null = null;

			if (geom.type === 'Point') {
				const coords = geom.coordinates as [number, number]; // [lon, lat]
				layer = L.circleMarker([coords[1], coords[0]], {
					radius: 8,
					fillColor,
					color,
					weight,
					fillOpacity,
					opacity,
				});
			} else if (geom.type === 'LineString') {
				const coords = geom.coordinates as [number, number][];
				const latlngs = coords.map((c) => [c[1], c[0]] as L.LatLngExpression);
				layer = L.polyline(latlngs, { color, weight, opacity });
			} else if (geom.type === 'Polygon') {
				const rings = geom.coordinates as [number, number][][];
				const latlngs = rings[0].map((c) => [c[1], c[0]] as L.LatLngExpression);
				layer = L.polygon(latlngs, { color, weight, opacity, fillColor, fillOpacity });
			}

			if (layer) {
				(layer as L.Layer & { bindTooltip: Function }).bindTooltip(ann.label, {
					permanent: false,
					direction: 'top',
					className: 'annotation-tooltip',
				});
				(layer as L.Layer & { on: Function }).on('click', (e: L.LeafletMouseEvent) => {
					layerClick(
						e,
						{ kind: 'annotation', id: ann.id },
						null,
						() => onAnnotationClick?.(ann.id)
					);
				});
				(layer as L.Layer & { addTo: Function }).addTo(map);
				annotationLayers.set(ann.id, layer);
			}
		}

		// Remove stale annotation layers
		for (const [id, layer] of annotationLayers) {
			if (!currentIds.has(id)) {
				(layer as L.Layer & { remove: Function }).remove();
				annotationLayers.delete(id);
			}
		}
	}

	// Station name tooltip options. When call-sign labels are on the tooltip is
	// permanent and sits below the symbol so it never covers the icon; otherwise
	// it stays a hover tooltip above the marker.
	function stationTooltipOpts(): L.TooltipOptions {
		return showCallsigns
			? { permanent: true, direction: 'bottom', className: 'station-tooltip station-label' }
			: { permanent: false, direction: 'top', className: 'station-tooltip' };
	}

	/**
	 * Off-screen stations that must be rendered anyway (see CULL_PAD above):
	 * the selection, whose marker and highlighted track are part of the UI state
	 * and which the operator can select from a list while it is off-screen; a
	 * station whose track the operator is hovering; a station with an open hover
	 * tooltip they are still reading; and any station whose track crosses the
	 * padded viewport — the polyline is visible even when the symbol is not, and
	 * dropping the key would take the track with it.
	 */
	function mustRenderOffscreen(key: string, st: Station, bounds: L.LatLngBounds, trackCutoff: number): boolean {
		if (key === selectedCallsign) return true;
		if (trackHighlights.has(key)) return true;
		const marker = markers.get(key);
		// Permanent call-sign labels report open for every marker, so they say
		// nothing about what is being read — only a hover tooltip does.
		if (marker && marker.isTooltipOpen() && !tooltipState.get(key)?.permanent) return true;

		if (showTracks && st.track && st.track.length > 1) {
			const bb = trackBBox(key, st, trackCutoff);
			if (bb) {
				const sw = bounds.getSouthWest();
				const ne = bounds.getNorthEast();
				if (bb.maxLat >= sw.lat && bb.minLat <= ne.lat && bb.maxLon >= sw.lng && bb.minLon <= ne.lng) return true;
			}
		}
		return false;
	}

	/**
	 * Bounding box of the track as it is actually DRAWN, cached per station.
	 *
	 * Two things this must not do. It must not use the unfiltered history: with
	 * the default trackDuration of 'all', a station's whole-day track spans the
	 * course, overlaps any zoomed-in viewport, and would protect every station
	 * from culling — the paint win would never materialise at exactly the event
	 * size that needs it. And it must not walk the points on every packet:
	 * updateMarkers() runs per packet, so an O(points) scan per off-screen
	 * station would hand back the cost culling is meant to save.
	 *
	 * Keyed on the same signature the polyline itself is cached against, so the
	 * box is recomputed only when the drawn line actually changes.
	 */
	function trackBBox(
		key: string,
		st: Station,
		cutoff: number
	): { minLat: number; maxLat: number; minLon: number; maxLon: number } | null {
		const pts = st.track ?? [];
		const last = pts[pts.length - 1];
		const sig = `${trackDurationMs}|${pts.length}|${last?.time ?? ''}|${last?.lat},${last?.lon}`;
		const hit = trackBBoxes.get(key);
		if (hit && hit.sig === sig) return hit.box;

		let minLat = Infinity, maxLat = -Infinity, minLon = Infinity, maxLon = -Infinity;
		let n = 0;
		for (const tp of pts) {
			if (cutoff !== -Infinity && new Date(tp.time).getTime() < cutoff) continue;
			n++;
			if (tp.lat < minLat) minLat = tp.lat;
			if (tp.lat > maxLat) maxLat = tp.lat;
			if (tp.lon < minLon) minLon = tp.lon;
			if (tp.lon > maxLon) maxLon = tp.lon;
		}
		const box = n > 1 ? { minLat, maxLat, minLon, maxLon } : null;
		trackBBoxes.set(key, { sig, box });
		return box;
	}

	function updateMarkers() {
		if (!map) return;

		// Clear highlights for previously selected stations
		for (const [key, hl] of trackHighlights) {
			if (key !== selectedCallsign) {
				hl.remove();
				trackHighlights.delete(key);
			}
		}

		const currentKeys = new Set<string>();
		// Resolve the tactical-alias lookup once: `getTacticalAlias` is a cold
		// derived store, so get() would subscribe/recompute/unsubscribe once per
		// station per packet. It cannot change while this loop runs.
		const aliasFor = get(getTacticalAlias);
		// Hoisted out of the per-station track filter below — one clock read for
		// the whole pass instead of one per station.
		const trackCutoff = trackDurationMs === Infinity ? -Infinity : Date.now() - trackDurationMs;
		// Below the threshold every station is rendered exactly as before, so
		// small nets and normal use are untouched by culling.
		const cullBounds = stations.length > CULL_MIN_STATIONS ? map.getBounds().pad(CULL_PAD) : null;

		for (const st of stations) {
			if (!st.position) continue;
			const key = stationKey(st);
			// A culled station never joins currentKeys, so the removal loops at
			// the end of this function prune its marker, tooltip, track and
			// highlight through the same path a disappeared station takes.
			if (
				cullBounds &&
				!cullBounds.contains([st.position.lat, st.position.lon]) &&
				!mustRenderOffscreen(key, st, cullBounds, trackCutoff)
			) continue;
			currentKeys.add(key);

			const info = symbolInfo(st.symbol);
			const baseName = stationDisplayName(st.callsign, st.ssid);
			const tacAlias = aliasFor(key);
			const name = tacAlias ? `${tacAlias} (${baseName})` : baseName;
			const isSelected = key === selectedCallsign;

			// Enumerates every input createStationIcon() has plus everything
			// createMarkerHtml() branches on (symbol table+code, colour,
			// selected, and the arrow's isMoving()/course). Equal signature ⇒
			// identical HTML ⇒ identical DOM, so setIcon() can be skipped.
			// `course` is deliberately unrounded so the arrow angle is exact.
			const iconSig = `${st.symbol?.table ?? ''}|${st.symbol?.code ?? ''}|${info.color}|${isSelected ? 1 : 0}|${isMoving(st.position.speed, st.position.course) ? st.position.course : ''}`;

			// Update or create marker
			let marker = markers.get(key);
			if (marker) {
				marker.setLatLng([st.position.lat, st.position.lon]);
				if (iconSigs.get(key) !== iconSig) {
					marker.setIcon(L.divIcon(createStationIcon(st.symbol, info.color, isSelected, st.position?.speed, st.position?.course)));
					iconSigs.set(key, iconSig);
				}
				// The tooltip options object only varies with `showCallsigns`
				// (see stationTooltipOpts), so a full unbind/rebind — the
				// expensive half, it tears down and re-adds every focus
				// listener — is only needed when the label toggle flips. A
				// changed tactical alias is just new content.
				const prevTip = tooltipState.get(key);
				if (!prevTip || prevTip.permanent !== showCallsigns) {
					marker.unbindTooltip();
					marker.bindTooltip(name, stationTooltipOpts());
					tooltipState.set(key, { name, permanent: showCallsigns });
				} else if (prevTip.name !== name) {
					marker.setTooltipContent(name);
					prevTip.name = name;
				}
			} else {
				marker = L.marker([st.position.lat, st.position.lon], {
					icon: L.divIcon(createStationIcon(st.symbol, info.color, isSelected, st.position?.speed, st.position?.course)),
				}).addTo(map);
				iconSigs.set(key, iconSig);

				marker.bindTooltip(name, stationTooltipOpts());
				tooltipState.set(key, { name, permanent: showCallsigns });

				// Captured as a definite (non-undefined) const — `marker` itself
				// is a `let` (Map.get()'s return type includes undefined), and
				// TS can't narrow a `let` across the closure boundary below.
				const stationMarker = marker;
				stationMarker.on('click', (e: L.LeafletMouseEvent) => {
					layerClick(
						e,
						{ kind: 'station', callsign: key, label: name },
						stationMarker.getLatLng(),
						() => onStationClick?.(key)
					);
				});
				marker.on('mouseover', () => {
					highlightTrack(key, info.color);
				});
				marker.on('mouseout', () => {
					unhighlightTrack(key);
				});

				markers.set(key, marker);
			}

			// Update track line
			if (showTracks && st.track && st.track.length > 1) {
				let trackPoints = st.track;
				if (trackDurationMs !== Infinity) {
					// The filter itself always runs, so points ageing out of a
					// finite window still change trackPoints.length and so the
					// signature below — only the projection work is cached.
					trackPoints = trackPoints.filter((tp) => new Date(tp.time).getTime() >= trackCutoff);
				}

				if (trackPoints.length > 1) {
					const lastTp = trackPoints[trackPoints.length - 1];
					// Track points are only ever appended (or dropped off the
					// front by the window filter), so length + first/last
					// timestamp + last coordinate identifies the point set.
					// trackDurationMs is included so changing the window always
					// re-projects.
					const trackSig = `${trackDurationMs}|${trackPoints.length}|${trackPoints[0].time}|${lastTp.time}|${lastTp.lat},${lastTp.lon}`;
					let line = trackLines.get(key);
					if (!line) {
						line = L.polyline(trackPoints.map((tp) => [tp.lat, tp.lon] as L.LatLngExpression), {
							color: info.color,
							weight: 2,
							opacity: 0.6,
							dashArray: '4 4',
						}).addTo(map);
						trackLines.set(key, line);
						trackSigs.set(key, trackSig);
					} else if (trackSigs.get(key) !== trackSig) {
						const latlngs: L.LatLngExpression[] = trackPoints.map((tp) => [tp.lat, tp.lon]);
						line.setLatLngs(latlngs);
						// Keep highlight in sync with track data. When the
						// signature is unchanged the highlight already holds
						// these exact points (highlightTrack copies them from
						// the base line), so there is nothing to sync.
						const hl = trackHighlights.get(key);
						if (hl) hl.setLatLngs(latlngs);
						trackSigs.set(key, trackSig);
					}

					// Persistent highlight for selected station
					if (key === selectedCallsign && !trackHighlights.has(key)) {
						highlightTrack(key, info.color);
					}
				} else {
					// Too few points after filter — remove track
					const line = trackLines.get(key);
					if (line) { line.remove(); trackLines.delete(key); }
					trackSigs.delete(key);
					const hl = trackHighlights.get(key);
					if (hl) { hl.remove(); trackHighlights.delete(key); }
				}
			} else if (!showTracks) {
				// Tracks disabled — remove any existing track for this station
				const line = trackLines.get(key);
				if (line) { line.remove(); trackLines.delete(key); }
				trackSigs.delete(key);
				const hl = trackHighlights.get(key);
				if (hl) { hl.remove(); trackHighlights.delete(key); }
			}
		}

		// Remove stale markers and tracks
		for (const [key, marker] of markers) {
			if (!currentKeys.has(key)) {
				marker.remove();
				markers.delete(key);
				iconSigs.delete(key);
				tooltipState.delete(key);
				trackBBoxes.delete(key);
			}
		}
		for (const [key, line] of trackLines) {
			if (!currentKeys.has(key)) {
				line.remove();
				trackLines.delete(key);
				trackSigs.delete(key);
			}
		}
		for (const [key, hl] of trackHighlights) {
			if (!currentKeys.has(key)) {
				hl.remove();
				trackHighlights.delete(key);
			}
		}
	}

	function updateDRCones() {
		if (!map) return;
		// Also called from a 30s setInterval; skip layer work for a tab nobody
		// is looking at. The marker effect re-runs on unhide and restores them.
		if (typeof document !== 'undefined' && document.hidden) return;

		if (!showDRCones) {
			for (const [, layer] of drCones) layer.remove();
			drCones.clear();
			for (const [, layer] of drCenterLines) layer.remove();
			drCenterLines.clear();
			return;
		}

		const now = Date.now();
		const activeKeys = new Set<string>();

		for (const st of stations) {
			if (!st.position) continue;
			const key = stationKey(st);
			const speed = st.position.speed;
			const course = st.position.course;

			if (!isMoving(speed, course)) continue;

			const lastHeardMs = new Date(st.lastHeard).getTime();
			const cone = computeDRCone(st.position.lat, st.position.lon, speed!, course!, lastHeardMs, now);
			if (!cone) continue;

			activeKeys.add(key);
			const info = symbolInfo(st.symbol);
			const opacity = cone.confidence * 0.3;
			const lineOpacity = cone.confidence * 0.5;

			// Build cone polygon: left edge + reverse right edge to close
			const polyCoords = [...cone.left, ...cone.right.slice().reverse()] as [number, number][];

			let coneLayer = drCones.get(key);
			if (coneLayer) {
				coneLayer.setLatLngs(polyCoords);
				coneLayer.setStyle({ fillOpacity: opacity, opacity: lineOpacity });
			} else {
				coneLayer = L.polygon(polyCoords, {
					color: info.color,
					fillColor: info.color,
					fillOpacity: opacity,
					weight: 1,
					opacity: lineOpacity,
					dashArray: '4 4',
					interactive: false,
				}).addTo(map);
				drCones.set(key, coneLayer);
			}

			// Center projection line
			const centerCoords: [number, number][] = [
				[st.position.lat, st.position.lon],
				cone.center,
			];
			let centerLine = drCenterLines.get(key);
			if (centerLine) {
				centerLine.setLatLngs(centerCoords);
				centerLine.setStyle({ opacity: lineOpacity });
			} else {
				centerLine = L.polyline(centerCoords, {
					color: info.color,
					weight: 2,
					opacity: lineOpacity,
					dashArray: '6 4',
					interactive: false,
				}).addTo(map);
				drCenterLines.set(key, centerLine);
			}
		}

		// Remove stale DR layers
		for (const [key, layer] of drCones) {
			if (!activeKeys.has(key)) {
				layer.remove();
				drCones.delete(key);
			}
		}
		for (const [key, layer] of drCenterLines) {
			if (!activeKeys.has(key)) {
				layer.remove();
				drCenterLines.delete(key);
			}
		}
	}

	// --- NWS Alerts (internal/wxalert) rendering ----------------------------
	// See lib/wxAlertMap.ts for the pure geometry/style helpers this wires
	// into real Leaflet layers. BUILD-PLAN.md §5 (Work packages, WP4).

	function wxTierColorFor(alert: WxAlert, tokens: WxTokens): string {
		switch (alert.tier) {
			case 'warning': return tokens.warning;
			case 'watch': return tokens.watch;
			case 'advisory': return tokens.advisory;
			default: return tokens.statement;
		}
	}

	function wxMinTierRank(mode: WxMapMode): number {
		switch (mode) {
			case 'warnings': return tierRank('warning');
			case 'watches': return tierRank('watch');
			case 'all': return 0;
			default: return Infinity; // 'off'
		}
	}

	/** Mean position of an alert's affected items — the chip/fallback anchor when nothing else places it. */
	function wxAffectsCentroid(alert: WxAlert): L.LatLngTuple | null {
		const items = [...alert.affects.checkpoints, ...alert.affects.locations, ...alert.affects.stations];
		if (!items.length) return null;
		const lat = items.reduce((s, i) => s + i.lat, 0) / items.length;
		const lon = items.reduce((s, i) => s + i.lon, 0) / items.length;
		return [lat, lon];
	}

	/** Invisible hover/tap target width, in px, around an alert's outline. */
	const WX_HIT_WEIGHT = 18;

	/**
	 * Hover tooltip for an alert's geometry. The event name leads, timing and
	 * office sit under it, and the "no published polygon" honesty line is kept
	 * visually distinct. Escaped: areaDesc/senderName are upstream NWS text.
	 */
	function wxTooltipHtml(alert: WxAlert, honesty: string): string {
		const esc = (v: string) =>
			v.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		const office = alert.senderId || alert.senderName || '';
		const meta: string[] = [`until ${esc(clock(alert.endsAt))}`];
		if (office) meta.push(`NWS ${esc(office)}`);
		return (
			`<span class="wx-tt-title">${esc(alert.event)}</span>` +
			`<span class="wx-tt-meta">${meta.join(' · ')}</span>` +
			(alert.areaDesc ? `<span class="wx-tt-area">${esc(alert.areaDesc)}</span>` : '') +
			(honesty ? `<span class="wx-tt-note">${esc(honesty)}</span>` : '')
		);
	}

	/**
	 * Builds one alert's map presence: a LayerGroup (fill/hatch/casing/stroke/
	 * inner-line, plus the own-geometry fallback for a zone-only alert whose
	 * zone is not cached) and the point a chip for this alert should sit at.
	 * Geometry resolution order is BUILD-PLAN.md §5.4 / spec-frontend.md §5.4:
	 * published polygon -> cached zone polygon(s) -> own-geometry fallback ->
	 * chip only at the footprint centroid.
	 */
	function buildWxAlert(
		alert: WxAlert,
		zoneGeo: Map<string, WxPolygonGeometry>,
		linkState: WxLinkState,
		anns: Annotation[],
		centroid: { lat: number; lon: number } | null,
		tokens: WxTokens
	): { group: L.FeatureGroup | null; chipPoint: L.LatLngTuple | null } {
		if (!map || !wxAlertRenderer) return { group: null, chipPoint: null };
		const renderer = wxAlertRenderer;
		const opts = pathOptions(alert.tier, alert.severity, linkState, tokens);
		const layers: L.Layer[] = [];
		let rings: L.LatLngTuple[][] = [];
		let honesty = '';
		const filled = alert.tier === 'warning' || alert.tier === 'watch';

		if (alert.geometry) {
			rings = ringsOf(alert.geometry);
		} else if (alert.geometrySource === 'zone') {
			// Ask for every affected zone, not just the ones the server has
			// already cached: GET /wx/zones/{ugc} resolves on demand
			// (allowFetch=true). Gating on `cached` was a deadlock — the
			// server's cache is warmed from the footprint, so a zone outside
			// it stayed uncached forever and its polygon was never requested,
			// leaving every zone-only alert (~85% of the feed) undrawable.
			const cachedUgcs = alert.zones.map((z) => z.ugc);
			const notYetFetched = cachedUgcs.filter((u) => !zoneGeo.has(u) && !wxRequestedZones.has(u));
			if (notYetFetched.length) {
				for (const u of notYetFetched) wxRequestedZones.add(u);
				void ensureZoneGeometry(notYetFetched);
			}
			for (const ugc of cachedUgcs) {
				const geo = zoneGeo.get(ugc);
				if (geo) rings.push(...ringsOf(geo));
			}
			if (rings.length) {
				honesty = `Zone-based alert (${zoneLabel(alert)}). NWS did not publish a polygon; this is the NWS zone outline.`;
			}
		}

		if (rings.length) {
			if (filled) {
				layers.push(
					L.polygon(rings, { pane: 'wxAlertPane', renderer, stroke: false, fillColor: opts.color, fillOpacity: opts.fillOpacity, interactive: false })
				);
				if (opts.hatch) {
					const hatchId = linkState === 'stale' || linkState === 'down' ? 'wx-hatch-warning-stale' : 'wx-hatch-warning';
					layers.push(
						L.polygon(rings, { pane: 'wxAlertPane', renderer, stroke: false, fillColor: `url(#${hatchId})`, fillOpacity: 0.55, interactive: false })
					);
				}
			}
			// Casing under the tier stroke (a wider line in a near-white color) so the
			// stroke reads against any basemap color.
			layers.push(L.polyline(rings, { pane: 'wxAlertPane', renderer, color: opts.casing, weight: opts.weight + 2, opacity: 1, lineCap: 'round', interactive: false }));
			const strokeLine = L.polyline(rings, {
				pane: 'wxAlertPane', renderer, color: opts.color, weight: opts.weight, dashArray: opts.dashArray,
				lineCap: alert.tier === 'advisory' ? 'round' : 'butt', opacity: opts.strokeOpacity, interactive: true
			});
			layers.push(strokeLine);
			// A tier stroke is 1.5-3px wide — a hard target to hover, especially
			// on a touch screen. An invisible fat line along the same rings
			// carries the tooltip and the click instead. `pointer-events: stroke`
			// (set in CSS) makes it hit-test regardless of paint, so opacity 0
			// stays genuinely invisible. Added last so it sits on top.
			const hitLine = L.polyline(rings, {
				pane: 'wxAlertPane',
				renderer,
				color: opts.color,
				weight: WX_HIT_WEIGHT,
				opacity: 0,
				fill: false,
				lineCap: 'round',
				lineJoin: 'round',
				interactive: true,
				className: 'wx-alert-hit'
			});
			hitLine.bindTooltip(wxTooltipHtml(alert, honesty), {
				direction: 'top',
				className: `wx-alert-tooltip wx-alert-tooltip-${alert.tier}`,
				sticky: true,
				opacity: 1
			});
			hitLine.on('click', () => onWxAlertClick?.(alert.id));
			layers.push(hitLine);
			if (alert.tier === 'warning') {
				layers.push(L.polyline(rings, { pane: 'wxAlertPane', renderer, color: opts.color, weight: 1, opacity: 0.9, interactive: false }));
			}
		}

		// Own-geometry fallback — a zone-only alert with at least one uncached
		// zone gets the ribbon/halo treatment for the items in that zone,
		// regardless of whether other zones on the same alert were cached above.
		if (alert.geometrySource === 'zone' && alert.zones.some((z) => !z.cached)) {
			const fb = fallbackLayers(
				alert,
				anns,
				{
					polyline: (latlngs, options) => L.polyline(latlngs, { ...options, pane: 'wxAlertPane', renderer }),
					circleMarker: (latlng, options) => L.circleMarker(latlng, { ...options, pane: 'wxAlertPane', renderer })
				},
				tokens
			);
			for (const layer of fb) {
				if ('on' in layer) (layer as L.Path).on('click', () => onWxAlertClick?.(alert.id));
			}
			layers.push(...fb);
			if (!honesty) {
				honesty = `Zone-based alert (${zoneLabel(alert)}). NWS did not publish a polygon and the zone outline is not cached; highlighted where your course passes through the zone.`;
			}
			if (!layers.some((l) => (l as L.Path).bindTooltip)) {
				// No cached-zone stroke exists to carry the tooltip — give the
				// fallback layers one each so the honesty line is still reachable.
				for (const layer of fb) {
					(layer as L.Path).bindTooltip(wxTooltipHtml(alert, honesty), {
						direction: 'top',
						className: `wx-alert-tooltip wx-alert-tooltip-${alert.tier}`,
						sticky: true,
						opacity: 1
					});
				}
			}
		}

		// Chip anchor: nearest polygon/zone vertex to the footprint centroid,
		// else the mean of the affected items (fallback case), else the raw
		// centroid, else nothing (an IN alert with no drawable geometry and no
		// footprint centroid is a server bug — never silently invisible).
		let chipPoint: L.LatLngTuple | null = null;
		if (rings.length) {
			chipPoint = centroid ? nearestVertex(rings, centroid) : null;
			if (!chipPoint) {
				const c = L.polygon(rings).getBounds().getCenter();
				chipPoint = [c.lat, c.lng];
			}
		} else {
			chipPoint = wxAffectsCentroid(alert) ?? (centroid ? [centroid.lat, centroid.lon] : null);
		}
		if (!chipPoint && filled) {
			console.warn(`wx alert ${alert.id} (${alert.event}) has no drawable geometry and no footprint centroid`);
		}

		if (!layers.length) return { group: null, chipPoint };
		return { group: L.featureGroup(layers), chipPoint };
	}

	function wxChipMarkerFor(alert: WxAlert, point: L.LatLngTuple, linkState: WxLinkState): L.Marker {
		const icon = L.divIcon({ className: 'wx-alert-chip-wrap', html: chipHtml(alert, linkState, Date.now()), iconSize: undefined, iconAnchor: [0, 12] });
		const marker = L.marker(point, { icon, interactive: true, keyboard: false, zIndexOffset: -1000 });
		marker.on('click', () => onWxAlertClick?.(alert.id));
		return marker;
	}

	/** 60 s tick: refreshes each chip's countdown text without rebuilding the marker. */
	function refreshWxChipLabels(): void {
		const linkState = wxLinkState;
		for (const alert of wxAlerts) {
			const marker = wxAlertChips.get(alert.id);
			if (!marker) continue;
			marker.setIcon(L.divIcon({ className: 'wx-alert-chip-wrap', html: chipHtml(alert, linkState, Date.now()), iconSize: undefined, iconAnchor: [0, 12] }));
		}
	}

	/** Chips within this many CSS pixels of an already-placed chip are skipped — only the higher-sorted (drawOrder-last) alert keeps its chip. */
	const WX_CHIP_DEDUPE_PX = 24;

	function applyWxChipVisibility(): void {
		if (!map) return;
		const hidden = wxMapMode === 'off' || map.getZoom() < 8;
		for (const [, marker] of wxAlertChips) {
			const el = marker.getElement();
			if (el) el.style.display = hidden ? 'none' : '';
		}
	}

	$effect(() => {
		if (!map || !wxAlertRenderer) return;
		const mode = wxMapMode;
		const alerts = wxAlerts;
		const zoneGeo = wxZoneGeometry;
		const linkState = wxLinkState;
		const centroid = wxFootprintCentroid;
		const anns = annotations;
		const tokens = readWxTokens();

		for (const [, g] of wxAlertLayerGroups) g.remove();
		wxAlertLayerGroups.clear();
		for (const [, m] of wxAlertChips) m.remove();
		wxAlertChips.clear();

		if (mode === 'off') return;
		const minRank = wxMinTierRank(mode);
		const visible = alerts.filter((a) => tierRank(a.tier) >= minRank);

		const placedChips: L.LatLngTuple[] = [];
		for (const alert of drawOrder(visible)) {
			const { group, chipPoint } = buildWxAlert(alert, zoneGeo, linkState, anns, centroid, tokens);
			if (group) {
				group.addTo(map);
				wxAlertLayerGroups.set(alert.id, group);
			}
			if ((alert.tier === 'warning' || alert.tier === 'watch') && chipPoint) {
				const tooClose = placedChips.some((p) => {
					const a = map!.latLngToLayerPoint(p);
					const b = map!.latLngToLayerPoint(chipPoint);
					return a.distanceTo(b) < WX_CHIP_DEDUPE_PX;
				});
				if (!tooClose) {
					placedChips.push(chipPoint);
					const marker = wxChipMarkerFor(alert, chipPoint, linkState);
					marker.addTo(map);
					wxAlertChips.set(alert.id, marker);
				}
			}
		}
		applyWxChipVisibility();
	});

	// Footprint preview — the watch-area sheet's "Show on map" (~10 s outline).
	$effect(() => {
		if (!map || !wxAlertRenderer) return;
		wxFootprintPreviewLayer?.remove();
		wxFootprintPreviewLayer = null;
		const geo = wxFootprintPreview;
		if (!geo) return;
		const rings = ringsOf(geo);
		if (!rings.length) return;
		const mutedColor =
			(typeof getComputedStyle === 'function' && document.documentElement
				? getComputedStyle(document.documentElement).getPropertyValue('--color-text-muted').trim()
				: '') || '#aaaaaa';
		wxFootprintPreviewLayer = L.polyline(rings, {
			pane: 'wxAlertPane', renderer: wxAlertRenderer,
			color: mutedColor,
			weight: 1.5, dashArray: '2 6', interactive: false
		}).addTo(map);
	});

	// "Show on map" focus — fits the alert's own layer bounds (or a small box
	// around its chip) then hands control back via onWxFocusConsumed().
	$effect(() => {
		if (!map) return;
		const id = wxFocusAlertId;
		if (!id) return;
		const group = wxAlertLayerGroups.get(id);
		const chip = wxAlertChips.get(id);
		let bounds: L.LatLngBounds | null = null;
		if (group && group.getLayers().length) {
			bounds = group.getBounds();
		} else if (chip) {
			const c = chip.getLatLng();
			bounds = L.latLngBounds([c.lat - 0.02, c.lng - 0.02], [c.lat + 0.02, c.lng + 0.02]);
		}
		if (bounds && bounds.isValid()) {
			programmaticMove = true;
			map.fitBounds(bounds, { padding: [40, 40], maxZoom: 13 });
			map.once('moveend', () => { programmaticMove = false; });
		}
		onWxFocusConsumed?.();
	});
</script>

<svelte:window onkeydown={handleKeyDown} />

<div class="map-container" class:drawing={drawingMode !== null} class:placing={placingOperator !== null || placingAnnotation !== null} bind:this={mapEl}></div>

<!-- Hatch patterns for warning-tier Severe/Extreme NWS alert polygons. A
     sibling of the map container by design: fragment-id references
     (fill="url(#wx-hatch-warning)") resolve document-wide, so Leaflet's own
     <svg> inside .wxAlertPane can reference a pattern defined out here. -->
<svg class="wx-defs" width="0" height="0" aria-hidden="true" focusable="false">
	<defs>
		<pattern id="wx-hatch-warning" patternUnits="userSpaceOnUse" width="6" height="6" patternTransform="rotate(45)">
			<line x1="0" y1="0" x2="0" y2="6" stroke-width="1.5" style="stroke: var(--color-wx-warning)" />
		</pattern>
		<pattern id="wx-hatch-warning-stale" patternUnits="userSpaceOnUse" width="6" height="6" patternTransform="rotate(45)">
			<line x1="0" y1="0" x2="0" y2="6" stroke-width="1.5" style="stroke: var(--color-wx-expired)" />
		</pattern>
	</defs>
</svg>

{#if drawingMode}
	<div class="draw-hint">
		{#if drawingMode === 'point'}
			Click to place point
		{:else if drawingMode === 'line'}
			Click to add points, double-click to finish
		{:else}
			Click to add points, double-click to close polygon
		{/if}
		<span class="draw-hint-cancel">Press Esc to cancel</span>
	</div>
{/if}

{#if placingOperator}
	<div class="place-hint">
		Click to set position for <strong>{placingOperator.callsign}</strong>
		<kbd>Esc</kbd> cancel
	</div>
{/if}

{#if placingAnnotation}
	<div class="place-hint" style="border-color: #3b82f6;">
		Click to set position for <strong>{placingAnnotation.name || 'location'}</strong>
		<kbd>Esc</kbd> cancel
	</div>
{/if}

{#if placingMissionLocation}
	<div class="place-hint" style="border-color: var(--color-accent);">
		Click to set the mission location
		<kbd>Esc</kbd> cancel
	</div>
{/if}

<style>
	.map-container {
		width: 100%;
		height: 100%;
	}

	.map-container.drawing,
	.map-container.placing {
		cursor: crosshair;
	}

	.place-hint {
		position: absolute;
		top: 60px;
		left: 50%;
		transform: translateX(-50%);
		z-index: var(--z-toolbar, 1000);
		background: var(--color-surface, #1a1a2e);
		border: 1px solid #22c55e;
		border-radius: var(--radius-md, 8px);
		padding: 0.5rem 1rem;
		font-size: 0.85rem;
		color: var(--color-text, #eee);
		pointer-events: none;
		display: flex;
		gap: 0.75rem;
		align-items: center;
		box-shadow: 0 2px 8px rgba(0,0,0,0.3);
	}

	.place-hint kbd {
		background: rgba(255,255,255,0.1);
		border: 1px solid rgba(255,255,255,0.2);
		border-radius: 3px;
		padding: 1px 6px;
		font-size: 0.75rem;
		color: var(--color-text-muted, #888);
	}

	.draw-hint {
		position: absolute;
		top: 60px;
		left: 50%;
		transform: translateX(-50%);
		z-index: var(--z-toolbar, 1000);
		background: var(--color-surface, #1a1a2e);
		border: 1px solid var(--color-accent, #e63946);
		border-radius: var(--radius-md, 8px);
		padding: 0.5rem 1rem;
		font-size: 0.85rem;
		color: var(--color-text, #eee);
		pointer-events: none;
		display: flex;
		gap: 0.75rem;
		align-items: center;
		box-shadow: 0 2px 8px rgba(0,0,0,0.3);
	}

	.draw-hint-cancel {
		font-size: 0.75rem;
		color: var(--color-text-muted, #888);
	}

	:global(.vertex-handle) {
		background: transparent !important;
		border: none !important;
	}

	:global(.station-tooltip) {
		font-family: monospace;
		font-weight: 600;
		font-size: 12px;
		box-shadow: none;
	}

	/* Permanent call-sign label: quieter than a hover tooltip so a dense map
	   stays readable — no arrow, tight padding, translucent plate. */
	:global(.station-label) {
		background: rgba(10, 12, 24, 0.72);
		border: none;
		border-radius: 3px;
		padding: 0 4px;
		font-size: 11px;
		line-height: 15px;
		white-space: nowrap;
		color: var(--color-text, #eee);
	}

	:global(.station-label::before) {
		display: none;
	}

	:global(.annotation-tooltip) {
		font-family: inherit;
		font-weight: 500;
		font-size: 12px;
	}

	:global(.preview-tooltip) {
		font-family: inherit;
		font-weight: 600;
		font-size: 11px;
		font-style: italic;
		opacity: 0.7;
		border-style: dashed;
	}

	:global(.aprs-station-icon) {
		background: transparent !important;
		border: none !important;
	}

	:global(.net-voice-marker) {
		background: transparent !important;
		border: none !important;
	}

	:global(.net-mission-flag) {
		background: transparent !important;
		border: none !important;
	}

	:global(.leaflet-popup-content-wrapper) {
		background: #1a1a2e;
		color: #eee;
		border-radius: 8px;
	}

	:global(.leaflet-popup-tip) {
		background: #1a1a2e;
	}

	:global(.highlight-pulse-ring) {
		animation: highlight-pulse 1.5s ease-in-out infinite;
		pointer-events: none;
	}

	@keyframes highlight-pulse {
		0%, 100% { opacity: 0.6; }
		50% { opacity: 1; }
	}

	/* Weather overlay markers — must be :global since injected via L.divIcon */
	:global(.wx-marker) {
		background: none !important;
		border: none !important;
	}

	:global(.wx-marker-pill) {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		background: rgba(10, 15, 30, 0.82);
		border: 1px solid rgba(255, 255, 255, 0.15);
		border-radius: 10px;
		padding: 2px 7px;
		font-size: 11px;
		font-weight: 600;
		color: #e0e0e0;
		white-space: nowrap;
		pointer-events: none;
	}

	:global(.wx-marker-pill .wx-temp) {
		color: #fbbf24;
	}

	:global(.wx-marker-pill .wx-wind) {
		color: #94a3b8;
		font-size: 10px;
	}

	:global(.wx-marker-pill .wx-stale-dot) {
		width: 5px;
		height: 5px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	/* DF overlay */
	:global(.df-target-marker) {
		background: none !important;
		border: none !important;
	}

	:global(.df-target-icon) {
		filter: drop-shadow(0 1px 3px rgba(0, 0, 0, 0.5));
	}

	:global(.df-tooltip) {
		font-family: monospace;
		font-weight: 600;
		font-size: 11px;
	}

	/* Location preset markers */
	:global(.loc-preset-icon) {
		background: none !important;
		border: none !important;
	}

	:global(.loc-preset-marker) {
		display: flex;
		flex-direction: column;
		align-items: center;
		pointer-events: auto;
	}

	:global(.loc-preset-label) {
		display: inline-block;
		padding: 1px 6px;
		font-size: 10px;
		font-weight: 700;
		font-family: 'SF Mono', 'Fira Code', monospace;
		letter-spacing: 0.02em;
		background: var(--loc-color);
		color: #fff;
		border-radius: 3px;
		white-space: nowrap;
		text-shadow: 0 1px 2px rgba(0, 0, 0, 0.4);
		box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
	}

	:global(.loc-preset-pin) {
		width: 2px;
		height: 8px;
		background: var(--loc-color);
		opacity: 0.7;
	}

	/* Mission location draft pin — must be :global since injected via L.divIcon */
	:global(.mission-draft-pin) {
		width: 16px; height: 16px; margin: 0 auto;
		background: var(--color-accent); border: 2px solid #fff;
		border-radius: var(--radius-full) var(--radius-full) 2px var(--radius-full);
		transform: rotate(45deg); box-shadow: var(--shadow-md);
	}
	:global(.mission-draft-pulse) {
		position: absolute; left: 50%; top: 50%; width: 34px; height: 34px;
		margin: -17px 0 0 -17px; border-radius: var(--radius-full);
		background: var(--color-accent); opacity: 0.25;
		animation: missionDraftPulse 1.8s ease-out infinite;
	}
	:global(.mission-draft-marker) { position: relative; width: 28px; height: 34px; }
	@keyframes missionDraftPulse { 0% { transform: scale(0.6); opacity: 0.35 } 100% { transform: scale(1.5); opacity: 0 } }

	/* Own-position (live GPS) marker — must be :global since injected via L.divIcon */
	:global(.own-pos-marker) {
		background: none !important;
		border: none !important;
	}
	:global(.own-pos-wrap) {
		position: relative;
		width: 40px;
		height: 40px;
		overflow: visible;
		pointer-events: none;
	}
	:global(.own-pos-dot) {
		position: absolute;
		left: 50%; top: 50%;
		width: 14px; height: 14px;
		margin: -7px 0 0 -7px;
		border-radius: var(--radius-full);
		background: var(--color-accent);
		border: 2px solid #fff;
		box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.4);
	}
	:global(.own-pos-dot--stale) {
		opacity: 0.45;
		border: 1px dashed #fff;
	}
	:global(.own-pos-pulse) {
		position: absolute;
		left: 50%; top: 50%;
		width: 34px; height: 34px;
		margin: -17px 0 0 -17px;
		border-radius: var(--radius-full);
		border: 2px solid var(--color-accent);
		animation: own-pos-pulse 2s var(--ease-out) infinite;
	}
	:global(.own-pos-stale-ring) {
		position: absolute;
		left: 50%; top: 50%;
		width: 20px; height: 20px;
		margin: -10px 0 0 -10px;
		border-radius: var(--radius-full);
		border: 1px dashed rgba(255, 255, 255, 0.6);
	}
	@keyframes own-pos-pulse {
		0% { transform: scale(0.6); opacity: 0.8; }
		100% { transform: scale(1); opacity: 0; }
	}
	@media (prefers-reduced-motion: reduce) {
		:global(.own-pos-pulse) { animation: none; opacity: 0.35; }
	}
	:global(.own-pos-cone) {
		position: absolute;
		left: 50%; top: 50%;
		width: 56px; height: 56px;
		margin: -28px 0 0 -28px;
		pointer-events: none;
	}
	:global(.own-pos-tooltip) {
		font-size: 11px;
	}

	/* NWS Alerts (internal/wxalert) — see lib/wxAlertMap.ts chipHtml() for the markup this styles. */
	.wx-defs {
		position: absolute;
		width: 0;
		height: 0;
		overflow: hidden;
	}

	:global(.wx-alert-chip-wrap) { background: none !important; border: none !important; }
	:global(.wx-alert-chip) {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		background: rgba(10, 15, 30, 0.82);
		border: 1px solid rgba(255, 255, 255, 0.15);
		border-left: 3px solid var(--color-wx-statement);
		border-radius: 10px;
		padding: 2px 7px 2px 6px;
		font-size: 11px;
		font-weight: 600;
		color: var(--color-text);
		white-space: nowrap;
		font-variant-numeric: tabular-nums;
		pointer-events: auto;
		cursor: pointer;
	}
	:global(.wx-alert-chip-warning) { border-left-color: var(--color-wx-warning); }
	:global(.wx-alert-chip-watch) { border-left-color: var(--color-wx-watch); }
	:global(.wx-alert-chip svg) { color: inherit; }
	:global(.wx-alert-chip-warning svg) { color: var(--color-wx-warning); }
	:global(.wx-alert-chip-watch svg) { color: var(--color-wx-watch); }
	:global(.wx-alert-chip .wx-chip-ttl) { color: var(--color-text-muted); }
	/* Leaflet's default tooltip is a light chip with nowrap text — unreadable
	   in this theme and prone to running past its own box. Own the whole box. */
	:global(.wx-alert-tooltip) {
		display: flex;
		flex-direction: column;
		gap: 2px;
		/* A Leaflet tooltip is absolutely positioned, so it is shrink-to-fit:
		   its width comes from the content's min/max-content size. `width:
		   max-content` makes it lay out at its natural single-line width and
		   only then get clamped by max-width. Without it the box collapses
		   toward min-content — and `overflow-wrap: anywhere` (unlike
		   `break-word`) counts toward min-content, which collapsed it to
		   about one character per line. */
		width: max-content;
		max-width: min(280px, 70vw);
		padding: var(--space-sm) var(--space-md);
		white-space: normal;
		overflow-wrap: break-word;
		background: var(--color-surface);
		color: var(--color-text);
		border: 1px solid rgba(255, 255, 255, 0.12);
		border-left: 3px solid var(--color-text-muted);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-md);
		font-size: 0.75rem;
		line-height: 1.4;
	}
	:global(.wx-alert-tooltip::before) { display: none; }
	/* Hit-test on the stroke area even though the line paints nothing. */
	:global(path.wx-alert-hit) {
		pointer-events: stroke;
		cursor: pointer;
	}
	:global(.wx-alert-tooltip-warning) { border-left-color: var(--color-wx-warning); }
	:global(.wx-alert-tooltip-watch) { border-left-color: var(--color-wx-watch); }
	:global(.wx-alert-tooltip-advisory) { border-left-color: var(--color-wx-advisory); }
	:global(.wx-alert-tooltip-statement) { border-left-color: var(--color-wx-statement); }
	:global(.wx-alert-tooltip .wx-tt-title) {
		font-weight: 600;
		font-size: 0.8rem;
	}
	:global(.wx-alert-tooltip .wx-tt-meta),
	:global(.wx-alert-tooltip .wx-tt-area) {
		color: var(--color-text-muted);
	}
	:global(.wx-alert-tooltip .wx-tt-area) {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	:global(.wx-alert-tooltip .wx-tt-note) {
		margin-top: 2px;
		padding-top: 4px;
		border-top: 1px solid rgba(255, 255, 255, 0.1);
		color: var(--color-text-muted);
		font-style: italic;
	}
</style>
