// Package s3iam checks connectivity to an S3-compatible endpoint's LDAP
// identity provider (MinIO's AssumeRoleWithLDAPIdentity STS action) using a
// fixed service account, and reports operator-configured role/bucket
// labels — the same "connectivity check + config-driven labels" shape as
// internal/trino, not a live bucket-policy lookup. Kept separate from
// storage/auth/audit per the project's functional-separation convention.
package s3iam

import (
	"os"
	"strings"

	"github.com/accesslens/accesslens/internal/model"
)

// Config configures the S3 IAM status client. All values are sourced from
// the environment (or a Kubernetes Secret projected into it) — the LDAP
// username/password are never hardcoded, per project policy.
type Config struct {
	// Endpoint is the S3 API host:port, e.g. "minio.example:9000". Always
	// dialed over HTTP — MinIO in this deployment has no TLS in front of
	// it (unlike Trino's coordinator).
	Endpoint string
	// LDAPUsername/LDAPPassword authenticate a fixed service account
	// against the S3 endpoint's LDAP identity provider via
	// AssumeRoleWithLDAPIdentity — the same directory AccessLens itself
	// authenticates against (see internal/auth), just a dedicated
	// least-privilege account rather than the logged-in user's own
	// credentials (which AccessLens doesn't retain after login).
	LDAPUsername string
	LDAPPassword string
	// RoleMap maps an AccessLens role to the label shown on the
	// dashboard. Defaults to reusing the AccessLens role name unchanged.
	RoleMap map[model.Role]string
	// Buckets is the configured list of bucket names to display — not
	// queried live from the S3 endpoint.
	Buckets []string
}

// Enabled reports whether enough configuration is present to check S3 IAM
// connectivity. Opt-in: leaving ACCESSLENS_S3IAM_ENDPOINT unset disables it
// entirely rather than failing startup.
func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.LDAPUsername != "" && c.LDAPPassword != ""
}

// LoadConfigFromEnv builds a Config from environment variables:
//
//	ACCESSLENS_S3IAM_ENDPOINT       (unset disables the integration entirely)
//	ACCESSLENS_S3IAM_LDAP_USERNAME
//	ACCESSLENS_S3IAM_LDAP_PASSWORD
//	ACCESSLENS_S3IAM_ROLE_ADM       (default "adm")
//	ACCESSLENS_S3IAM_ROLE_DEV       (default "dev")
//	ACCESSLENS_S3IAM_ROLE_VIEW      (default "view")
//	ACCESSLENS_S3IAM_BUCKETS        (comma-separated, default empty)
func LoadConfigFromEnv() Config {
	cfg := Config{
		Endpoint:     os.Getenv("ACCESSLENS_S3IAM_ENDPOINT"),
		LDAPUsername: os.Getenv("ACCESSLENS_S3IAM_LDAP_USERNAME"),
		LDAPPassword: os.Getenv("ACCESSLENS_S3IAM_LDAP_PASSWORD"),
		RoleMap: map[model.Role]string{
			model.RoleAdmin: envOr("ACCESSLENS_S3IAM_ROLE_ADM", "adm"),
			model.RoleDev:   envOr("ACCESSLENS_S3IAM_ROLE_DEV", "dev"),
			model.RoleView:  envOr("ACCESSLENS_S3IAM_ROLE_VIEW", "view"),
		},
		// Non-nil so the /api/s3iam response serializes as `[]`, not
		// `null`, when no buckets are configured.
		Buckets: []string{},
	}
	if v := os.Getenv("ACCESSLENS_S3IAM_BUCKETS"); v != "" {
		for _, b := range strings.Split(v, ",") {
			b = strings.TrimSpace(b)
			if b != "" {
				cfg.Buckets = append(cfg.Buckets, b)
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
