/**
 * Offline geocode formats — Open Location Code (Plus Codes) and MGRS/USNG.
 *
 * Everything here resolves fully client-side: no server call, no network, no
 * API key. That is the point — these are the formats that still work when
 * what3words can't be reached (issue #94), which during an incident is the
 * case that matters.
 *
 * Detection precedence is w3w -> pluscode -> mgrs (see detectFormat). The
 * component asks this module "what did they type?" once per keystroke and
 * then asks it to parse; nothing else in the app knows the libraries exist.
 */

import { OpenLocationCode } from 'open-location-code';
import { toPoint, forward } from 'mgrs';
import { isFullAddress, looksLikePartial } from './w3w';

const olc = new OpenLocationCode();

// --- Types ---------------------------------------------------------------

export type GeocodeFormat = 'w3w' | 'pluscode' | 'mgrs';

export interface GeocodeResult {
	lat: number;
	lon: number;
	/** Normalized, display-ready code — '8FVC9G8F+6X', '16SEG1234567890'. */
	label: string;
	/** Cell size in metres (the longest side), for the precision hint. */
	precisionM: number;
	format: 'pluscode' | 'mgrs';
	/** Plus Codes only: the full code a short code was recovered to. */
	recoveredFrom?: string;
}

export type GeocodeErrorCode =
	| 'invalid' // not parseable as this format at all
	| 'needs_reference' // short Plus Code, no reference point available
	| 'too_coarse'; // padded Plus Code / MGRS below 1 km

export interface GeocodeError {
	error: GeocodeErrorCode;
}

export type GeocodeParse = GeocodeResult | GeocodeError;

export function isGeocodeError(r: GeocodeParse): r is GeocodeError {
	return 'error' in r;
}

export interface RefPoint {
	lat: number;
	lon: number;
}

// --- Normalization ---------------------------------------------------------

/** Upper-case, trim, collapse away ALL internal whitespace (the libraries
 *  reject even one space), normalize unicode minus/fullwidth plus. */
export function normalizeGeocode(s: string): string {
	return s
		.trim()
		.toUpperCase()
		.replace(/＋/g, '+') // fullwidth plus, from IME keyboards
		.replace(/\s+/g, '');
}

// --- Detection ---------------------------------------------------------

/** Plus Code: 2-8 code chars, a '+', 0-7 code chars. The '+' is the whole
 *  reason this can never collide with a place name. Code alphabet excludes
 *  A/E/I/L/O/S/U/Z to avoid spelling words and confusable glyphs; '0' is
 *  included even though it isn't part of the encoding alphabet because the
 *  OLC spec uses it as a padding character in coarse codes (e.g.
 *  '8FVC0000+') — those must reach olc.isValid()/the padding check below to
 *  be reported as "too coarse" rather than falling through as "invalid". */
export const PLUSCODE_RE = /^[023456789CFGHJMPQRVWX]{2,8}\+[023456789CFGHJMPQRVWX]{0,7}$/;

/** MGRS/USNG: zone 1-60, latitude band C-X excluding I and O, two 100 km
 *  square letters (also excluding I and O), then an EVEN number of digits,
 *  4-10 of them (1 km .. 1 m). The trailing-digits requirement is what stops
 *  '18T Trailhead' and '3 Mile Rd' from matching. */
export const MGRS_RE =
	/^([1-9]|[1-5][0-9]|60)([C-HJ-NP-X])([A-HJ-NP-Z][A-HJ-NP-Z])(\d{4}|\d{6}|\d{8}|\d{10})$/;

/**
 * What format is this text, if any? Precedence is fixed and total:
 *
 *   1. w3w      — 'a.b.c' shape. Checked FIRST (issue #93 owns it) and it is
 *                 mutually exclusive with both others anyway: a w3w address
 *                 has dots and no '+' and cannot start with digits+band.
 *   2. pluscode — contains '+' and matches PLUSCODE_RE. Nothing else in a
 *                 mission location field contains a '+'.
 *   3. mgrs     — MGRS_RE. Last because its shape is the loosest.
 *
 * Returns null for ordinary location names, which is the common case and
 * must stay the common case: a false positive silently teleports the draft
 * pin, which is worse than no detection at all.
 */
export function detectFormat(input: string): GeocodeFormat | null {
	const raw = input.trim();
	if (!raw) return null;
	if (isFullAddress(raw) || looksLikePartial(raw)) return 'w3w';
	const n = normalizeGeocode(raw);
	if (n.includes('+') && PLUSCODE_RE.test(n)) return 'pluscode';
	if (MGRS_RE.test(n)) return 'mgrs';
	return null;
}

// --- Plus Code -----------------------------------------------------------

/** Longest cell side in metres, from the code length. Latitude-independent
 *  approximation using 1 deg lat = 111320 m — good enough for a "±14 m" hint. */
export function plusCodePrecisionM(codeLength: number): number {
	switch (codeLength) {
		case 8:
			return 275; // 0.0025 deg
		case 10:
			return 14; // 0.000125 deg
		case 11:
			return 3;
		default:
			return codeLength >= 12 ? 1 : 14000;
	}
}

export function parsePlusCode(input: string, ref: RefPoint | null): GeocodeParse {
	const code = normalizeGeocode(input);
	if (!PLUSCODE_RE.test(code)) return { error: 'invalid' };
	if (!olc.isValid(code)) return { error: 'invalid' };

	const plusIdx = code.indexOf('+');
	const beforePlus = code.slice(0, plusIdx);

	// Padded codes (e.g. '8FVC0000+', '8FVC00+') — too coarse to place a pin.
	if (beforePlus.includes('0')) return { error: 'too_coarse' };

	let full = code;
	let recoveredFrom: string | undefined;
	if (olc.isShort(code)) {
		// Reject a code with fewer than 2 significant chars before the '+'
		// (e.g. '+6X') — nothing left to recover against.
		if (beforePlus.length < 2) return { error: 'invalid' };
		// Reject a short code with NOTHING after the '+' either (e.g. 'CP+',
		// 'HQ+', 'RP+', 'WX+'). Structurally this is the sparsest possible
		// short code — 2 significant chars drawn from a 21-symbol alphabet —
		// and it collides constantly with ordinary 2-letter abbreviations
		// that happen to be followed by a stray '+'. Every short code this
		// module is meant to recover (see '6H+59' in the test vectors)
		// carries at least one extra character of precision after the '+';
		// one that doesn't is treated as not a Plus Code at all, rather than
		// as one that merely needs a reference point to resolve.
		if (code.endsWith('+')) return { error: 'invalid' };
		if (!ref) return { error: 'needs_reference' };
		full = olc.recoverNearest(code, ref.lat, ref.lon);
		recoveredFrom = full;
	}

	let area: ReturnType<OpenLocationCode['decode']>;
	try {
		area = olc.decode(full);
	} catch {
		return { error: 'invalid' };
	}
	if (area.codeLength < 8) return { error: 'too_coarse' };

	return {
		lat: area.latitudeCenter,
		lon: area.longitudeCenter,
		label: full,
		precisionM: plusCodePrecisionM(area.codeLength),
		format: 'pluscode',
		...(recoveredFrom ? { recoveredFrom } : {}),
	};
}

// --- MGRS ------------------------------------------------------------------

export function parseMGRS(input: string): GeocodeParse {
	const code = normalizeGeocode(input);
	const m = MGRS_RE.exec(code);
	if (!m) return { error: 'invalid' };

	const digits = m[4];
	const perAxis = digits.length / 2;

	let point: [number, number];
	try {
		point = toPoint(code);
	} catch {
		return { error: 'invalid' };
	}
	const [lon, lat] = point;
	if (!Number.isFinite(lat) || !Number.isFinite(lon) || Math.abs(lat) > 90 || Math.abs(lon) > 180) {
		return { error: 'invalid' };
	}

	return {
		lat,
		lon,
		label: code,
		precisionM: 10 ** (5 - perAxis),
		format: 'mgrs',
	};
}

/** '16SEG1234567890' -> '16S EG 12345 67890' (readable, radio-friendly). */
export function formatMGRSGroups(code: string): string {
	const m = /^(\d{1,2})([A-Z])([A-Z]{2})(\d+)$/.exec(code);
	if (!m) return code;
	const [, zone, band, square, digits] = m;
	const half = digits.length / 2;
	const easting = digits.slice(0, half);
	const northing = digits.slice(half);
	return `${zone}${band} ${square} ${easting} ${northing}`;
}

// --- Forward formatting (for the reverse row) -----------------------------

/** Full 10-char Plus Code, plus the short form relative to `ref` when the
 *  library can produce one (it returns the full code unchanged when the
 *  reference is too far away — both fall back to the full code). */
export function formatPlusCode(
	lat: number,
	lon: number,
	ref?: RefPoint | null
): { full: string; short: string } {
	const key = `${lat.toFixed(6)},${lon.toFixed(6)},pc,${ref ? `${ref.lat.toFixed(6)},${ref.lon.toFixed(6)}` : ''}`;
	const cached = formatCache.get(key);
	if (cached) return cached as { full: string; short: string };

	const full = olc.encode(lat, lon, 10);
	let short = full;
	if (ref) {
		try {
			short = olc.shorten(full, ref.lat, ref.lon) || full;
		} catch {
			short = full;
		}
	}
	const result = { full, short };
	putFormatCache(key, result);
	return result;
}

/** MGRS at `precisionDigits` per axis (default 4 = 10 m, per #94). */
export function formatMGRS(lat: number, lon: number, precisionDigits = 4): string {
	const key = `${lat.toFixed(6)},${lon.toFixed(6)},mg,${precisionDigits}`;
	const cached = formatCache.get(key);
	if (cached !== undefined) return cached as string;
	let result = '';
	try {
		result = forward([lon, lat], precisionDigits);
	} catch {
		result = '';
	}
	putFormatCache(key, result);
	return result;
}

// Module-level memo cache shared by both forward formatters above — these are
// ~µs computations, so this exists only so the reverse row re-renders during
// a map pan without recomputing, matching w3w.ts's reverse-cache convention.
const FORMAT_CACHE_MAX = 256;
const formatCache = new Map<string, { full: string; short: string } | string>();

function putFormatCache(key: string, value: { full: string; short: string } | string): void {
	if (!formatCache.has(key) && formatCache.size >= FORMAT_CACHE_MAX) {
		const oldestKey = formatCache.keys().next().value;
		if (oldestKey !== undefined) formatCache.delete(oldestKey);
	}
	formatCache.set(key, value);
}

// --- Round-trip helpers for stored labels ---------------------------------

/** '<CODE> · near <place>' — identical separator to w3w's ' · near '. */
export function geocodeLocationLabel(code: string, nearestPlace?: string): string {
	return nearestPlace ? `${code} · near ${nearestPlace}` : code;
}

/** Pull a stored geocode label back apart, or null for anything else. */
export function extractGeocode(
	location: string
): { code: string; format: 'pluscode' | 'mgrs' } | null {
	const trimmed = location.trim();
	if (!trimmed) return null;
	const marker = ' · near ';
	const idx = trimmed.indexOf(marker);
	const head = (idx === -1 ? trimmed : trimmed.slice(0, idx)).trim();
	if (!head) return null;
	const format = detectFormat(head);
	if (format !== 'pluscode' && format !== 'mgrs') return null;
	return { code: normalizeGeocode(head), format };
}
