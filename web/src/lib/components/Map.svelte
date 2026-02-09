<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { Station, Annotation } from '$lib/types';
	import { symbolInfo, symbolChar } from '$lib/symbols';
	import { stationDisplayName } from '$lib/utils';
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
	} = $props();

	let mapEl: HTMLDivElement;
	let map: L.Map;
	let markers: Map<string, L.CircleMarker> = new Map();
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

	onMount(() => {
		map = L.map(mapEl, {
			zoomControl: true,
			attributionControl: true,
		}).setView([39.8283, -98.5795], 4);

		L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
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
			mapEl.style.cursor = '';
			map.off('click', handleDrawClick);
			map.off('dblclick', handleDrawDblClick);
			map.doubleClickZoom.enable();
			clearDrawState();
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

	// Escape key cancels drawing
	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'Escape' && drawingMode) {
			clearDrawState();
			onDrawComplete?.('');
		}
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
			const char = symbolChar(st.symbol);
			const name = stationDisplayName(st.callsign, st.ssid);
			const isSelected = key === selectedCallsign;

			// Update or create marker
			let marker = markers.get(key);
			if (marker) {
				marker.setLatLng([st.position.lat, st.position.lon]);
				marker.setStyle({
					fillColor: info.color,
					radius: isSelected ? 12 : 7,
					weight: isSelected ? 3 : 1,
					color: isSelected ? '#fff' : info.color,
					fillOpacity: isSelected ? 1 : 0.9,
				});
			} else {
				marker = L.circleMarker([st.position.lat, st.position.lon], {
					radius: isSelected ? 12 : 7,
					fillColor: info.color,
					color: isSelected ? '#fff' : info.color,
					weight: isSelected ? 3 : 1,
					fillOpacity: isSelected ? 1 : 0.9,
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

<div class="map-container" class:drawing={drawingMode !== null} bind:this={mapEl}></div>

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

<style>
	.map-container {
		width: 100%;
		height: 100%;
	}

	.map-container.drawing {
		cursor: crosshair;
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

	:global(.leaflet-popup-content-wrapper) {
		background: #1a1a2e;
		color: #eee;
		border-radius: 8px;
	}

	:global(.leaflet-popup-tip) {
		background: #1a1a2e;
	}
</style>
