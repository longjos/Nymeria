/**
 * Command parser for the net control command palette.
 * Stateless, pure functions — no side effects or store dependencies.
 */

// --- Constants ---

const TRAFFIC_SHORTCUTS: Record<string, string> = {
	R: 'routine',
	P: 'priority',
	W: 'welfare',
	E: 'emergency',
};

const TRAFFIC_NAMES = new Set(['routine', 'priority', 'welfare', 'emergency']);

const STATION_CATEGORIES = new Set([
	'general', 'command', 'medical', 'sag', 'marshal', 'fixed', 'mobile', 'tactical',
]);

const OPERATOR_STATUSES = new Set([
	'available', 'assigned', 'enroute', 'onscene', 'brb', 'missing', 'released',
]);

// Callsign pattern: 1-2 letters, 1+ digit, 0-3 letters, optional -SSID
const CALLSIGN_RE = /^[A-Z]{1,2}\d+[A-Z]{0,3}(-\d{1,2})?$/;

// --- Types ---

export type ParsedCommand =
	| { type: 'checkin'; callsign: string; traffic: string; category: string }
	| { type: 'status'; callsign: string; status: string }
	| { type: 'checkout'; callsign: string }
	| { type: 'note'; callsign: string; text: string }
	| { type: 'mission_create'; title: string }
	| { type: 'mission_assign'; callsign: string; missionTitle: string }
	| { type: 'location'; callsign: string; locationName?: string; lat?: number; lon?: number }
	| { type: 'checkpoint_passage'; checkpointRef: string; label: string }
	| { type: 'unknown'; raw: string };

export interface AutocompleteContext {
	/** What the user is currently typing (for suggestions) */
	phase: 'callsign' | 'action' | 'traffic' | 'category' | 'status' | 'note_text' | 'mission_title' | 'mission_name' | 'location';
	/** Partial text of the current token */
	partial: string;
	/** Suggestions to show */
	suggestions: string[];
}

// --- Parser ---

export function parseCommand(
	input: string,
	checkedInCallsigns: string[],
): ParsedCommand {
	const raw = input.trim();
	if (!raw) return { type: 'unknown', raw: '' };

	const parts = raw.split(/\s+/);
	const firstToken = parts[0];

	// Checkpoint passage: "cp3 lead" or "cp1"
	const cpMatch = firstToken.match(/^cp(\d+)$/i);
	if (cpMatch) {
		const checkpointRef = cpMatch[1];
		const label = parts.slice(1).join(' ').trim() || 'through';
		return { type: 'checkpoint_passage', checkpointRef, label };
	}

	// Mission create: "mission <title>"
	if (firstToken.toLowerCase() === 'mission') {
		const title = parts.slice(1).join(' ').trim();
		if (!title) return { type: 'unknown', raw };
		return { type: 'mission_create', title };
	}

	// Everything else: first token is a callsign
	const callsign = firstToken.toUpperCase();
	const checkedInSet = new Set(checkedInCallsigns.map((c) => c.toUpperCase()));
	const isCheckedIn = checkedInSet.has(callsign);

	// Single token: check-in with no traffic
	if (parts.length === 1) {
		return { type: 'checkin', callsign, traffic: '', category: '' };
	}

	const secondToken = parts[1];
	const secondLower = secondToken.toLowerCase();

	// Checkout: "CALL out"
	if (secondLower === 'out') {
		return { type: 'checkout', callsign };
	}

	// Note: "CALL note <text>" or "CALL n <text>"
	if (secondLower === 'note' || secondLower === 'n') {
		const text = parts.slice(2).join(' ').trim();
		return { type: 'note', callsign, text };
	}

	// Mission assign: "CALL assign <mission-title>"
	if (secondLower === 'assign') {
		const missionTitle = parts.slice(2).join(' ').trim();
		return { type: 'mission_assign', callsign, missionTitle };
	}

	// Location: "CALL loc <name>" or "CALL loc <lat> <lon>"
	if (secondLower === 'loc') {
		if (parts.length >= 4) {
			const lat = parseFloat(parts[2]);
			const lon = parseFloat(parts[3]);
			if (!isNaN(lat) && !isNaN(lon)) {
				return { type: 'location', callsign, lat, lon };
			}
		}
		const locationName = parts.slice(2).join(' ').trim();
		if (locationName) {
			return { type: 'location', callsign, locationName };
		}
		return { type: 'location', callsign };
	}

	// Status change: only for already-checked-in operators
	if (isCheckedIn && OPERATOR_STATUSES.has(secondLower)) {
		return { type: 'status', callsign, status: secondLower };
	}

	// Check-in with traffic/category tokens
	let traffic = '';
	let category = '';
	for (let i = 1; i < parts.length; i++) {
		const upper = parts[i].toUpperCase();
		const lower = parts[i].toLowerCase();
		if (!traffic && TRAFFIC_SHORTCUTS[upper]) {
			traffic = TRAFFIC_SHORTCUTS[upper];
		} else if (!traffic && TRAFFIC_NAMES.has(lower)) {
			traffic = lower;
		} else if (!category && STATION_CATEGORIES.has(lower)) {
			category = lower;
		}
	}

	return { type: 'checkin', callsign, traffic, category };
}

// --- Mode indicator ---

export function getModeIndicator(parsed: ParsedCommand): string {
	switch (parsed.type) {
		case 'checkin': {
			const parts = ['CHECK IN'];
			if (parsed.traffic) parts.push(parsed.traffic);
			if (parsed.category) parts.push(parsed.category);
			return parts.length > 1 ? `[${parts.join(' · ')}]` : `[${parts[0]}]`;
		}
		case 'status':
			return `[STATUS → ${parsed.status}]`;
		case 'checkout':
			return '[CHECKOUT]';
		case 'note':
			return '[NOTE]';
		case 'mission_create':
			return '[NEW MISSION]';
		case 'mission_assign':
			return '[ASSIGN MISSION]';
		case 'location':
			return '[LOCATION]';
		case 'checkpoint_passage':
			return `[CP${parsed.checkpointRef} \u2192 ${parsed.label}]`;
		case 'unknown':
			return '';
	}
}

// --- Autocomplete ---

export function getAutocompleteContext(
	input: string,
	checkedInCallsigns: string[],
	missionTitles: string[],
): AutocompleteContext | null {
	const raw = input.trim();
	if (!raw) return null;

	const parts = raw.split(/\s+/);
	// Detect if user is mid-token or has typed a space after
	const endsWithSpace = input.endsWith(' ');

	// First token: callsign, "mission", or "cpN"
	if (parts.length === 1 && !endsWithSpace) {
		const partial = parts[0].toUpperCase();
		const partialLower = parts[0].toLowerCase();
		// Suggest "mission" if partial matches
		const suggestions: string[] = [];
		if ('mission'.startsWith(partialLower)) {
			suggestions.push('mission');
		}
		// Suggest cpN if partial starts with "cp"
		if (partialLower.startsWith('cp') || 'cp'.startsWith(partialLower)) {
			// Suggest cp1 through cp9 for convenience
			for (let i = 1; i <= 9; i++) {
				const cpToken = `cp${i}`;
				if (cpToken.startsWith(partialLower)) suggestions.push(cpToken);
			}
		}
		// Suggest matching callsigns
		for (const cs of checkedInCallsigns) {
			if (cs.toUpperCase().startsWith(partial)) {
				suggestions.push(cs);
			}
		}
		return { phase: 'callsign', partial: parts[0], suggestions };
	}

	// After "cpN ": suggest common labels
	const cpAutoMatch = parts[0].match(/^cp(\d+)$/i);
	if (cpAutoMatch) {
		const knownLabels = ['lead', 'sweep', 'tail', 'main pack'];
		if (parts.length === 1 && endsWithSpace) {
			return { phase: 'action', partial: '', suggestions: knownLabels };
		}
		const labelPartial = parts.slice(1).join(' ').toLowerCase();
		const suggestions = knownLabels.filter(l => l.startsWith(labelPartial));
		return { phase: 'action', partial: labelPartial, suggestions };
	}

	// After "mission ": suggest nothing (free-form title)
	if (parts[0].toLowerCase() === 'mission') {
		return {
			phase: 'mission_title',
			partial: parts.slice(1).join(' '),
			suggestions: [],
		};
	}

	const callsign = parts[0].toUpperCase();
	const checkedInSet = new Set(checkedInCallsigns.map((c) => c.toUpperCase()));
	const isCheckedIn = checkedInSet.has(callsign);

	// Second token: action keyword
	if (parts.length === 1 && endsWithSpace) {
		// User typed callsign + space, suggest actions
		const actions = ['out', 'note', 'n', 'assign', 'loc'];
		if (isCheckedIn) {
			actions.push(...['assigned', 'enroute', 'onscene', 'brb', 'missing']);
		}
		// Also suggest traffic shortcuts
		actions.push('R', 'P', 'W', 'E');
		return { phase: 'action', partial: '', suggestions: actions };
	}

	if (parts.length === 2 && !endsWithSpace) {
		const partial = parts[1];
		const partialLower = partial.toLowerCase();
		const partialUpper = partial.toUpperCase();
		const suggestions: string[] = [];

		// Action keywords
		for (const kw of ['out', 'note', 'n', 'assign', 'loc']) {
			if (kw.startsWith(partialLower)) suggestions.push(kw);
		}

		// Status keywords (only for checked-in)
		if (isCheckedIn) {
			for (const st of OPERATOR_STATUSES) {
				if (st.startsWith(partialLower)) suggestions.push(st);
			}
		}

		// Traffic shortcuts
		for (const key of Object.keys(TRAFFIC_SHORTCUTS)) {
			if (key.startsWith(partialUpper)) suggestions.push(key);
		}

		// Traffic names
		for (const t of TRAFFIC_NAMES) {
			if (t.startsWith(partialLower)) suggestions.push(t);
		}

		// Category names
		for (const c of STATION_CATEGORIES) {
			if (c.startsWith(partialLower)) suggestions.push(c);
		}

		return { phase: 'action', partial, suggestions };
	}

	// After "CALL assign ": suggest mission titles
	if (parts.length >= 2 && parts[1].toLowerCase() === 'assign') {
		const partial = parts.slice(2).join(' ').toLowerCase();
		const suggestions = missionTitles.filter((m) =>
			m.toLowerCase().startsWith(partial) || m.toLowerCase().includes(partial),
		);
		return { phase: 'mission_name', partial, suggestions };
	}

	// After "CALL note " or "CALL n ": free text, no suggestions
	if (parts.length >= 2 && (parts[1].toLowerCase() === 'note' || parts[1].toLowerCase() === 'n')) {
		return {
			phase: 'note_text',
			partial: parts.slice(2).join(' '),
			suggestions: [],
		};
	}

	// After "CALL loc ": location context
	if (parts.length >= 2 && parts[1].toLowerCase() === 'loc') {
		return {
			phase: 'location',
			partial: parts.slice(2).join(' '),
			suggestions: [],
		};
	}

	// Third+ token on check-in: category suggestions
	// This covers both "CALL R " (2 parts + trailing space) and "CALL R me" (3 parts, mid-token)
	if (parts.length >= 3 || (parts.length === 2 && endsWithSpace)) {
		if (endsWithSpace) {
			return {
				phase: 'category',
				partial: '',
				suggestions: [...STATION_CATEGORIES],
			};
		}
		const lastPartial = parts[parts.length - 1].toLowerCase();
		const suggestions = [...STATION_CATEGORIES].filter((c) => c.startsWith(lastPartial));
		return { phase: 'category', partial: lastPartial, suggestions };
	}

	return null;
}
