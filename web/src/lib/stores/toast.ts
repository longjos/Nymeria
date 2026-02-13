import { writable } from 'svelte/store';

export interface ToastMessage {
	id: string;
	message: string;
	type: 'success' | 'error' | 'info';
	duration: number;
}

export const toasts = writable<ToastMessage[]>([]);

let nextId = 0;

export function showToast(message: string, type: 'success' | 'error' | 'info' = 'info', duration = 3000): void {
	const id = String(++nextId);
	toasts.update(t => [...t, { id, message, type, duration }]);
	setTimeout(() => dismissToast(id), duration);
}

export function dismissToast(id: string): void {
	toasts.update(t => t.filter(toast => toast.id !== id));
}
