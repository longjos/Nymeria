// Dev/test fixture for NWS Alerts (internal/wxalert). A hand-built WxSnapshot
// with three alerts that exercise the three geometry paths a real feed
// produces: a published polygon, a zone-only alert with a mix of cached and
// missing zones, and a NEAR alert. Used by component/store tests and by any
// dev-mode stub that wants a snapshot without a live NWS feed.
//
// All timestamps are fixed (no Date.now()) so tests using this fixture are
// deterministic; FIXTURE_NOW is the instant the data was "fetched".
import type { WxAlert, WxSnapshot } from '../types';
import { FLOOR_TEXT } from '../wxAlertMeta';

/** 2026-09-17T20:21:00Z == 4:21 PM America/Detroit (EDT). */
export const FIXTURE_NOW = '2026-09-17T20:21:00Z';

const tornadoWarning: WxAlert = {
	id: 'urn:oid:2.49.0.1.840.0.wx-fixture-tor-1',
	provider: 'nws',
	providerUrl: 'https://api.weather.gov/alerts/urn:oid:2.49.0.1.840.0.wx-fixture-tor-1',
	event: 'Tornado Warning',
	effectiveEvent: 'Tornado Warning',
	tier: 'warning',
	shortCode: 'TOR WARN',
	headline: 'Tornado Warning issued September 17 at 4:15PM EDT until September 17 at 5:03PM EDT by NWS Grand Rapids MI',
	description:
		'At 415 PM EDT, a severe thunderstorm capable of producing a tornado was located near the course, moving northeast at 35 mph.',
	instruction: 'TAKE COVER NOW! Move to a basement or an interior room on the lowest floor of a sturdy building.',
	response: 'Shelter',
	category: 'Met',
	severity: 'Extreme',
	certainty: 'Observed',
	urgency: 'Immediate',
	status: 'Actual',
	messageType: 'Alert',
	sent: '2026-09-17T20:15:00Z',
	effective: '2026-09-17T20:15:00Z',
	onset: '2026-09-17T20:15:00Z',
	expires: '2026-09-17T21:03:00Z',
	ends: '2026-09-17T21:03:00Z',
	senderName: 'NWS Grand Rapids MI',
	sender: 'w-nws.grr@noaa.gov',
	senderId: 'KGRR',
	areaDesc: 'Kent, MI',
	ugc: ['MIC081'],
	same: ['026081'],
	references: [],
	parameters: {
		VTEC: ['/O.NEW.KGRR.TO.W.0042.260917T2015Z-260917T2103Z/'],
		tornadoDetection: ['RADAR INDICATED']
	},
	geometry: {
		type: 'Polygon',
		coordinates: [
			[
				[-85.75, 42.9],
				[-85.6, 42.9],
				[-85.6, 43.0],
				[-85.75, 43.0],
				[-85.75, 42.9]
			]
		]
	},

	state: 'active',
	endsAt: '2026-09-17T21:03:00Z',
	proximity: 'in',
	distanceMiles: 0,
	bearingDeg: 0,
	notifyClass: 'interrupt',
	notifyReason: 'new',
	floored: false,
	affects: {
		summary: 'CP 4–CP 7 · Aid 2 · 3 stations',
		entireCourse: false,
		routeMiles: 6.2,
		checkpoints: [
			{ kind: 'checkpoint', id: 'cp-4', label: 'Checkpoint 4', shortName: 'CP-4', seq: 4, lat: 42.94, lon: -85.7 },
			{ kind: 'checkpoint', id: 'cp-7', label: 'Checkpoint 7', shortName: 'CP-7', seq: 7, lat: 42.97, lon: -85.65 }
		],
		locations: [{ kind: 'location', id: 'ann-aid-2', label: 'Aid Station 2', shortName: 'Aid 2', lat: 42.95, lon: -85.68 }],
		stations: [
			{ kind: 'station', id: 'W8ABC-9', checkInId: 'ci-1', label: 'W8ABC-9', lat: 42.955, lon: -85.69 },
			{ kind: 'station', id: 'K8DEF', checkInId: 'ci-2', label: 'K8DEF', lat: 42.96, lon: -85.66 },
			{ kind: 'own', id: 'own', label: 'Net Control', lat: 42.95, lon: -85.7 }
		],
		checkpointSeqRange: [4, 7],
		routeSpans: [{ annotationId: 'ann-route-1', startIndex: 40, endIndex: 96 }]
	},
	geometrySource: 'polygon',
	zones: [{ ugc: 'MIC081', name: 'Kent County', state: 'MI', type: 'county', cached: true }],
	fetchedAt: FIXTURE_NOW,
	firstSeenAt: '2026-09-17T20:15:05Z',
	updatedAt: '2026-09-17T20:15:05Z',
	netId: 'net-fixture-1'
};

const floodWatch: WxAlert = {
	id: 'urn:oid:2.49.0.1.840.0.wx-fixture-ffa-1',
	provider: 'nws',
	providerUrl: 'https://api.weather.gov/alerts/urn:oid:2.49.0.1.840.0.wx-fixture-ffa-1',
	event: 'Flood Watch',
	effectiveEvent: 'Flood Watch',
	tier: 'watch',
	shortCode: 'FLOOD WATCH',
	headline: 'Flood Watch issued September 17 at 1:00PM EDT until September 18 at 2:00AM EDT by NWS Grand Rapids MI',
	description: 'Excessive rainfall may result in flooding of rivers, creeks, streams and other low-lying and flood-prone locations.',
	instruction: 'Monitor later forecasts and be alert for possible flood warnings.',
	response: 'Monitor',
	category: 'Met',
	severity: 'Moderate',
	certainty: 'Possible',
	urgency: 'Expected',
	status: 'Actual',
	messageType: 'Update',
	sent: '2026-09-17T17:00:00Z',
	effective: '2026-09-17T17:00:00Z',
	expires: '2026-09-18T06:00:00Z',
	senderName: 'NWS Grand Rapids MI',
	sender: 'w-nws.grr@noaa.gov',
	senderId: 'KGRR',
	areaDesc: 'Allegan, MI; Barry, MI',
	ugc: ['MIC005', 'MIC015'],
	same: ['026005', '026015'],
	references: [{ id: 'urn:oid:2.49.0.1.840.0.wx-fixture-ffa-0', sent: '2026-09-17T13:00:00Z' }],
	parameters: {},

	state: 'active',
	endsAt: '2026-09-18T06:00:00Z',
	proximity: 'in',
	distanceMiles: 0,
	bearingDeg: 0,
	notifyClass: 'badge',
	notifyReason: 'update',
	floored: false,
	affects: {
		summary: 'Entire course',
		entireCourse: true,
		routeMiles: 48,
		checkpoints: [],
		locations: [],
		stations: [],
		checkpointSeqRange: [],
		routeSpans: [{ annotationId: 'ann-route-1', startIndex: 0, endIndex: 40, ugc: 'MIC015' }]
	},
	geometrySource: 'zone',
	// MIC005 (Allegan) is cached — draws the real zone outline. MIC015 (Barry)
	// is not — the map falls back to the own-geometry route ribbon above.
	zones: [
		{ ugc: 'MIC005', name: 'Allegan County', state: 'MI', type: 'county', cached: true },
		{ ugc: 'MIC015', name: 'Barry County', state: 'MI', type: 'county', cached: false }
	],
	fetchedAt: FIXTURE_NOW,
	firstSeenAt: '2026-09-17T13:00:05Z',
	updatedAt: '2026-09-17T17:00:05Z',
	netId: 'net-fixture-1'
};

const severeThunderstormWarning: WxAlert = {
	id: 'urn:oid:2.49.0.1.840.0.wx-fixture-svr-1',
	provider: 'nws',
	providerUrl: 'https://api.weather.gov/alerts/urn:oid:2.49.0.1.840.0.wx-fixture-svr-1',
	event: 'Severe Thunderstorm Warning',
	effectiveEvent: 'Severe Thunderstorm Warning',
	tier: 'warning',
	shortCode: 'SVR WARN',
	headline: 'Severe Thunderstorm Warning issued September 17 at 4:05PM EDT until September 17 at 4:45PM EDT by NWS Grand Rapids MI',
	description: 'A severe thunderstorm capable of producing quarter size hail and 60 mph wind gusts was located 18 miles west of the course.',
	instruction: 'For your protection move to an interior room on the lowest floor of a building.',
	response: 'Shelter',
	category: 'Met',
	severity: 'Severe',
	certainty: 'Observed',
	urgency: 'Expected',
	status: 'Actual',
	messageType: 'Alert',
	sent: '2026-09-17T20:05:00Z',
	effective: '2026-09-17T20:05:00Z',
	expires: '2026-09-17T20:45:00Z',
	ends: '2026-09-17T20:45:00Z',
	senderName: 'NWS Grand Rapids MI',
	sender: 'w-nws.grr@noaa.gov',
	senderId: 'KGRR',
	areaDesc: 'Ottawa, MI',
	ugc: ['MIC139'],
	same: ['026139'],
	references: [],
	parameters: { maxWindGust: ['60 MPH'], maxHailSize: ['1.00 IN'] },
	geometry: {
		type: 'Polygon',
		coordinates: [
			[
				[-86.05, 42.85],
				[-85.95, 42.85],
				[-85.95, 42.95],
				[-86.05, 42.95],
				[-86.05, 42.85]
			]
		]
	},

	state: 'active',
	endsAt: '2026-09-17T20:45:00Z',
	proximity: 'near',
	distanceMiles: 18,
	bearingDeg: 270,
	notifyClass: 'toast',
	notifyReason: 'new',
	floored: false,
	affects: { summary: '', entireCourse: false, routeMiles: 0, checkpoints: [], locations: [], stations: [], checkpointSeqRange: [], routeSpans: [] },
	geometrySource: 'polygon',
	zones: [{ ugc: 'MIC139', name: 'Ottawa County', state: 'MI', type: 'county', cached: true }],
	fetchedAt: FIXTURE_NOW,
	firstSeenAt: '2026-09-17T20:05:05Z',
	updatedAt: '2026-09-17T20:05:05Z',
	netId: 'net-fixture-1'
};

export const wxAlertFixtureAlerts: WxAlert[] = [tornadoWarning, floodWatch, severeThunderstormWarning];

export const wxAlertFixtureSnapshot: WxSnapshot = {
	alerts: wxAlertFixtureAlerts,
	status: {
		state: 'live',
		enabled: true,
		contactConfigured: true,
		lastSuccessAt: FIXTURE_NOW,
		lastAttemptAt: FIXTURE_NOW,
		consecutiveFailures: 0,
		fromCache: false,
		regionCount: 9,
		inAreaCount: 2,
		nearbyCount: 1,
		zonesCached: 2,
		zonesMissing: 1,
		sounds: true
	},
	footprint: {
		netId: 'net-fixture-1',
		bufferMiles: 10,
		routeMiles: 48,
		locationCount: 6,
		checkpointCount: 8,
		rosterPositions: 11,
		trackedPositions: 2,
		ownStation: 'gps',
		ownStationAgeSec: 40,
		zones: [
			{ ugc: 'MIC081', name: 'Kent County', state: 'MI', type: 'county', cached: true },
			{ ugc: 'MIC005', name: 'Allegan County', state: 'MI', type: 'county', cached: true },
			{ ugc: 'MIC015', name: 'Barry County', state: 'MI', type: 'county', cached: false }
		],
		nearZones: [{ ugc: 'MIC139', name: 'Ottawa County', state: 'MI', type: 'county', cached: true }],
		extraZones: [],
		unresolvedSamples: 0,
		centroid: { lat: 42.95, lon: -85.68 },
		empty: false,
		computedAt: FIXTURE_NOW
	},
	policy: {
		netId: 'net-fixture-1',
		bufferMiles: 10,
		interruptEvents: [
			'Tornado Warning',
			'Flash Flood Emergency',
			'Severe Thunderstorm Warning',
			'Flash Flood Warning',
			'Extreme Wind Warning',
			'Ice Storm Warning',
			'Blizzard Warning'
		],
		interruptCustom: false,
		watchNotify: 'toast',
		advisoryNotify: 'badge',
		statementNotify: 'panel',
		muteAdvisories: false,
		floorText: FLOOR_TEXT
	}
};
