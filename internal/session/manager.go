package session

// CreateOpts holds options for creating a new session.
type CreateOpts struct {
	PIN   string // PIN for legacy/emergency authentication
	Token string // Saved token for returning-user reconnection
}

// Manager manages user sessions.
type Manager interface {
	// Create creates a new user session with the given options.
	// First user auto-becomes Admin. Subsequent users are pending approval.
	Create(name string, opts CreateOpts) (*User, error)

	// Get returns a user by token.
	Get(token string) (*User, bool)

	// GetByID returns a user by ID.
	GetByID(id string) (*User, bool)

	// All returns all active users.
	All() []User

	// Remove removes a user session by ID.
	Remove(id string) error

	// Touch updates the LastActivity timestamp for the given token.
	Touch(token string)

	// UpdateRole changes a user's role.
	UpdateRole(id string, role Role) error

	// Approve approves a pending user with the given role.
	Approve(userID string, role Role) (*User, error)

	// Deny denies a pending user's access request.
	Deny(userID string) error

	// Pending returns all users with StatusPending.
	Pending() []User

	// Events returns a channel of session lifecycle events.
	Events() <-chan Event
}
