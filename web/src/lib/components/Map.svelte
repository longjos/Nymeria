<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { Station } from '$lib/types';
	import { symbolInfo, symbolChar } from '$lib/symbols';
	import { stationDisplayName } from '$lib/utils';
	import L from 'leaflet';

	let { stations = [], selectedCallsign = '' }: { stations?: Station[]; selectedCallsign?: string } = $props();

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
					weight: isSelected ? 3 : 1,
					color: isSelected ? '#fff' : info.color,
				});
			} else {
				marker = L.circleMarker([st.position.lat, st.position.lon], {
					radius: isSelected ? 10 : 7,
					fillColor: info.color,
					color: isSelected ? '#fff' : info.color,
					weight: isSelected ? 3 : 1,
					fillOpacity: 0.9,
				}).addTo(map);

				marker.bindTooltip(name, {
					permanent: false,
					direction: 'top',
					className: 'station-tooltip',
				});

				const popupHtml = `
					<div style="font-family: monospace; min-width: 140px;">
						<div style="font-weight: 700; font-size: 14px; margin-bottom: 4px;">
							<span style="display: inline-block; width: 18px; height: 18px; border-radius: 50%; background: ${info.color}; color: #fff; text-align: center; line-height: 18px; font-size: 10px; margin-right: 4px;">${char}</span>
							${name}
						</div>
						<div style="font-size: 12px; color: #888;">${info.label}</div>
						${st.comment ? `<div style="font-size: 12px; margin-top: 4px; color: #ccc;">${st.comment}</div>` : ''}
						<div style="font-size: 11px; margin-top: 4px;">
							<a href="/stations/${st.callsign}" style="color: #e94560;">Details</a>
						</div>
					</div>
				`;
				marker.bindPopup(popupHtml);
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

	export function flyTo(lat: number, lon: number, zoom = 14): void {
		map?.flyTo([lat, lon], zoom);
	}
</script>

<div class="map-container" bind:this={mapEl}></div>

<style>
	.map-container {
		width: 100%;
		height: 100%;
		min-height: 400px;
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
