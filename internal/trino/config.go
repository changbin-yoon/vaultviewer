// Package trino queries a Trino coordinator for a lightweight connection
// status, and maps AccessLens roles to Trino role labels/catalog names
// configured by the operator. It does not query Trino's own GRANT model —
// the role/catalog info shown to users is config-driven, not a live
// per-user permission lookup. Kept separate from storage/auth/audit per the
// project's functional-separation convention.
package trino

import (
	"os"
	"strings"

	"github.com/accesslens/accesslens/internal/model"
)

// Config configures the Trino status client. All values are sourced from
// the environment (or a Kubernetes Secret projected into it) — the
// username/password are never hardcoded, per project policy.
type Config struct {
	// Endpoint is the coordinator's host:port, e.g.
	// "trino.trino-verify.svc.cluster.local:8443" or a NodePort address.
	// Always dialed over HTTPS.
	Endpoint string
	// Username/Password authenticate against Trino's PASSWORD auth (backed
	// by its own LDAP, independent of AccessLens's).
	Username string
	Password string
	// InsecureSkipVerify skips TLS certificate verification — needed for
	// coordinators using a self-signed cert. Off by default; only turn on
	// for known test/internal deployments.
	InsecureSkipVerify bool
	// RoleMap maps an AccessLens role to the Trino role label shown on the
	// dashboard. Defaults to reusing the AccessLens role name unchanged.
	RoleMap map[model.Role]string
	// Catalogs is the configured list of catalog names to display —
	// not queried live from Trino.
	Catalogs []string
}

// Enabled reports whether enough configuration is present to query Trino.
// Opt-in: leaving ACCESSLENS_TRINO_ENDPOINT unset disables it entirely
// rather than failing startup.
func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.Username != "" && c.Password != ""
}

// LoadConfigFromEnv builds a Config from environment variables:
//
//	ACCESSLENS_TRINO_ENDPOINT               (unset disables the integration entirely)
//	ACCESSLENS_TRINO_USERNAME
//	ACCESSLENS_TRINO_PASSWORD
//	ACCESSLENS_TRINO_INSECURE_SKIP_VERIFY   (default false)
//	ACCESSLENS_TRINO_ROLE_ADM               (default "adm")
//	ACCESSLENS_TRINO_ROLE_DEV               (default "dev")
//	ACCESSLENS_TRINO_ROLE_VIEW              (default "view")
//	ACCESSLENS_TRINO_CATALOGS               (comma-separated, default empty)
func LoadConfigFromEnv() Config {
	cfg := Config{
		Endpoint:           os.Getenv("ACCESSLENS_TRINO_ENDPOINT"),
		Username:           os.Getenv("ACCESSLENS_TRINO_USERNAME"),
		Password:           os.Getenv("ACCESSLENS_TRINO_PASSWORD"),
		InsecureSkipVerify: os.Getenv("ACCESSLENS_TRINO_INSECURE_SKIP_VERIFY") == "true",
		RoleMap: map[model.Role]string{
			model.RoleAdmin: envOr("ACCESSLENS_TRINO_ROLE_ADM", "adm"),
			model.RoleDev:   envOr("ACCESSLENS_TRINO_ROLE_DEV", "dev"),
			model.RoleView:  envOr("ACCESSLENS_TRINO_ROLE_VIEW", "view"),
		},
		// Non-nil so the /api/trino response serializes as `[]`, not
		// `null`, when no catalogs are configured.
		Catalogs: []string{},
	}
	if v := os.Getenv("ACCESSLENS_TRINO_CATALOGS"); v != "" {
		for _, c := range strings.Split(v, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				cfg.Catalogs = append(cfg.Catalogs, c)
			}
		}
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
