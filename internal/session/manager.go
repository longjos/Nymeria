package session

// Manager manages user sessions.
type Manager interface {
	// Create creates a new user session.
	Create(name string, role Role) (*User, error)

	// Get returns a user by ID.
	Get(id string) (*User, bool)

	// All returns all active users.
	All() []User

	// Remove removes a user session.
	Remove(id string) error
}
