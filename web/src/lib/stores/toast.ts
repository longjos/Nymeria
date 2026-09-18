import { writable } from 'svelte/store';
import type { WxTier } from '$lib/types';

export interface ToastAction {
	label: string;
	run: () => void;
}

export interface ToastMessage {
	id: string;
	message: string;
	type: 'success' | 'error' | 'info' | 'wx';
	duration: number;
	action?: ToastAction;
	/** Only meaningful when type === 'wx' — picks the tier color/glyph. */
	tier?: WxTier;
}

export const toasts = writable<ToastMessage[]>([]);

/**
 * Text most recently pushed into the hidden `aria-live="polite"` region that
 * `Toast.svelte` renders alongside the toast stack. Used for announcements
 * that have no visible toast of their own (e.g. an NWS link state
 * transition) — screen readers hear it without a toast card appearing.
 */
export const liveAnnouncement = writable<string>('');

let announceSeq = 0;

/** Announce a sentence to screen readers via the shared polite live region. */
export function announce(text: string): void {
	// Re-set even for a repeated sentence: append a zero-width, invisible
	// sequence marker so the DOM text actually changes and the region fires
	// again (some screen readers do not re-announce unchanged text).
	announceSeq += 1;
	liveAnnouncement.set(`${text}${'​'.repeat(announceSeq % 2)}`);
}

let nextId = 0;

interface Timer {
	handle: ReturnType<typeof setTimeout>;
	/** Epoch ms at which the toast is due to dismiss. */
	dueAt: number;
	/** Milliseconds left when paused; null while running. */
	remaining: number | null;
}

const timers = new Map<string, Timer>();

function arm(id: string, ms: number): void {
	timers.set(id, {
		handle: setTimeout(() => dismissToast(id), ms),
		dueAt: Date.now() + ms,
		remaining: null,
	});
}

/**
 * Show a transient notification.
 *
 * An `action` renders a button inside the toast (for example Undo). Because an
 * action must stay clickable, hovering or focusing the toast pauses its dismiss
 * timer — see `pauseToast` / `resumeToast`.
 */
export function showToast(
	message: string,
	type: 'success' | 'error' | 'info' | 'wx' = 'info',
	duration = 3000,
	action?: ToastAction,
	tier?: WxTier
): void {
	const id = String(++nextId);
	toasts.update(t => [...t, { id, message, type, duration, action, tier }]);
	arm(id, duration);
}

export function dismissToast(id: string): void {
	const timer = timers.get(id);
	if (timer) {
		clearTimeout(timer.handle);
		timers.delete(id);
	}
	toasts.update(t => t.filter(toast => toast.id !== id));
}

/** Freeze a toast's dismiss countdown (pointer or keyboard focus is on it). */
export function pauseToast(id: string): void {
	const timer = timers.get(id);
	if (!timer || timer.remaining !== null) return;
	clearTimeout(timer.handle);
	timer.remaining = Math.max(0, timer.dueAt - Date.now());
}

/** Resume a paused countdown from where it stopped. */
export function resumeToast(id: string): void {
	const timer = timers.get(id);
	if (!timer || timer.remaining === null) return;
	arm(id, timer.remaining);
}
