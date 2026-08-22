package auth

import (
	"net/http"
	"strings"

	"github.com/vaultviewer/vaultviewer/internal/model"
)

// AuthenticatedHandler is an http.HandlerFunc that also receives the
// caller resolved from their session token.
type AuthenticatedHandler func(w http.ResponseWriter, r *http.Request, user model.User)

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header.
func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimPrefix(header, prefix)
}

// RequireAuth wraps next so it only runs for requests bearing a valid
// session token, satisfying the project's "RBAC always enforced" rule for
// every API endpoint. The resolved user is passed through to next.
func RequireAuth(sm *SessionManager, next AuthenticatedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		user, err := sm.Verify(token)
		if err != nil {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}
		next(w, r, *user)
	}
}

// RequireWrite wraps next so it also rejects callers whose role cannot
// create/update resources (RoleView), on top of RequireAuth's checks.
func RequireWrite(sm *SessionManager, next AuthenticatedHandler) http.HandlerFunc {
	return RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
		if !user.Role.CanWrite() {
			http.Error(w, "role does not permit write access", http.StatusForbidden)
			return
		}
		next(w, r, user)
	})
}

// RequireDelete wraps next so it also rejects callers whose role cannot
// delete resources (only RoleAdmin), on top of RequireAuth's checks.
func RequireDelete(sm *SessionManager, next AuthenticatedHandler) http.HandlerFunc {
	return RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
		if !user.Role.CanDelete() {
			http.Error(w, "role does not permit delete access", http.StatusForbidden)
			return
		}
		next(w, r, user)
	})
}
