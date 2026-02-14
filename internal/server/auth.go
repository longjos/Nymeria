package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/narvel/nymeria/internal/session"
)

type contextKey string

const userContextKey contextKey = "user"

// UserFromContext extracts the authenticated user from the request context.
func UserFromContext(ctx context.Context) (*session.User, bool) {
	user, ok := ctx.Value(userContextKey).(*session.User)
	return user, ok
}

// SessionMiddleware extracts a session token from the Authorization header
// and attaches the user to the request context. Requests without a valid
// token pass through with no user (for public endpoints).
func SessionMiddleware(mgr session.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token != "" {
				if user, ok := mgr.Get(token); ok {
					mgr.Touch(token)
					ctx := context.WithValue(r.Context(), userContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole returns middleware that rejects requests from users below
// the given minimum role. Returns 401 if no session, 403 if insufficient role
// or if the user is not yet approved.
func RequireRole(minRole session.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
				return
			}
			if user.Status != session.StatusApproved {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "access pending approval"})
				return
			}
			if session.RoleLevel(user.Role) < session.RoleLevel(minRole) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
