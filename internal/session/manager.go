package session

// Manager manages user sessions.
type Manager interface {
	// Create creates a new user session. If a PIN is configured and
	// the provided pin matches, the user gets Operator role. Otherwise
	// Observer. If no PIN is configured, everyone gets Operator.
	Create(name string, pin string) (*User, error)

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
}
