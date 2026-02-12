<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { Station, Annotation, NetCheckIn, NetMission } from '$lib/types';
	import { symbolInfo } from '$lib/symbols';
	import { createStationIcon } from '$lib/aprs-icons';
	import { stationDisplayName } from '$lib/utils';
	import { getTacticalAlias } from '$lib/stores/tactical';
	import { weatherUnits } from '$lib/stores/weather';
	import { convertTemp, convertWindSpeed } from '$lib/units';
	import type { UnitSystem } from '$lib/units';
	import { get } from 'svelte/store';
	import L from 'leaflet';

	const DEFAULT_ANN_COLOR = '#e63946';

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
	} = $props();

	let mapEl: HTMLDivElement;
	let map: L.Map;

	export function getViewport(): { lat: number; lon: number; zoom: number } | null {
		if (!map) return null;
		const c = map.getCenter();
		return { lat: c.lat, lon: c.lng, zoom: map.getZoom() };
	}
	let markers: Map<string, L.Marker> = new Map();
	let trackLines: Map<string, L.Polyline> = new Map();
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
	// DF overlay layers
	let dfLines: Map<string, L.Polyline> = new Map();
	let dfRangeCircles: Map<string, L.Circle> = new Map();
	let dfTargetMarker: L.Marker | null = null;
	let dfTargetCircle: L.Circle | null = null;

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

	onMount(() => {
		map = L.map(mapEl, {
			zoomControl: true,
			attributionControl: true,
		}).setView([39.8283, -98.5795], 4);

		L.tileLayer('/tiles/{z}/{x}/{y}.png', {
			attribution: '&copy; OpenStreetMap contributors',
			maxZoom: 19,
		}).addTo(map);

		// Fix Leaflet icon path issue with bundlers
		delete (L.Icon.Default.prototype as Record<string, unknown>)._getIconUrl;
		L.Icon.Default.mergeOptions({
			iconRetinaUrl: undefined,
			iconUrl: undefined,
			shadowUrl: undefined,
		});

		updateMarkers();
		updateAnnotations();
	});

	onDestroy(() => {
		for (const layer of highlightOverlays) layer.remove();
		highlightOverlays = [];
		operatorHighlight?.remove();
		map?.remove();
	});

	$effect(() => {
		if (map) updateMarkers();
	});

	$effect(() => {
		if (map) updateAnnotations();
	});

	// Fly to target when it changes
	$effect(() => {
		if (map && flyToTarget) {
			map.flyTo([flyToTarget.lat, flyToTarget.lon], flyToTarget.zoom ?? 14);
		}
	});

	// Fly to bounds (multi-point) when they change
	$effect(() => {
		if (map && flyToBounds && flyToBounds.length > 0) {
			const bounds = L.latLngBounds(flyToBounds.map(p => [p.lat, p.lon] as L.LatLngExpression));
			map.flyToBounds(bounds, { padding: [50, 50], maxZoom: 16 });
		}
	});

	// Invalidate map size when panel opens/closes
	$effect(() => {
		const _open = panelOpen;
		if (map) {
			setTimeout(() => map.invalidateSize(), 400);
		}
	});

	// Drawing mode effects
	$effect(() => {
		if (!map) return;
		if (drawingMode) {
			mapEl.style.cursor = 'crosshair';
			map.on('click', handleDrawClick);
			map.on('dblclick', handleDrawDblClick);
			map.doubleClickZoom.disable();
		} else {
			if (!placingOperator) {
				mapEl.style.cursor = '';
				map.doubleClickZoom.enable();
			}
			map.off('click', handleDrawClick);
			map.off('dblclick', handleDrawDblClick);
			clearDrawState();
		}
	});

	// Place mode effects (click-to-set position for net operators)
	$effect(() => {
		if (!map) return;
		if (placingOperator) {
			mapEl.style.cursor = 'crosshair';
			map.on('click', handlePlaceClick);
			map.doubleClickZoom.disable();
		} else {
			if (!drawingMode) {
				mapEl.style.cursor = '';
				map.doubleClickZoom.enable();
			}
			map.off('click', handlePlaceClick);
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

			(layer as L.Layer & { bindTooltip: Function }).bindTooltip(
				ci.tacticalCall ? `${ci.callsign} "${ci.tacticalCall}"` : ci.callsign,
				{ permanent: false, direction: 'top', className: 'station-tooltip' }
			);
			(layer as L.Layer & { on: Function }).on('click', () => {
				onNetOperatorClick?.(ci.id);
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
			marker.on('click', () => {
				onNetMissionClick?.(m.id);
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
		const wxStations = weatherOverlay;
		const units = get(weatherUnits) as UnitSystem;

		// Remove all existing weather markers when hidden or data changes
		for (const [, m] of wxMarkers) m.remove();
		wxMarkers.clear();

		if (!show || !wxStations.length) return;

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

			const html = `<div class="wx-marker-pill" style="border-color:${staleColor};opacity:${staleOpacity}">
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
		const dfStations = dfOverlay;

		// Clear existing DF layers
		for (const [, line] of dfLines) line.remove();
		dfLines.clear();
		for (const [, circle] of dfRangeCircles) circle.remove();
		dfRangeCircles.clear();
		dfTargetMarker?.remove();
		dfTargetMarker = null;
		dfTargetCircle?.remove();
		dfTargetCircle = null;

		if (!show || !dfStations.length) return;

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
			if (placingOperator) {
				onPlaceCancelled?.();
				return;
			}
			if (drawingMode) {
				clearDrawState();
				onDrawComplete?.('');
			}
		}
	}

	function handlePlaceClick(e: L.LeafletMouseEvent) {
		if (!placingOperator) return;
		onOperatorPlaced?.(placingOperator.id, e.latlng.lat, e.latlng.lng);
	}

	function handleDrawClick(e: L.LeafletMouseEvent) {
		if (!drawingMode) return;

		if (drawingMode === 'point') {
			const geojson = JSON.stringify({
				type: 'Point',
				coordinates: [e.latlng.lng, e.latlng.lat]
			});
			clearDrawState();
			onDrawComplete?.(geojson);
			return;
		}

		// Line or Area — accumulate vertices
		drawVertices.push(e.latlng);
		const m = L.circleMarker(e.latlng, {
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
				(layer as L.Layer & { on: Function }).on('click', () => {
					onAnnotationClick?.(ann.id);
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

	function updateMarkers() {
		if (!map) return;

		const currentKeys = new Set<string>();

		for (const st of stations) {
			if (!st.position) continue;
			const key = stationKey(st);
			currentKeys.add(key);

			const info = symbolInfo(st.symbol);
			const baseName = stationDisplayName(st.callsign, st.ssid);
			const tacAlias = get(getTacticalAlias)(key);
			const name = tacAlias ? `${tacAlias} (${baseName})` : baseName;
			const isSelected = key === selectedCallsign;

			const iconOpts = createStationIcon(st.symbol, info.color, isSelected);
			const divIcon = L.divIcon(iconOpts);

			// Update or create marker
			let marker = markers.get(key);
			if (marker) {
				marker.setLatLng([st.position.lat, st.position.lon]);
				marker.setIcon(divIcon);
				// Update tooltip in case tactical alias changed
				marker.unbindTooltip();
				marker.bindTooltip(name, {
					permanent: false,
					direction: 'top',
					className: 'station-tooltip',
				});
			} else {
				marker = L.marker([st.position.lat, st.position.lon], {
					icon: divIcon,
				}).addTo(map);

				marker.bindTooltip(name, {
					permanent: false,
					direction: 'top',
					className: 'station-tooltip',
				});

				marker.on('click', () => {
					onStationClick?.(key);
				});

				markers.set(key, marker);
			}

			// Update track line
			if (st.track && st.track.length > 1) {
				const latlngs: L.LatLngExpression[] = st.track.map((tp) => [tp.lat, tp.lon]);
				let line = trackLines.get(key);
				if (line) {
					line.setLatLngs(latlngs);
				} else {
					line = L.polyline(latlngs, {
						color: info.color,
						weight: 2,
						opacity: 0.6,
						dashArray: '4 4',
					}).addTo(map);
					trackLines.set(key, line);
				}
			}
		}

		// Remove stale markers and tracks
		for (const [key, marker] of markers) {
			if (!currentKeys.has(key)) {
				marker.remove();
				markers.delete(key);
			}
		}
		for (const [key, line] of trackLines) {
			if (!currentKeys.has(key)) {
				line.remove();
				trackLines.delete(key);
			}
		}
	}
</script>

<svelte:window onkeydown={handleKeyDown} />

<div class="map-container" class:drawing={drawingMode !== null} class:placing={placingOperator !== null} bind:this={mapEl}></div>

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
</style>
