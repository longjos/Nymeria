/**
 * Pure map-rendering helpers for NWS Alerts (internal/wxalert). Kept
 * separate from Map.svelte (which wires these into Leaflet layers, panes and
 * store subscriptions — WP4) so the geometry math, draw ordering and chip
 * markup are unit-testable without a DOM/map instance.
 *
 * `leaflet` is imported as a TYPE only: the real leaflet package touches
 * `window` at module load (it is a browser-only UMD bundle) and this module
 * must also load cleanly under plain Node (`pnpm test`, no jsdom). Functions
 * that build actual layers (`fallbackLayers`) take the two factory
 * functions they need as a parameter — Map.svelte passes its already-loaded
 * `L` object, which satisfies the shape structurally.
 */
import type L from 'leaflet';
import type { Annotation, WxAlert, WxLinkState, WxPolygonGeometry, WxSeverity, WxTier } from './types';
import { severityStyle, tierMeta } from './wxAlertMeta';
import { countdown } from './wxAlertTime';

export interface WxTokens {
	warning: string;
	watch: string;
	advisory: string;
	statement: string;
	expired: string;
	dashWarning: string;
	dashWatch: string;
	dashAdvisory: string;
	dashStatement: string;
	casing: string;
	zPane: string;
}

const TOKEN_FALLBACKS: WxTokens = {
	warning: '#e74c3c',
	watch: '#f59e0b',
	advisory: '#3b82f6',
	statement: '#8b5cf6',
	expired: '#6b7280',
	dashWarning: '10 6',
	dashWatch: '4 6',
	dashAdvisory: '1 6',
	dashStatement: '2 6',
	casing: 'rgba(255, 255, 255, 0.55)',
	zPane: '350'
};

/**
 * Reads the --color-wx-…, --wx-dash-…, --wx-casing and --z-wx-pane custom
 * properties off `:root` once at map mount. Leaflet path options are plain
 * strings/numbers, not `var()`, so these values must be resolved in JS.
 */
export function readWxTokens(root?: HTMLElement): WxTokens {
	if (typeof getComputedStyle !== 'function') return { ...TOKEN_FALLBACKS };
	const el = root ?? (typeof document !== 'undefined' ? document.documentElement : undefined);
	if (!el) return { ...TOKEN_FALLBACKS };
	const cs = getComputedStyle(el);
	const v = (name: string, fallback: string) => cs.getPropertyValue(name).trim() || fallback;
	return {
		warning: v('--color-wx-warning', TOKEN_FALLBACKS.warning),
		watch: v('--color-wx-watch', TOKEN_FALLBACKS.watch),
		advisory: v('--color-wx-advisory', TOKEN_FALLBACKS.advisory),
		statement: v('--color-wx-statement', TOKEN_FALLBACKS.statement),
		expired: v('--color-wx-expired', TOKEN_FALLBACKS.expired),
		dashWarning: v('--wx-dash-warning', TOKEN_FALLBACKS.dashWarning),
		dashWatch: v('--wx-dash-watch', TOKEN_FALLBACKS.dashWatch),
		dashAdvisory: v('--wx-dash-advisory', TOKEN_FALLBACKS.dashAdvisory),
		dashStatement: v('--wx-dash-statement', TOKEN_FALLBACKS.dashStatement),
		casing: v('--wx-casing', TOKEN_FALLBACKS.casing),
		zPane: v('--z-wx-pane', TOKEN_FALLBACKS.zPane)
	};
}

function tierColor(tokens: WxTokens, tier: WxTier): string {
	switch (tier) {
		case 'warning':
			return tokens.warning;
		case 'watch':
			return tokens.watch;
		case 'advisory':
			return tokens.advisory;
		default:
			return tokens.statement;
	}
}

function tierDashArray(tokens: WxTokens, tier: WxTier): string {
	switch (tier) {
		case 'warning':
			return tokens.dashWarning;
		case 'watch':
			return tokens.dashWatch;
		case 'advisory':
			return tokens.dashAdvisory;
		default:
			return tokens.dashStatement;
	}
}

export interface WxPathOptions {
	color: string;
	dashArray: string;
	weight: number;
	fillOpacity: number;
	casing: string;
	/** 1 when the link is live, 0.5 when stale/down (§5.3 "dated" look). */
	strokeOpacity: number;
	/** Whether this tier/severity/link-state combination gets the hatch overlay. */
	hatch: boolean;
}

/** Resolves the stroke/fill recipe for one alert's tier+severity, dimmed when the link is stale/down. */
export function pathOptions(tier: WxTier, severity: WxSeverity, linkState: WxLinkState, tokens: WxTokens = readWxTokens()): WxPathOptions {
	const sev = severityStyle(tier, severity);
	const dimmed = linkState === 'stale' || linkState === 'down';
	return {
		color: tierColor(tokens, tier),
		dashArray: tierDashArray(tokens, tier),
		weight: sev.weight,
		fillOpacity: sev.fillOpacity,
		casing: tokens.casing,
		strokeOpacity: dimmed ? 0.5 : 1,
		hatch: sev.hatch && !dimmed
	};
}

/**
 * Every ring of a Polygon/MultiPolygon, converted from GeoJSON [lon, lat]
 * order to Leaflet [lat, lon] and closed (first vertex repeated at the end)
 * so a polyline drawn from the ring closes visually like the fill does.
 */
export function ringsOf(geometry?: WxPolygonGeometry | null): L.LatLngTuple[][] {
	if (!geometry || !geometry.coordinates) return [];
	const polygons: number[][][][] =
		geometry.type === 'MultiPolygon' ? (geometry.coordinates as number[][][][]) : [geometry.coordinates as number[][][]];

	const rings: L.LatLngTuple[][] = [];
	for (const polygon of polygons) {
		for (const ring of polygon) {
			if (!Array.isArray(ring) || ring.length === 0) continue;
			const latlngs: L.LatLngTuple[] = ring.map(([lon, lat]) => [lat, lon] as L.LatLngTuple);
			const [firstLat, firstLon] = latlngs[0];
			const [lastLat, lastLon] = latlngs[latlngs.length - 1];
			if (firstLat !== lastLat || firstLon !== lastLon) latlngs.push([firstLat, firstLon]);
			rings.push(latlngs);
		}
	}
	return rings;
}

/** The ring vertex closest to `point` (haversine-free planar distance is fine at chip-placement precision). */
export function nearestVertex(rings: L.LatLngTuple[][], point: { lat: number; lon: number }): L.LatLngTuple | null {
	let best: L.LatLngTuple | null = null;
	let bestDist = Infinity;
	for (const ring of rings) {
		for (const v of ring) {
			const d = (v[0] - point.lat) ** 2 + (v[1] - point.lon) ** 2;
			if (d < bestDist) {
				bestDist = d;
				best = v;
			}
		}
	}
	return best;
}

function escapeHtml(s: string): string {
	return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

/** Builds the divIcon HTML for one warning/watch alert's map chip (§5.5). */
export function chipHtml(alert: WxAlert, linkState: WxLinkState, nowMs: number): string {
	const meta = tierMeta[alert.tier];
	const stale = linkState === 'stale' || linkState === 'down';
	const zoneOnly = alert.geometrySource === 'zone';
	const cd = countdown(alert.endsAt, nowMs);
	const fillAttr = meta.glyphFill ? 'fill="currentColor"' : 'fill="none" stroke="currentColor" stroke-width="1.5"';
	return (
		`<div class="wx-alert-chip wx-alert-chip-${alert.tier}" role="presentation">` +
		`<svg width="12" height="12" viewBox="0 0 16 16" aria-hidden="true"><path d="${meta.glyph}" ${fillAttr}/></svg>` +
		`<span class="wx-chip-code">${stale ? '? ' : ''}${escapeHtml(alert.shortCode)}${zoneOnly ? ' · ZONE' : ''}</span>` +
		`<span class="wx-chip-ttl">· ${escapeHtml(cd.text)}</span>` +
		`</div>`
	);
}

function pointInRing(lat: number, lon: number, ring: L.LatLngTuple[]): boolean {
	let inside = false;
	for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
		const [latI, lonI] = ring[i];
		const [latJ, lonJ] = ring[j];
		const crosses = lonI > lon !== lonJ > lon && lat < ((latJ - latI) * (lon - lonI)) / (lonJ - lonI) + latI;
		if (crosses) inside = !inside;
	}
	return inside;
}

/** Ray-cast even-odd test against `alert.geometry`. Zone-only alerts (no published polygon) always return false — zone membership is resolved server-side. */
export function pointInAlert(alert: WxAlert, lat: number, lon: number): boolean {
	if (!alert.geometry) return false;
	let inside = false;
	for (const ring of ringsOf(alert.geometry)) {
		if (pointInRing(lat, lon, ring)) inside = !inside;
	}
	return inside;
}

const TIER_PAINT_RANK: Record<WxTier, number> = { statement: 0, advisory: 1, watch: 2, warning: 3 };
const SEVERITY_PAINT_RANK: Record<WxSeverity, number> = { Unknown: 0, Minor: 1, Moderate: 2, Severe: 3, Extreme: 4 };

/**
 * Paint order for a batch of alert layers: statements -> advisories ->
 * watches -> warnings, Minor -> Extreme within a tier — the reverse of the
 * list's importance sort, so the most important edge is drawn last (on top).
 */
export function drawOrder(alerts: WxAlert[]): WxAlert[] {
	return [...alerts].sort((a, b) => {
		const t = TIER_PAINT_RANK[a.tier] - TIER_PAINT_RANK[b.tier];
		if (t !== 0) return t;
		return SEVERITY_PAINT_RANK[a.severity] - SEVERITY_PAINT_RANK[b.severity];
	});
}

function parseLineString(ann: Annotation): L.LatLngTuple[] | null {
	let geo: { type?: string; coordinates?: unknown };
	try {
		geo = typeof ann.geometry === 'string' ? JSON.parse(ann.geometry) : (ann.geometry as unknown as { type?: string; coordinates?: unknown });
	} catch {
		return null;
	}
	if (geo?.type !== 'LineString' || !Array.isArray(geo.coordinates)) return null;
	return (geo.coordinates as [number, number][]).map(([lon, lat]) => [lat, lon] as L.LatLngTuple);
}

function annotationWeight(ann: Annotation): number {
	if (!ann.style) return 2;
	try {
		const style = JSON.parse(ann.style) as Record<string, unknown>;
		return typeof style.weight === 'number' && style.weight > 0 ? style.weight : 2;
	} catch {
		return 2;
	}
}

/** The two Leaflet factory functions `fallbackLayers` needs — the real `L` object satisfies this structurally. */
export interface WxLeafletFactories {
	polyline: (latlngs: L.LatLngExpression[], options?: L.PolylineOptions) => L.Polyline;
	circleMarker: (latlng: L.LatLngExpression, options?: L.CircleMarkerOptions) => L.CircleMarker;
}

/**
 * Own-geometry fallback for a zone-only alert whose zone polygon is not
 * cached (§5.6): a wide translucent ribbon over the touched route span(s),
 * plus a dashed halo around every affected point whose zone is missing.
 */
export function fallbackLayers(
	alert: WxAlert,
	annotations: Annotation[],
	leaflet: WxLeafletFactories,
	tokens: WxTokens = readWxTokens()
): L.Layer[] {
	const color = tierColor(tokens, alert.tier);
	const dash = tierDashArray(tokens, alert.tier);
	const byId = new Map(annotations.map((a) => [a.id, a]));
	const layers: L.Layer[] = [];
	const missingUgc = new Set(alert.zones.filter((z) => !z.cached).map((z) => z.ugc));

	for (const span of alert.affects.routeSpans) {
		// A span tagged with a specific zone only matters when that zone is
		// missing; an untagged span (single-zone alert) matters only when
		// there is a missing zone at all. Either way, nothing to draw when
		// every zone touching this alert is already cached.
		if (span.ugc ? !missingUgc.has(span.ugc) : missingUgc.size === 0) continue;
		const ann = byId.get(span.annotationId);
		if (!ann) continue;
		const latlngs = parseLineString(ann);
		if (!latlngs) continue;
		const slice = latlngs.slice(span.startIndex, span.endIndex + 1);
		if (slice.length < 2) continue;
		layers.push(
			leaflet.polyline(slice, {
				color,
				weight: annotationWeight(ann) + 8,
				opacity: 0.35,
				lineCap: 'round',
				lineJoin: 'round',
				interactive: true
			})
		);
	}

	const items = [...alert.affects.checkpoints, ...alert.affects.locations, ...alert.affects.stations];
	for (const item of items) {
		// Same rule as the route spans above: an item tagged with a zone only
		// gets a halo when that zone is missing; an untagged item only when
		// there is a missing zone at all.
		if (item.ugc ? !missingUgc.has(item.ugc) : missingUgc.size === 0) continue;
		layers.push(
			leaflet.circleMarker([item.lat, item.lon], {
				radius: 14,
				color,
				weight: 2,
				dashArray: dash,
				fill: false,
				interactive: true
			})
		);
	}

	return layers;
}
