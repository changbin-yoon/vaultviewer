package api

import (
	"encoding/json"
	"net/http"

	"github.com/accesslens/accesslens/internal/auth"
	"github.com/accesslens/accesslens/internal/model"
	"github.com/accesslens/accesslens/internal/opa"
)

func registerIntegrationRoutes(mux *http.ServeMux, d Deps) {
	// Dashboard status card for Trino — a connectivity check plus the
	// operator-configured role/catalog labels (not a live GRANT lookup, see
	// internal/trino). Always returns 200; "enabled: false" is how the
	// frontend tells "not configured" apart from "configured but down".
	mux.HandleFunc("GET /api/trino", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		w.Header().Set("Content-Type", "application/json")
		if !d.Trino.Enabled() {
			json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
			return
		}
		connected, err := d.TrinoClient.CheckConnection(r.Context())
		if err != nil {
			connected = false
		}
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":   true,
			"connected": connected,
			"role":      d.Trino.RoleMap[user.Role],
			"catalogs":  d.Trino.Catalogs,
		})
	}))

	// Dashboard status card for OPA — resolves the caller's mapped LDAP
	// group against OPA's live grants document (see internal/opa). One
	// step more live than Trino's card: team/catalogs/operations come from
	// OPA itself, not AccessLens's own Helm values.
	mux.HandleFunc("GET /api/opa", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		w.Header().Set("Content-Type", "application/json")
		if !d.Opa.Enabled() {
			json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
			return
		}
		grants, err := d.OpaClient.Resolve(r.Context(), d.Opa.GroupMap[user.Role])
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"enabled": true, "connected": false})
			return
		}
		if grants == nil {
			grants = []opa.Grant{} // serialize as `[]`, not `null`, when no group is mapped
		}
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":   true,
			"connected": true,
			"grants":    grants,
		})
	}))

	// Dashboard status card for S3 IAM — a connectivity check (does the
	// fixed LDAP service account still successfully AssumeRoleWithLDAPIdentity
	// against the S3 endpoint) plus operator-configured role/bucket labels,
	// same shape as Trino's card (see internal/s3iam).
	mux.HandleFunc("GET /api/s3iam", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		w.Header().Set("Content-Type", "application/json")
		if !d.S3Iam.Enabled() {
			json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
			return
		}
		creds, err := d.S3IamClient.CheckConnection(r.Context())
		if err != nil {
			creds = nil
		}
		resp := map[string]any{
			"enabled":   true,
			"connected": creds != nil,
			"role":      d.S3Iam.RoleMap[user.Role],
			"buckets":   d.S3Iam.Buckets,
		}
		// accessKeyId/expiresAt are the temporary STS session's own
		// identifier and expiry — not a secret on their own (no secret key
		// or session token is ever parsed/returned, see internal/s3iam.
		// Credentials) — shown as proof the check produced a real, live
		// session, not just that the endpoint answered.
		if creds != nil {
			resp["accessKeyId"] = creds.AccessKeyID
			if !creds.Expiration.IsZero() {
				resp["expiresAt"] = creds.Expiration
			}
		}
		json.NewEncoder(w).Encode(resp)
	}))

	mux.HandleFunc("GET /api/config", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(d.ConfigInfo)
	}))

	mux.HandleFunc("GET /api/group-teams", auth.RequireAdmin(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		m, err := d.TeamsStore.Get()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	}))

	mux.HandleFunc("PUT /api/group-teams", auth.RequireAdmin(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		var m map[string]string
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := d.TeamsStore.Set(m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
