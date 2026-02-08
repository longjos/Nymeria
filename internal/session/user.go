package session

// Role represents a user's permission level.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// User represents a connected user session.
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Role     Role   `json:"role"`
	Callsign string `json:"callsign,omitempty"`
}
