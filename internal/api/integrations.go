package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/accesslens/accesslens/internal/auth"
	"github.com/accesslens/accesslens/internal/model"
	"github.com/accesslens/accesslens/internal/opa"
)

// teamRoles converts a user's resolved team grants (see auth.ResolveTeams)
// into the opa package's own TeamRole type, so callers here don't need
// internal/opa to depend on internal/model.
func teamRoles(teams []model.TeamGrant) []opa.TeamRole {
	out := make([]opa.TeamRole, len(teams))
	for i, t := range teams {
		out[i] = opa.TeamRole{Team: t.Team, Role: string(t.Role)}
	}
	return out
}

// teamNames extracts just the team names, for the "teams" field shown on
// the Trino/S3 IAM cards. user.Teams is already sorted by team name (see
// auth.ResolveTeams), so this stays sorted too.
func teamNames(teams []model.TeamGrant) []string {
	names := make([]string, len(teams))
	for i, t := range teams {
		names[i] = t.Team
	}
	return names
}

// uniqueSorted flattens and deduplicates one or more string lists.
func uniqueSorted(lists ...[]string) []string {
	set := map[string]struct{}{}
	for _, l := range lists {
		for _, v := range l {
			set[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func registerIntegrationRoutes(mux *http.ServeMux, d Deps) {
	// Dashboard status card for Trino — a connectivity check plus a
	// catalog list. For an account with team-scoped LDAP groups (e.g.
	// "bi-adm"), catalogs are the union of every team's catalogs from
	// OPA's live teams map (deduplicated) — Trino's real access control is
	// OPA in this deployment, so this is what the account can actually
	// query, not just an operator-set label. Accounts with no team grants
	// fall back to the flat operator-configured Trino.Catalogs list.
	// "role" always stays the caller's single overall resolved role
	// (highest-precedence across every group they're in), never per-team.
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
		catalogs := d.Trino.Catalogs
		if len(user.Teams) > 0 && d.Opa.Enabled() {
			if grants, err := d.OpaClient.ResolveTeams(r.Context(), teamRoles(user.Teams)); err == nil {
				lists := make([][]string, len(grants))
				for i, g := range grants {
					lists[i] = g.Catalogs
				}
				catalogs = uniqueSorted(lists...)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":   true,
			"connected": connected,
			"role":      d.Trino.RoleMap[user.Role],
			"catalogs":  catalogs,
			"teams":     teamNames(user.Teams),
		})
	}))

	// Dashboard status card for OPA — resolves the caller's live grants
	// document (see internal/opa). An account with team-scoped LDAP groups
	// resolves each team directly by name (ResolveTeams); an account
	// without any (e.g. plain "adm") falls back to the legacy
	// role->group->wildcard-team resolution (Resolve).
	mux.HandleFunc("GET /api/opa", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		w.Header().Set("Content-Type", "application/json")
		if !d.Opa.Enabled() {
			json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
			return
		}
		var grants []opa.Grant
		var err error
		if len(user.Teams) > 0 {
			grants, err = d.OpaClient.ResolveTeams(r.Context(), teamRoles(user.Teams))
		} else {
			grants, err = d.OpaClient.Resolve(r.Context(), d.Opa.GroupMap[user.Role])
		}
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
	// against the S3 endpoint) plus a bucket list. Same team-union pattern
	// as Trino's catalogs: an account with team-scoped groups gets the
	// union of S3Iam.BucketMap[team] across every team it belongs to
	// (deduplicated); otherwise the flat operator-configured Buckets list.
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
		buckets := d.S3Iam.Buckets
		if len(user.Teams) > 0 {
			lists := make([][]string, len(user.Teams))
			for i, t := range user.Teams {
				lists[i] = d.S3Iam.BucketMap[t.Team]
			}
			if unioned := uniqueSorted(lists...); len(unioned) > 0 {
				buckets = unioned
			}
		}
		resp := map[string]any{
			"enabled":   true,
			"connected": creds != nil,
			"role":      d.S3Iam.RoleMap[user.Role],
			"buckets":   buckets,
			"teams":     teamNames(user.Teams),
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
