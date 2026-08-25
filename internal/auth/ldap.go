package auth

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"text/template"
	"time"

	goldap "github.com/go-ldap/ldap/v3"

	"github.com/accesslens/accesslens/internal/model"
	"github.com/accesslens/accesslens/internal/teams"
)

// ErrInvalidCredentials is returned when a username/password pair does not
// authenticate against the directory.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ErrNoRole is returned when a user authenticates successfully but belongs
// to no LDAP group mapped to a AccessLens role, so they are granted no
// access.
var ErrNoRole = errors.New("auth: user has no role-granting group membership")

const dialTimeout = 10 * time.Second

// Authenticator verifies a username/password pair and resolves the
// caller's RBAC role.
type Authenticator interface {
	Authenticate(username, password string) (*model.User, error)
}

// LDAPAuthenticator implements Authenticator against an LDAP/Active
// Directory-style server using a search-then-bind flow: it binds as a
// service account to locate the user's DN and group memberships, then
// binds as that DN with the supplied password to verify credentials.
type LDAPAuthenticator struct {
	cfg             Config
	teams           teams.Store
	groupFilterTmpl *template.Template
}

var _ Authenticator = (*LDAPAuthenticator)(nil)

// NewLDAPAuthenticator constructs an LDAPAuthenticator from cfg. teamsStore
// resolves a user's displayed "소속" (affiliation) from their LDAP group
// membership — see resolveDepartment. groupFilterTmpl is the parsed
// group-search-filter template (see ParseGroupFilterTemplate) — callers
// should parse cfg.GroupSearchFilter once at startup and pass the result
// here so a malformed filter fails fast rather than on first login; a nil
// groupFilterTmpl falls back to DefaultGroupSearchFilter.
func NewLDAPAuthenticator(cfg Config, teamsStore teams.Store, groupFilterTmpl *template.Template) *LDAPAuthenticator {
	if groupFilterTmpl == nil {
		groupFilterTmpl = template.Must(ParseGroupFilterTemplate(DefaultGroupSearchFilter))
	}
	return &LDAPAuthenticator{cfg: cfg, teams: teamsStore, groupFilterTmpl: groupFilterTmpl}
}

func (a *LDAPAuthenticator) dial() (*goldap.Conn, error) {
	scheme := "ldap"
	if a.cfg.UseTLS {
		scheme = "ldaps"
	}
	addr := fmt.Sprintf("%s://%s:%d", scheme, a.cfg.Host, a.cfg.Port)
	conn, err := goldap.DialURL(addr, goldap.DialWithDialer(&net.Dialer{Timeout: dialTimeout}))
	if err != nil {
		return nil, fmt.Errorf("dial ldap server: %w", err)
	}
	return conn, nil
}

func (a *LDAPAuthenticator) Authenticate(username, password string) (*model.User, error) {
	if username == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	search, err := a.dial()
	if err != nil {
		return nil, err
	}
	defer search.Close()

	if err := search.Bind(a.cfg.BindDN, a.cfg.BindPassword); err != nil {
		return nil, fmt.Errorf("bind service account: %w", err)
	}

	userDN, department, err := a.lookupUser(search, username)
	if err != nil {
		return nil, err
	}

	groups, err := a.lookupGroupCNs(search, userDN, username)
	if err != nil {
		return nil, err
	}

	// Verify the supplied password with a separate bind as the user,
	// rather than reusing the service connection, so a failed user bind
	// can never be mistaken for the service account's session state.
	verify, err := a.dial()
	if err != nil {
		return nil, err
	}
	defer verify.Close()
	if err := verify.Bind(userDN, password); err != nil {
		var ldapErr *goldap.Error
		if errors.As(err, &ldapErr) && ldapErr.ResultCode == goldap.LDAPResultInvalidCredentials {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("verify user credentials: %w", err)
	}

	role, ok := ResolveRole(groups, a.cfg.GroupRoleMap)
	if !ok {
		return nil, ErrNoRole
	}

	return &model.User{
		Username:   username,
		Role:       role,
		Department: a.resolveDepartment(groups, department),
		Teams:      ResolveTeams(groups),
	}, nil
}

// resolveDepartment prefers the admin-managed group-to-team-name mapping
// (Settings page, editable at runtime) over the LDAP "o" attribute
// (ldapOrg, fixed per-directory-entry): the mapping is what an admin
// without LDAP write access can actually keep up to date. The first of the
// user's groups with an entry in the mapping wins; if none match, ldapOrg
// is used as-is (possibly empty).
func (a *LDAPAuthenticator) resolveDepartment(groups []string, ldapOrg string) string {
	if a.teams == nil {
		return ldapOrg
	}
	mapping, err := a.teams.Get()
	if err != nil {
		return ldapOrg
	}
	for _, g := range groups {
		if name, ok := mapping[g]; ok && name != "" {
			return name
		}
	}
	return ldapOrg
}

// lookupUser resolves username to its DN and its "o" (organizationName)
// attribute, shown in the UI as the user's affiliation — a directory entry
// with no "o" set just yields an empty department, not an error.
func (a *LDAPAuthenticator) lookupUser(conn *goldap.Conn, username string) (dn, department string, err error) {
	req := goldap.NewSearchRequest(
		a.cfg.UserBaseDN(),
		goldap.ScopeWholeSubtree, goldap.NeverDerefAliases, 2, 0, false,
		fmt.Sprintf("(uid=%s)", goldap.EscapeFilter(username)),
		[]string{"dn", "o"},
		nil,
	)
	result, err := conn.Search(req)
	if err != nil {
		return "", "", fmt.Errorf("search user %q: %w", username, err)
	}
	if len(result.Entries) == 0 {
		return "", "", ErrInvalidCredentials
	}
	if len(result.Entries) > 1 {
		return "", "", fmt.Errorf("ambiguous user %q: %d entries found", username, len(result.Entries))
	}
	entry := result.Entries[0]
	return entry.DN, entry.GetAttributeValue("o"), nil
}

func (a *LDAPAuthenticator) lookupGroupCNs(conn *goldap.Conn, userDN, username string) ([]string, error) {
	filter, err := a.buildGroupFilter(userDN, username)
	if err != nil {
		return nil, err
	}
	req := goldap.NewSearchRequest(
		a.cfg.GroupBaseDN(),
		goldap.ScopeWholeSubtree, goldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{"cn"},
		nil,
	)
	result, err := conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("search groups for %q: %w", userDN, err)
	}
	cns := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		if cn := entry.GetAttributeValue("cn"); cn != "" {
			cns = append(cns, cn)
		}
	}
	return cns, nil
}

// buildGroupFilter renders a.groupFilterTmpl with the escaped user DN/uid.
// Escaping happens here, once, so a filter template author never needs to
// (and can't forget to) escape user-supplied values themselves.
func (a *LDAPAuthenticator) buildGroupFilter(userDN, username string) (string, error) {
	params := GroupFilterParams{
		UserDN: goldap.EscapeFilter(userDN),
		UID:    goldap.EscapeFilter(username),
	}
	var buf bytes.Buffer
	if err := a.groupFilterTmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("render group search filter: %w", err)
	}
	return buf.String(), nil
}
