import { describe, it, expect } from 'vitest';
import {
	tierMeta,
	severityStyle,
	FLOOR_TEXT,
	sortAlerts,
	toastLine,
	bulletinSummary,
	announceSentence,
	zoneLabel,
	compass,
	param,
	tierRank,
	notifyRank,
	proximityLabel,
	proximityNote
} from './wxAlertMeta';
import type { WxAlert, WxSeverity, WxTier } from './types';
import { wxAlertFixtureAlerts } from './data/wxAlertFixture';

// Timestamp-formatting assertions below assume the test runner's timezone is
// UTC, which is true of this repo's dev/CI sandbox (verified via
// `Intl.DateTimeFormat().resolvedOptions().timeZone`).

function makeAlert(overrides: Partial<WxAlert>): WxAlert {
	const base: WxAlert = {
		id: 'base',
		provider: 'nws',
		providerUrl: 'https://api.weather.gov/alerts/base',
		event: 'Special Weather Statement',
		effectiveEvent: 'Special Weather Statement',
		tier: 'statement',
		shortCode: 'SPS',
		headline: '',
		description: '',
		instruction: '',
		response: '',
		category: 'Met',
		severity: 'Minor',
		certainty: 'Unknown',
		urgency: 'Unknown',
		status: 'Actual',
		messageType: 'Alert',
		sent: '2026-09-17T20:00:00Z',
		effective: '2026-09-17T20:00:00Z',
		expires: '2026-09-17T21:00:00Z',
		senderName: 'NWS Grand Rapids MI',
		sender: '',
		senderId: 'KGRR',
		areaDesc: '',
		ugc: [],
		same: [],
		references: [],
		parameters: {},
		state: 'active',
		endsAt: '2026-09-17T21:00:00Z',
		proximity: 'in',
		distanceMiles: 0,
		bearingDeg: 0,
		notifyClass: 'panel',
		notifyReason: 'new',
		floored: false,
		affects: { summary: '', entireCourse: false, routeMiles: 0, checkpoints: [], locations: [], stations: [], checkpointSeqRange: [], routeSpans: [] },
		geometrySource: 'none',
		zones: [],
		fetchedAt: '2026-09-17T20:21:00Z',
		firstSeenAt: '2026-09-17T20:00:00Z',
		updatedAt: '2026-09-17T20:00:00Z',
		netId: 'net-1'
	};
	return { ...base, ...overrides };
}

describe('tierMeta', () => {
	it('has exactly the four tier keys', () => {
		expect(Object.keys(tierMeta).sort()).toEqual(['advisory', 'statement', 'warning', 'watch']);
	});

	it('never aliases the plain --color-warning/--color-error tokens', () => {
		for (const meta of Object.values(tierMeta)) {
			expect(meta.colorVar.startsWith('--color-wx-')).toBe(true);
			expect(meta.softVar.startsWith('--color-wx-')).toBe(true);
			expect(meta.colorVar).not.toBe('--color-warning');
			expect(meta.colorVar).not.toBe('--color-error');
		}
	});

	it('gives warning tier a filled glyph and the others outline glyphs', () => {
		expect(tierMeta.warning.glyphFill).toBe(true);
		expect(tierMeta.watch.glyphFill).toBe(false);
		expect(tierMeta.advisory.glyphFill).toBe(false);
		expect(tierMeta.statement.glyphFill).toBe(false);
	});

	it('ranks tiers warning > watch > advisory > statement', () => {
		expect(tierRank('warning')).toBeGreaterThan(tierRank('watch'));
		expect(tierRank('watch')).toBeGreaterThan(tierRank('advisory'));
		expect(tierRank('advisory')).toBeGreaterThan(tierRank('statement'));
	});
});

describe('notifyRank', () => {
	it('orders interrupt > toast > badge > panel', () => {
		expect(notifyRank('interrupt')).toBeGreaterThan(notifyRank('toast'));
		expect(notifyRank('toast')).toBeGreaterThan(notifyRank('badge'));
		expect(notifyRank('badge')).toBeGreaterThan(notifyRank('panel'));
	});
});

describe('severityStyle', () => {
	const table: Array<[WxSeverity, { weight: number; fillOpacity: number; hatch: boolean; fontWeight: number; uppercase: boolean }]> = [
		['Extreme', { weight: 3, fillOpacity: 0.14, hatch: true, fontWeight: 700, uppercase: true }],
		['Severe', { weight: 2.5, fillOpacity: 0.12, hatch: true, fontWeight: 700, uppercase: false }],
		['Moderate', { weight: 2, fillOpacity: 0.08, hatch: false, fontWeight: 600, uppercase: false }],
		['Minor', { weight: 1.5, fillOpacity: 0.05, hatch: false, fontWeight: 500, uppercase: false }],
		['Unknown', { weight: 1.5, fillOpacity: 0.05, hatch: false, fontWeight: 500, uppercase: false }]
	];

	for (const [severity, want] of table) {
		it(`${severity} on warning tier -> ${JSON.stringify(want)}`, () => {
			expect(severityStyle('warning', severity)).toEqual(want);
		});
	}

	it('forces fillOpacity to 0 for advisory and statement regardless of severity', () => {
		for (const tier of ['advisory', 'statement'] as WxTier[]) {
			for (const severity of ['Extreme', 'Severe', 'Moderate', 'Minor', 'Unknown'] as WxSeverity[]) {
				expect(severityStyle(tier, severity).fillOpacity).toBe(0);
			}
		}
	});

	it('never hatches a non-warning tier even at Extreme', () => {
		expect(severityStyle('watch', 'Extreme').hatch).toBe(false);
		expect(severityStyle('advisory', 'Extreme').hatch).toBe(false);
	});
});

describe('FLOOR_TEXT', () => {
	it('matches internal/wxalert/tier.go FloorText() verbatim', () => {
		expect(FLOOR_TEXT).toBe(
			'An Extreme-severity, Immediate-urgency alert inside the watch area always shows at least a toast. Nothing on this page can turn that off.'
		);
	});
});

describe('sortAlerts', () => {
	it('orders tier desc, severity desc, soonest ends, newest sent within a shuffled fixture', () => {
		const tor = makeAlert({ id: 'tor', tier: 'warning', severity: 'Extreme', endsAt: '2026-09-17T21:00:00Z', sent: '2026-09-17T20:00:00Z' });
		const svr = makeAlert({ id: 'svr', tier: 'warning', severity: 'Severe', endsAt: '2026-09-17T21:00:00Z', sent: '2026-09-17T20:00:00Z' });
		const watch = makeAlert({ id: 'watch', tier: 'watch', severity: 'Moderate', endsAt: '2026-09-17T22:00:00Z', sent: '2026-09-17T19:00:00Z' });
		const adv = makeAlert({ id: 'adv', tier: 'advisory', severity: 'Minor', endsAt: '2026-09-18T02:00:00Z', sent: '2026-09-17T18:00:00Z' });
		const sws = makeAlert({ id: 'sws', tier: 'statement', severity: 'Unknown', endsAt: '2026-09-18T06:00:00Z', sent: '2026-09-17T17:00:00Z' });

		const shuffled = [adv, sws, tor, watch, svr];
		expect(sortAlerts(shuffled).map((a) => a.id)).toEqual(['tor', 'svr', 'watch', 'adv', 'sws']);
	});

	it('groups in before near before ended, before applying tier/severity order', () => {
		const endedWarning = makeAlert({ id: 'ended', tier: 'warning', severity: 'Extreme', state: 'expired', proximity: 'in' });
		const nearWarning = makeAlert({ id: 'near', tier: 'warning', severity: 'Extreme', proximity: 'near' });
		const inAdvisory = makeAlert({ id: 'in-adv', tier: 'advisory', severity: 'Minor', proximity: 'in' });

		expect(sortAlerts([endedWarning, nearWarning, inAdvisory]).map((a) => a.id)).toEqual(['in-adv', 'near', 'ended']);
	});
});

describe('param', () => {
	it('returns the first value of a CAP parameter', () => {
		const a = makeAlert({ parameters: { maxWindGust: ['60 MPH', 'ignored'] } });
		expect(param(a, 'maxWindGust')).toBe('60 MPH');
	});

	it('returns undefined for an absent key without throwing', () => {
		const a = makeAlert({ parameters: {} });
		expect(param(a, 'nope')).toBeUndefined();
	});
});

describe('zoneLabel', () => {
	it('names the single zone', () => {
		const a = makeAlert({ zones: [{ ugc: 'MIC081', name: 'Kent County', state: 'MI', type: 'county', cached: true }] });
		expect(zoneLabel(a)).toBe('Kent County');
	});

	it('appends +N for additional zones', () => {
		const a = makeAlert({
			zones: [
				{ ugc: 'MIC081', name: 'Kent County', state: 'MI', type: 'county', cached: true },
				{ ugc: 'MIC005', name: 'Allegan County', state: 'MI', type: 'county', cached: true },
				{ ugc: 'MIC015', name: 'Barry County', state: 'MI', type: 'county', cached: false }
			]
		});
		expect(zoneLabel(a)).toBe('Kent County +2');
	});

	it('is empty for a polygon alert with no zones', () => {
		expect(zoneLabel(makeAlert({ zones: [] }))).toBe('');
	});
});

describe('compass', () => {
	it('maps degrees to the nearest 8-point label', () => {
		expect(compass(0)).toBe('N');
		expect(compass(90)).toBe('E');
		expect(compass(180)).toBe('S');
		expect(compass(270)).toBe('W');
		expect(compass(359)).toBe('N');
	});

	it('is empty when bearing is absent', () => {
		expect(compass(undefined)).toBe('');
	});
});

describe('toastLine', () => {
	it('formats an IN alert with the affects summary and shortCode', () => {
		const tor = wxAlertFixtureAlerts[0];
		expect(toastLine(tor)).toBe(`TOR WARN · ${tor.affects.summary} · until 9:03 PM · NWS`);
	});

	it('formats a NEAR alert with distance and compass', () => {
		const svr = wxAlertFixtureAlerts[2];
		expect(toastLine(svr)).toBe('SVR WARN · 18 mi W of course · until 8:45 PM · NWS');
	});

	it('falls back to "in your watch area" when the affects summary is empty', () => {
		const a = makeAlert({ shortCode: 'FLOOD WATCH', proximity: 'in', affects: { ...makeAlert({}).affects, summary: '' }, endsAt: '2026-09-17T21:00:00Z' });
		expect(toastLine(a)).toBe('FLOOD WATCH · in your watch area · until 9:00 PM · NWS');
	});

	it('reads "cancelled" for a cancelled alert and "expired" otherwise', () => {
		const cancelled = makeAlert({ event: 'Tornado Warning', state: 'cancelled', endedReason: 'cancelled' });
		const expired = makeAlert({ event: 'Tornado Warning', state: 'expired', endedReason: 'expired' });
		expect(toastLine(cancelled)).toBe('Tornado Warning cancelled · NWS');
		expect(toastLine(expired)).toBe('Tornado Warning expired · NWS');
	});
});

describe('bulletinSummary', () => {
	it('builds the exact TOR WARN example and stays within 67 chars', () => {
		const a = makeAlert({
			shortCode: 'TOR WARN',
			response: 'Shelter',
			endsAt: '2026-09-17T19:45:00Z',
			zones: [{ ugc: 'MIC081', name: 'Kent County', state: 'MI', type: 'county', cached: true }]
		});
		const line = bulletinSummary(a, 'W8ABC');
		expect(line).toBe('TOR WARN KENT CO UNTIL 1945 SEEK SHELTER -W8ABC');
		expect(line.length).toBeLessThanOrEqual(67);
	});

	it('collapses a 33-zone alert to "+32" and stays within 67 chars', () => {
		const zones = Array.from({ length: 33 }, (_, i) => ({
			ugc: `MIC${String(i).padStart(3, '0')}`,
			name: i === 0 ? 'Kent County' : `Zone ${i}`,
			state: 'MI',
			type: 'county' as const,
			cached: true
		}));
		const a = makeAlert({ shortCode: 'HEAT ADV', response: '', endsAt: '2026-09-17T23:00:00Z', zones });
		const line = bulletinSummary(a, 'W8ABC');
		expect(line).toContain('+32');
		expect(line.length).toBeLessThanOrEqual(67);
	});

	it('hard-truncates an extreme case (long code, zone name and callsign) without exceeding 67 chars', () => {
		const a = makeAlert({
			shortCode: 'EXTREME WIND WARN',
			response: 'Evacuate',
			endsAt: '2026-09-17T23:00:00Z',
			zones: [{ ugc: 'MIC081', name: 'A Very Long Hypothetical County Name That Goes On', state: 'MI', type: 'county', cached: true }]
		});
		const line = bulletinSummary(a, 'NETCONTROL9');
		expect(line.length).toBeLessThanOrEqual(67);
		expect(line.startsWith('EXTREME WIND WARN')).toBe(true);
	});
});

describe('announceSentence', () => {
	it('produces the exact visible-and-announced banner sentence', () => {
		const tor = wxAlertFixtureAlerts[0];
		const now = Date.parse(tor.endsAt) - 42 * 60_000;
		expect(announceSentence(tor, '4:21:00 PM', now)).toBe(
			'Tornado Warning in your watch area, affects CP 4–CP 7 · Aid 2 · 3 stations, ends 9:03 PM, 42 minutes from now. Source NWS Grand Rapids MI, fetched 4:21:00 PM.'
		);
	});

	it('reads "now" once the alert has already ended', () => {
		const a = makeAlert({ event: 'Tornado Warning', endsAt: '2026-09-17T21:00:00Z', senderName: 'NWS Grand Rapids MI' });
		const now = Date.parse(a.endsAt) + 60_000;
		expect(announceSentence(a, '9:05 PM', now)).toBe(
			'Tornado Warning in your watch area, affects the watch area, ends 9:00 PM, now. Source NWS Grand Rapids MI, fetched 9:05 PM.'
		);
	});
});

describe('proximityLabel / proximityNote', () => {
	it('names the watch area for an in-area alert', () => {
		const a = makeAlert({ proximity: 'in', distanceMiles: 0, bearingDeg: 0 });
		expect(proximityLabel(a)).toBe('In watch area');
		expect(proximityNote(a)).toContain('Inside the watch area');
	});

	it('gives distance and bearing for a nearby alert', () => {
		const a = makeAlert({ proximity: 'near', distanceMiles: 12.4, bearingDeg: 45 });
		expect(proximityLabel(a)).toBe('12 mi NE');
		expect(proximityNote(a)).toBe('12 mi NE of the watch area — nothing of yours is inside it.');
	});

	it('omits the bearing when it is unknown', () => {
		const a = makeAlert({ proximity: 'near', distanceMiles: 8, bearingDeg: Number.NaN });
		expect(proximityLabel(a)).toBe('8 mi');
	});
});
