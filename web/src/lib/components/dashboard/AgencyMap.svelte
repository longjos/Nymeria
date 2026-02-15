<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import type { NetCheckIn, NetMission, Annotation } from '$lib/types';
	import { stationCategoryMeta } from '$lib/stationCategoryMeta';
	import { humanName, priorityLabels } from '$lib/agencyTranslations';

	let {
		operators,
		missions,
		annotations,
		assignments,
		opsView,
	}: {
		operators: NetCheckIn[];
		missions: NetMission[];
		annotations: Annotation[];
		assignments: { operator: NetCheckIn; mission: NetMission }[];
		opsView: { lat: number; lon: number; zoom: number } | null;
	} = $props();

	let mapEl: HTMLDivElement;
	let map: any;
	let L: any;
	let layerGroup: any;

	onMount(async () => {
		if (!browser) return;
		L = await import('leaflet');

		const center: [number, number] = opsView
			? [opsView.lat, opsView.lon]
			: [39.8283, -98.5795]; // US center fallback
		const zoom = opsView?.zoom ?? 5;

		map = L.map(mapEl, {
			zoomControl: true,
			attributionControl: false,
			dragging: true,
			scrollWheelZoom: true,
		}).setView(center, zoom);

		L.tileLayer('/tiles/{z}/{x}/{y}', {
			maxZoom: 19,
			errorTileUrl: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
		}).addTo(map);

		// Fallback tile layer
		L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
			maxZoom: 19,
		}).addTo(map);

		layerGroup = L.layerGroup().addTo(map);
		renderLayers();

		return () => {
			map?.remove();
		};
	});

	$effect(() => {
		// Reactively re-render when data changes
		operators; missions; annotations; assignments;
		if (map && layerGroup && L) {
			renderLayers();
		}
	});

	function renderLayers() {
		layerGroup.clearLayers();

		// Annotation geometries (lines, areas, points)
		for (const ann of annotations) {
			try {
				const geo = JSON.parse(ann.geometry);
				if (ann.type === 'point' && geo.lat != null && geo.lon != null) {
					L.circleMarker([geo.lat, geo.lon], {
						radius: 5,
						color: '#888',
						fillColor: '#888',
						fillOpacity: 0.4,
						weight: 1,
					}).bindTooltip(ann.label, { direction: 'top', offset: [0, -8] })
					  .addTo(layerGroup);
				} else if (ann.type === 'line' && Array.isArray(geo)) {
					L.polyline(geo.map((p: any) => [p.lat, p.lon]), {
						color: '#888',
						weight: 2,
						opacity: 0.5,
						dashArray: '6,4',
					}).bindTooltip(ann.label)
					  .addTo(layerGroup);
				} else if (ann.type === 'area' && Array.isArray(geo)) {
					L.polygon(geo.map((p: any) => [p.lat, p.lon]), {
						color: '#888',
						fillColor: '#888',
						fillOpacity: 0.1,
						weight: 1,
					}).bindTooltip(ann.label)
					  .addTo(layerGroup);
				}
			} catch {
				// Skip malformed geometry
			}
		}

		// Assignment lines (dashed)
		for (const a of assignments) {
			if (a.operator.lat == null || a.operator.lon == null) continue;
			if (a.mission.lat == null || a.mission.lon == null) continue;
			L.polyline(
				[[a.operator.lat, a.operator.lon], [a.mission.lat, a.mission.lon]],
				{ color: '#ffffff', weight: 1, opacity: 0.3, dashArray: '4,6' }
			).addTo(layerGroup);
		}

		// Mission markers
		for (const m of missions) {
			if (m.lat == null || m.lon == null) continue;
			const pColor = m.priority === 'emergency' ? '#ef4444'
				: m.priority === 'priority' ? '#f59e0b'
				: m.priority === 'welfare' ? '#3b82f6'
				: '#6b7280';
			const label = priorityLabels[m.priority] || m.priority;

			L.marker([m.lat, m.lon], {
				icon: L.divIcon({
					className: 'agency-mission-icon',
					html: `<div style="background:${pColor};width:12px;height:16px;clip-path:polygon(50% 0%,100% 35%,100% 100%,0% 100%,0% 35%);"></div>`,
					iconSize: [12, 16],
					iconAnchor: [6, 16],
				}),
			}).bindTooltip(`${label}: ${m.title}`, { direction: 'top', offset: [0, -18] })
			  .addTo(layerGroup);
		}

		// Operator markers
		for (const op of operators) {
			if (op.lat == null || op.lon == null) continue;
			const cat = op.category || 'general';
			const meta = stationCategoryMeta[cat] || stationCategoryMeta.general;
			const name = humanName(op);

			L.circleMarker([op.lat, op.lon], {
				radius: 8,
				color: meta.color,
				fillColor: meta.color,
				fillOpacity: 0.8,
				weight: 2,
			}).bindTooltip(name, {
				direction: 'top',
				offset: [0, -10],
				permanent: false,
			}).addTo(layerGroup);

			// Name label below marker
			L.marker([op.lat, op.lon], {
				icon: L.divIcon({
					className: 'agency-name-label',
					html: `<span style="color:${meta.color}">${name}</span>`,
					iconSize: [80, 14],
					iconAnchor: [40, -12],
				}),
				interactive: false,
			}).addTo(layerGroup);
		}

		// Auto-fit bounds if no opsView and we have positioned items
		if (!opsView) {
			const points: [number, number][] = [];
			for (const op of operators) {
				if (op.lat != null && op.lon != null) points.push([op.lat, op.lon]);
			}
			for (const m of missions) {
				if (m.lat != null && m.lon != null) points.push([m.lat, m.lon]);
			}
			if (points.length > 1) {
				map.fitBounds(L.latLngBounds(points), { padding: [30, 30] });
			} else if (points.length === 1) {
				map.setView(points[0], 14);
			}
		}
	}
</script>

<div class="agency-map" bind:this={mapEl}></div>

<style>
	.agency-map {
		width: 100%;
		height: 100%;
		min-height: 200px;
		background: var(--color-bg);
	}

	:global(.agency-name-label) {
		background: none !important;
		border: none !important;
		box-shadow: none !important;
	}

	:global(.agency-name-label span) {
		font-size: 0.65rem;
		font-weight: 700;
		white-space: nowrap;
		text-align: center;
		display: block;
		text-shadow: 0 1px 3px rgba(0,0,0,0.8), 0 0 6px rgba(0,0,0,0.6);
	}

	:global(.agency-mission-icon) {
		background: none !important;
		border: none !important;
		box-shadow: none !important;
	}
</style>
