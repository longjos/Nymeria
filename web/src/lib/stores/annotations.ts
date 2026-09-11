import { writable, derived } from 'svelte/store';
import type { Annotation, AnnotationCategory, Operation } from '$lib/types';
import { api } from '$lib/api';
import { groupByBatch } from '$lib/annotationBatches';
import { wsClient } from './stations';

export const annotations = writable<Map<string, Annotation>>(new Map());
export const operations = writable<Operation[]>([]);
export const activeOperationId = writable<string>('');
export const annotationList = derived(annotations, ($annotations) =>
	Array.from($annotations.values()).sort(
		(a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
	)
);

/**
 * Imported sets derived from the `batchId` stamp, newest first.
 * Ungrouped annotations (hand-created, or created before schema v22) are absent.
 */
export const annotationBatches = derived(annotationList, ($list) => groupByBatch($list));

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

	// Aggregate frames: one message per bulk operation instead of one per item.
	// An import of 50 waypoints is a single store mutation and a single render.
	wsClient.on('annotations_imported', (msg) => {
		const data = msg.data as { annotations?: Annotation[] } | undefined;
		const list = data?.annotations;
		if (!list?.length) return;
		annotations.update((m) => {
			// Upsert, so an undo-restore replaying the same ids is idempotent.
			for (const a of list) m.set(a.id, a);
			return new Map(m);
		});
	});

	wsClient.on('annotations_batch_deleted', (msg) => {
		const data = msg.data as { ids?: string[] } | undefined;
		const ids = data?.ids;
		if (!ids?.length) return;
		annotations.update((m) => {
			for (const id of ids) m.delete(id);
			return new Map(m);
		});
	});

	wsClient.on('annotations_batch_updated', (msg) => {
		const data = msg.data as { batchId?: string; batchLabel?: string } | undefined;
		if (!data?.batchId) return;
		const { batchId, batchLabel } = data;
		annotations.update((m) => {
			for (const [id, a] of m) {
				if (a.batchId === batchId) m.set(id, { ...a, batchLabel: batchLabel ?? '' });
			}
			return new Map(m);
		});
	});
}
