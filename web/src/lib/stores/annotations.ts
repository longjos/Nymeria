import { writable, derived } from 'svelte/store';
import type { Annotation, AnnotationCategory, Operation } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';

export const annotations = writable<Map<string, Annotation>>(new Map());
export const operations = writable<Operation[]>([]);
export const activeOperationId = writable<string>('');
export const annotationList = derived(annotations, ($annotations) =>
	Array.from($annotations.values()).sort(
		(a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
	)
);

/** Annotations grouped by category. */
export const annotationsByCategory = derived(annotations, ($annotations) => {
	const groups = new Map<AnnotationCategory, Annotation[]>();
	for (const ann of $annotations.values()) {
		const cat = ann.category || 'general';
		if (!groups.has(cat)) groups.set(cat, []);
		groups.get(cat)!.push(ann);
	}
	return groups;
});

let initialized = false;

export function initAnnotationStore(): void {
	if (initialized) return;
	initialized = true;

	api.annotations().then((list) => {
		annotations.set(new Map(list.map((a) => [a.id, a])));
	}).catch(() => {});

	api.operations().then((list) => {
		operations.set(list || []);
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

	wsClient.on('annotation_status_changed', (msg) => {
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
