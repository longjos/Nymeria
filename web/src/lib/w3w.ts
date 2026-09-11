/**
 * what3words — pure, dependency-free helpers shared by the mission location
 * combobox, the mission card, and the Settings panel.
 *
 * Validation rules here MUST stay identical to internal/geocode/w3w/words.go
 * (Go) — both sides are built against the same test-vector table so
 * client-side gating and server-side rejection can never disagree. See
 * w3w.test.ts.
 */

// Full address, once normalizeWords has already stripped any leading
// slashes — these patterns never need to (and never can) match one.
export const W3W_FULL = /^[\p{L}'’]+\.[\p{L}'’]+\.[\p{L}'’]+$/u;
// Partial: at least "a.b" typed, third component may be empty or missing.
export const W3W_PARTIAL = /^[\p{L}'’]+\.[\p{L}'’]*(\.[\p{L}'’]*)?$/u;

/**
 * Lowercase, trim, strip every leading slash (not just up to three — a few
 * fat-fingered extra slashes still resolve), normalize 。/・ separators.
 */
export function normalizeWords(s: string): string {
	return s
		.trim()
		.toLowerCase()
		.replace(/^\/+/, '')
		.replace(/[。・]/g, '.');
}

/** Syntactically complete three-word address. */
export function isFullAddress(s: string): boolean {
	return W3W_FULL.test(normalizeWords(s));
}

/** On its way to being one — worth an autosuggest call, not yet resolvable. */
export function looksLikePartial(s: string): boolean {
	const n = normalizeWords(s);
	if (W3W_FULL.test(n)) return false;
	return W3W_PARTIAL.test(n);
}

/** Display form: always "///filled.count.soap". */
export function formatWords(words: string): string {
	return '///' + normalizeWords(words);
}

/** "///a.b.c · near X"  (near part omitted when place is empty). */
export function w3wLocationLabel(words: string, nearestPlace?: string): string {
	const base = formatWords(words);
	return nearestPlace ? `${base} · near ${nearestPlace}` : base;
}

/** True when a stored mission location string came from what3words. */
export function isW3WLocation(location: string): boolean {
	return /^\/\/\//.test(location.trim());
}

/** Pull just the address back out of a stored location string, or null. */
export function extractWords(location: string): string | null {
	const trimmed = location.trim();
	if (!isW3WLocation(trimmed)) return null;
	const withoutSlashes = trimmed.replace(/^\/+/, '');
	const sepIdx = withoutSlashes.indexOf(' · ');
	const words = (sepIdx === -1 ? withoutSlashes : withoutSlashes.slice(0, sepIdx)).trim();
	return words || null;
}

/** Pull the "near X" fragment (without the leading "· near ") out of a
 * stored location string, or null when there isn't one. */
export function w3wSuffix(location: string): string | null {
	const trimmed = location.trim();
	if (!isW3WLocation(trimmed)) return null;
	const marker = ' · near ';
	const idx = trimmed.indexOf(marker);
	if (idx === -1) return null;
	const suffix = trimmed.slice(idx + marker.length).trim();
	return suffix || null;
}

// --- Reverse cache -------------------------------------------------------
// Module-level so the mission list, the location chip, and any future
// consumer share hits. The what3words grid never changes, so there is no
// invalidation — only a size cap.

const REVERSE_CACHE_MAX = 256;
const reverseCache = new Map<string, string>();

function reverseKey(lat: number, lon: number): string {
	return `${lat.toFixed(6)},${lon.toFixed(6)}`;
}

/** Module-level reverse cache: key = lat/lon rounded to 6dp. */
export function cachedReverse(lat: number, lon: number): string | undefined {
	return reverseCache.get(reverseKey(lat, lon));
}

export function putReverse(lat: number, lon: number, words: string): void {
	const key = reverseKey(lat, lon);
	if (!reverseCache.has(key) && reverseCache.size >= REVERSE_CACHE_MAX) {
		// Evict the oldest-inserted entry (Map preserves insertion order).
		const oldestKey = reverseCache.keys().next().value;
		if (oldestKey !== undefined) reverseCache.delete(oldestKey);
	}
	reverseCache.set(key, words);
}
