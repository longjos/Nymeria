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

// Station categories / resource types.
const (
	CatGeneral  = "general"
	CatCommand  = "command"
	CatMedical  = "medical"
	CatSAG      = "sag"
	CatMarshal  = "marshal"
	CatFixed    = "fixed"
	CatMobile   = "mobile"
	CatTactical = "tactical"
)

// ValidCategories is the set of allowed station categories.
var ValidCategories = map[string]bool{
	CatGeneral: true, CatCommand: true, CatMedical: true,
	CatSAG: true, CatMarshal: true, CatFixed: true,
	CatMobile: true, CatTactical: true,
}

// MissedRollCallThreshold is the number of missed roll calls before auto-marking missing.
const MissedRollCallThreshold = 2

// Event types for WebSocket broadcast.
const (
	EventNetCreated     = "net_created"
	EventNetUpdated     = "net_updated"
	EventCheckInCreated = "checkin_created"
	EventCheckInUpdated = "checkin_updated"
	EventMissionCreated    = "mission_created"
	EventMissionUpdated    = "mission_updated"
	EventTimelineEntry = "net_timeline_entry"
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
