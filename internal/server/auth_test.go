package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/session"
)

func testSessionManager() *session.MemoryManager {
	return session.NewMemoryManager(session.MemoryManagerConfig{
		PIN:               "secret",
		InactivityTimeout: 30 * time.Minute,
	})
}

func TestUserFromContext_NoUser(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	user, ok := UserFromContext(r.Context())
	if ok {
		t.Error("expected no user in context")
	}
	if user != nil {
		t.Error("expected nil user")
	}
}

func TestSessionMiddleware_BearerToken(t *testing.T) {
	mgr := testSessionManager()
	user, _ := mgr.Create("Alice", "secret")

	mw := SessionMiddleware(mgr)

	var gotUser *session.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFromContext(r.Context())
		gotUser = u
		w.WriteHeader(200)
	}))

	r := httptest.NewRequest("GET", "/api/stations", nil)
	r.Header.Set("Authorization", "Bearer "+user.Token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.ID != user.ID {
		t.Errorf("user ID = %q, want %q", gotUser.ID, user.ID)
	}
}

func TestSessionMiddleware_NoToken(t *testing.T) {
	mgr := testSessionManager()
	mw := SessionMiddleware(mgr)

	var gotUser *session.User
	var gotOK bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotOK = UserFromContext(r.Context())
		w.WriteHeader(200)
	}))

	r := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if gotOK {
		t.Error("expected no user for unauthenticated request")
	}
	if gotUser != nil {
		t.Error("expected nil user")
	}
	if w.Code != 200 {
		t.Errorf("status = %d, want 200 (passthrough for public routes)", w.Code)
	}
}

func TestSessionMiddleware_InvalidToken(t *testing.T) {
	mgr := testSessionManager()
	mw := SessionMiddleware(mgr)

	var gotOK bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotOK = UserFromContext(r.Context())
		w.WriteHeader(200)
	}))

	r := httptest.NewRequest("GET", "/api/stations", nil)
	r.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if gotOK {
		t.Error("expected no user for invalid token")
	}
}

func TestRequireRole_Sufficient(t *testing.T) {
	mgr := testSessionManager()
	user, _ := mgr.Create("Alice", "secret") // Operator role

	smw := SessionMiddleware(mgr)
	rmw := RequireRole(session.RoleObserver) // Operator >= Observer

	handler := smw(rmw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})))

	r := httptest.NewRequest("POST", "/api/messages", nil)
	r.Header.Set("Authorization", "Bearer "+user.Token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRequireRole_Insufficient(t *testing.T) {
	mgr := testSessionManager()
	user, _ := mgr.Create("Bob", "") // Observer role (wrong PIN)

	smw := SessionMiddleware(mgr)
	rmw := RequireRole(session.RoleOperator) // Observer < Operator

	handler := smw(rmw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})))

	r := httptest.NewRequest("POST", "/api/messages", nil)
	r.Header.Set("Authorization", "Bearer "+user.Token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestRequireRole_NoSession(t *testing.T) {
	rmw := RequireRole(session.RoleObserver)

	handler := rmw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	r := httptest.NewRequest("GET", "/api/stations", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireRole_Hierarchy(t *testing.T) {
	mgr := testSessionManager()

	tests := []struct {
		name     string
		userRole session.Role
		minRole  session.Role
		wantCode int
	}{
		{"observer can view", session.RoleObserver, session.RoleObserver, 200},
		{"observer cannot send", session.RoleObserver, session.RoleOperator, 403},
		{"operator can send", session.RoleOperator, session.RoleOperator, 200},
		{"operator cannot admin", session.RoleOperator, session.RoleAdmin, 403},
		{"admin can do all", session.RoleAdmin, session.RoleAdmin, 200},
		{"plotter can annotate", session.RolePlotter, session.RolePlotter, 200},
		{"plotter cannot send", session.RolePlotter, session.RoleOperator, 403},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, _ := mgr.Create("Test", "")
			mgr.UpdateRole(user.ID, tt.userRole)

			smw := SessionMiddleware(mgr)
			rmw := RequireRole(tt.minRole)

			handler := smw(rmw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			})))

			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", "Bearer "+user.Token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}
