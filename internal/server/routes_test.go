package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/config"
	"github.com/narvel/nymeria/internal/session"
	"github.com/narvel/nymeria/internal/station"
	"github.com/narvel/nymeria/internal/transport"
)

func testServer(opts ...Option) *Server {
	tracker := station.NewMemoryTracker(config.StationConfig{
		Callsign:       "N0CALL",
		TrackMaxPoints: 10,
		StaleTimeout:   time.Hour,
	})
	tm := transport.NewManager()
	return New(tracker, tm, nil, nil, opts...)
}

func TestHandleHealth(t *testing.T) {
	srv := testServer()
	r := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestHandleConfig_PINRequired(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "1234",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	r := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["pinRequired"] != true {
		t.Errorf("pinRequired = %v, want true", resp["pinRequired"])
	}
}

func TestHandleConfig_NoPIN(t *testing.T) {
	srv := testServer()

	r := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["pinRequired"] != false {
		t.Errorf("pinRequired = %v, want false", resp["pinRequired"])
	}
}

func TestHandleLogin(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	tests := []struct {
		name     string
		body     string
		wantCode int
		wantRole session.Role
	}{
		{"operator with correct pin", `{"name":"Alice","pin":"secret"}`, 200, session.RoleOperator},
		{"observer with wrong pin", `{"name":"Bob","pin":"wrong"}`, 200, session.RoleObserver},
		{"observer with no pin", `{"name":"Charlie"}`, 200, session.RoleObserver},
		{"empty name", `{"name":""}`, 400, ""},
		{"no body", `{`, 400, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/session", bytes.NewBufferString(tt.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)

			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantCode)
			}

			if tt.wantCode == 200 {
				var resp session.User
				json.NewDecoder(w.Body).Decode(&resp)
				if resp.Role != tt.wantRole {
					t.Errorf("role = %q, want %q", resp.Role, tt.wantRole)
				}
				if resp.Token == "" {
					t.Error("expected non-empty token")
				}
				if resp.Name == "" {
					t.Error("expected non-empty name")
				}
			}
		})
	}
}

func TestHandleGetSession(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	// Create a user first
	user, _ := mgr.Create("Alice", "secret")

	// Authenticated request
	r := httptest.NewRequest("GET", "/api/session", nil)
	r.Header.Set("Authorization", "Bearer "+user.Token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp session.User
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ID != user.ID {
		t.Errorf("id = %q, want %q", resp.ID, user.ID)
	}

	// Unauthenticated request
	r = httptest.NewRequest("GET", "/api/session", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 401 {
		t.Errorf("unauthenticated status = %d, want 401", w.Code)
	}
}

func TestHandleLogout(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	user, _ := mgr.Create("Alice", "secret")

	r := httptest.NewRequest("DELETE", "/api/session", nil)
	r.Header.Set("Authorization", "Bearer "+user.Token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Verify session was removed
	_, ok := mgr.Get(user.Token)
	if ok {
		t.Error("expected session to be removed after logout")
	}
}

func TestHandleGetUsers(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	mgr.Create("Alice", "secret")
	mgr.Create("Bob", "")

	r := httptest.NewRequest("GET", "/api/users", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 2 {
		t.Fatalf("got %d users, want 2", len(resp))
	}

	// Tokens should not be exposed
	for _, u := range resp {
		if _, hasToken := u["token"]; hasToken {
			t.Error("token should not be exposed in user list")
		}
	}
}

func TestHandleUpdateUserRole(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	admin, _ := mgr.Create("Admin", "secret")
	mgr.UpdateRole(admin.ID, session.RoleAdmin)

	target, _ := mgr.Create("Bob", "")

	// Admin promotes Bob to Operator
	body := `{"role":"operator"}`
	r := httptest.NewRequest("PUT", "/api/users/"+target.ID+"/role", bytes.NewBufferString(body))
	r.Header.Set("Authorization", "Bearer "+admin.Token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Verify role changed
	updated, _ := mgr.GetByID(target.ID)
	if updated.Role != session.RoleOperator {
		t.Errorf("role = %q, want %q", updated.Role, session.RoleOperator)
	}
}

func TestHandleUpdateUserRole_Forbidden(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	operator, _ := mgr.Create("Operator", "secret") // Operator, not Admin

	body := `{"role":"admin"}`
	r := httptest.NewRequest("PUT", "/api/users/some-id/role", bytes.NewBufferString(body))
	r.Header.Set("Authorization", "Bearer "+operator.Token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 403 {
		t.Errorf("status = %d, want 403 (operator cannot use admin endpoints)", w.Code)
	}
}

func TestHandleRemoveUser(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	admin, _ := mgr.Create("Admin", "secret")
	mgr.UpdateRole(admin.ID, session.RoleAdmin)

	target, _ := mgr.Create("Bob", "")

	r := httptest.NewRequest("DELETE", "/api/users/"+target.ID, nil)
	r.Header.Set("Authorization", "Bearer "+admin.Token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Verify user was removed
	_, ok := mgr.GetByID(target.ID)
	if ok {
		t.Error("expected user to be removed")
	}
}

func TestRoleGating_OperatorEndpoints(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	observer, _ := mgr.Create("Observer", "")

	// Observer should not be able to POST /api/beacon
	r := httptest.NewRequest("POST", "/api/beacon", nil)
	r.Header.Set("Authorization", "Bearer "+observer.Token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 403 {
		t.Errorf("observer POST /api/beacon: status = %d, want 403", w.Code)
	}
}

func TestRoleGating_PlotterEndpoints(t *testing.T) {
	mgr := session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
	srv := testServer(WithSessionManager(mgr))

	observer, _ := mgr.Create("Observer", "")

	// Observer should not be able to POST /api/annotations
	body := `{"type":"point","label":"test","geometry":"{\"type\":\"Point\",\"coordinates\":[0,0]}"}`
	r := httptest.NewRequest("POST", "/api/annotations", bytes.NewBufferString(body))
	r.Header.Set("Authorization", "Bearer "+observer.Token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != 403 {
		t.Errorf("observer POST /api/annotations: status = %d, want 403", w.Code)
	}
}
