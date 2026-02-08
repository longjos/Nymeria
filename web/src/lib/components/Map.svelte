<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { Station } from '$lib/types';
	import { symbolInfo, symbolChar } from '$lib/symbols';
	import { stationDisplayName } from '$lib/utils';
	import L from 'leaflet';

	let {
		stations = [],
		selectedCallsign = '',
		onStationClick,
		flyToTarget,
		panelOpen = false
	}: {
		stations?: Station[];
		selectedCallsign?: string;
		onStationClick?: (stationKey: string) => void;
		flyToTarget?: { lat: number; lon: number; zoom?: number } | null;
		panelOpen?: boolean;
	} = $props();

	let mapEl: HTMLDivElement;
	let map: L.Map;
	let markers: Map<string, L.CircleMarker> = new Map();
	let trackLines: Map<string, L.Polyline> = new Map();

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
	});

	onDestroy(() => {
		map?.remove();
	});

	$effect(() => {
		if (map) updateMarkers();
	});

	// Fly to target when it changes
	$effect(() => {
		if (map && flyToTarget) {
			map.flyTo([flyToTarget.lat, flyToTarget.lon], flyToTarget.zoom ?? 14);
		}
	});

	// Invalidate map size when panel opens/closes
	$effect(() => {
		// Read panelOpen to subscribe to changes
		const _open = panelOpen;
		if (map) {
			setTimeout(() => map.invalidateSize(), 400);
		}
	});

	function stationKey(s: Station): string {
		return s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
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

<div class="map-container" bind:this={mapEl}></div>

<style>
	.map-container {
		width: 100%;
		height: 100%;
	}

	:global(.station-tooltip) {
		font-family: monospace;
		font-weight: 600;
		font-size: 12px;
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
