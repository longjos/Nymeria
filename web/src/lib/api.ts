import type {
	Station, Message, Conversation, Bulletin, HealthResponse, TransportStatus,
	SessionUser, PublicUser, ConfigResponse, SetupData, Annotation, ActivityResponse,
	Net, NetCheckIn, NetMission, NetNote, NetEvent, NetSummary, TacticalAlias, CreateMissionResponse,
	AnnotationTemplate, Operation, ICS309Report, TileCacheStatus,
	WeatherReading, WeatherConfig, TelemetryReading, TelemetryReadingsResponse,
	SettingsResponse, SettingsUpdateResponse, SerialPortsResponse, KissTncsResponse,
	StationSettings, ServerSettings, TransportSettings, BeaconSettings,
	SessionSettings, LoggingSettings, WeatherSettings, TileCacheSettings,
	CheckpointWithPassages, CheckpointMeta, CheckpointPassage, CheckpointProgress,
	ImportResult, BulkDeleteResult, RenameBatchResult, GpsStatus, GpsSettings,
	W3WStatus, W3WResult, W3WSuggestResponse, What3WordsSettings,
	WxSnapshot, WxAlert, WxLinkStatus, WxFootprintSummary, WxPolygonGeometry,
	WxZoneRef, WxZone, WxEventType, WxNetWatch, WxNetWatchZones, WxRelayRequest,
	WxRelayResult, WxAlertsSettings,
	NetProfileView, NetProfile, NetRideConfig, RidePhaseStatus, CourseState, SAGBoard, SAGRequest, SAGVehicleStatus,
	SAGConfigResponse, SAGLocation, CreateRequestInput, UpdateRequestInput, SlotInput, LoadInput, DispatchInput,
	MedicalNotification, SupplyRequest, SupplyCatalogEntry, CreateSupplyInput, AddItemsInput,
	ReadbackInput, RelayInput, RideETAInput, CancelInput, CreateMedicalInput, MedicalETAInput,
	OnSceneInput, DepartInput, ReleaseInput,
	HandoffItem, ShiftHandoff, ShiftBriefing, CloseoutStatus, Accounting,
	StationClosure, CourseConfig, ShutoffPoint, RiderException, SweepReport, StationView, SweepPosition
} from './types';

const BASE = '/api';

const TOKEN_KEY = 'nymeria_token';

let authToken: string | null = null;

export function setAuthToken(token: string | null) {
	authToken = token;
	try {
		if (token) {
			localStorage.setItem(TOKEN_KEY, token);
		} else {
			localStorage.removeItem(TOKEN_KEY);
		}
	} catch {
		// localStorage may be unavailable (SSR, privacy mode)
	}
}

export function loadSavedToken(): string | null {
	try {
		return localStorage.getItem(TOKEN_KEY);
	} catch {
		return null;
	}
}

function headers(): Record<string, string> {
	const h: Record<string, string> = { 'Content-Type': 'application/json' };
	if (authToken) {
		h['Authorization'] = `Bearer ${authToken}`;
	}
	return h;
}

/**
 * An HTTP failure from the API. `status` and `body` let callers branch on
 * documented status codes (403 / 404 / 409 / 410) and read structured details
 * such as the `transmitting` array on a 409.
 */
export class ApiError extends Error {
	status: number;
	body: Record<string, unknown>;

	constructor(status: number, message: string, body: Record<string, unknown>) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.body = body;
	}
}

/** Build an ApiError from a non-2xx response, preferring the server's `error` field. */
async function failure(res: Response): Promise<ApiError> {
	let body: Record<string, unknown> = {};
	try {
		const parsed = await res.json();
		if (parsed && typeof parsed === 'object') body = parsed as Record<string, unknown>;
	} catch {
		// Non-JSON error body (proxy error page, empty 502) — fall back to the status.
	}
	const message = typeof body.error === 'string' && body.error
		? body.error
		: `API error: ${res.status}`;
	return new ApiError(res.status, message, body);
}

async function get<T>(path: string): Promise<T> {
	const res = await fetch(`${BASE}${path}`, { headers: headers() });
	if (!res.ok) throw await failure(res);
	return res.json();
}

async function post<T>(path: string, body: unknown): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method: 'POST',
		headers: headers(),
		body: JSON.stringify(body)
	});
	if (!res.ok) throw await failure(res);
	// A 204 (e.g. POST /wx/alerts/{id}/ack) has no body to parse.
	if (res.status === 204) return undefined as T;
	return res.json();
}

async function put<T>(path: string, body: unknown): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method: 'PUT',
		headers: headers(),
		body: JSON.stringify(body)
	});
	if (!res.ok) throw await failure(res);
	return res.json();
}

async function patch<T>(path: string, body?: unknown): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method: 'PATCH',
		headers: headers(),
		...(body === undefined ? {} : { body: JSON.stringify(body) })
	});
	if (!res.ok) throw await failure(res);
	return res.json();
}

async function del<T>(path: string): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		method: 'DELETE',
		headers: headers()
	});
	if (!res.ok) throw await failure(res);
	return res.json();
}

async function uploadForm<T>(path: string, form: FormData): Promise<T> {
	const h: Record<string, string> = {};
	if (authToken) h['Authorization'] = `Bearer ${authToken}`;
	const res = await fetch(`${BASE}${path}`, {
		method: 'POST',
		headers: h,
		body: form
	});
	if (!res.ok) throw await failure(res);
	return res.json();
}

export const api = {
	// Public
	health: () => get<HealthResponse>('/health'),
	config: () => get<ConfigResponse>('/config'),
	setup: (data: SetupData) => post<{ status: string; restartRequired: boolean }>('/setup', data),

	// Session
	login: (name: string, savedToken?: string) => post<SessionUser>('/session', { name, savedToken }),
	session: () => get<SessionUser>('/session'),
	logout: () => del<{ status: string }>('/session'),

	// Access approval (admin)
	approveUser: (userId: string, role: string) => post<SessionUser>('/session/approve', { userId, role }),
	denyUser: (userId: string) => post<{ userId: string; status: string }>('/session/deny', { userId }),
	getPending: () => get<SessionUser[]>('/session/pending'),

	// Users
	users: () => get<PublicUser[]>('/users'),
	updateUserRole: (id: string, role: string) => put<{ id: string; role: string }>(`/users/${id}/role`, { role }),
	removeUser: (id: string) => del<{ id: string; status: string }>(`/users/${id}`),

	// Stations
	stations: () => get<Station[]>('/stations'),
	stationsInBounds: (s: number, w: number, n: number, e: number) =>
		get<Station[]>(`/stations?bounds=${s},${w},${n},${e}`),
	searchStations: (q: string) => get<Station[]>(`/stations?q=${encodeURIComponent(q)}`),
	station: (callsign: string) => get<Station>(`/stations/${encodeURIComponent(callsign)}`),

	// Bulletins
	bulletins: () => get<Bulletin[]>('/bulletins'),

	// Messages
	conversations: () => get<Conversation[]>('/messages'),
	messages: (callsign: string) => get<Message[]>(`/messages/${encodeURIComponent(callsign)}`),
	sendMessage: (to: string, body: string, path?: string) =>
		post<Message>('/messages', path !== undefined ? { to, body, path } : { to, body }),
	claimConversation: (callsign: string, userId: string, userName: string) =>
		post<unknown>(`/messages/${encodeURIComponent(callsign)}/claim`, { userId, userName }),
	unclaimConversation: (callsign: string) =>
		del<unknown>(`/messages/${encodeURIComponent(callsign)}/claim`),
	markConversationRead: (callsign: string) =>
		post<{ callsign: string; unreadCount: number; lastReadAt: string | null }>(
			`/messages/${encodeURIComponent(callsign)}/read`,
			{}
		),

	// Transports
	transports: () => get<TransportStatus[]>('/transports'),

	// Live GPS
	gps: () => get<GpsStatus>('/gps'),

	// Annotations
	annotations: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return get<Annotation[]>(`/annotations${qs}`);
	},
	createAnnotation: (ann: Partial<Annotation>) => post<Annotation>('/annotations', ann),
	updateAnnotation: (id: string, ann: Partial<Annotation>) => put<Annotation>(`/annotations/${id}`, ann),
	deleteAnnotation: (id: string) => del<{ status: string }>(`/annotations/${id}`),
	changeAnnotationStatus: (id: string, status: string) => post<Annotation>(`/annotations/${id}/status`, { status }),
	promoteAnnotation: (id: string) => post<{ annotation: Annotation; mission: NetMission }>(`/annotations/${id}/promote`, {}),
	linkAnnotation: (id: string, missionId: string) =>
		post<Annotation>(`/annotations/${id}/link`, { missionId }),
	unlinkAnnotation: (id: string, missionId?: string) => {
		const qs = missionId ? `?missionId=${encodeURIComponent(missionId)}` : '';
		return del<Annotation>(`/annotations/${id}/link${qs}`);
	},

	// Templates & Operations
	annotationTemplates: () => get<AnnotationTemplate[]>('/annotation-templates'),
	operations: () => get<Operation[]>('/operations'),
	operation: (id: string) => get<Operation>(`/operations/${id}`),
	createOperation: (data: Partial<Operation>) => post<Operation>('/operations', data),
	archiveOperation: (id: string) => post<Operation>(`/operations/${id}/archive`, {}),

	// APRS Object Bridge
	transmitAnnotation: (id: string) => post<Annotation>(`/annotations/${id}/transmit`, {}),
	stopTransmitAnnotation: (id: string) => del<{ status: string }>(`/annotations/${id}/transmit`),

	// Activity
	activity: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return get<ActivityResponse>(`/activity${qs}`);
	},
	activityExportUrl: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return `${BASE}/activity/export${qs}`;
	},

	// Net Control
	nets: () => get<Net[]>('/nets'),
	net: (id: string) => get<{ net: Net; checkIns: NetCheckIn[]; missions: NetMission[]; annotations: Annotation[] }>(`/nets/${id}`),
	createNet: (data: Partial<Net>) => post<Net>('/nets', data),
	openNet: (id: string) => post<Net>(`/nets/${id}/open`, {}),
	closeNet: (id: string) => post<{ net: Net; summary: NetSummary }>(`/nets/${id}/close`, {}),
	transferNCS: (id: string, callsign: string, userId: string) => post<Net>(`/nets/${id}/transfer`, { callsign, userId }),
	checkIn: (netId: string, callsign: string, traffic?: string, category?: string) => post<NetCheckIn>(`/nets/${netId}/checkin`, { callsign, traffic, category }),
	updateCheckIn: (netId: string, ciId: string, data: Partial<NetCheckIn>) => put<NetCheckIn>(`/nets/${netId}/checkin/${ciId}`, data),
	checkOut: (netId: string, ciId: string) => post<{ status: string }>(`/nets/${netId}/checkout/${ciId}`, {}),
	createMission: (netId: string, data: Partial<NetMission>) => post<CreateMissionResponse>(`/nets/${netId}/missions`, data),
	updateMission: (netId: string, mId: string, data: Partial<NetMission>) => put<NetMission>(`/nets/${netId}/missions/${mId}`, data),
	addNetNote: (netId: string, data: { checkInId?: string; missionId?: string; content: string; category?: string; severity?: string }) => post<NetNote>(`/nets/${netId}/notes`, data),
	netEvents: (netId: string) => get<NetEvent[]>(`/nets/${netId}/events`),
	netNotes: (netId: string) => get<NetNote[]>(`/nets/${netId}/notes`),
	toggleNotePin: (netId: string, noteId: string) => patch<NetNote>(`/nets/${netId}/notes/${noteId}/pin`),
	initiateRollCall: (netId: string) => post<{ status: string }>(`/nets/${netId}/rollcall`, {}),
	recordRollCallResponse: (netId: string, ciId: string) => post<{ status: string }>(`/nets/${netId}/rollcall/${ciId}`, {}),
	searchOperators: (q: string) => get<Station[]>(`/nets/search?q=${encodeURIComponent(q)}`),
	assignMission: (netId: string, ciId: string, missionId: string) =>
		post<NetCheckIn>(`/nets/${netId}/checkin/${ciId}/assign`, { missionId }),
	unassignMission: (netId: string, ciId: string, missionId: string) =>
		del<NetCheckIn>(`/nets/${netId}/checkin/${ciId}/assign?missionId=${encodeURIComponent(missionId)}`),
	// Mission-scoped mirrors of the two above — same manager operation,
	// addressable from whichever object the UI is holding.
	assignMissionOperator: (netId: string, mId: string, ciId: string) =>
		post<NetCheckIn>(`/nets/${netId}/missions/${mId}/operators/${ciId}`, {}),
	unassignMissionOperator: (netId: string, mId: string, ciId: string) =>
		del<NetCheckIn>(`/nets/${netId}/missions/${mId}/operators/${ciId}`),
	addTrackedStation: (netId: string, ciId: string, callsign: string) =>
		post<NetCheckIn>(`/nets/${netId}/checkin/${ciId}/devices`, { callsign }),
	removeTrackedStation: (netId: string, ciId: string, callsign: string) =>
		del<NetCheckIn>(`/nets/${netId}/checkin/${ciId}/devices/${encodeURIComponent(callsign)}`),
	setOpsView: (netId: string, lat: number, lon: number, zoom: number) =>
		post<Net>(`/nets/${netId}/opsview`, { lat, lon, zoom }),
	rosterExportUrl: (netId: string) => `${BASE}/nets/${netId}/roster/export`,

	// Pinned stations
	pinStation: (netId: string, callsign: string) =>
		post<Net>(`/nets/${netId}/pin/${encodeURIComponent(callsign)}`, {}),
	unpinStation: (netId: string, callsign: string) =>
		del<Net>(`/nets/${netId}/pin/${encodeURIComponent(callsign)}`),
	reorderPins: (netId: string, callsigns: string[]) =>
		put<Net>(`/nets/${netId}/pins`, { callsigns }),

	// Checkpoint progress
	getCheckpoints: (netId: string) => get<CheckpointWithPassages[]>(`/nets/${netId}/checkpoints`),
	getProgress: (netId: string) => get<CheckpointProgress>(`/nets/${netId}/progress`),
	updateCheckpointMeta: (netId: string, cpId: string, data: Partial<CheckpointMeta>) =>
		put<CheckpointMeta>(`/nets/${netId}/checkpoints/${cpId}/meta`, data),
	logPassage: (netId: string, cpId: string, data: { label: string; direction?: string; notes?: string }) =>
		post<CheckpointPassage>(`/nets/${netId}/checkpoints/${cpId}/passages`, data),

	// Net-scoped annotations
	netAnnotations: (netId: string) => get<Annotation[]>(`/nets/${netId}/annotations`),
	importAnnotations: async (file: File, batchId?: string): Promise<ImportResult> => {
		const form = new FormData();
		form.append('file', file);
		if (batchId) form.append('batchId', batchId);
		return uploadForm<ImportResult>(`/annotations/import`, form);
	},
	importNetAnnotations: async (netId: string, file: File, batchId?: string): Promise<ImportResult> => {
		const form = new FormData();
		form.append('file', file);
		if (batchId) form.append('batchId', batchId);
		return uploadForm<ImportResult>(`/nets/${netId}/annotations/import`, form);
	},
	copyNetAnnotations: (netId: string, sourceNetId: string) =>
		post<ImportResult>(`/nets/${netId}/annotations/copy/${sourceNetId}`, {}),

	// Imported annotation sets (batches)
	bulkDeleteAnnotations: (payload: {
		batchId?: string;
		ids?: string[];
		includeMissionLinked?: boolean;
		stopTransmit?: boolean;
	}) => post<BulkDeleteResult>('/annotations/bulk-delete', payload),
	undoDeleteAnnotations: (undoToken: string) =>
		post<ImportResult>('/annotations/undo-delete', { undoToken }),
	renameAnnotationBatch: (batchId: string, batchLabel: string) =>
		patch<RenameBatchResult>('/annotations/batch-label', { batchId, batchLabel }),

	// Tactical Aliases
	tacticalAliases: () => get<TacticalAlias[]>('/tactical'),
	setTacticalAlias: (callsign: string, alias: string) =>
		put<TacticalAlias>(`/tactical/${encodeURIComponent(callsign)}`, { alias }),
	deleteTacticalAlias: (callsign: string) =>
		del<{ status: string }>(`/tactical/${encodeURIComponent(callsign)}`),

	// ICS-309 Communications Log
	ics309: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return get<ICS309Report>(`/ics309${qs}`);
	},
	ics309ExportUrl: (params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return `${BASE}/ics309/export${qs}`;
	},

	// NWS Alerts (internal/wxalert)
	wxAlerts: () => get<WxSnapshot>('/wx/alerts'),
	wxAlert: (id: string) => get<{ alert: WxAlert; history: WxAlert[] }>(`/wx/alerts/${encodeURIComponent(id)}`),
	ackWxAlert: (id: string) => post<void>(`/wx/alerts/${encodeURIComponent(id)}/ack`, {}),
	ackWxAlertForNet: (id: string) => post<WxAlert>(`/wx/alerts/${encodeURIComponent(id)}/ack-net`, {}),
	relayWxAlert: (id: string, req: WxRelayRequest) =>
		post<WxRelayResult>(`/wx/alerts/${encodeURIComponent(id)}/relay`, req),
	wxStatus: () => get<WxLinkStatus>('/wx/status'),
	wxRefresh: () => post<WxLinkStatus>('/wx/refresh', {}),
	wxFootprint: () => get<{ summary: WxFootprintSummary; outline: WxPolygonGeometry | null }>('/wx/footprint'),
	wxZoneSearch: (q: string, state: string) =>
		get<WxZoneRef[]>(`/wx/zones/search?q=${encodeURIComponent(q)}&state=${encodeURIComponent(state)}`),
	wxZone: (ugc: string) => get<WxZone>(`/wx/zones/${encodeURIComponent(ugc)}`),
	wxEventTypes: () => get<WxEventType[]>('/wx/event-types'),
	getNetWxWatch: (netId: string) => get<WxNetWatch>(`/nets/${netId}/wxwatch`),
	updateNetWxWatch: (netId: string, data: WxNetWatch) => put<WxNetWatch>(`/nets/${netId}/wxwatch`, data),
	netWxWatchZones: (netId: string) => get<WxNetWatchZones>(`/nets/${netId}/wxwatch/zones`),
	updateWxAlertsSettings: (data: WxAlertsSettings) => put<SettingsUpdateResponse>('/settings/wxalerts', data),

	// Weather
	weatherStations: () => get<WeatherReading[]>('/weather/stations'),
	weatherReadings: (callsign: string, params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return get<WeatherReading[]>(`/weather/${encodeURIComponent(callsign)}${qs}`);
	},
	weatherConfig: () => get<WeatherConfig>('/weather/config'),

	// Telemetry
	telemetryStations: () => get<TelemetryReading[]>('/telemetry/stations'),
	telemetryReadings: (callsign: string, params?: Record<string, string>) => {
		const qs = params ? '?' + new URLSearchParams(params).toString() : '';
		return get<TelemetryReadingsResponse>(`/telemetry/${encodeURIComponent(callsign)}${qs}`);
	},

	// Tile Cache
	tileCacheStatus: () => get<TileCacheStatus>('/tiles/cache'),
	preloadTiles: (south: number, west: number, north: number, east: number, zoomMin: number, zoomMax: number) =>
		post<{ status: string; tileCount: number }>('/tiles/cache', { south, west, north, east, zoomMin, zoomMax }),
	estimateTiles: (south: number, west: number, north: number, east: number, zoomMin: number, zoomMax: number) =>
		post<{ tileCount: number }>('/tiles/estimate', { south, west, north, east, zoomMin, zoomMax }),

	// Settings (admin only)
	getSettings: () => get<SettingsResponse>('/settings'),
	updateStation: (data: StationSettings) => put<SettingsUpdateResponse>('/settings/station', data),
	updateServer: (data: ServerSettings) => put<SettingsUpdateResponse>('/settings/server', data),
	updateTransports: (data: TransportSettings[]) => put<SettingsUpdateResponse>('/settings/transports', data),
	serialPorts: () => get<SerialPortsResponse>('/serial-ports'),
	kissTncs: () => get<KissTncsResponse>('/kiss-tncs'),
	updateBeacon: (data: BeaconSettings) => put<SettingsUpdateResponse>('/settings/beacon', data),
	updateSession: (data: SessionSettings) => put<SettingsUpdateResponse>('/settings/session', data),
	updateLogging: (data: LoggingSettings) => put<SettingsUpdateResponse>('/settings/logging', data),
	updateWeather: (data: WeatherSettings) => put<SettingsUpdateResponse>('/settings/weather', data),
	updateTileCache: (data: TileCacheSettings) => put<SettingsUpdateResponse>('/settings/tilecache', data),
	updateGPS: (data: GpsSettings) => put<SettingsUpdateResponse>('/settings/gps', data),
	updateWhat3Words: (data: What3WordsSettings) => put<SettingsUpdateResponse>('/settings/what3words', data),
	deleteWhat3WordsKey: () => del<SettingsUpdateResponse>('/settings/what3words/key'),

	// what3words proxy (operator+; status is observer+)
	w3wStatus: () => get<W3WStatus>('/w3w/status'),
	w3wResolve: (words: string) => get<W3WResult>(`/w3w/resolve?words=${encodeURIComponent(words)}`),
	w3wSuggest: (input: string, focus?: { lat: number; lon: number }, n?: number) => {
		const params = new URLSearchParams({ input });
		if (focus) {
			params.set('lat', String(focus.lat));
			params.set('lon', String(focus.lon));
		}
		if (n) params.set('n', String(n));
		return get<W3WSuggestResponse>(`/w3w/suggest?${params.toString()}`);
	},
	w3wReverse: (lat: number, lon: number) =>
		get<W3WResult>(`/w3w/reverse?lat=${encodeURIComponent(String(lat))}&lon=${encodeURIComponent(String(lon))}`),

	// --- Ride mode (bike-ride profile): net profile, phase, course, SAG, supply/medical, reconciliation ---

	netProfiles: () => get<NetProfile[]>('/net-profiles'),
	netProfile: (netId: string) => get<NetProfileView>(`/nets/${netId}/profile`),
	setNetProfile: (netId: string, profile: string) => put<NetProfileView>(`/nets/${netId}/profile`, { profile }),
	ridePhase: (netId: string) => get<RidePhaseStatus>(`/nets/${netId}/ride/phase`),
	setRidePhase: (netId: string, data: { phase: string; reason?: string }) =>
		post<RidePhaseStatus>(`/nets/${netId}/ride/phase`, data),

	rideConfig: (netId: string) => get<NetRideConfig>(`/nets/${netId}/ride-config`),
	updateRideConfig: (netId: string, data: NetRideConfig) => put<NetRideConfig>(`/nets/${netId}/ride-config`, data),

	courseState: (netId: string) => get<CourseState>(`/nets/${netId}/course`),
	courseConfig: (netId: string) => get<CourseConfig>(`/nets/${netId}/course/config`),
	updateCourseConfig: (netId: string, data: Partial<CourseConfig>) =>
		put<CourseConfig>(`/nets/${netId}/course/config`, data),
	courseStations: (netId: string) => get<StationView[]>(`/nets/${netId}/course/stations`),

	shutoffs: (netId: string) => get<ShutoffPoint[]>(`/nets/${netId}/course/shutoffs`),
	createShutoff: (netId: string, data: Partial<ShutoffPoint>) =>
		post<ShutoffPoint>(`/nets/${netId}/course/shutoffs`, data),
	updateShutoff: (netId: string, sId: string, data: Partial<ShutoffPoint>) =>
		put<ShutoffPoint>(`/nets/${netId}/course/shutoffs/${sId}`, data),
	deleteShutoff: (netId: string, sId: string) => del<void>(`/nets/${netId}/course/shutoffs/${sId}`),
	fireShutoff: (netId: string, sId: string, data: { note?: string; bibs?: string[]; rerouteCount?: number; bibWithheld?: boolean }) =>
		post<{ shutoff: ShutoffPoint; riders: RiderException[] }>(`/nets/${netId}/course/shutoffs/${sId}/fire`, data),
	cancelShutoff: (netId: string, sId: string, reason: string) =>
		post<ShutoffPoint>(`/nets/${netId}/course/shutoffs/${sId}/cancel`, { reason }),
	reinstateShutoff: (netId: string, sId: string, reason = '') =>
		post<ShutoffPoint>(`/nets/${netId}/course/shutoffs/${sId}/reinstate`, { reason }),

	riderExceptions: (netId: string, status?: string) =>
		get<RiderException[]>(`/nets/${netId}/course/riders${status ? `?status=${encodeURIComponent(status)}` : ''}`),
	recordRider: (netId: string, data: Partial<RiderException>) =>
		post<RiderException>(`/nets/${netId}/course/riders`, data),
	setRiderStatus: (netId: string, rId: string, supportStatus: string, reason: string) =>
		post<RiderException>(`/nets/${netId}/course/riders/${rId}/status`, { supportStatus, reason }),

	sweep: (netId: string, limit = 20) =>
		get<{ position: SweepPosition; reports: SweepReport[] }>(`/nets/${netId}/course/sweep?limit=${limit}`),
	reportSweep: (netId: string, data: Partial<SweepReport>) =>
		post<SweepReport>(`/nets/${netId}/course/sweep`, data),

	stationRidersClear: (netId: string, cpId: string) =>
		post<StationClosure>(`/nets/${netId}/course/stations/${cpId}/riders-clear`, {}),
	stationSweepPassed: (netId: string, cpId: string) =>
		post<StationClosure>(`/nets/${netId}/course/stations/${cpId}/sweep-passed`, {}),
	stationClose: (netId: string, cpId: string, data: { override?: boolean; reason?: string }) =>
		post<StationClosure>(`/nets/${netId}/course/stations/${cpId}/close`, data),
	stationReopen: (netId: string, cpId: string, reason: string) =>
		post<StationClosure>(`/nets/${netId}/course/stations/${cpId}/reopen`, { reason }),

	postRideCloseout: (netId: string, data: { force?: boolean; reason?: string }) =>
		post<{ net: Net; summary: unknown; closeout: CloseoutStatus }>(`/nets/${netId}/ride/closeout`, data),
	ics211ExportUrl: (netId: string) => `${BASE}/nets/${netId}/ics211/export`,
	ics214ExportUrl: (netId: string) => `${BASE}/nets/${netId}/ics214/export`,

	sagBoard: (netId: string) => get<SAGBoard>(`/nets/${netId}/sag`),
	sagConfig: (netId: string) => get<SAGConfigResponse>(`/nets/${netId}/sag/config`),
	sagRequests: (netId: string, params?: { status?: string; active?: boolean }) => {
		const qs = new URLSearchParams();
		if (params?.status) qs.set('status', params.status);
		if (params?.active) qs.set('active', 'true');
		const s = qs.toString();
		return get<SAGRequest[]>(`/nets/${netId}/sag/requests${s ? `?${s}` : ''}`);
	},
	sagRequest: (netId: string, reqId: string) => get<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}`),
	createSagRequest: (netId: string, data: CreateRequestInput) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests`, data),
	updateSagRequest: (netId: string, reqId: string, data: UpdateRequestInput) =>
		put<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}`, data),
	cancelSagRequest: (netId: string, reqId: string, reason: string) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/cancel`, { reason }),
	addSagSlot: (netId: string, reqId: string, data: SlotInput) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/slots`, data),
	updateSagSlot: (netId: string, reqId: string, slotId: string, data: SlotInput) =>
		put<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/slots/${slotId}`, data),
	resolveSagSlot: (netId: string, reqId: string, slotId: string, data: { disposition: string; note?: string }) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/slots/${slotId}/resolve`, data),
	dispatchSagLeg: (netId: string, reqId: string, data: DispatchInput) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/legs`, data),
	advanceSagLeg: (netId: string, reqId: string, legId: string, status: string) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/legs/${legId}/status`, { status }),
	loadSagSlots: (netId: string, reqId: string, legId: string, data: LoadInput) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/legs/${legId}/load`, data),
	deliverSagSlots: (netId: string, reqId: string, legId: string, data: { slotIds?: string[]; destination?: SAGLocation }) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/legs/${legId}/deliver`, data),
	releaseSagLeg: (netId: string, reqId: string, legId: string, reason?: string) =>
		post<SAGRequest>(`/nets/${netId}/sag/requests/${reqId}/legs/${legId}/release`, { reason }),
	setSagVehicle: (netId: string, ciId: string, data: { seats: number; rackSlots: number; notes?: string }) =>
		put<SAGVehicleStatus>(`/nets/${netId}/sag/vehicles/${ciId}`, data),

	rideMedical: (netId: string, open?: boolean) =>
		get<MedicalNotification[]>(`/nets/${netId}/ride/medical${open ? '?status=open' : ''}`),
	rideMedicalFull: (netId: string, mid: string) =>
		get<MedicalNotification>(`/nets/${netId}/ride/medical/${mid}`),
	medicalReadback: (netId: string, mid: string, data: { confirmed: boolean; readBackBy?: string; correction?: string }) =>
		post<MedicalNotification>(`/nets/${netId}/ride/medical/${mid}/readback`, data),
	createRideMedical: (netId: string, data: CreateMedicalInput, callsign?: string) =>
		post<MedicalNotification>(`/nets/${netId}/ride/medical`, { ...data, callsign }),
	etaRideMedical: (netId: string, mid: string, data: MedicalETAInput, callsign?: string) =>
		post<MedicalNotification>(`/nets/${netId}/ride/medical/${mid}/eta`, { ...data, callsign }),
	onSceneRideMedical: (netId: string, mid: string, data: OnSceneInput, callsign?: string) =>
		post<MedicalNotification>(`/nets/${netId}/ride/medical/${mid}/on-scene`, { ...data, callsign }),
	departRideMedical: (netId: string, mid: string, data: DepartInput, callsign?: string) =>
		post<MedicalNotification>(`/nets/${netId}/ride/medical/${mid}/depart`, { ...data, callsign }),
	releaseRideMedical: (netId: string, mid: string, data: ReleaseInput, callsign?: string) =>
		post<MedicalNotification>(`/nets/${netId}/ride/medical/${mid}/release`, { ...data, callsign }),
	cancelRideMedical: (netId: string, mid: string, data: CancelInput, callsign?: string) =>
		post<MedicalNotification>(`/nets/${netId}/ride/medical/${mid}/cancel`, { ...data, callsign }),

	rideSupply: (netId: string, open?: boolean) =>
		get<SupplyRequest[]>(`/nets/${netId}/ride/supply${open ? '?status=open' : ''}`),
	rideSupplyOne: (netId: string, sid: string) =>
		get<SupplyRequest>(`/nets/${netId}/ride/supply/${sid}`),
	supplyCatalog: (netId: string) => get<SupplyCatalogEntry[]>(`/nets/${netId}/ride/supply-catalog`),
	createRideSupply: (netId: string, data: CreateSupplyInput, callsign?: string) =>
		post<SupplyRequest>(`/nets/${netId}/ride/supply`, { ...data, callsign }),
	addRideSupplyItems: (netId: string, sid: string, data: AddItemsInput, callsign?: string) =>
		post<SupplyRequest>(`/nets/${netId}/ride/supply/${sid}/items`, { ...data, callsign }),
	readbackRideSupply: (netId: string, sid: string, data: ReadbackInput, callsign?: string) =>
		post<SupplyRequest>(`/nets/${netId}/ride/supply/${sid}/readback`, { ...data, callsign }),
	relayRideSupply: (netId: string, sid: string, data: RelayInput, callsign?: string) =>
		post<SupplyRequest>(`/nets/${netId}/ride/supply/${sid}/relay`, { ...data, callsign }),
	etaRideSupply: (netId: string, sid: string, data: RideETAInput, callsign?: string) =>
		post<SupplyRequest>(`/nets/${netId}/ride/supply/${sid}/eta`, { ...data, callsign }),
	deliverRideSupply: (netId: string, sid: string, callsign?: string) =>
		post<SupplyRequest>(`/nets/${netId}/ride/supply/${sid}/deliver`, { callsign }),
	cancelRideSupply: (netId: string, sid: string, data: CancelInput, callsign?: string) =>
		post<SupplyRequest>(`/nets/${netId}/ride/supply/${sid}/cancel`, { ...data, callsign }),
	mergeRideSupply: (netId: string, sid: string, otherSid: string, callsign?: string) =>
		post<SupplyRequest>(`/nets/${netId}/ride/supply/${sid}/merge/${otherSid}`, { callsign }),

	rideHandoff: (netId: string, status?: string) =>
		get<HandoffItem[]>(`/nets/${netId}/ride/handoff${status ? `?status=${encodeURIComponent(status)}` : ''}`),
	addHandoffItem: (netId: string, data: Partial<HandoffItem>) =>
		post<HandoffItem>(`/nets/${netId}/ride/handoff`, data),
	updateHandoffItem: (netId: string, hid: string, data: { action: 'resolve' | 'cancel'; resolution?: string }) =>
		patch<HandoffItem>(`/nets/${netId}/ride/handoff/${hid}`, data),
	ackHandoff: (netId: string) => post<ShiftHandoff>(`/nets/${netId}/ride/handoff/ack`, {}),

	rideBriefing: (netId: string) => get<ShiftBriefing>(`/nets/${netId}/ride/briefing`),
	rideCloseout: (netId: string) => get<CloseoutStatus>(`/nets/${netId}/ride/closeout`),
	rideAccounting: (netId: string) => get<Accounting>(`/nets/${netId}/ride/accounting`)
};
