// Shared interrupt-banner plumbing (spec: docs/ride-strip-spec.md §4/§0).
// The premise: the strip/panel is the Notify tier, never the interrupt — the
// interrupt channel is a SINGLE full-width banner above the map, and there
// must be exactly one in the app at any time. wxAlerts.ts keeps its own
// queue (wxInterruptQueue); this file adds the ride-side queue and the
// arbitration between the two, plus the one shared "how loud can the app be
// right now" budget InterruptBanner.svelte enforces.
import { writable, derived } from 'svelte/store';
import type { WxTier, PriorityTier } from '$lib/types';
import { wxInterruptQueue } from './wxAlerts';

export type InterruptGlyphDescriptor = { kind: 'wx'; tier: WxTier } | { kind: 'ride'; tier: PriorityTier };

export interface InterruptItem {
	id: string;
	source: 'wx' | 'ride';
	title: string;
	sub: string;
	body: string;
	where: string;
	endsOrAt?: string;
	instruction?: string;
	footer: string;
	borderVar: string;
	hatched: boolean;
	glyph: InterruptGlyphDescriptor;
	/** 60_000 for ride rank-1 — re-chimes until acknowledged. */
	repeatEveryMs?: number;
	ackTextDark?: boolean;
	showOnMap?: () => void;
	details?: () => void;
	ackForNet?: () => Promise<void>;
}

/** Ride record ids (medical/sag/supply/checkin) awaiting the operator, FIFO. */
export const rideInterruptQueue = writable<string[]>([]);

/** EMERGENCY (ride) always wins arbitration over a weather interrupt. */
export const activeInterruptSource = derived([rideInterruptQueue, wxInterruptQueue], ([r, w]) =>
	r.length ? 'ride' : w.length ? 'wx' : null
);

/** Sound budget: at most one audible event per 30s app-wide, enforced by InterruptBanner.playSignal(). */
export const lastAudibleAt = writable<number>(0);

export const SOUND_BUDGET_MS = 30_000;
