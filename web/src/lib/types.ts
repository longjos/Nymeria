export interface Station {
	callsign: string;
	ssid: number;
	lastHeard: string;
	position?: Position;
	symbol: APRSSymbol;
	track: TrackPoint[];
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
	table: string;
	code: string;
}

export interface Message {
	id: string;
	from: string;
	to: string;
	body: string;
	ackRequired: boolean;
	timestamp: string;
	acked: boolean;
}

export interface Conversation {
	callsign: string;
	messages: Message[];
	unreadCount: number;
}

export interface HealthResponse {
	status: string;
}

export interface TransportStatus {
	id: string;
	type: string;
	connected: boolean;
	lastActivity?: string;
}
