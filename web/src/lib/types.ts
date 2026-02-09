export interface Station {
	callsign: string;
	ssid: number;
	lastHeard: string;
	position?: Position;
	symbol: APRSSymbol;
	comment?: string;
	track: TrackPoint[];
	source: string;
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
}

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
	location: string;
	lat?: number;
	lon?: number;
	assignment: string;
	assignmentLat?: number;
	assignmentLon?: number;
	missionId?: string;
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

export interface NetNote {
	id: string;
	netId: string;
	checkInId?: string;
	missionId?: string;
	authorId: string;
	authorName: string;
	content: string;
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

export interface NetSummary {
	netId: string;
	name: string;
	duration: string;
	totalCheckIns: number;
	totalMissions: number;
	trafficCounts: Record<string, number>;
}
