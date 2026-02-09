import type { AnnotationCategory, AnnotationPriority } from './types';

export interface CategoryMeta {
	label: string;
	icon: string; // SVG path for 16x16 viewBox
	allowedGeometry: ('point' | 'line' | 'area')[];
	defaultColor: string;
}

export interface StatusMeta {
	label: string;
	color: string;
}

export interface PriorityMeta {
	label: string;
	color: string;
	pulse: boolean;
}

export const categoryMeta: Record<AnnotationCategory, CategoryMeta> = {
	incident: {
		label: 'Incident',
		icon: 'M8 1.5l6.5 13H1.5L8 1.5zM8 6v3M8 11h.01',
		allowedGeometry: ['point'],
		defaultColor: '#e63946',
	},
	resource: {
		label: 'Resource',
		icon: 'M8 2a6 6 0 100 12A6 6 0 008 2zm0 3v6M5 8h6',
		allowedGeometry: ['point'],
		defaultColor: '#457b9d',
	},
	checkpoint: {
		label: 'Checkpoint',
		icon: 'M4 2v12M4 3h7l-2 3 2 3H4',
		allowedGeometry: ['point'],
		defaultColor: '#2a9d8f',
	},
	hazard: {
		label: 'Hazard',
		icon: 'M8 1.5l6.5 13H1.5L8 1.5zM8 6v3M8 11h.01',
		allowedGeometry: ['point', 'area'],
		defaultColor: '#f4a261',
	},
	route: {
		label: 'Route',
		icon: 'M2 14L7 6l3 4 4-8',
		allowedGeometry: ['line'],
		defaultColor: '#264653',
	},
	boundary: {
		label: 'Boundary',
		icon: 'M3 3h10v10H3zM1 1h2M13 1h2M1 13h2M13 13h2',
		allowedGeometry: ['area'],
		defaultColor: '#a8dadc',
	},
	assignment: {
		label: 'Assignment',
		icon: 'M4 2h8v12H4zM7 5h2M7 8h2M7 11h2',
		allowedGeometry: ['point', 'area'],
		defaultColor: '#e9c46a',
	},
	general: {
		label: 'General',
		icon: 'M8 1a5 5 0 00-5 5c0 4 5 9 5 9s5-5 5-9a5 5 0 00-5-5zm0 7a2 2 0 110-4 2 2 0 010 4z',
		allowedGeometry: ['point', 'line', 'area'],
		defaultColor: '#e63946',
	},
};

export const statusMeta: Record<AnnotationCategory, StatusMeta[]> = {
	incident: [
		{ label: 'Reported', color: '#e63946' },
		{ label: 'Responding', color: '#f4a261' },
		{ label: 'On-Scene', color: '#e9c46a' },
		{ label: 'Resolved', color: '#2a9d8f' },
		{ label: 'Escalated', color: '#d62828' },
	],
	resource: [
		{ label: 'Planned', color: '#a8dadc' },
		{ label: 'Open', color: '#457b9d' },
		{ label: 'Active', color: '#2a9d8f' },
		{ label: 'At Capacity', color: '#f4a261' },
		{ label: 'Closing', color: '#e9c46a' },
		{ label: 'Closed', color: '#6c757d' },
	],
	checkpoint: [
		{ label: 'Planned', color: '#a8dadc' },
		{ label: 'Open', color: '#2a9d8f' },
		{ label: 'Closed', color: '#6c757d' },
	],
	hazard: [
		{ label: 'Reported', color: '#e63946' },
		{ label: 'Confirmed', color: '#f4a261' },
		{ label: 'Mitigated', color: '#e9c46a' },
		{ label: 'Cleared', color: '#2a9d8f' },
	],
	route: [
		{ label: 'Planned', color: '#a8dadc' },
		{ label: 'Active', color: '#2a9d8f' },
		{ label: 'Closed', color: '#6c757d' },
	],
	boundary: [
		{ label: 'Planned', color: '#a8dadc' },
		{ label: 'Active', color: '#2a9d8f' },
		{ label: 'Complete', color: '#457b9d' },
		{ label: 'Needs Re-search', color: '#f4a261' },
	],
	assignment: [
		{ label: 'Planned', color: '#a8dadc' },
		{ label: 'Assigned', color: '#457b9d' },
		{ label: 'In Progress', color: '#e9c46a' },
		{ label: 'Complete', color: '#2a9d8f' },
		{ label: 'Incomplete', color: '#e63946' },
	],
	general: [
		{ label: 'Active', color: '#2a9d8f' },
		{ label: 'Resolved', color: '#6c757d' },
	],
};

/** Map status label back to the API status value (lowercase, hyphenated). */
export function statusLabelToValue(label: string): string {
	return label.toLowerCase().replace(/ /g, '-');
}

/** Map API status value to display label. */
export function statusValueToLabel(category: AnnotationCategory, value: string): string {
	const metas = statusMeta[category] || statusMeta.general;
	const found = metas.find((m) => statusLabelToValue(m.label) === value);
	return found?.label || value;
}

/** Get the color for a status value. */
export function statusColor(category: AnnotationCategory, value: string): string {
	const metas = statusMeta[category] || statusMeta.general;
	const found = metas.find((m) => statusLabelToValue(m.label) === value);
	return found?.color || '#6c757d';
}

export const priorityMeta: Record<AnnotationPriority, PriorityMeta> = {
	routine: { label: 'Routine', color: '#6c757d', pulse: false },
	priority: { label: 'Priority', color: '#f4a261', pulse: false },
	urgent: { label: 'Urgent', color: '#e63946', pulse: false },
	emergency: { label: 'Emergency', color: '#d62828', pulse: true },
};

/** All valid categories in display order. */
export const allCategories: AnnotationCategory[] = [
	'incident', 'resource', 'checkpoint', 'hazard', 'route', 'boundary', 'assignment', 'general',
];

/** Whether a status is terminal (annotation considered done). */
export function isTerminalStatus(status: string): boolean {
	return ['resolved', 'closed', 'cleared', 'complete', 'escalated'].includes(status);
}
