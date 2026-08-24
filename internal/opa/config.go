// Package opa queries an OPA (Open Policy Agent) server for the grants
// document it evaluates access decisions from, and resolves what a given
// AccessLens role is actually entitled to — team, allowed operations, and
// catalogs — by replicating the same group/team/role-ops lookup its Rego
// policy performs. Kept separate from storage/auth/audit per the project's
// functional-separation convention.
package opa

import (
	"os"

	"github.com/accesslens/accesslens/internal/model"
)

// Config configures the OPA client. No credentials — this package assumes
// an internal/trusted OPA instance with no authentication configured
// (matching how Trino itself calls the same OPA for access-control
// decisions, unauthenticated).
type Config struct {
	// Endpoint is the OPA server's host:port, e.g.
	// "opa.trino-verify.svc.cluster.local:8181". Always dialed over HTTP —
	// OPA has no PASSWORD-style auth like Trino's, so there's no reason to
	// require TLS.
	Endpoint string
	// GroupMap maps an AccessLens role to the LDAP group name OPA's policy
	// resolves grants for (e.g. RoleAdmin -> "dt-bi-adm"). This is a
	// different LDAP directory than AccessLens's own — see
	// internal/trino's Config for the same distinction.
	GroupMap map[model.Role]string
}

// Enabled reports whether enough configuration is present to query OPA.
// Opt-in: leaving ACCESSLENS_OPA_ENDPOINT unset disables it entirely.
func (c Config) Enabled() bool {
	return c.Endpoint != ""
}

// LoadConfigFromEnv builds a Config from environment variables:
//
//	ACCESSLENS_OPA_ENDPOINT     (unset disables the integration entirely)
//	ACCESSLENS_OPA_GROUP_ADM
//	ACCESSLENS_OPA_GROUP_DEV
//	ACCESSLENS_OPA_GROUP_VIEW
//
// A role with no configured group simply resolves to no grants (queried
// against an empty group name) rather than an error — an honest "nothing
// mapped yet" rather than a hard failure.
func LoadConfigFromEnv() Config {
	cfg := Config{
		Endpoint: os.Getenv("ACCESSLENS_OPA_ENDPOINT"),
		GroupMap: map[model.Role]string{
			model.RoleAdmin: os.Getenv("ACCESSLENS_OPA_GROUP_ADM"),
			model.RoleDev:   os.Getenv("ACCESSLENS_OPA_GROUP_DEV"),
			model.RoleView:  os.Getenv("ACCESSLENS_OPA_GROUP_VIEW"),
		},
	}
	return cfg
}
