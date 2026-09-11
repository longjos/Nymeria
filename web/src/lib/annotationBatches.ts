import type { Annotation, AnnotationBatch } from '$lib/types';
import { timeAgo } from '$lib/utils';

/**
 * Group annotations into imported sets by their `batchId` stamp.
 *
 * Sets are derived, never stored: an annotation without a `batchId` (everything
 * created by hand, and everything that predates schema v22) is simply skipped,
 * so a group can never drift out of sync with its members — delete rows one at
 * a time and the count shrinks until the group disappears on its own.
 *
 * Returned newest-first by the earliest `createdAt` in each set, matching the
 * annotation list's own createdAt-descending order.
 */
export function groupByBatch(anns: Annotation[]): AnnotationBatch[] {
	const byId = new Map<string, AnnotationBatch>();

	for (const ann of anns) {
		const id = ann.batchId;
		if (!id) continue;

		let batch = byId.get(id);
		if (!batch) {
			batch = {
				id,
				label: ann.batchLabel || 'Imported set',
				netId: ann.netId,
				count: 0,
				createdAt: ann.createdAt,
				items: [],
				missionLinkedCount: 0,
				checkpointCount: 0,
			};
			byId.set(id, batch);
		}

		batch.items.push(ann);
		batch.count = batch.items.length;
		// A rename rewrites every member, but a stale member can arrive mid-flight;
		// the last non-empty label wins so the header never blanks out.
		if (ann.batchLabel) batch.label = ann.batchLabel;
		if (ann.missionIds?.length) batch.missionLinkedCount++;
		if (ann.category === 'checkpoint') batch.checkpointCount++;
		if (ann.createdAt && ann.createdAt < batch.createdAt) batch.createdAt = ann.createdAt;
	}

	return Array.from(byId.values()).sort(
		(a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
	);
}

export { timeAgo };
