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

// MemoryManagerConfig holds configuration for the in-memory session manager.
type MemoryManagerConfig struct {
	PIN               string
	InactivityTimeout time.Duration
}

// MemoryManager is an in-memory implementation of Manager.
type MemoryManager struct {
	cfg    MemoryManagerConfig
	mu     sync.RWMutex
	users  map[string]*User   // keyed by user ID
	tokens map[string]string  // token → user ID

	// OnDisconnect is called when a session is swept due to inactivity.
	OnDisconnect func(user *User)
}

// NewMemoryManager creates a new in-memory session manager.
func NewMemoryManager(cfg MemoryManagerConfig) *MemoryManager {
	return &MemoryManager{
		cfg:    cfg,
		users:  make(map[string]*User),
		tokens: make(map[string]string),
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Create creates a new user session. The pin determines the user's role.
func (m *MemoryManager) Create(name string, pin string) (*User, error) {
	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	// No PIN configured → everyone is Operator (field-friendly, no friction).
	// PIN configured → correct PIN = Operator, no/wrong PIN = Observer.
	var role Role
	if m.cfg.PIN == "" {
		role = RoleOperator
	} else if pin != "" && pin == m.cfg.PIN {
		role = RoleOperator
	} else {
		role = RoleObserver
	}

	now := time.Now()
	user := &User{
		ID:           uuid.NewString(),
		Name:         name,
		Role:         role,
		Token:        token,
		ConnectedAt:  now,
		LastActivity: now,
	}

	m.mu.Lock()
	m.users[user.ID] = user
	m.tokens[token] = user.ID
	m.mu.Unlock()

	return user, nil
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

// Sweep removes sessions that have been inactive longer than InactivityTimeout.
func (m *MemoryManager) Sweep() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg.InactivityTimeout <= 0 {
		return
	}

	cutoff := time.Now().Add(-m.cfg.InactivityTimeout)
	for id, user := range m.users {
		if user.LastActivity.Before(cutoff) {
			delete(m.tokens, user.Token)
			delete(m.users, id)
			if m.OnDisconnect != nil {
				m.OnDisconnect(user)
			}
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
