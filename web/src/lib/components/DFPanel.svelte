<script lang="ts">
	import { dfStations } from '$lib/stores/df';
	import type { Station } from '$lib/types';

	let {
		onFlyTo,
		onFlyToTarget
	}: {
		onFlyTo?: (lat: number, lon: number) => void;
		onFlyToTarget?: (lat: number, lon: number) => void;
	} = $props();

	/** Compute pairwise bearing line intersections and their centroid. */
	let targetEstimate = $derived.by(() => {
		const stations = $dfStations;
		if (stations.length < 2) return null;

		const intersections: Array<{ lat: number; lon: number }> = [];

		for (let i = 0; i < stations.length; i++) {
			for (let j = i + 1; j < stations.length; j++) {
				const a = stations[i];
				const b = stations[j];
				if (!a.position || !b.position || !a.df || !b.df) continue;

				const pt = bearingIntersection(
					a.position.lat, a.position.lon, a.df.bearing,
					b.position.lat, b.position.lon, b.df.bearing
				);
				if (pt) intersections.push(pt);
			}
		}

		if (intersections.length === 0) return null;

		// Centroid
		let latSum = 0, lonSum = 0;
		for (const p of intersections) {
			latSum += p.lat;
			lonSum += p.lon;
		}
		const lat = latSum / intersections.length;
		const lon = lonSum / intersections.length;

		// Spread (max distance from centroid to any intersection)
		let maxDist = 0;
		for (const p of intersections) {
			const d = haversineDistance(lat, lon, p.lat, p.lon);
			if (d > maxDist) maxDist = d;
		}

		return { lat, lon, spreadKm: maxDist, count: intersections.length };
	});

	function qualityColor(q: number): string {
		if (q >= 7) return '#22c55e';
		if (q >= 4) return '#f59e0b';
		return '#ef4444';
	}

	function qualityLabel(q: number): string {
		if (q >= 7) return 'High';
		if (q >= 4) return 'Medium';
		if (q === 0) return 'None';
		return 'Low';
	}

	function handleFlyToStation(s: Station) {
		if (s.position) {
			onFlyTo?.(s.position.lat, s.position.lon);
		}
	}

	function handleFlyToTarget() {
		if (targetEstimate) {
			onFlyToTarget?.(targetEstimate.lat, targetEstimate.lon);
		}
	}

	function stationKey(s: Station): string {
		return s.ssid > 0 ? `${s.callsign}-${s.ssid}` : s.callsign;
	}

	// Planar bearing line intersection — good enough for foxhunt distances
	function bearingIntersection(
		lat1: number, lon1: number, brg1: number,
		lat2: number, lon2: number, brg2: number
	): { lat: number; lon: number } | null {
		const toRad = Math.PI / 180;
		const b1 = brg1 * toRad;
		const b2 = brg2 * toRad;

		// Direction vectors
		const dx1 = Math.sin(b1);
		const dy1 = Math.cos(b1);
		const dx2 = Math.sin(b2);
		const dy2 = Math.cos(b2);

		// Solve parametric intersection
		const det = dx1 * dy2 - dx2 * dy1;
		if (Math.abs(det) < 1e-10) return null; // parallel lines

		// Scale lon difference by cos(lat) to approximate planar
		const cosLat = Math.cos(((lat1 + lat2) / 2) * toRad);
		const dLon = (lon2 - lon1) * cosLat;
		const dLat = lat2 - lat1;

		const t = (dLon * dy2 - dLat * dx2) / det;

		// Only accept forward intersections (bearing is a ray, not a line)
		if (t < 0) return null;

		const lat = lat1 + t * dy1;
		const lon = lon1 + t * dx1 / cosLat;

		// Sanity: reject if intersection is very far away (>500km)
		if (haversineDistance(lat1, lon1, lat, lon) > 500) return null;

		return { lat, lon };
	}

	function haversineDistance(lat1: number, lon1: number, lat2: number, lon2: number): number {
		const R = 6371; // km
		const toRad = Math.PI / 180;
		const dLat = (lat2 - lat1) * toRad;
		const dLon = (lon2 - lon1) * toRad;
		const a = Math.sin(dLat / 2) ** 2 +
			Math.cos(lat1 * toRad) * Math.cos(lat2 * toRad) * Math.sin(dLon / 2) ** 2;
		return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
	}
</script>

<div class="df-panel">
	<div class="df-header">
		<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
			<circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.3"/>
			<circle cx="8" cy="8" r="2" stroke="currentColor" stroke-width="1.3"/>
			<path d="M8 2v3M8 11v3M2 8h3M11 8h3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
		</svg>
		<h2 class="panel-title">Direction Finding</h2>
		<span class="station-count">{$dfStations.length}</span>
	</div>

	{#if $dfStations.length === 0}
		<div class="df-empty">
			<p>No DF stations heard yet.</p>
			<p class="df-empty-hint">Direction Finding data appears automatically when stations report bearings with the DF symbol (/\).</p>
		</div>
	{:else}
		<!-- Target estimate card -->
		{#if targetEstimate}
			<div class="df-target-card">
				<div class="df-target-header">
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
						<circle cx="8" cy="8" r="6" stroke="#ef4444" stroke-width="1.5"/>
						<circle cx="8" cy="8" r="2" fill="#ef4444"/>
						<path d="M8 1v3M8 12v3M1 8h3M12 8h3" stroke="#ef4444" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
					<span class="target-label">Estimated Target</span>
					<button class="fly-btn" onclick={handleFlyToTarget} title="Fly to target">
						<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
							<path d="M3 8h10M8 3l5 5-5 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
						</svg>
					</button>
				</div>
				<div class="target-coords">
					{targetEstimate.lat.toFixed(5)}, {targetEstimate.lon.toFixed(5)}
				</div>
				<div class="target-meta">
					{targetEstimate.count} intersection{targetEstimate.count !== 1 ? 's' : ''}
					{#if targetEstimate.spreadKm > 0.01}
						<span class="target-spread">
							&middot; spread {targetEstimate.spreadKm < 1
								? `${Math.round(targetEstimate.spreadKm * 1000)}m`
								: `${targetEstimate.spreadKm.toFixed(1)}km`}
						</span>
					{/if}
				</div>
			</div>
		{/if}

		<!-- DF station list -->
		<div class="df-list">
			{#each $dfStations as station (stationKey(station))}
				{@const df = station.df!}
				<div class="df-card">
					<div class="df-card-header">
						<span class="df-callsign">{stationKey(station)}</span>
						<button class="fly-btn" onclick={() => handleFlyToStation(station)} title="Fly to station">
							<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
								<path d="M3 8h10M8 3l5 5-5 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
							</svg>
						</button>
					</div>
					<div class="df-stats">
						<div class="df-stat">
							<span class="df-stat-label">Bearing</span>
							<span class="df-stat-value">{df.bearing.toFixed(0)}&deg;</span>
						</div>
						<div class="df-stat">
							<span class="df-stat-label">Range</span>
							<span class="df-stat-value">{df.range > 0 ? `${df.range.toFixed(0)} mi` : '—'}</span>
						</div>
						<div class="df-stat">
							<span class="df-stat-label">Quality</span>
							<span class="df-stat-value">
								<span class="quality-bar">
									{#each Array(9) as _, i}
										<span
											class="quality-segment"
											class:filled={i < df.quality}
											style="background: {i < df.quality ? qualityColor(df.quality) : 'var(--color-primary)'}"
										></span>
									{/each}
								</span>
								<span class="quality-text" style="color: {qualityColor(df.quality)}">{qualityLabel(df.quality)}</span>
							</span>
						</div>
						<div class="df-stat">
							<span class="df-stat-label">Hits</span>
							<span class="df-stat-value">{df.number}</span>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.df-panel {
		padding: var(--space-md);
	}

	.df-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-bottom: var(--space-md);
		color: var(--color-text);
	}

	.panel-title {
		font-size: 1rem;
		font-weight: 600;
		margin: 0;
	}

	.station-count {
		margin-left: auto;
		font-size: 0.75rem;
		color: var(--color-text-muted);
		background: var(--color-primary);
		padding: 2px 8px;
		border-radius: 10px;
	}

	.df-empty {
		text-align: center;
		padding: var(--space-lg) var(--space-md);
		color: var(--color-text-muted);
	}

	.df-empty p {
		margin: 0 0 var(--space-sm);
	}

	.df-empty-hint {
		font-size: 0.8rem;
		opacity: 0.7;
	}

	/* Target estimate card */
	.df-target-card {
		background: rgba(239, 68, 68, 0.08);
		border: 1px solid rgba(239, 68, 68, 0.3);
		border-radius: var(--radius-md);
		padding: var(--space-sm) var(--space-md);
		margin-bottom: var(--space-md);
	}

	.df-target-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-bottom: var(--space-xs);
	}

	.target-label {
		font-size: 0.85rem;
		font-weight: 600;
		color: #ef4444;
	}

	.target-coords {
		font-family: monospace;
		font-size: 0.8rem;
		color: var(--color-text);
		margin-bottom: 2px;
	}

	.target-meta {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.target-spread {
		color: var(--color-text-muted);
	}

	/* Station cards */
	.df-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.df-card {
		background: var(--color-primary);
		border-radius: var(--radius-md);
		padding: var(--space-sm) var(--space-md);
	}

	.df-card-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: var(--space-xs);
	}

	.df-callsign {
		font-family: monospace;
		font-weight: 600;
		font-size: 0.85rem;
		color: var(--color-text);
	}

	.fly-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 24px;
		height: 24px;
		background: none;
		border: 1px solid var(--color-text-muted);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: color var(--duration-fast), border-color var(--duration-fast);
	}

	.fly-btn:hover {
		color: var(--color-text);
		border-color: var(--color-text);
	}

	.df-stats {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-xs) var(--space-md);
	}

	.df-stat {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}

	.df-stat-label {
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}

	.df-stat-value {
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--color-text);
	}

	.quality-bar {
		display: inline-flex;
		gap: 2px;
		vertical-align: middle;
		margin-right: 4px;
	}

	.quality-segment {
		width: 4px;
		height: 10px;
		border-radius: 1px;
	}

	.quality-text {
		font-size: 0.7rem;
		font-weight: 600;
	}
</style>
