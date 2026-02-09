package netcontrol

// Net statuses.
const (
	StatusDraft    = "draft"
	StatusOpen     = "open"
	StatusClosed   = "closed"
	StatusArchived = "archived"
)

// Operator statuses.
const (
	OpAvailable = "available"
	OpAssigned  = "assigned"
	OpEnRoute   = "enroute"
	OpOnScene   = "onscene"
	OpBRB       = "brb"
	OpMissing   = "missing"
	OpReleased  = "released"
)

// Traffic precedence.
const (
	TrafficNone      = "none"
	TrafficRoutine   = "routine"
	TrafficPriority  = "priority"
	TrafficWelfare   = "welfare"
	TrafficEmergency = "emergency"
)

// Event types for WebSocket broadcast.
const (
	EventNetCreated     = "net_created"
	EventNetUpdated     = "net_updated"
	EventCheckInCreated = "checkin_created"
	EventCheckInUpdated = "checkin_updated"
	EventMissionCreated = "mission_created"
	EventMissionUpdated = "mission_updated"
	EventTimelineEntry  = "net_timeline_entry"
)

// Event represents a net control event for WebSocket broadcast.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// NetSummary is returned when a net is closed, summarizing its activity.
type NetSummary struct {
	NetID         string `json:"netId"`
	Name          string `json:"name"`
	Duration      string `json:"duration"`
	TotalCheckIns int    `json:"totalCheckIns"`
	TotalMissions int    `json:"totalMissions"`
	TrafficCounts map[string]int `json:"trafficCounts"`
}
