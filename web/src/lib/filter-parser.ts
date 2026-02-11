import type { FilterRule, FilterType } from './types';

/** Parse an APRS-IS filter string into structured rules. */
export function parseFilter(raw: string): FilterRule[] {
	if (!raw || !raw.trim()) return [];

	const rules: FilterRule[] = [];
	const tokens = raw.trim().split(/\s+/);

	for (const token of tokens) {
		if (!token) continue;

		const exclude = token.startsWith('-');
		const clean = exclude ? token.slice(1) : token;

		// Split on '/' but keep the prefix
		const slashIdx = clean.indexOf('/');
		if (slashIdx < 0) continue; // malformed

		const prefix = clean.slice(0, slashIdx);
		const rest = clean.slice(slashIdx + 1);
		const parts = rest.split('/');

		const rule = parseToken(prefix, parts, exclude);
		if (rule) {
			rules.push(rule);
		}
	}

	return rules;
}

function parseToken(prefix: string, parts: string[], exclude: boolean): FilterRule | null {
	switch (prefix) {
		case 'r':
			if (parts.length < 3) return null;
			return {
				type: 'range',
				exclude,
				lat: parseFloat(parts[0]),
				lon: parseFloat(parts[1]),
				dist: parseFloat(parts[2])
			};

		case 'a':
			if (parts.length < 4) return null;
			return {
				type: 'area',
				exclude,
				latN: parseFloat(parts[0]),
				lonW: parseFloat(parts[1]),
				latS: parseFloat(parts[2]),
				lonE: parseFloat(parts[3])
			};

		case 't':
			if (parts.length < 1) return null;
			return {
				type: 'type',
				exclude,
				types: parts[0],
				callForType: parts.length >= 3 ? parts[1] : undefined,
				distForType: parts.length >= 3 ? parseFloat(parts[2]) : undefined
			};

		case 'p':
			return {
				type: 'prefix',
				exclude,
				items: parts.filter(p => p.length > 0)
			};

		case 'b':
			return {
				type: 'budlist',
				exclude,
				items: parts.filter(p => p.length > 0)
			};

		case 'o':
			return {
				type: 'object',
				exclude,
				items: parts.filter(p => p.length > 0)
			};

		case 'os':
			return {
				type: 'strictObject',
				exclude,
				items: parts.filter(p => p.length > 0)
			};

		case 's': {
			return {
				type: 'symbol',
				exclude,
				primaryTable: parts[0] || '',
				altTable: parts[1] || '',
				overlay: parts[2] || ''
			};
		}

		case 'd':
			return {
				type: 'digipeater',
				exclude,
				items: parts.filter(p => p.length > 0)
			};

		case 'e':
			return {
				type: 'entry',
				exclude,
				items: parts.filter(p => p.length > 0)
			};

		case 'g':
			return {
				type: 'group',
				exclude,
				items: parts.filter(p => p.length > 0)
			};

		case 'u':
			return {
				type: 'unproto',
				exclude,
				items: parts.filter(p => p.length > 0)
			};

		case 'q': {
			return {
				type: 'qConstruct',
				exclude,
				qCodes: parts[0] || '',
				iFlag: parts[1] === 'I'
			};
		}

		case 'm':
			if (parts.length < 1) return null;
			return {
				type: 'myRange',
				exclude,
				dist: parseFloat(parts[0])
			};

		case 'f':
			if (parts.length < 2) return null;
			return {
				type: 'friendRange',
				exclude,
				friendCall: parts[0],
				dist: parseFloat(parts[1])
			};

		default:
			return null;
	}
}

/** Serialize structured rules back to an APRS-IS filter string.
 *  The os/ (strict object) filter must be last per APRS-IS spec. */
export function serializeFilter(rules: FilterRule[]): string {
	// Sort: non-strictObject first, strictObject last
	const sorted = [...rules].sort((a, b) => {
		if (a.type === 'strictObject' && b.type !== 'strictObject') return 1;
		if (a.type !== 'strictObject' && b.type === 'strictObject') return -1;
		return 0;
	});
	return sorted.map(serializeRule).filter(Boolean).join(' ');
}

function serializeRule(rule: FilterRule): string {
	const pre = rule.exclude ? '-' : '';

	switch (rule.type) {
		case 'range':
			return `${pre}r/${fmtNum(rule.lat)}/${fmtNum(rule.lon)}/${fmtNum(rule.dist)}`;

		case 'area':
			return `${pre}a/${fmtNum(rule.latN)}/${fmtNum(rule.lonW)}/${fmtNum(rule.latS)}/${fmtNum(rule.lonE)}`;

		case 'type':
			if (rule.callForType && rule.distForType != null) {
				return `${pre}t/${rule.types || ''}/${rule.callForType}/${fmtNum(rule.distForType)}`;
			}
			return `${pre}t/${rule.types || ''}`;

		case 'prefix':
			return `${pre}p/${(rule.items || []).join('/')}`;

		case 'budlist':
			return `${pre}b/${(rule.items || []).join('/')}`;

		case 'object':
			return `${pre}o/${(rule.items || []).join('/')}`;

		case 'strictObject':
			return `${pre}os/${(rule.items || []).join('/')}`;

		case 'symbol': {
			const parts = [rule.primaryTable || '', rule.altTable || '', rule.overlay || ''];
			// Trim trailing empty fields for cleaner output
			while (parts.length > 1 && parts[parts.length - 1] === '') parts.pop();
			return `${pre}s/${parts.join('/')}`;
		}

		case 'digipeater':
			return `${pre}d/${(rule.items || []).join('/')}`;

		case 'entry':
			return `${pre}e/${(rule.items || []).join('/')}`;

		case 'group':
			return `${pre}g/${(rule.items || []).join('/')}`;

		case 'unproto':
			return `${pre}u/${(rule.items || []).join('/')}`;

		case 'qConstruct': {
			const parts = [rule.qCodes || ''];
			if (rule.iFlag) parts.push('I');
			return `${pre}q/${parts.join('/')}`;
		}

		case 'myRange':
			return `${pre}m/${fmtNum(rule.dist)}`;

		case 'friendRange':
			return `${pre}f/${rule.friendCall || ''}/${fmtNum(rule.dist)}`;

		default:
			return '';
	}
}

function fmtNum(n: number | undefined): string {
	if (n == null || isNaN(n)) return '0';
	// Remove trailing zeros after decimal
	return parseFloat(n.toFixed(4)).toString();
}

/** Validate a filter rule. Returns error message or null if valid. */
export function validateRule(rule: FilterRule): string | null {
	switch (rule.type) {
		case 'range':
			if (rule.lat == null || rule.lon == null || rule.dist == null) return 'Latitude, longitude, and radius are required';
			if (rule.lat < -90 || rule.lat > 90) return 'Latitude must be between -90 and 90';
			if (rule.lon < -180 || rule.lon > 180) return 'Longitude must be between -180 and 180';
			if (rule.dist <= 0) return 'Radius must be greater than 0';
			return null;

		case 'area':
			if (rule.latN == null || rule.lonW == null || rule.latS == null || rule.lonE == null) return 'All bounds are required';
			if (rule.latN < -90 || rule.latN > 90 || rule.latS < -90 || rule.latS > 90) return 'Latitude must be between -90 and 90';
			if (rule.lonW < -180 || rule.lonW > 180 || rule.lonE < -180 || rule.lonE > 180) return 'Longitude must be between -180 and 180';
			if (rule.latN <= rule.latS) return 'North latitude must be greater than south latitude';
			if (rule.lonW >= rule.lonE) return 'West longitude must be less than east longitude';
			return null;

		case 'type':
			if (!rule.types || rule.types.length === 0) return 'Select at least one packet type';
			if (rule.callForType && (rule.distForType == null || rule.distForType <= 0)) return 'Distance is required when specifying a callsign';
			return null;

		case 'prefix':
		case 'budlist':
		case 'object':
		case 'strictObject':
		case 'digipeater':
		case 'entry':
		case 'group':
		case 'unproto':
			if (!rule.items || rule.items.length === 0) return 'At least one entry is required';
			return null;

		case 'symbol':
			if (!rule.primaryTable && !rule.altTable) return 'At least one symbol table entry is required';
			return null;

		case 'qConstruct':
			if ((!rule.qCodes || rule.qCodes.length === 0) && !rule.iFlag) return 'Select at least one q-construct type or enable IGate positions';
			return null;

		case 'myRange':
			if (rule.dist == null || rule.dist <= 0) return 'Distance must be greater than 0';
			return null;

		case 'friendRange':
			if (!rule.friendCall || rule.friendCall.trim().length === 0) return 'Friend callsign is required';
			if (rule.dist == null || rule.dist <= 0) return 'Distance must be greater than 0';
			return null;

		default:
			return null;
	}
}

/** Human-readable label for a filter type. */
export function filterTypeLabel(type: FilterType): string {
	const labels: Record<FilterType, string> = {
		range: 'Range (Circle)',
		area: 'Area (Box)',
		type: 'Packet Type',
		prefix: 'Callsign Prefix',
		budlist: 'Callsign List',
		object: 'Object Name',
		strictObject: 'Strict Object',
		symbol: 'Symbol',
		digipeater: 'Digipeater',
		entry: 'Entry Station',
		group: 'Group Message',
		unproto: 'Unproto/Dest',
		qConstruct: 'Q-Construct',
		myRange: 'My Range',
		friendRange: 'Friend Range'
	};
	return labels[type] || type;
}

/** Short human-readable summary of a filter rule. */
export function ruleSummary(rule: FilterRule): string {
	const pre = rule.exclude ? 'Exclude: ' : '';
	switch (rule.type) {
		case 'range':
			return `${pre}${fmtNum(rule.dist)}km around ${fmtNum(rule.lat)}, ${fmtNum(rule.lon)}`;
		case 'area':
			return `${pre}Box ${fmtNum(rule.latN)},${fmtNum(rule.lonW)} to ${fmtNum(rule.latS)},${fmtNum(rule.lonE)}`;
		case 'type': {
			const typeLabels: Record<string, string> = {
				p: 'Position', o: 'Object', i: 'Item', m: 'Message',
				q: 'Query', s: 'Status', t: 'Telemetry', u: 'User-defined',
				n: 'NWS', w: 'Weather'
			};
			const names = (rule.types || '').split('').map(c => typeLabels[c] || c).join(', ');
			if (rule.callForType) return `${pre}${names} near ${rule.callForType} (${fmtNum(rule.distForType)}km)`;
			return `${pre}${names}`;
		}
		case 'prefix':
			return `${pre}Prefix: ${(rule.items || []).join(', ')}`;
		case 'budlist':
			return `${pre}Calls: ${(rule.items || []).join(', ')}`;
		case 'object':
		case 'strictObject':
			return `${pre}Objects: ${(rule.items || []).join(', ')}`;
		case 'symbol':
			return `${pre}Symbol: pri=${rule.primaryTable || '-'} alt=${rule.altTable || '-'} over=${rule.overlay || '-'}`;
		case 'digipeater':
			return `${pre}Via: ${(rule.items || []).join(', ')}`;
		case 'entry':
			return `${pre}IGate: ${(rule.items || []).join(', ')}`;
		case 'group':
			return `${pre}To: ${(rule.items || []).join(', ')}`;
		case 'unproto':
			return `${pre}Dest: ${(rule.items || []).join(', ')}`;
		case 'qConstruct':
			return `${pre}Q: ${rule.qCodes || ''}${rule.iFlag ? ' +IGate' : ''}`;
		case 'myRange':
			return `${pre}${fmtNum(rule.dist)}km from my station`;
		case 'friendRange':
			return `${pre}${fmtNum(rule.dist)}km from ${rule.friendCall}`;
		default:
			return pre + rule.type;
	}
}

/** Create a new default filter rule for a given type. */
export function createDefaultRule(type: FilterType): FilterRule {
	const base: FilterRule = { type, exclude: false };
	switch (type) {
		case 'range':
			return { ...base, lat: 0, lon: 0, dist: 100 };
		case 'area':
			return { ...base, latN: 1, lonW: -1, latS: -1, lonE: 1 };
		case 'type':
			return { ...base, types: 'p' };
		case 'prefix':
		case 'budlist':
		case 'object':
		case 'strictObject':
		case 'digipeater':
		case 'entry':
		case 'group':
		case 'unproto':
			return { ...base, items: [''] };
		case 'symbol':
			return { ...base, primaryTable: '', altTable: '', overlay: '' };
		case 'qConstruct':
			return { ...base, qCodes: '', iFlag: false };
		case 'myRange':
			return { ...base, dist: 100 };
		case 'friendRange':
			return { ...base, friendCall: '', dist: 100 };
		default:
			return base;
	}
}

/** All filter types grouped by category for the type picker UI. */
export const filterTypeGroups: { label: string; types: FilterType[] }[] = [
	{
		label: 'Geographic',
		types: ['range', 'area', 'myRange', 'friendRange']
	},
	{
		label: 'Content',
		types: ['type', 'symbol', 'object', 'strictObject']
	},
	{
		label: 'Callsign',
		types: ['prefix', 'budlist', 'group', 'unproto']
	},
	{
		label: 'Path',
		types: ['digipeater', 'entry', 'qConstruct']
	}
];
