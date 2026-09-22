export interface WeatherData {
	windDir?: number;
	windSpeed?: number;
	windGust?: number;
	temperature?: number;
	humidity?: number;
	pressure?: number;
	rain1h?: number;
	rain24h?: number;
	rainToday?: number;
	luminosity?: number;
	radiation?: number;
	voltage?: number;
	floodLevel?: number;
}

export interface WeatherReading {
	id: number;
	callsign: string;
	timestamp: string;
	temperature?: number;
	windDir?: number;
	windSpeed?: number;
	windGust?: number;
	humidity?: number;
	pressure?: number;
	rain1h?: number;
	rain24h?: number;
	rainToday?: number;
	luminosity?: number;
}

// UI label: "Reading thresholds" (not an NWS alert). internal/wxalert owns
// the word "alert" everywhere else in this app; this type is unrelated
// station-reading min/max highlighting and keeps its existing name and JSON
// shape (`alerts`) to avoid an unrelated rename.
export interface WeatherAlertThreshold {
	min?: number;
	max?: number;
}

export interface WeatherConfig {
	retentionDays: number;
	alerts?: Record<string, WeatherAlertThreshold>;
	units: 'metric' | 'imperial';
}

// --- NWS Alerts (internal/wxalert) ---
// Mirrors internal/wxalert types byte-for-byte (BUILD-PLAN.md §2). Times are
// RFC 3339 strings. The word "alert" from here down means an NWS watch,
// warning, advisory or statement, and nothing else in this app does.

export type WxTier = 'warning' | 'watch' | 'advisory' | 'statement';
export type WxSeverity = 'Extreme' | 'Severe' | 'Moderate' | 'Minor' | 'Unknown';
export type WxCertainty = 'Observed' | 'Likely' | 'Possible' | 'Unlikely' | 'Unknown';
export type WxUrgency = 'Immediate' | 'Expected' | 'Future' | 'Past' | 'Unknown';
export type WxStatus = 'Actual' | 'Exercise' | 'System' | 'Test' | 'Draft';
export type WxMessageType = 'Alert' | 'Update' | 'Cancel';
/** 'far' is computed server-side but never sent on the wire (never broadcast/retained). */
export type WxProximity = 'in' | 'near';
export type WxNotifyClass = 'interrupt' | 'toast' | 'badge' | 'panel';
export type WxNotifyReason = 'new' | 'update' | 'escalated' | 'ended' | 'none';
export type WxAlertState = 'active' | 'expired' | 'cancelled' | 'dropped';
export type WxEndedReason = 'expired' | 'cancelled' | 'dropped' | 'clock';
export type WxGeometrySource = 'polygon' | 'zone' | 'none';
export type WxLinkState = 'live' | 'stale' | 'down' | 'off';
export type WxMapMode = 'off' | 'warnings' | 'watches' | 'all';
export type WxZoneType = 'county' | 'forecast' | 'fire' | 'marine' | 'unknown';

export interface WxLatLon {
	lat: number;
	lon: number;
}

/** GeoJSON geometry exactly as internal/wxalert.Geometry marshals it (lon, lat order) — byte-compatible with Leaflet. */
export interface WxPolygonGeometry {
	type: 'Polygon' | 'MultiPolygon';
	coordinates: number[][][] | number[][][][];
}

/** One entry of the CAP supersession chain. */
export interface WxReference {
	id: string;
	sent: string;
}

/** Contiguous vertex range of a route annotation that lies inside the alert (own-geometry fallback ribbon). */
export interface WxRouteSpan {
	annotationId: string;
	startIndex: number;
	endIndex: number;
	ugc?: string;
}

/** One thing of ours an alert touches. */
export interface WxAffectedItem {
	kind: 'checkpoint' | 'location' | 'station' | 'own';
	/** annotation id | callsign | "own" */
	id: string;
	/** Roster check-in id — station items only. */
	checkInId?: string;
	label: string;
	shortName?: string;
	/** Set only for checkpoints. */
	seq?: number;
	lat: number;
	lon: number;
	/** UGC of the zone this item resolved to (zone-only alerts). */
	ugc?: string;
}

/** "What of ours it touches", computed server-side. */
export interface WxAffects {
	/** Server-built, <= 80 runes, e.g. "CP 4-CP 7 - Aid 2 - 3 stations"; "" when nothing. */
	summary: string;
	entireCourse: boolean;
	routeMiles: number;
	/** Never nil; sorted by seq. */
	checkpoints: WxAffectedItem[];
	locations: WxAffectedItem[];
	/** Roster + tracked + own. */
	stations: WxAffectedItem[];
	/** [] or [lowest seq, highest seq] of touched checkpoints. */
	checkpointSeqRange: number[];
	routeSpans: WxRouteSpan[];
}

/** The NCS "ack for net" record — clears the banner for everyone (decision 5), not per-station proof of receipt. */
export interface WxNetAck {
	userId: string;
	userName: string;
	callsign: string;
	at: string;
}

/** Lightweight reference to a zone — enough to render a chip without shipping the whole polygon. */
export interface WxZoneRef {
	ugc: string;
	/** "" until cached. */
	name: string;
	state: string;
	type: WxZoneType;
	cached: boolean;
}

/** The alert plus everything derived for the requesting net's footprint (wxalert.MatchedAlert). */
export interface WxAlert {
	/** properties.id ("urn:oid:…"), never the feature URL. */
	id: string;
	provider: 'nws';
	/** feature.id URL, for "Open on weather.gov". */
	providerUrl: string;
	/** NWS event name, verbatim ("Tornado Warning"). */
	event: string;
	/** "Flash Flood Emergency" / "Tornado Emergency" when applicable, else === event. */
	effectiveEvent: string;
	/** Server-derived; the client never re-derives this. */
	tier: WxTier;
	/** <= 14 chars, server-derived. */
	shortCode: string;
	headline: string;
	description: string;
	/** "" when NWS sends null. */
	instruction: string;
	response: string;
	category: string;
	severity: WxSeverity;
	certainty: WxCertainty;
	urgency: WxUrgency;
	status: WxStatus;
	messageType: WxMessageType;
	sent: string;
	effective: string;
	onset?: string;
	expires: string;
	ends?: string;
	/** "NWS Grand Rapids MI" */
	senderName: string;
	sender: string;
	/** WFO id from VTEC, "KGRR" ("" if unknown). */
	senderId: string;
	areaDesc: string;
	/** Never nil; dedup union of geocode.UGC and affectedZones. */
	ugc: string[];
	same: string[];
	/** Never nil. */
	references: WxReference[];
	/** Never nil ({}); first value only in most client reads — see wxAlertMeta.param(). */
	parameters: Record<string, string[]>;
	/** Present only when NWS published a polygon. */
	geometry?: WxPolygonGeometry;

	state: WxAlertState;
	endedAt?: string;
	/** 'expired'|'cancelled'|'dropped'|'clock' */
	endedReason?: WxEndedReason;
	/** Resolved server-side: ends ?? parameters.eventEndingTime[0] ?? expires. The client never recomputes this. */
	endsAt: string;
	/** 'in'|'near' on the wire — 'far' is never sent. */
	proximity: WxProximity;
	/** Miles from footprint edge; 0 when IN. */
	distanceMiles: number;
	/** Degrees true, for NEAR only; 0 when IN. */
	bearingDeg: number;
	/** After settings, per-net mutes, allowlist AND the hard floor. */
	notifyClass: WxNotifyClass;
	/** What changed at the poll that produced this version. */
	notifyReason: WxNotifyReason;
	/** True when the hard floor (Extreme + Immediate + in-footprint) raised the class. */
	floored: boolean;
	affects: WxAffects;
	geometrySource: WxGeometrySource;
	/** Never nil; one per UGC. */
	zones: WxZoneRef[];
	replacedBy?: string;
	ackedForNet?: WxNetAck;
	/** Wall clock of the poll that produced this version. */
	fetchedAt: string;
	firstSeenAt: string;
	/** Changes on every new version — the client's dedupe key. */
	updatedAt: string;
	/** "" = no-net footprint. */
	netId: string;
}

export interface WxFootprintSummary {
	/** "" when no net. */
	netId: string;
	bufferMiles: number;
	routeMiles: number;
	/** Point annotations incl. checkpoints. */
	locationCount: number;
	checkpointCount: number;
	rosterPositions: number;
	trackedPositions: number;
	ownStation: 'gps' | 'config' | 'none';
	/** 0 unless gps. */
	ownStationAgeSec: number;
	/** The IN set, never nil. */
	zones: WxZoneRef[];
	/** Never nil. */
	nearZones: WxZoneRef[];
	/** Never nil. */
	extraZones: string[];
	unresolvedSamples: number;
	centroid?: WxLatLon;
	/** Nothing to watch: no course, positions or own position. */
	empty: boolean;
	computedAt: string;
}

/**
 * Why the link is `off`. `off` is a status, not a failure — each reason is a
 * different fix, so the UI must not collapse them into "can't connect".
 */
export type WxLinkReason = 'disabled' | 'contactMissing' | 'initFailed' | 'noWatchArea';

export interface WxLinkStatus {
	state: WxLinkState;
	/** Only set when state is 'off'. */
	reason?: WxLinkReason;
	enabled: boolean;
	/** NWS requires a User-Agent with contact info and returns 403 without one. */
	contactConfigured: boolean;
	lastSuccessAt?: string;
	lastAttemptAt?: string;
	nextAttemptAt?: string;
	/** Plain words: "HTTP 503", "DNS lookup failed", "timeout after 10 s". */
	lastError?: string;
	consecutiveFailures: number;
	/** True until the first successful poll after boot (alerts came from SQLite). */
	fromCache: boolean;
	/** Alerts fetched for the area query. */
	regionCount: number;
	inAreaCount: number;
	nearbyCount: number;
	/** Footprint zones with a cached polygon. */
	zonesCached: number;
	zonesMissing: number;
	/** Mirrored from settings. */
	sounds: boolean;
}

/** The resolved (config + per-net) notification policy, echoed on every snapshot. */
export interface WxEffectivePolicy {
	netId: string;
	bufferMiles: number;
	interruptEvents: string[];
	interruptCustom: boolean;
	watchNotify: 'toast' | 'badge';
	advisoryNotify: 'badge' | 'panel';
	statementNotify: 'panel' | 'badge';
	muteAdvisories: boolean;
	/** === wxAlertMeta.FLOOR_TEXT. */
	floorText: string;
}

/** Per-net watch settings (GET/PUT /nets/{id}/wxwatch). NCS/admin writable. */
export interface WxNetWatch {
	netId: string;
	/** 0 = inherit config default. 2-50 otherwise. */
	bufferMiles: number;
	/** UGC codes, never nil. */
	extraZones: string[];
	/** Advisories AND statements → panel only for this net. */
	muteAdvisories: boolean;
	/** false = inherit config allowlist. */
	interruptCustom: boolean;
	/** Never nil; used only when interruptCustom. */
	interruptEvents: string[];
	/** Read-only echo; ignored on PUT. */
	effective: WxEffectivePolicy;
}

export interface WxNetWatchZones {
	/** From the footprint (the IN set). */
	resolved: WxZoneRef[];
	/** Adjacent zones union any extra zones — pre-listed as candidates. */
	neighbors: WxZoneRef[];
}

/** Top-level payload of GET /wx/alerts and the wx_alerts WS event. */
export interface WxSnapshot {
	/** Never nil; IN + NEAR active, plus ended <= 60 min; server-sorted. */
	alerts: WxAlert[];
	status: WxLinkStatus;
	/** null only before the first build. */
	footprint: WxFootprintSummary | null;
	policy: WxEffectivePolicy;
}

/** GET /wx/zones/{ugc}. */
export interface WxZone {
	ugc: string;
	type: WxZoneType;
	name: string;
	state: string;
	/** Polygon | MultiPolygon (GeometryCollection flattened to MultiPolygon). */
	geometry: WxPolygonGeometry;
	fetchedAt: string;
}

/** One row of GET /wx/event-types (the allowlist picker's data). */
export interface WxEventType {
	event: string;
	tier: WxTier;
}

/** POST /wx/alerts/{id}/relay body. */
export interface WxRelayRequest {
	note: boolean;
	bulletin: boolean;
	messages: boolean;
	/** <= 67 bytes; used for both the bulletin and the per-roster messages. */
	text: string;
}

export interface WxRelayResult {
	noteId?: string;
	bulletinSent: boolean;
	messagesSent: number;
	messagesFailed: string[];
}

/** Admin settings (Settings › NWS Alerts). Part of SettingsResponse as `wxAlerts`. */
export interface WxAlertsSettings {
	enabled: boolean;
	/** Go duration string, "60s". */
	pollInterval: string;
	contact: string;
	baseUrl: string;
	defaultBufferMiles: number;
	defaultZones: string[];
	/** Global default allowlist; per-net copies it on first edit. */
	interruptEvents: string[];
	watchNotify: 'toast' | 'badge';
	advisoryNotify: 'badge' | 'panel';
	statementNotify: 'panel' | 'badge';
	sounds: boolean;
	includeTest: boolean;
	/** Server-supplied validation bounds — never hard-code these in a component. */
	limits: WxAlertLimits;
}

/** Mirrors config.Validate's constraints so inputs enforce them at entry. */
export interface WxAlertLimits {
	bufferMilesMin: number;
	bufferMilesMax: number;
	/** Go duration strings, e.g. "30s" / "10m0s". */
	pollIntervalMin: string;
	pollIntervalMax: string;
	notifyChoices: string[];
}

export interface DFData {
	bearing: number;
	number: number;
	range: number;
	quality: number;
}

export interface TelemetryData {
	seq: number;
	analog: [number, number, number, number, number];
	digital: number;
	comment?: string;
}

export interface TelemetryParams {
	paramNames: [string, string, string, string, string];
	unitLabels: [string, string, string, string, string];
	equations: [[number, number, number], [number, number, number], [number, number, number], [number, number, number], [number, number, number]];
	bitSense: number;
	bitLabels: [string, string, string, string, string, string, string, string];
	projectTitle?: string;
}

export interface TelemetryReading {
	id: number;
	callsign: string;
	timestamp: string;
	seq: number;
	analog1: number;
	analog2: number;
	analog3: number;
	analog4: number;
	analog5: number;
	digital: number;
}

export interface TelemetryReadingsResponse {
	readings: TelemetryReading[];
	params: TelemetryParams | null;
}

export interface Station {
	callsign: string;
	ssid: number;
	lastHeard: string;
	position?: Position;
	symbol: APRSSymbol;
	comment?: string;
	track: TrackPoint[];
	/** Human-readable summary of the transport(s) heard (legacy). */
	source: string;
	/** Set of transport display names this station was heard on (custom transport name if configured, else the type). */
	sources?: string[];
	weather?: WeatherData;
	df?: DFData;
	telemetry?: TelemetryData;
	telemetryParams?: TelemetryParams;
}

export interface Position {
	lat: number;
	lon: number;
	altitude?: number;
	speed?: number;
	course?: number;
}

export interface TrackPoint {
	lat: number;
	lon: number;
	time: string;
	speed?: number;
	course?: number;
}

export interface APRSSymbol {
	table: number; // byte value: 47 = '/', 92 = '\'
	code: number;  // byte value of symbol code
}

export interface Message {
	id: string;
	from: string;
	to: string;
	body: string;
	msgNo?: string;
	state: MessageState;
	retries: number;
	inbound: boolean;
	timestamp: string;
	/** TNC2 path used for this outbound message. Empty/omitted = Direct or inbound. */
	path?: string;
}

export type MessageState = 0 | 1 | 2 | 3 | 4;
export const STATE_PENDING: MessageState = 0;
export const STATE_SENT: MessageState = 1;
export const STATE_ACKED: MessageState = 2;
export const STATE_REJECTED: MessageState = 3;
export const STATE_FAILED: MessageState = 4;

export interface Conversation {
	callsign: string;
	messages: Message[];
	unreadCount: number;
	lastActive: string;
	claimedBy?: string;
	claimedName?: string;
	claimedAt?: string;
	/** Per-conversation read marker. Inbound messages newer than this are unread. */
	lastReadAt?: string;
}

export interface Bulletin {
	id: string;
	from: string;
	bulletinId: string;
	body: string;
	timestamp: string;
	isAnnouncement: boolean;
}

export interface HealthResponse {
	status: string;
}

export type Role = 'observer' | 'plotter' | 'operator' | 'admin';

export type UserStatus = 'pending' | 'approved' | 'denied';

export interface SessionUser {
	id: string;
	name: string;
	role: Role;
	status: UserStatus;
	callsign?: string;
	token: string;
	connectedAt: string;
	lastActivity: string;
}

export interface PublicUser {
	id: string;
	name: string;
	role: Role;
	status: UserStatus;
	callsign?: string;
	connectedAt: string;
}

export interface ConfigResponse {
	transports: number;
	wsClients: number;
	authMode: string;
	needsSetup: boolean;
	messagePath: string;
	beaconPath: string;
}

export interface SetupData {
	callsign: string;
	ssid: number;
	comment: string;
	lat: number;
	lon: number;
	aprisEnabled: boolean;
	aprisHost: string;
	aprisPort: number;
	aprisFilter: string;
}

export type AnnotationCategory = 'incident' | 'resource' | 'checkpoint' | 'hazard' | 'route' | 'boundary' | 'assignment' | 'general' | 'aid' | 'staging' | 'shelter' | 'parking' | 'start' | 'finish';
export type AnnotationPriority = 'routine' | 'priority' | 'urgent' | 'emergency';

export interface Annotation {
	id: string;
	type: 'point' | 'line' | 'area';
	label: string;
	description?: string;
	geometry: string;
	style?: string;
	createdBy?: string;
	createdByName?: string;
	createdAt: string;
	updatedAt: string;
	category: AnnotationCategory;
	status: string;
	priority: AnnotationPriority;
	operationId?: string;
	missionIds: string[];
	resources?: string;
	reportedBy?: string;
	reportedAt?: string;
	resolvedAt?: string;
	expiresAt?: string;
	transmitting?: boolean;
	netId?: string;
	shortName?: string;
	sortOrder?: number;
	batchId?: string;
	batchLabel?: string;
}

/** Envelope returned by GPX/KML import, cross-net copy, and undo-delete. */
export interface ImportResult {
	batchId: string;
	batchLabel: string;
	netId?: string;
	count: number;
	annotations: Annotation[];
}

/** Result of POST /annotations/bulk-delete. */
export interface BulkDeleteResult {
	batchId?: string;
	batchLabel?: string;
	deleted: string[];
	deletedCount: number;
	skippedMissionLinked: string[];
	killedObjects: number;
	undoToken?: string;
	undoExpiresAt?: string;
}

/** Result of PATCH /annotations/batch-label. */
export interface RenameBatchResult {
	batchId: string;
	batchLabel: string;
	updated: number;
}

/** One annotation in a 409 "still transmitting" response. */
export interface TransmittingMember {
	id: string;
	label: string;
}

/** A set of annotations created by one bulk operation, derived client-side. */
export interface AnnotationBatch {
	id: string;
	label: string;
	netId?: string;
	count: number;
	/** Earliest createdAt among members. */
	createdAt: string;
	items: Annotation[];
	missionLinkedCount: number;
	checkpointCount: number;
}

export interface ActivityEntry {
	id: number;
	timestamp: string;
	userId?: string;
	userName?: string;
	action: string;
	target?: string;
	details?: string;
}

export interface ActivityResponse {
	entries: ActivityEntry[];
	total: number;
}

export interface TransportStatus {
	id: string;
	type: string;
	/** Display name: custom transport name if configured, else the type. */
	name?: string;
	connected: boolean;
	lastActivity?: string;
	error?: string;
	packetsRx: number;
	packetsTx: number;
	/** When this transport last delivered a decoded frame. */
	lastRx?: string;
	/** When this transport last accepted an outbound frame. */
	lastTx?: string;
}

// --- Net Control ---

export type NetStatus = 'draft' | 'open' | 'closed' | 'archived';
export type OperatorStatus = 'available' | 'assigned' | 'enroute' | 'onscene' | 'brb' | 'missing' | 'released';
export type TrafficType = 'none' | 'routine' | 'priority' | 'welfare' | 'emergency';
export type StationCategory = 'general' | 'command' | 'medical' | 'sag' | 'marshal' | 'fixed' | 'mobile' | 'tactical';
export type MissionStatus = 'open' | 'active' | 'complete';

export interface Net {
	id: string;
	name: string;
	type: string;
	frequency: string;
	ncsCallsign: string;
	ncsUserId: string;
	status: NetStatus;
	openedAt?: string;
	closedAt?: string;
	notes: string;
	missionBrief: string;
	opsViewLat?: number;
	opsViewLon?: number;
	opsViewZoom?: number;
	pinnedStations: string[];
	/** NWS weather watch area (internal/wxalert), per net. 0 = inherit the config default. */
	wxBufferMiles: number;
	wxExtraZones: string[];
	wxMuteAdvisories: boolean;
	/** false = inherit the config allowlist. */
	wxInterruptCustom: boolean;
	/** Used only when wxInterruptCustom is true. */
	wxInterruptEvents: string[];
	/** Selects the net-control vocabulary set (internal/netprofile). */
	profile: NetProfileID;
}

// --- Net profile / ride config (internal/netprofile) ---

export type NetProfileID = 'general' | 'bike-ride';

export interface PriorityTier {
	id: string;
	label: string;
	rank: number;
	description: string;
	examples: string[];
}

export interface RideRoute {
	id: string;
	name: string;
	distanceMiles: number;
	startTime?: string;
	cutoffAt?: string;
	division?: string;
}

export interface RideCutoffPolicy {
	courseOpensAt?: string;
	courseClosesAt?: string;
	mandatorySagAfterCutoff: boolean;
	declinedSagIsUnsupported: boolean;
	notes: string;
}

export interface NetRideConfig {
	netId: string;
	agencyName: string;
	eventName: string;
	eventDate: string;
	routes: RideRoute[];
	cutoff: RideCutoffPolicy;
	withholdBibOnSevereInjury: boolean;
	priorityTiers: PriorityTier[];
	division?: string;
	updatedAt: string;
}

export interface NetProfile {
	id: NetProfileID;
	label: string;
	description: string;
	panels: string[];
	annotationCategories: string[];
	checkInCategories: string[];
	defaultCheckInCategory: string;
	priorityLadderId: string;
	priorityTiers: PriorityTier[];
	hasRideConfig: boolean;
}

export interface NetProfileView {
	netId: string;
	profile: NetProfile;
	rideConfig?: NetRideConfig;
	effectivePriorityTiers: PriorityTier[];
}

// --- Ride phase (internal/ride/phase, WP5b) ---
//
// Only meaningful for a bike-ride profile net — GET/POST /nets/{id}/ride/phase
// answer 409 { code: 'profile_mismatch' } for any other profile. Phase is
// ALWAYS operator-set; `suggestion` is a live-computed hint the NCS must
// explicitly confirm via POST, never something the frontend should act on by
// itself. Never hardcode the phase label text elsewhere — RidePhaseID is the
// full legal set, in forward order.

export type RidePhaseID = 'pre-start' | 'launched' | 'mid-ride' | 'closing' | 'collapse' | 'reconcile';

export interface RidePhaseSuggestion {
	/** '' when the current phase's own trigger has not fired. */
	phase: RidePhaseID | '';
	reason: string;
}

/** GET/POST /nets/{id}/ride/phase response. */
export interface RidePhaseStatus {
	netId: string;
	phase: RidePhaseID;
	setBy: string;
	/** The operator's note. Required by the API for a backward move, optional for forward. */
	reason: string;
	updatedAt: string;
	suggestion: RidePhaseSuggestion;
}

// --- Course closure (internal/course, WP3) ---

export interface CourseConfig {
	netId: string;
	division: string;
	sweepLabel: string;
	leadLabel: string;
	closeRequiresSweep: boolean;
	autoSweepFromPassage: boolean;
	updatedAt: string;
}

export type ShutoffStatus = 'planned' | 'fired' | 'cancelled';

export interface ShutoffPoint {
	id: string;
	netId: string;
	division: string;
	name: string;
	lat: number;
	lon: number;
	routeMile?: number;
	annotationId?: string;
	scheduledAt: string;
	rerouteDirection: string;
	rerouteDestination: string;
	rerouteInstructions: string;
	staffedByCheckInId: string;
	status: ShutoffStatus;
	firedAt?: string;
	firedBy: string;
	fireNote: string;
	rerouteCount: number;
	createdAt: string;
	updatedAt: string;
}

export type RiderSupportStatus = 'supported' | 'unsupported';

export interface RiderException {
	id: string;
	netId: string;
	division: string;
	bib: string;
	bibWithheld: boolean;
	kind: string;
	supportStatus: RiderSupportStatus;
	reason: string;
	routeLabel: string;
	routeMile?: number;
	lat?: number;
	lon?: number;
	shutoffId?: string;
	sagRequestId?: string;
	reportedBy: string;
	recordedAt: string;
	statusChangedAt: string;
	statusChangedBy: string;
	note: string;
}

export interface SweepReport {
	id: string;
	netId: string;
	division: string;
	routeLabel: string;
	checkInId: string;
	reportedBy: string;
	routeMile?: number;
	lat?: number;
	lon?: number;
	lastRiderBib: string;
	estimatedSpeedMph?: number;
	note: string;
	reportedAt: string;
}

export type StationClosureState = 'open' | 'riders_clear' | 'sweep_passed' | 'closed';

export interface StationClosure {
	netId: string;
	checkpointId: string;
	division: string;
	state: StationClosureState;
	ridersClearAt?: string;
	ridersClearBy: string;
	sweepPassedAt?: string;
	sweepPassedBy: string;
	sweepPassageId?: string;
	closedAt?: string;
	closedBy: string;
	closedByOverride: boolean;
	overrideReason: string;
	reopenCount: number;
	note: string;
	updatedAt: string;
}

export interface StationView {
	checkpointId: string;
	label: string;
	category: string;
	sequenceNumber: number;
	closure: StationClosure;
	outOfOrder: boolean;
	passageCount: number;
}

export interface SweepPosition {
	lastCheckpointId: string;
	lastCheckpointSeq: number;
	lastPassageTime?: string;
	latestReport?: SweepReport;
	nextStationId: string;
	nextStationLabel: string;
	etaToNextMinutes?: number;
}

export interface CourseState {
	netId: string;
	config: CourseConfig;
	stations: StationView[];
	clearThroughSeq: number;
	clearThroughLabel: string;
	stationsOpen: number;
	stationsClosed: number;
	allStationsClosed: boolean;
	sweep: SweepPosition;
	shutoffs: ShutoffPoint[];
	nextShutoff?: ShutoffPoint;
	supportedExceptions: number;
	unsupportedExceptions: number;
	rerouteCountTotal: number;
	updatedAt: string;
}

// --- Ride mode: SAG (internal/ride, WP2) ---

export interface SAGLocation {
	kind: string;
	annotationId?: string;
	route?: string;
	mileMarker?: number;
	milesRemaining?: number;
	lat?: number;
	lon?: number;
	description?: string;
}

export interface SAGSlot {
	id: string;
	bib?: string;
	riderName?: string;
	note?: string;
	/** Legacy mirror of `bike === 'with_rider'`. Read `bike`. */
	hasBike: boolean;
	/** ride.Bike* — a SEPARATE axis from `disposition`; only 'with_rider' takes a rack. */
	bike: string;
	disposition: string;
	legId?: string;
	deliveredTo?: SAGLocation;
	updatedAt: string;
}

export interface SAGLeg {
	id: string;
	vehicleCheckInId: string;
	vehicleLabel: string;
	slotIds: string[];
	status: string;
	releaseReason?: string;
	overcommitted: boolean;
	dispatchedAt: string;
	enrouteAt?: string;
	onSceneAt?: string;
	loadedAt?: string;
	deliveredAt?: string;
	releasedAt?: string;
}

export interface SAGRequest {
	id: string;
	netId: string;
	division?: string;
	sequence: number;
	pickup: SAGLocation;
	dropoff: SAGLocation;
	reason: string;
	priority: string;
	status: string;
	needsVehicle: boolean;
	slots: SAGSlot[];
	legs: SAGLeg[];
	requestedBy: string;
	createdByName?: string;
	notes: string;
	cancelReason?: string;
	createdAt: string;
	updatedAt: string;
	closedAt?: string;
}

export interface SAGVehicle {
	netId: string;
	checkInId: string;
	division?: string;
	seats: number;
	rackSlots: number;
	notes: string;
	updatedAt: string;
}

export interface SAGVehicleStatus extends SAGVehicle {
	callsign: string;
	tacticalCall: string;
	checkInStatus: string;
	committedSeats: number;
	committedRacks: number;
	availableSeats: number;
	availableRacks: number;
	activeLegIds: string[];
	activeRequestIds: string[];
}

export interface RideSagConfig {
	priorities: string[];
	defaultCoursePriority: string;
	defaultStopPriority: string;
	reasons: string[];
	defaultSeats: number;
	defaultRackSlots: number;
	requireNameForHospitalStart: boolean;
}

export interface SAGBoard {
	netId: string;
	requests: SAGRequest[];
	vehicles: SAGVehicleStatus[];
	counts: Record<string, number>;
	config: RideSagConfig;
}

// --- Ride mode: supply / medical traffic (internal/ride, WP4) ---

export interface SupplyItem {
	item: string;
	quantity: number;
	unit?: string;
	note?: string;
	addedAt: string;
}

export interface SupplyETA {
	minutes: number;
	givenAt: string;
	dueAt: string;
	source?: string;
}

export interface SupplyRequest {
	id: string;
	netId: string;
	division?: string;
	requestedByCheckInId?: string;
	requestedByCall: string;
	location: string;
	locationAnnotationId?: string;
	milesRemaining?: number;
	routeId?: string;
	lat?: number;
	lon?: number;
	items: SupplyItem[];
	askedWhatElse: boolean;
	priority: string;
	notes?: string;
	status: string;
	createdAt: string;
	readBackAt?: string;
	readBackBy?: string;
	relayedAt?: string;
	relayedTo?: string;
	etas: SupplyETA[];
	deliveredAt?: string;
	cancelledAt?: string;
	cancelReason?: string;
	mergedIntoId?: string;
	updatedAt: string;
}

export interface MedicalNotification {
	id: string;
	netId: string;
	division?: string;
	reportedByCheckInId?: string;
	reportedByCall: string;
	bib?: string;
	bibWithheld: boolean;
	sex: string;
	age: string;
	location: string;
	milesRemaining?: number;
	routeId?: string;
	locationAnnotationId?: string;
	lat?: number;
	lon?: number;
	chiefComplaint: string;
	readBackAt?: string;
	readBackBy?: string;
	severity: string;
	priority: string;
	status: string;
	emsUnit?: string;
	etaMinutes?: number;
	etaGivenAt?: string;
	etaDueAt?: string;
	onSceneAt?: string;
	departedAt?: string;
	onSceneSeconds?: number;
	destination?: string;
	destinationName?: string;
	patientCount: number;
	/** Never present in a redacted payload (observer GET/WS). */
	patientName?: string;
	releasedAt?: string;
	cancelledAt?: string;
	cancelReason?: string;
	notes?: string;
	createdAt: string;
	updatedAt: string;
}

// --- Ride mode: SAG/supply/medical write inputs (internal/ride, WP7) ---
//
// Mirrors of the Go *Input structs the handlers in internal/server/sag.go
// and ride_traffic.go decode request bodies into. Field names/optionality
// match the json tags exactly — see those files, not this comment, if the
// two ever disagree.

export interface SlotInput {
	bib: string;
	riderName: string;
	note: string;
	/** Legacy; omitted -> server defaults to true. Prefer `bike`. */
	hasBike?: boolean | null;
	/** ride.Bike*; omitted -> derived from hasBike. */
	bike?: string;
	/** AddSlot only: put this rider straight onto a leg already on its way. */
	attachToLegId?: string;
	/** BIKE RACKS only — a seat overflow is refused outright. */
	allowOvercommit?: boolean;
}

/** POST .../legs/{legId}/load — `bike` is slotId -> ride.Bike*, as the driver reported it. */
export interface LoadInput {
	slotIds?: string[];
	bike?: Record<string, string>;
}

export interface CreateRequestInput {
	pickup: SAGLocation;
	dropoff: SAGLocation;
	reason: string;
	/** '' -> server derives from the net's ladder + pickup.kind. */
	priority: string;
	slots: SlotInput[];
	requestedBy: string;
	notes: string;
	division?: string;
}

export interface UpdateRequestInput {
	pickup: SAGLocation;
	dropoff: SAGLocation;
	reason: string;
	priority: string;
	notes: string;
	requestedBy: string;
}

export interface DispatchInput {
	vehicleCheckInId: string;
	/** Empty -> every currently waiting, unassigned slot. */
	slotIds: string[];
	allowOvercommit: boolean;
}

/** GET /nets/{id}/sag/config. */
export interface SAGConfigResponse {
	config: RideSagConfig;
	pickupKinds: string[];
	dropoffKinds: string[];
	bikeKinds: string[];
}

/**
 * The 409 body writeSAGError sends for an *ride.OverCapacityError.
 *
 * The two dimensions are NOT the same kind of answer:
 *  - `code: 'seats_exceeded'` (`fatal: true`) is a REFUSAL. A seat is a
 *    seatbelt. Never offer an override — retrying with allowOvercommit is
 *    refused again by the server.
 *  - `code: 'racks_exceeded'` is a QUESTION. A bike can ride in the bed of a
 *    truck, so confirm-and-proceed is legitimate.
 */
export interface OverCapacityBody {
	error: string;
	code: 'seats_exceeded' | 'racks_exceeded';
	fatal: boolean;
	seatsExceeded: boolean;
	racksExceeded: boolean;
	committedSeats: number;
	seats: number;
	needSeats: number;
	committedRacks: number;
	rackSlots: number;
	needRacks: number;
}

export interface SupplyItemInput {
	item: string;
	quantity: number;
	unit?: string;
	note?: string;
}

export interface CreateSupplyInput {
	requestedByCheckInId?: string;
	requestedByCall: string;
	location: string;
	locationAnnotationId?: string;
	milesRemaining?: number | null;
	routeId?: string;
	lat?: number | null;
	lon?: number | null;
	items: SupplyItemInput[];
	askedWhatElse: boolean;
	priority: string;
	notes?: string;
	division?: string;
}

export interface AddItemsInput {
	items: SupplyItemInput[];
	/** Lets the "what else?" prompt flip the flag on an append. */
	askedWhatElse?: boolean;
}

export interface ReadbackInput {
	/** false = requester corrected something; the record stays draft/reported. */
	confirmed: boolean;
	readBackBy?: string;
	correction?: string;
}

export interface RelayInput {
	relayedTo: string;
}

export interface RideETAInput {
	minutes: number;
	source?: string;
}

export interface CancelInput {
	reason: string;
}

export interface SupplyCatalogEntry {
	item: string;
	defaultTier: string;
	when?: string;
}

export interface CreateMedicalInput {
	reportedByCheckInId?: string;
	reportedByCall: string;
	bib?: string;
	sex: string;
	age: string;
	location: string;
	milesRemaining?: number | null;
	routeId?: string;
	locationAnnotationId?: string;
	lat?: number | null;
	lon?: number | null;
	chiefComplaint: string;
	severity: string;
	priority: string;
	notes?: string;
	division?: string;
	/** The composer may take the read-back in the same breath as the report. */
	readbackConfirmed?: boolean;
	readBackBy?: string;
}

export interface MedicalETAInput {
	emsUnit: string;
	minutes: number;
}

export interface OnSceneInput {
	emsUnit?: string;
	/** ISO timestamp; omitted = now (lets a late entry say "5 min ago"). */
	at?: string;
}

export interface DepartInput {
	destination: string;
	destinationName: string;
	patientCount: number;
	/** Only honored for destination hospital|start; else the server 400s. */
	patientName?: string;
	at?: string;
}

export interface ReleaseInput {
	reason: string;
	at?: string;
}

// --- Ride reconciliation (internal/ride/reconcile, WP5) ---

export interface HandoffItem {
	id: string;
	netId: string;
	division?: string;
	kind: 'awaiting_reply' | 'pending_action' | 'fyi';
	summary: string;
	sentTo: string;
	replyTo: string;
	refType?: string;
	refId?: string;
	dueAt?: string;
	status: 'open' | 'resolved' | 'cancelled';
	handoverCount: number;
	createdBy: string;
	createdAt: string;
	resolvedBy?: string;
	resolvedAt?: string;
	resolution?: string;
}

export interface ShiftHandoff {
	id: string;
	netId: string;
	division?: string;
	fromCallsign: string;
	toCallsign: string;
	at: string;
	openItemIds: string[];
	briefing: string;
	acknowledgedAt?: string;
	acknowledgedBy?: string;
}

export interface ExceptionCounts {
	supported: number;
	unsupported: number;
	byKind: Record<string, number>;
}

export interface Accounting {
	netId: string;
	sweepComplete: boolean;
	sweepDetail: string;
	counts: ExceptionCounts;
	openExceptions: RiderException[];
	computedAt: string;
}

export interface CloseoutItem {
	key: string;
	label: string;
	done: boolean;
	blocking: boolean;
	count: number;
	total: number;
	detail: string;
}

export interface CloseoutStatus {
	netId: string;
	ready: boolean;
	items: CloseoutItem[];
	computedAt: string;
}

export interface BriefingLine {
	label: string;
	value: string;
	severity: 'info' | 'warn' | 'urgent';
	refType?: string;
	refId?: string;
}

export interface BriefingSection {
	key: string;
	title: string;
	lines: BriefingLine[];
}

export interface AwaitingReply {
	messageId: string;
	to: string;
	body: string;
	sentAt: string;
	state: 'pending' | 'sent';
}

export interface ShiftBriefing {
	netId: string;
	netName: string;
	ncsCallsign: string;
	ncsSince?: string;
	shiftDueAt?: string;
	openItems: HandoffItem[];
	awaitingReplies: AwaitingReply[];
	accounting: Accounting;
	closeout: CloseoutStatus;
	sections: BriefingSection[];
	recentEvents: NetEvent[];
	generatedAt: string;
}

export interface TrackedStation {
	callsign: string;
	autoLinked: boolean;
}

export interface NetCheckIn {
	id: string;
	netId: string;
	callsign: string;
	tacticalCall: string;
	operatorName: string;
	status: OperatorStatus;
	traffic: TrafficType;
	source: 'aprs' | 'voice';
	category: StationCategory;
	location: string;
	lat?: number;
	lon?: number;
	missionIds: string[];
	trackedStations: TrackedStation[];
	checkedInAt: string;
	checkedOutAt?: string;
	lastHeard: string;
	missedRollCalls: number;
}

export interface NetMission {
	id: string;
	netId: string;
	title: string;
	description: string;
	priority: string;
	status: MissionStatus;
	/** @deprecated Create-time input only; always "" in responses. Do not read. */
	assignedTo?: string;
	/** Create-time input only (check-in IDs). Never present on a loaded mission. */
	assigneeIds?: string[];
	location: string;
	lat?: number;
	lon?: number;
	createdAt: string;
	completedAt?: string;
}

/**
 * POST /nets/{id}/missions response: the mission plus the operators that were
 * actually assigned as part of the same operation, and the assignee tokens the
 * server could not resolve to a roster check-in (never an error — the mission
 * is still created).
 */
export interface CreateMissionResponse extends NetMission {
	assignedOperators: NetCheckIn[];
	skippedAssignees: string[];
}

export type NoteCategory = 'general' | 'medical' | 'logistical' | 'tactical' | 'weather' | 'resource' | 'hazard' | 'comms';
export type NoteSeverity = 'info' | 'routine' | 'priority' | 'urgent';

export interface NetNote {
	id: string;
	netId: string;
	checkInId?: string;
	missionId?: string;
	authorId: string;
	authorName: string;
	content: string;
	category: NoteCategory;
	severity?: NoteSeverity;
	pinned: boolean;
	createdAt: string;
}

export interface NetEvent {
	id: string;
	netId: string;
	type: string;
	callsign: string;
	summary: string;
	details: string;
	createdAt: string;
}

export interface TacticalAlias {
	callsign: string;
	alias: string;
	assignedBy: string;
	updatedAt: string;
}

export interface AnnotationTemplate {
	id: string;
	name: string;
	pack: string;
	category: AnnotationCategory;
	type: 'point' | 'line' | 'area';
	defaultPriority: AnnotationPriority;
	description: string;
}

export interface Operation {
	id: string;
	name: string;
	description?: string;
	status: string;
	createdBy?: string;
	createdAt: string;
	archivedAt?: string;
}

export interface NetSummary {
	netId: string;
	name: string;
	duration: string;
	totalCheckIns: number;
	totalMissions: number;
	trafficCounts: Record<string, number>;
}

// --- ICS-309 ---

export interface ICS309Header {
	incidentName: string;
	dateFrom: string;
	dateTo: string;
	operatorName: string;
	stationId: string;
}

export interface ICS309Row {
	dateTime: string;
	from: string;
	to: string;
	subject: string;
	method: string;
}

export interface ICS309Report {
	header: ICS309Header;
	rows: ICS309Row[];
}

// --- Checkpoint Progress ---

export interface CheckpointMeta {
	annotationId: string;
	netId: string;
	sequenceNumber: number;
	expectedTime?: string;
	openedAt?: string;
	closedAt?: string;
}

export interface CheckpointPassage {
	id: string;
	checkpointId: string;
	netId: string;
	label: string;
	passageTime: string;
	direction: string;
	reportedBy: string;
	notes?: string;
}

export interface CheckpointWithPassages {
	annotation: Annotation;
	meta: CheckpointMeta;
	passages: CheckpointPassage[];
	passageCount: number;
	latestPassage?: string;
}

export interface ProgressElement {
	label: string;
	lastCheckpointId: string;
	lastCheckpointSeq: number;
	lastPassageTime: string;
}

export interface CheckpointProgress {
	netId: string;
	checkpoints: CheckpointWithPassages[];
	elements: ProgressElement[];
}

// --- Tile Cache ---

export interface TileCacheStatus {
	enabled: boolean;
	tileCount: number;
	diskUsage: number;
	maxZoom?: number;
}

export interface TilePreloadProgress {
	done: number;
	total: number;
	skipped: number;
}

// --- Live GPS (own position) ---
// Mirrors internal/gps.Fix / gps.Status and the server's GET /api/gps +
// "own_position" WS frame byte-for-byte. See internal/server/gps.go.

export type GpsFixMode = 0 | 1 | 2 | 3;

export interface GpsFix {
	mode: GpsFixMode;
	lat: number;
	lon: number;
	altitude?: number; // meters MSL
	hasAltitude: boolean;
	speedKnots: number;
	course: number;
	hasCourse: boolean;
	satellites?: number;
	hdop?: number;
	accuracy?: number; // meters, 0/undefined = unknown
	time?: string;
	receivedAt: string;
}

export interface GpsStatus {
	enabled: boolean;
	type?: 'gpsd' | 'nmea' | 'modemmanager';
	target?: string;
	connected: boolean;
	error?: string;
	fix: GpsFix | null;
	ageMillis: number | null;
	stale: boolean;
}

// --- Settings ---

export interface SettingsResponse {
	station: StationSettings;
	server: ServerSettings;
	beacon: BeaconSettings;
	session: SessionSettings;
	logging: LoggingSettings;
	transports: TransportSettings[];
	tileCache: TileCacheSettings;
	weather: WeatherSettings;
	store: StoreSettings;
	gps: GpsSettings;
	what3words: What3WordsSettings;
	wxAlerts: WxAlertsSettings;
}

export interface StationSettings {
	callsign: string;
	ssid: number;
	lat: number;
	lon: number;
	symbolTable: string;
	symbolCode: string;
	comment: string;
	trackMaxPoints: number;
	staleTimeout: string;
	dedupWindow: string;
	tacticalAliases?: Record<string, string>;
	/** TNC2 digipeater path for messages and acks (e.g. "WIDE1-1,WIDE2-1"). */
	messagePath: string;
	/** TNC2 digipeater path for beacons and APRS objects/items. */
	beaconPath: string;
}

export interface ServerSettings {
	listen: string;
}

export interface BeaconSettings {
	enabled: boolean;
	interval: string;
	comment: string;
}

export interface SessionSettings {
	pinConfigured: boolean;
	pin?: string;
	inactivityTimeout: string;
}

export interface LoggingSettings {
	level: string;
}

export interface TransportSettings {
	type: string;
	/** Optional human-friendly label for this transport instance. */
	name?: string;
	host?: string;
	port?: number;
	device?: string;
	baud?: number;
	filter?: string;
	callsign?: string;
	passcode?: string;
}

export interface SerialPortInfo {
	name: string;
	label: string;
	present: boolean;
	isUSB?: boolean;
	vid?: string;
	pid?: string;
	serialNumber?: string;
	product?: string;
	stablePath?: string;
	suggestedProfile?: string;
	highlight?: boolean;
}

export interface SerialProfile {
	id: string;
	label: string;
	baud: number;
	help: string;
}

export interface SerialPortsResponse {
	hostOS: string;
	ports: SerialPortInfo[];
	profiles: SerialProfile[];
	baudRates: number[];
	error?: string;
}

export interface KissTncInfo {
	name: string;
	label: string;
	host: string;
	port: number;
	source: string;
	local?: boolean;
	highlight?: boolean;
	portsNote?: string;
	present?: boolean;
}

export interface KissTncsResponse {
	hostOS: string;
	tncs: KissTncInfo[];
	error?: string;
}

// --- APRS-IS Filter Builder ---

export type FilterType =
	| 'range'        // r/lat/lon/dist
	| 'area'         // a/latN/lonW/latS/lonE
	| 'type'         // t/types or t/types/call/dist
	| 'prefix'       // p/prefix1/prefix2...
	| 'budlist'      // b/call1/call2...
	| 'object'       // o/obj1/obj2...
	| 'strictObject' // os/obj1/obj2...
	| 'symbol'       // s/pri/alt/over
	| 'digipeater'   // d/call1/call2...
	| 'entry'        // e/call1/call2...
	| 'group'        // g/call1/call2...
	| 'unproto'      // u/unproto1/unproto2...
	| 'qConstruct'   // q/con/I
	| 'myRange'      // m/dist
	| 'friendRange'; // f/call/dist

export interface FilterRule {
	type: FilterType;
	exclude: boolean;

	// Range (r/lat/lon/dist) and shared geo fields
	lat?: number;
	lon?: number;
	dist?: number;

	// Area (a/latN/lonW/latS/lonE)
	latN?: number;
	lonW?: number;
	latS?: number;
	lonE?: number;

	// Type (t/types or t/types/call/dist)
	types?: string;
	callForType?: string;
	distForType?: number;

	// List-based (prefix, budlist, object, strictObject, digipeater, entry, group, unproto)
	items?: string[];

	// Symbol (s/pri/alt/over)
	primaryTable?: string;
	altTable?: string;
	overlay?: string;

	// Q-Construct (q/con/I)
	qCodes?: string;
	iFlag?: boolean;

	// Friend Range (f/call/dist)
	friendCall?: string;
}

export interface TileCacheSettings {
	enabled: boolean;
	dataDir: string;
	tileUrl: string;
	maxZoom: number;
}

export interface WeatherSettings {
	retentionDays: number;
	alerts?: Record<string, WeatherAlertThreshold>;
	units: string;
}

export interface StoreSettings {
	path: string;
}

export interface GpsSettings {
	enabled: boolean;
	type: 'gpsd' | 'nmea' | 'modemmanager';
	host: string;
	port: number;
	device: string; // nmea serial device, or modem index / D-Bus object path for modemmanager
	baud: number;
	minInterval: string;
	staleAfter: string;
	useForBeacon: boolean;
}

export interface SettingsUpdateResponse {
	restartRequired: boolean;
}

// --- what3words ---

export interface W3WStatus {
	configured: boolean;
	enabled: boolean;
}

export interface W3WResult {
	words: string;
	lat: number;
	lon: number;
	nearestPlace: string;
	country: string;
	language: string;
}

export interface W3WSuggestion {
	words: string;
	nearestPlace: string;
	country: string;
	distanceToFocusKm: number;
	rank: number;
}

export interface W3WSuggestResponse {
	suggestions: W3WSuggestion[];
}

export interface What3WordsSettings {
	enabled: boolean;
	apiKeyConfigured: boolean;
	apiKeySource: 'env' | 'config' | 'none';
	/** Write-only; never populated on GET. */
	apiKey?: string;
	baseUrl: string;
	results: number;
}

// --- Packet Inspector ---

export type APRSPacketType = 'position' | 'message' | 'object' | 'item' | 'weather' | 'status' | 'telemetry' | 'micE' | 'query' | 'thirdParty' | 'unknown';

export interface APRSAddress {
	call: string;
	ssid?: number;
	hBit?: boolean;
}

export interface RawPacket {
	type: 'packet';
	raw: string;
	timestamp: string;
	source: string;
	packetType: APRSPacketType;
	from: APRSAddress;
	to: APRSAddress;
	path: APRSAddress[];
	packet: Record<string, unknown>;
}
