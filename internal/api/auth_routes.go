package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/accesslens/accesslens/internal/auth"
	"github.com/accesslens/accesslens/internal/model"
)

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// teamGrantJSON is model.TeamGrant with lowercase JSON field names matching
// the rest of this package's API responses.
type teamGrantJSON struct {
	Team string `json:"team"`
	Role string `json:"role"`
}

func teamGrantsJSON(teams []model.TeamGrant) []teamGrantJSON {
	// Always serialize as [] rather than null for an empty/nil slice, so
	// frontend code can iterate without a null-check.
	out := make([]teamGrantJSON, len(teams))
	for i, t := range teams {
		out[i] = teamGrantJSON{Team: t.Team, Role: string(t.Role)}
	}
	return out
}

func registerAuthRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		throttleKey := strings.ToLower(strings.TrimSpace(creds.Username))
		if allowed, wait := d.LoginThrottle.Allow(throttleKey); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			http.Error(w, "too many failed login attempts, try again shortly", http.StatusTooManyRequests)
			return
		}

		user, err := d.Authenticator.Authenticate(creds.Username, creds.Password)
		if err != nil {
			status := http.StatusUnauthorized
			if !errors.Is(err, auth.ErrInvalidCredentials) && !errors.Is(err, auth.ErrNoRole) {
				// An LDAP/network failure isn't the caller's fault — don't
				// spend part of their backoff budget on it.
				log.Printf("ldap authenticate error: %v", err)
				status = http.StatusBadGateway
			} else {
				d.LoginThrottle.RecordFailure(throttleKey)
			}
			http.Error(w, "authentication failed", status)
			return
		}
		d.LoginThrottle.RecordSuccess(throttleKey)

		token, err := d.Sessions.Issue(*user)
		if err != nil {
			http.Error(w, "failed to issue session", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"token":      token,
			"username":   user.Username,
			"role":       string(user.Role),
			"department": user.Department,
			"teams":      teamGrantsJSON(user.Teams),
		})
	})

	mux.HandleFunc("GET /api/me", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"username":   user.Username,
			"role":       string(user.Role),
			"department": user.Department,
			"teams":      teamGrantsJSON(user.Teams),
		})
	}))
}
