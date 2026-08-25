// Package auth implements LDAP authentication and LDAP-group-to-role RBAC
// for AccessLens, kept separate from storage and audit per the project's
// functional-separation convention.
package auth

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/accesslens/accesslens/internal/model"
)

// DefaultGroupSearchFilter is the LDAP filter template used to find the
// groups a user belongs to when ACCESSLENS_LDAP_GROUP_SEARCH_FILTER is
// unset. It is byte-identical to the filter AccessLens has always used, so
// existing deployments (e.g. cluster-mesh1) see no behavior change on
// upgrade.
const DefaultGroupSearchFilter = "(&(objectClass=groupOfNames)(member={{.UserDN}}))"

// GroupFilterParams are the fields available to a group-search-filter
// template (see ParseGroupFilterTemplate). Both fields are pre-escaped with
// goldap.EscapeFilter by the caller before the template executes, so a
// template author never needs to escape them again.
type GroupFilterParams struct {
	// UserDN is the authenticated user's full directory DN.
	UserDN string
	// UID is the authenticated user's login username, useful for schemas
	// that key group membership off a bare uid rather than a full DN (e.g.
	// posixGroup's memberUid).
	UID string
}

// ParseGroupFilterTemplate parses filter as a Go template producing an LDAP
// search filter — the same configurability Trino's LDAP group-provider
// exposes via ldap.group-search-filter, letting a deployment match whatever
// group-membership schema its directory actually uses (groupOfNames member,
// posixGroup memberUid, groupOfUniqueNames uniqueMember, AD nested-group
// matching-rule filters, ...) without a code change.
//
// The template is validated immediately — parsed AND executed against
// dummy params — so a malformed filter (bad template syntax, or a typo'd
// field name like {{.Bogus}}) fails at process startup, not on a user's
// first login.
func ParseGroupFilterTemplate(filter string) (*template.Template, error) {
	tmpl, err := template.New("group-search-filter").Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("parse group search filter template: %w", err)
	}
	probe := GroupFilterParams{UserDN: "cn=probe,dc=example,dc=com", UID: "probe"}
	if err := tmpl.Execute(&bytes.Buffer{}, probe); err != nil {
		return nil, fmt.Errorf("group search filter template failed validation: %w", err)
	}
	return tmpl, nil
}

// Config configures an LDAPAuthenticator. All values are sourced from the
// environment (or a Kubernetes Secret projected into it) — nothing here is
// ever hardcoded, per project policy.
type Config struct {
	// Host and Port address the LDAP server, e.g. the in-cluster Service
	// DNS name "openldap.<namespace>.svc.cluster.local" and 389.
	Host   string
	Port   int
	UseTLS bool

	// BaseDN is the root of the directory, e.g. "dc=example,dc=com".
	BaseDN string
	// UserOU and GroupOU are the organizational units under BaseDN holding
	// user and group entries respectively.
	UserOU  string
	GroupOU string

	// BindDN/BindPassword authenticate the service account used to search
	// for a user's DN and group memberships before verifying their
	// password with a second bind as that user.
	BindDN       string
	BindPassword string

	// GroupRoleMap maps an LDAP group CN to the role it grants. Group
	// naming varies per directory (e.g. this environment's seed data uses
	// "dt-bi-adm" rather than a bare "adm"), so it is configurable rather
	// than assumed. Multiple CNs may map to the same role (e.g. both
	// "platform-admins" and "dt-bi-adm" granting RoleAdmin) — see
	// LoadConfigFromEnv's comma-separated parsing below.
	GroupRoleMap map[string]model.Role

	// GroupSearchFilter is a Go template producing the LDAP filter used to
	// find the groups a user belongs to (see ParseGroupFilterTemplate).
	// Defaults to DefaultGroupSearchFilter. This only changes which group
	// CNs the search returns — GroupRoleMap still decides how those CNs map
	// to a role.
	GroupSearchFilter string
}

// UserBaseDN returns the DN searched for user entries.
func (c Config) UserBaseDN() string {
	return c.UserOU + "," + c.BaseDN
}

// GroupBaseDN returns the DN searched for group entries.
func (c Config) GroupBaseDN() string {
	return c.GroupOU + "," + c.BaseDN
}

// LoadConfigFromEnv builds a Config from environment variables:
//
//	ACCESSLENS_LDAP_HOST          (required)
//	ACCESSLENS_LDAP_PORT          (default 389)
//	ACCESSLENS_LDAP_TLS           (default false)
//	ACCESSLENS_LDAP_BASE_DN       (required, e.g. "dc=example,dc=com")
//	ACCESSLENS_LDAP_USER_OU       (default "ou=users")
//	ACCESSLENS_LDAP_GROUP_OU      (default "ou=groups")
//	ACCESSLENS_LDAP_BIND_DN       (required, service bind DN for search)
//	ACCESSLENS_LDAP_BIND_PASSWORD (required)
//	ACCESSLENS_LDAP_GROUP_ADM     (default "adm", comma-separated for multiple groups)
//	ACCESSLENS_LDAP_GROUP_DEV     (default "dev", comma-separated for multiple groups)
//	ACCESSLENS_LDAP_GROUP_VIEW    (default "view", comma-separated for multiple groups)
//	ACCESSLENS_LDAP_GROUP_SEARCH_FILTER (default DefaultGroupSearchFilter; a Go
//	                              template producing the LDAP filter used to
//	                              find a user's groups — fields {{.UserDN}}
//	                              and {{.UID}} available, see GroupFilterParams)
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		Host:              os.Getenv("ACCESSLENS_LDAP_HOST"),
		Port:              389,
		UseTLS:            os.Getenv("ACCESSLENS_LDAP_TLS") == "true",
		BaseDN:            os.Getenv("ACCESSLENS_LDAP_BASE_DN"),
		UserOU:            envOr("ACCESSLENS_LDAP_USER_OU", "ou=users"),
		GroupOU:           envOr("ACCESSLENS_LDAP_GROUP_OU", "ou=groups"),
		BindDN:            os.Getenv("ACCESSLENS_LDAP_BIND_DN"),
		BindPassword:      os.Getenv("ACCESSLENS_LDAP_BIND_PASSWORD"),
		GroupRoleMap:      map[string]model.Role{},
		GroupSearchFilter: envOr("ACCESSLENS_LDAP_GROUP_SEARCH_FILTER", DefaultGroupSearchFilter),
	}
	addGroupRoles(cfg.GroupRoleMap, "ACCESSLENS_LDAP_GROUP_ADM", "adm", model.RoleAdmin)
	addGroupRoles(cfg.GroupRoleMap, "ACCESSLENS_LDAP_GROUP_DEV", "dev", model.RoleDev)
	addGroupRoles(cfg.GroupRoleMap, "ACCESSLENS_LDAP_GROUP_VIEW", "view", model.RoleView)

	if portStr := os.Getenv("ACCESSLENS_LDAP_PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return Config{}, fmt.Errorf("parse ACCESSLENS_LDAP_PORT: %w", err)
		}
		cfg.Port = port
	}

	var missing []string
	if cfg.Host == "" {
		missing = append(missing, "ACCESSLENS_LDAP_HOST")
	}
	if cfg.BaseDN == "" {
		missing = append(missing, "ACCESSLENS_LDAP_BASE_DN")
	}
	if cfg.BindDN == "" {
		missing = append(missing, "ACCESSLENS_LDAP_BIND_DN")
	}
	if cfg.BindPassword == "" {
		missing = append(missing, "ACCESSLENS_LDAP_BIND_PASSWORD")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required env vars: %v", missing)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// addGroupRoles splits a comma-separated group-CN list (from key, or
// fallback if unset) and maps each CN to role, so one role can be granted
// by several LDAP groups at once — e.g. "dt-bi-adm,platform-admins".
func addGroupRoles(mapping map[string]model.Role, key, fallback string, role model.Role) {
	for _, cn := range strings.Split(envOr(key, fallback), ",") {
		cn = strings.TrimSpace(cn)
		if cn != "" {
			mapping[cn] = role
		}
	}
}
