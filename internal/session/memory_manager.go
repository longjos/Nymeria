package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// expiredSession holds a snapshot of a recently-expired session for reconnection.
type expiredSession struct {
	user   User
	expiry time.Time // when the session expired (not when the reconnect window closes)
}

// MemoryManagerConfig holds configuration for the in-memory session manager.
type MemoryManagerConfig struct {
	PIN               string
	InactivityTimeout time.Duration
	ReconnectWindow   time.Duration // how long expired sessions are kept for reconnection
}

// MemoryManager is an in-memory implementation of Manager.
type MemoryManager struct {
	cfg    MemoryManagerConfig
	mu     sync.RWMutex
	users  map[string]*User   // keyed by user ID
	tokens map[string]string  // token → user ID
	expired map[string]expiredSession // token → expired session (for reconnection)
	events chan Event

	// OnDisconnect is called when a session is swept due to inactivity.
	OnDisconnect func(user *User)
}

// NewMemoryManager creates a new in-memory session manager.
func NewMemoryManager(cfg MemoryManagerConfig) *MemoryManager {
	if cfg.ReconnectWindow == 0 {
		cfg.ReconnectWindow = 4 * time.Hour
	}
	return &MemoryManager{
		cfg:     cfg,
		users:   make(map[string]*User),
		tokens:  make(map[string]string),
		expired: make(map[string]expiredSession),
		events:  make(chan Event, 64),
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Create creates a new user session. The first user auto-becomes Admin.
// Subsequent users are pending approval unless reconnecting with a saved token.
func (m *MemoryManager) Create(name string, opts CreateOpts) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for returning user reconnection via saved token.
	if opts.Token != "" {
		if es, ok := m.expired[opts.Token]; ok {
			reconnectDeadline := es.expiry.Add(m.cfg.ReconnectWindow)
			if time.Now().Before(reconnectDeadline) {
				// Reconnect: create new session with previous role
				token, err := generateToken()
				if err != nil {
					return nil, err
				}
				now := time.Now()
				user := &User{
					ID:           uuid.NewString(),
					Name:         name,
					Role:         es.user.Role,
					Status:       StatusApproved,
					Token:        token,
					ConnectedAt:  now,
					LastActivity: now,
				}
				m.users[user.ID] = user
				m.tokens[token] = user.ID
				delete(m.expired, opts.Token)
				return user, nil
			}
			// Token expired beyond reconnect window — fall through to normal flow
			delete(m.expired, opts.Token)
		}
	}

	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	user := &User{
		ID:           uuid.NewString(),
		Name:         name,
		Token:        token,
		ConnectedAt:  now,
		LastActivity: now,
	}

	// First user (no admin exists) → auto-approved as Admin.
	if !m.hasAdmin() {
		// Emergency PIN recovery: if PIN is configured and matches, grant admin.
		// Also handles no-admin case: first user always becomes admin.
		user.Role = RoleAdmin
		user.Status = StatusApproved
	} else {
		// Subsequent users → pending approval.
		user.Role = RoleObserver
		user.Status = StatusPending
	}

	m.users[user.ID] = user
	m.tokens[token] = user.ID

	// Emit event for pending users.
	if user.Status == StatusPending {
		m.emitEvent(Event{Type: EventAccessRequest, User: *user})
	}

	return user, nil
}

// hasAdmin returns true if any active approved user has the admin role. Must be called with mu held.
func (m *MemoryManager) hasAdmin() bool {
	for _, u := range m.users {
		if u.Role == RoleAdmin && u.Status == StatusApproved {
			return true
		}
	}
	return false
}

// Approve approves a pending user with the given role.
func (m *MemoryManager) Approve(userID string, role Role) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[userID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", userID)
	}
	if user.Status != StatusPending {
		return nil, fmt.Errorf("user is not pending: %s", userID)
	}

	user.Status = StatusApproved
	user.Role = role

	m.emitEvent(Event{Type: EventAccessApproved, User: *user})
	return user, nil
}

// Deny denies a pending user's access request.
func (m *MemoryManager) Deny(userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[userID]
	if !ok {
		return fmt.Errorf("session not found: %s", userID)
	}
	if user.Status != StatusPending {
		return fmt.Errorf("user is not pending: %s", userID)
	}

	user.Status = StatusDenied

	m.emitEvent(Event{Type: EventAccessDenied, User: *user})
	return nil
}

// Pending returns all users with StatusPending.
func (m *MemoryManager) Pending() []User {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pending := make([]User, 0)
	for _, u := range m.users {
		if u.Status == StatusPending {
			pending = append(pending, *u)
		}
	}
	return pending
}

// Events returns a channel of session lifecycle events.
func (m *MemoryManager) Events() <-chan Event {
	return m.events
}

// emitEvent sends an event to the events channel (non-blocking).
func (m *MemoryManager) emitEvent(evt Event) {
	select {
	case m.events <- evt:
	default:
		// Drop if channel is full
	}
}

// Get returns a user by token.
func (m *MemoryManager) Get(token string) (*User, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.tokens[token]
	if !ok {
		return nil, false
	}
	user, ok := m.users[id]
	if !ok {
		return nil, false
	}
	return user, true
}

// GetByID returns a user by ID.
func (m *MemoryManager) GetByID(id string) (*User, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.users[id]
	if !ok {
		return nil, false
	}
	return user, true
}

// All returns all active users.
func (m *MemoryManager) All() []User {
	m.mu.RLock()
	defer m.mu.RUnlock()

	users := make([]User, 0, len(m.users))
	for _, u := range m.users {
		users = append(users, *u)
	}
	return users
}

// Remove removes a user session by ID.
func (m *MemoryManager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[id]
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}

	delete(m.tokens, user.Token)
	delete(m.users, id)
	return nil
}

// Touch updates the LastActivity timestamp for the given token.
func (m *MemoryManager) Touch(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id, ok := m.tokens[token]
	if !ok {
		return
	}
	if user, ok := m.users[id]; ok {
		user.LastActivity = time.Now()
	}
}

// UpdateRole changes a user's role.
func (m *MemoryManager) UpdateRole(id string, role Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, ok := m.users[id]
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	user.Role = role
	return nil
}

// UpdateConfig updates the session manager configuration (PIN and timeout).
func (m *MemoryManager) UpdateConfig(cfg MemoryManagerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg.ReconnectWindow == 0 {
		cfg.ReconnectWindow = m.cfg.ReconnectWindow
	}
	m.cfg = cfg
}

// Sweep removes sessions that have been inactive longer than InactivityTimeout.
// Expired sessions are moved to the expired map for potential reconnection.
func (m *MemoryManager) Sweep() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg.InactivityTimeout <= 0 {
		return
	}

	now := time.Now()
	cutoff := now.Add(-m.cfg.InactivityTimeout)
	for id, user := range m.users {
		if user.LastActivity.Before(cutoff) {
			// Move approved sessions to expired map for reconnection.
			if user.Status == StatusApproved {
				m.expired[user.Token] = expiredSession{
					user:   *user,
					expiry: now,
				}
			}
			delete(m.tokens, user.Token)
			delete(m.users, id)
			if m.OnDisconnect != nil {
				m.OnDisconnect(user)
			}
		}
	}

	// Purge expired entries beyond ReconnectWindow.
	for token, es := range m.expired {
		if now.After(es.expiry.Add(m.cfg.ReconnectWindow)) {
			delete(m.expired, token)
		}
	}
}

// Start begins a background goroutine that periodically sweeps expired sessions.
func (m *MemoryManager) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.Sweep()
			}
		}
	}()
}
