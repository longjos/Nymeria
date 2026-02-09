import { writable, derived } from 'svelte/store';
import type { Annotation } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';

export const annotations = writable<Map<string, Annotation>>(new Map());
export const annotationList = derived(annotations, ($annotations) =>
	Array.from($annotations.values()).sort(
		(a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
	)
);

let initialized = false;

export function initAnnotationStore(): void {
	if (initialized) return;
	initialized = true;

	api.annotations().then((list) => {
		annotations.set(new Map(list.map((a) => [a.id, a])));
	}).catch(() => {});

	wsClient.on('annotation_created', (msg) => {
		const a = msg.data as Annotation;
		if (!a) return;
		annotations.update((m) => {
			m.set(a.id, a);
			return new Map(m);
		});
	});

	wsClient.on('annotation_updated', (msg) => {
		const a = msg.data as Annotation;
		if (!a) return;
		annotations.update((m) => {
			m.set(a.id, a);
			return new Map(m);
		});
	});

	wsClient.on('annotation_deleted', (msg) => {
		const a = msg.data as Annotation;
		if (!a) return;
		annotations.update((m) => {
			m.delete(a.id);
			return new Map(m);
		});
	});
}
