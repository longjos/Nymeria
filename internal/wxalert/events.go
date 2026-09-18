package wxalert

// Event is a manager/cache notification, bridged to a WebSocket broadcast
// verbatim by internal/server (the tilecache Event{Type,Data} shape).
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// WebSocket event type constants (BUILD-PLAN §2.6): exactly three events
// reach the frontend; everything else (zone prefetch progress, per-alert
// notify pings, etc.) is internal bookkeeping only.
const (
	EventAlerts      = "wx_alerts"
	EventLinkStatus  = "wx_link_status"
	EventAlertAckNet = "wx_alert_ack_net"
)
