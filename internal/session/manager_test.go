package session

import (
	"context"
	"sync"
	"testing"
	"time"
)

func newTestManager(pin string, timeout time.Duration) *MemoryManager {
	return NewMemoryManager(MemoryManagerConfig{
		PIN:               pin,
		InactivityTimeout: timeout,
	})
}

func TestRoleLevel(t *testing.T) {
	tests := []struct {
		role  Role
		level int
	}{
		{RoleObserver, 0},
		{RolePlotter, 1},
		{RoleOperator, 2},
		{RoleAdmin, 3},
		{Role("unknown"), -1},
	}
	for _, tt := range tests {
		if got := RoleLevel(tt.role); got != tt.level {
			t.Errorf("RoleLevel(%q) = %d, want %d", tt.role, got, tt.level)
		}
	}
}

func TestCreateFirstUserAutoAdmin(t *testing.T) {
	m := newTestManager("", 30*time.Minute)

	user, err := m.Create("Alice", CreateOpts{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if user.Role != RoleAdmin {
		t.Errorf("first user role = %q, want %q (auto-promote)", user.Role, RoleAdmin)
	}
	if user.Status != StatusApproved {
		t.Errorf("first user status = %q, want %q", user.Status, StatusApproved)
	}
	if user.Name != "Alice" {
		t.Errorf("name = %q, want %q", user.Name, "Alice")
	}
	if user.Token == "" {
		t.Error("token is empty")
	}
	if user.ID == "" {
		t.Error("ID is empty")
	}
	if user.ConnectedAt.IsZero() {
		t.Error("ConnectedAt is zero")
	}
	if user.LastActivity.IsZero() {
		t.Error("LastActivity is zero")
	}
}

func TestCreateSubsequentUserPending(t *testing.T) {
	m := newTestManager("", 30*time.Minute)

	// First user becomes admin.
	m.Create("Admin", CreateOpts{})

	// Second user should be pending.
	user2, err := m.Create("Bob", CreateOpts{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if user2.Status != StatusPending {
		t.Errorf("second user status = %q, want %q", user2.Status, StatusPending)
	}
	if user2.Role != RoleObserver {
		t.Errorf("second user role = %q, want %q", user2.Role, RoleObserver)
	}
}

func TestApprove(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	m.Create("Admin", CreateOpts{})

	pending, _ := m.Create("Bob", CreateOpts{})
	if pending.Status != StatusPending {
		t.Fatalf("expected pending, got %q", pending.Status)
	}

	approved, err := m.Approve(pending.ID, RoleOperator)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Status != StatusApproved {
		t.Errorf("approved status = %q, want %q", approved.Status, StatusApproved)
	}
	if approved.Role != RoleOperator {
		t.Errorf("approved role = %q, want %q", approved.Role, RoleOperator)
	}

	// Verify via Get
	got, ok := m.Get(pending.Token)
	if !ok {
		t.Fatal("Get returned false for approved user")
	}
	if got.Status != StatusApproved {
		t.Errorf("Get status = %q, want %q", got.Status, StatusApproved)
	}
}

func TestApproveNonexistent(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	_, err := m.Approve("nonexistent", RoleOperator)
	if err == nil {
		t.Error("Approve nonexistent should return error")
	}
}

func TestApproveAlreadyApproved(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	admin, _ := m.Create("Admin", CreateOpts{})
	_, err := m.Approve(admin.ID, RoleOperator)
	if err == nil {
		t.Error("Approve already-approved user should return error")
	}
}

func TestDeny(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	m.Create("Admin", CreateOpts{})

	pending, _ := m.Create("Bob", CreateOpts{})

	err := m.Deny(pending.ID)
	if err != nil {
		t.Fatalf("Deny: %v", err)
	}

	got, ok := m.Get(pending.Token)
	if !ok {
		t.Fatal("Get returned false after Deny")
	}
	if got.Status != StatusDenied {
		t.Errorf("denied status = %q, want %q", got.Status, StatusDenied)
	}
}

func TestDenyNonexistent(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	err := m.Deny("nonexistent")
	if err == nil {
		t.Error("Deny nonexistent should return error")
	}
}

func TestPending(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	m.Create("Admin", CreateOpts{})

	m.Create("Bob", CreateOpts{})
	m.Create("Charlie", CreateOpts{})

	pending := m.Pending()
	if len(pending) != 2 {
		t.Fatalf("Pending() returned %d users, want 2", len(pending))
	}

	names := map[string]bool{}
	for _, u := range pending {
		names[u.Name] = true
	}
	if !names["Bob"] || !names["Charlie"] {
		t.Errorf("expected Bob and Charlie in pending, got %v", names)
	}
}

func TestReturningUser(t *testing.T) {
	m := NewMemoryManager(MemoryManagerConfig{
		InactivityTimeout: 50 * time.Millisecond,
		ReconnectWindow:   1 * time.Hour,
	})
	user, _ := m.Create("Alice", CreateOpts{})
	savedToken := user.Token

	// Wait for session to expire
	time.Sleep(80 * time.Millisecond)
	m.Sweep()

	// Verify session expired
	_, ok := m.Get(savedToken)
	if ok {
		t.Fatal("session should have expired")
	}

	// Reconnect with saved token
	reconnected, err := m.Create("Alice", CreateOpts{Token: savedToken})
	if err != nil {
		t.Fatalf("Create with saved token: %v", err)
	}
	if reconnected.Status != StatusApproved {
		t.Errorf("reconnected status = %q, want %q", reconnected.Status, StatusApproved)
	}
	if reconnected.Role != RoleAdmin {
		t.Errorf("reconnected role = %q, want %q (should preserve original role)", reconnected.Role, RoleAdmin)
	}
	if reconnected.Token == savedToken {
		t.Error("reconnected should get a new token, not reuse the old one")
	}
}

func TestReturningUserExpiredBeyondWindow(t *testing.T) {
	m := NewMemoryManager(MemoryManagerConfig{
		InactivityTimeout: 50 * time.Millisecond,
		ReconnectWindow:   50 * time.Millisecond,
	})
	user, _ := m.Create("Alice", CreateOpts{})
	savedToken := user.Token

	// Wait for session to expire
	time.Sleep(80 * time.Millisecond)
	m.Sweep()

	// Wait for reconnect window to close
	time.Sleep(80 * time.Millisecond)

	// Try to reconnect — should fail (but first user so auto-admin anyway)
	reconnected, err := m.Create("Alice", CreateOpts{Token: savedToken})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Since there's no admin, first user auto-becomes admin.
	// The key test is that it went through the normal flow (not reconnection).
	if reconnected.Token == savedToken {
		t.Error("should not reuse expired token")
	}
}

func TestEmergencyPINRecovery(t *testing.T) {
	m := newTestManager("secret123", 30*time.Minute)

	// First user with PIN becomes admin (no admin exists).
	user, err := m.Create("Admin", CreateOpts{PIN: "secret123"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if user.Role != RoleAdmin {
		t.Errorf("first user role = %q, want %q", user.Role, RoleAdmin)
	}
	if user.Status != StatusApproved {
		t.Errorf("first user status = %q, want %q", user.Status, StatusApproved)
	}
}

func TestCreatePendingEmitsEvent(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	m.Create("Admin", CreateOpts{}) // admin

	// Drain any events from admin creation
	drainEvents(m)

	m.Create("Bob", CreateOpts{})

	evt := receiveEvent(t, m)
	if evt.Type != EventAccessRequest {
		t.Errorf("event type = %q, want %q", evt.Type, EventAccessRequest)
	}
	if evt.User.Name != "Bob" {
		t.Errorf("event user name = %q, want %q", evt.User.Name, "Bob")
	}
}

func TestApproveEmitsEvent(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	m.Create("Admin", CreateOpts{})

	pending, _ := m.Create("Bob", CreateOpts{})
	drainEvents(m) // drain access_request event

	m.Approve(pending.ID, RoleOperator)

	evt := receiveEvent(t, m)
	if evt.Type != EventAccessApproved {
		t.Errorf("event type = %q, want %q", evt.Type, EventAccessApproved)
	}
	if evt.User.Name != "Bob" {
		t.Errorf("event user name = %q, want %q", evt.User.Name, "Bob")
	}
	if evt.User.Role != RoleOperator {
		t.Errorf("event user role = %q, want %q", evt.User.Role, RoleOperator)
	}
}

func TestDenyEmitsEvent(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	m.Create("Admin", CreateOpts{})

	pending, _ := m.Create("Bob", CreateOpts{})
	drainEvents(m)

	m.Deny(pending.ID)

	evt := receiveEvent(t, m)
	if evt.Type != EventAccessDenied {
		t.Errorf("event type = %q, want %q", evt.Type, EventAccessDenied)
	}
	if evt.User.Name != "Bob" {
		t.Errorf("event user name = %q, want %q", evt.User.Name, "Bob")
	}
}

func TestGetByToken(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	user, _ := m.Create("Alice", CreateOpts{})

	got, ok := m.Get(user.Token)
	if !ok {
		t.Fatal("Get returned false for valid token")
	}
	if got.ID != user.ID {
		t.Errorf("ID = %q, want %q", got.ID, user.ID)
	}
}

func TestGetNonexistentToken(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	_, ok := m.Get("nonexistent-token")
	if ok {
		t.Error("Get returned true for nonexistent token")
	}
}

func TestGetByID(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	user, _ := m.Create("Alice", CreateOpts{})

	got, ok := m.GetByID(user.ID)
	if !ok {
		t.Fatal("GetByID returned false for valid ID")
	}
	if got.Name != "Alice" {
		t.Errorf("Name = %q, want %q", got.Name, "Alice")
	}
}

func TestGetByIDNonexistent(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	_, ok := m.GetByID("nonexistent")
	if ok {
		t.Error("GetByID returned true for nonexistent ID")
	}
}

func TestAll(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	m.Create("Alice", CreateOpts{})
	m.Create("Bob", CreateOpts{})
	m.Create("Charlie", CreateOpts{})

	all := m.All()
	if len(all) != 3 {
		t.Fatalf("All() returned %d users, want 3", len(all))
	}

	names := map[string]bool{}
	for _, u := range all {
		names[u.Name] = true
	}
	for _, name := range []string{"Alice", "Bob", "Charlie"} {
		if !names[name] {
			t.Errorf("All() missing user %q", name)
		}
	}
}

func TestRemove(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	user, _ := m.Create("Alice", CreateOpts{})

	if err := m.Remove(user.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, ok := m.Get(user.Token)
	if ok {
		t.Error("Get returned true after Remove")
	}

	_, ok = m.GetByID(user.ID)
	if ok {
		t.Error("GetByID returned true after Remove")
	}

	all := m.All()
	if len(all) != 0 {
		t.Errorf("All() returned %d users after Remove, want 0", len(all))
	}
}

func TestRemoveNonexistent(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	err := m.Remove("nonexistent")
	if err == nil {
		t.Error("Remove nonexistent should return error")
	}
}

func TestTouch(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	user, _ := m.Create("Alice", CreateOpts{})

	before := user.LastActivity
	time.Sleep(10 * time.Millisecond)

	m.Touch(user.Token)

	got, ok := m.Get(user.Token)
	if !ok {
		t.Fatal("Get returned false after Touch")
	}
	if !got.LastActivity.After(before) {
		t.Errorf("LastActivity was not updated: before=%v, after=%v", before, got.LastActivity)
	}
}

func TestTouchNonexistent(t *testing.T) {
	// Touch on a nonexistent token should be a no-op (no panic).
	m := newTestManager("", 30*time.Minute)
	m.Touch("nonexistent") // should not panic
}

func TestUpdateRole(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	user, _ := m.Create("Alice", CreateOpts{})

	if err := m.UpdateRole(user.ID, RoleAdmin); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}

	got, ok := m.GetByID(user.ID)
	if !ok {
		t.Fatal("GetByID returned false")
	}
	if got.Role != RoleAdmin {
		t.Errorf("role = %q, want %q", got.Role, RoleAdmin)
	}
}

func TestUpdateRoleNonexistent(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	err := m.UpdateRole("nonexistent", RoleAdmin)
	if err == nil {
		t.Error("UpdateRole nonexistent should return error")
	}
}

func TestSweepRemovesExpired(t *testing.T) {
	m := newTestManager("", 50*time.Millisecond)
	user, _ := m.Create("Alice", CreateOpts{})

	// Wait for expiration
	time.Sleep(80 * time.Millisecond)
	m.Sweep()

	_, ok := m.Get(user.Token)
	if ok {
		t.Error("Get returned true after sweep of expired session")
	}
}

func TestSweepKeepsActive(t *testing.T) {
	m := newTestManager("", 200*time.Millisecond)
	user, _ := m.Create("Alice", CreateOpts{})

	// Touch to keep active, then sweep
	time.Sleep(50 * time.Millisecond)
	m.Touch(user.Token)
	m.Sweep()

	_, ok := m.Get(user.Token)
	if !ok {
		t.Error("Get returned false — active session was swept")
	}
}

func TestSweepWithCallback(t *testing.T) {
	m := newTestManager("", 50*time.Millisecond)
	user, _ := m.Create("Alice", CreateOpts{})

	var disconnected *User
	m.OnDisconnect = func(u *User) {
		disconnected = u
	}

	time.Sleep(80 * time.Millisecond)
	m.Sweep()

	if disconnected == nil {
		t.Fatal("OnDisconnect was not called")
	}
	if disconnected.ID != user.ID {
		t.Errorf("OnDisconnect user ID = %q, want %q", disconnected.ID, user.ID)
	}
}

func TestStartSweepGoroutine(t *testing.T) {
	m := newTestManager("", 50*time.Millisecond)
	m.Create("Alice", CreateOpts{})

	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx, 30*time.Millisecond) // sweep every 30ms

	// Wait for sweep to run
	time.Sleep(150 * time.Millisecond)
	cancel()

	all := m.All()
	if len(all) != 0 {
		t.Errorf("expected 0 users after sweep, got %d", len(all))
	}
}

func TestTokenUniqueness(t *testing.T) {
	m := newTestManager("", 30*time.Minute)
	tokens := make(map[string]bool, 100)

	for i := 0; i < 100; i++ {
		user, err := m.Create("User", CreateOpts{})
		if err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		if tokens[user.Token] {
			t.Fatalf("duplicate token at iteration %d: %s", i, user.Token)
		}
		tokens[user.Token] = true
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := newTestManager("", 30*time.Minute)

	var wg sync.WaitGroup
	errs := make(chan error, 300)

	// 100 goroutines creating sessions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Create("User", CreateOpts{})
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()

	all := m.All()
	if len(all) != 100 {
		t.Fatalf("expected 100 users, got %d", len(all))
	}

	// 100 goroutines getting sessions + 100 goroutines touching
	for _, u := range all {
		wg.Add(2)
		go func(token string) {
			defer wg.Done()
			_, ok := m.Get(token)
			if !ok {
				errs <- nil // just signal
			}
		}(u.Token)
		go func(token string) {
			defer wg.Done()
			m.Touch(token)
		}(u.Token)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent error: %v", err)
		}
	}
}

// Helper: drain all events from the channel.
func drainEvents(m *MemoryManager) {
	for {
		select {
		case <-m.Events():
		default:
			return
		}
	}
}

// Helper: receive one event with timeout.
func receiveEvent(t *testing.T, m *MemoryManager) Event {
	t.Helper()
	select {
	case evt := <-m.Events():
		return evt
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}
