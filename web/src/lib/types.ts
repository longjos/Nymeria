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

export interface WeatherAlertThreshold {
	min?: number;
	max?: number;
}

export interface WeatherConfig {
	retentionDays: number;
	alerts?: Record<string, WeatherAlertThreshold>;
	units: 'metric' | 'imperial';
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
	source: string;
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

export interface SessionUser {
	id: string;
	name: string;
	role: Role;
	callsign?: string;
	token: string;
	connectedAt: string;
	lastActivity: string;
}

export interface PublicUser {
	id: string;
	name: string;
	role: Role;
	callsign?: string;
	connectedAt: string;
}

export interface ConfigResponse {
	transports: number;
	wsClients: number;
	pinRequired: boolean;
	needsSetup: boolean;
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
	connected: boolean;
	lastActivity?: string;
	error?: string;
	packetsRx: number;
	packetsTx: number;
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
	assignment: string;
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
	assignedTo: string;
	location: string;
	lat?: number;
	lon?: number;
	createdAt: string;
	completedAt?: string;
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
	host?: string;
	port?: number;
	device?: string;
	baud?: number;
	filter?: string;
	callsign?: string;
	passcode?: string;
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

export interface SettingsUpdateResponse {
	restartRequired: boolean;
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
