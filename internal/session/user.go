package session

import "time"

// Role represents a user's permission level.
type Role string

const (
	RoleObserver Role = "observer"
	RolePlotter  Role = "plotter"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

// RoleLevel returns a numeric level for role comparison.
// Observer=0, Plotter=1, Operator=2, Admin=3. Unknown roles return -1.
func RoleLevel(r Role) int {
	switch r {
	case RoleObserver:
		return 0
	case RolePlotter:
		return 1
	case RoleOperator:
		return 2
	case RoleAdmin:
		return 3
	default:
		return -1
	}
}

// Status represents a user's access approval status.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDenied   Status = "denied"
)

// User represents a connected user session.
type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	Status       Status    `json:"status"`
	Callsign     string    `json:"callsign,omitempty"`
	Token        string    `json:"token,omitempty"`
	ConnectedAt  time.Time `json:"connectedAt"`
	LastActivity time.Time `json:"lastActivity"`
}

// EventType represents a session event type.
type EventType string

const (
	EventAccessRequest  EventType = "access_request"
	EventAccessApproved EventType = "access_approved"
	EventAccessDenied   EventType = "access_denied"
)

// Event represents a session lifecycle event.
type Event struct {
	Type EventType `json:"type"`
	User User      `json:"user"`
}
