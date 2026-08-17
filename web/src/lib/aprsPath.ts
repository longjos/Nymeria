export interface PathPreset {
	id: string;
	label: string;
	shortLabel: string;
	value: string;
	hint: string;
}

/**
 * New-N paradigm presets (aprs.org / WA8LMF).
 * Stored value is the TNC2 string.
 */
export const APRS_PATH_PRESETS: PathPreset[] = [
	{ id: 'direct', label: 'Direct (no path)', shortLabel: 'DIRECT', value: '', hint: 'Simplex or airborne — no digipeaters' },
	{ id: 'local', label: 'Fill-in — WIDE1-1', shortLabel: 'WIDE1-1', value: 'WIDE1-1', hint: 'Home fill-in digipeaters only' },
	{ id: 'standard', label: 'Mobile — WIDE1-1,WIDE2-1', shortLabel: 'WIDE1-1,WIDE2-1', value: 'WIDE1-1,WIDE2-1', hint: 'Recommended default: fill-in plus one wide hop' },
	{ id: 'fixed', label: 'Fixed — WIDE2-1', shortLabel: 'WIDE2-1', value: 'WIDE2-1', hint: 'One wide hop; skip fill-in (typical home station)' },
	{ id: 'rural', label: 'Rural — WIDE1-1,WIDE2-2', shortLabel: 'WIDE1-1,WIDE2-2', value: 'WIDE1-1,WIDE2-2', hint: 'Three hops for sparse coverage — avoid in cities' },
	{ id: 'mountain', label: 'Mountain — WIDE2-2', shortLabel: 'WIDE2-2', value: 'WIDE2-2', hint: 'Two wide hops; western / high-level digis (SoCal)' },
	{ id: 'internet', label: 'Internet — TCPIP*', shortLabel: 'TCPIP*', value: 'TCPIP*', hint: 'APRS-IS only; will not be relayed on RF' }
];

export function normalizePath(s: string): string {
	return s
		.split(',')
		.map((p) => p.trim().toUpperCase())
		.filter(Boolean)
		.join(',');
}

export function presetIdFor(value: string): string {
	const n = normalizePath(value);
	const match = APRS_PATH_PRESETS.find((p) => normalizePath(p.value) === n);
	return match ? match.id : 'custom';
}

/** Compact label for send UIs: DIRECT, or the TNC2 path string. */
export function formatPathDisplay(path: string | undefined | null): string {
	const n = normalizePath(path ?? '');
	return n === '' ? 'DIRECT' : n;
}
