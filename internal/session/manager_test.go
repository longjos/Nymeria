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

func TestCreateNoPINConfigured(t *testing.T) {
	// No PIN configured → everyone gets Operator (field-friendly).
	m := newTestManager("", 30*time.Minute)

	user, err := m.Create("Alice", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if user.Role != RoleOperator {
		t.Errorf("role = %q, want %q", user.Role, RoleOperator)
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

func TestCreateNoPINConfiguredWithRandomInput(t *testing.T) {
	// No PIN configured → even if user sends a PIN, still Operator.
	m := newTestManager("", 30*time.Minute)
	user, err := m.Create("Bob", "anything")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if user.Role != RoleOperator {
		t.Errorf("role = %q, want %q (no PIN configured = always Operator)", user.Role, RoleOperator)
	}
}

func TestCreateWithPIN(t *testing.T) {
	tests := []struct {
		name     string
		pin      string
		wantRole Role
	}{
		{"no pin entered", "", RoleObserver},
		{"wrong pin", "wrong", RoleObserver},
		{"correct pin", "secret123", RoleOperator},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager("secret123", 30*time.Minute)
			user, err := m.Create("Bob", tt.pin)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if user.Role != tt.wantRole {
				t.Errorf("role = %q, want %q", user.Role, tt.wantRole)
			}
		})
	}
}

func TestGetByToken(t *testing.T) {
	m := newTestManager("secret", 30*time.Minute)
	user, _ := m.Create("Alice", "secret")

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
	user, _ := m.Create("Alice", "")

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
	m.Create("Alice", "")
	m.Create("Bob", "")
	m.Create("Charlie", "")

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
	user, _ := m.Create("Alice", "")

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
	user, _ := m.Create("Alice", "")

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
	user, _ := m.Create("Alice", "")

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
	user, _ := m.Create("Alice", "")

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
	user, _ := m.Create("Alice", "")

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
	user, _ := m.Create("Alice", "")

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
	m.Create("Alice", "")

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
		user, err := m.Create("User", "")
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
	m := newTestManager("secret", 30*time.Minute)

	var wg sync.WaitGroup
	errs := make(chan error, 300)

	// 100 goroutines creating sessions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Create("User", "")
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
