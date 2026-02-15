import type { NetCheckIn, OperatorStatus, MissionStatus } from './types';

/** Plain-language labels for operator statuses (no radio jargon). */
export const statusLabels: Record<OperatorStatus, string> = {
	available: 'Available',
	assigned:  'Assigned',
	enroute:   'Responding',
	onscene:   'On Scene',
	brb:       'Temporarily Unavailable',
	missing:   'UNREACHABLE',
	released:  'Off Duty',
};

/** Plain-language labels for traffic priority levels. */
export const priorityLabels: Record<string, string> = {
	emergency: 'CRITICAL',
	priority:  'URGENT',
	welfare:   'Welfare Check',
	routine:   'Routine',
};

/** Plain-language labels for mission statuses. */
export const missionStatusLabels: Record<MissionStatus, string> = {
	open:     'Reported',
	active:   'In Progress',
	complete: 'Resolved',
};

/** Returns a human-friendly name for a check-in (prefer tactical call > operator name > callsign). */
export function humanName(ci: NetCheckIn): string {
	return ci.tacticalCall || ci.operatorName || ci.callsign;
}
