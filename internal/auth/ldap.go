package auth

import (
	"errors"
	"fmt"
	"net"
	"time"

	goldap "github.com/go-ldap/ldap/v3"

	"github.com/vaultviewer/vaultviewer/internal/model"
)

// ErrInvalidCredentials is returned when a username/password pair does not
// authenticate against the directory.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ErrNoRole is returned when a user authenticates successfully but belongs
// to no LDAP group mapped to a VaultViewer role, so they are granted no
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
	cfg Config
}

var _ Authenticator = (*LDAPAuthenticator)(nil)

// NewLDAPAuthenticator constructs an LDAPAuthenticator from cfg.
func NewLDAPAuthenticator(cfg Config) *LDAPAuthenticator {
	return &LDAPAuthenticator{cfg: cfg}
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

	userDN, err := a.lookupUserDN(search, username)
	if err != nil {
		return nil, err
	}

	groups, err := a.lookupGroupCNs(search, userDN)
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

	return &model.User{Username: username, Role: role}, nil
}

func (a *LDAPAuthenticator) lookupUserDN(conn *goldap.Conn, username string) (string, error) {
	req := goldap.NewSearchRequest(
		a.cfg.UserBaseDN(),
		goldap.ScopeWholeSubtree, goldap.NeverDerefAliases, 2, 0, false,
		fmt.Sprintf("(uid=%s)", goldap.EscapeFilter(username)),
		[]string{"dn"},
		nil,
	)
	result, err := conn.Search(req)
	if err != nil {
		return "", fmt.Errorf("search user %q: %w", username, err)
	}
	if len(result.Entries) == 0 {
		return "", ErrInvalidCredentials
	}
	if len(result.Entries) > 1 {
		return "", fmt.Errorf("ambiguous user %q: %d entries found", username, len(result.Entries))
	}
	return result.Entries[0].DN, nil
}

func (a *LDAPAuthenticator) lookupGroupCNs(conn *goldap.Conn, userDN string) ([]string, error) {
	req := goldap.NewSearchRequest(
		a.cfg.GroupBaseDN(),
		goldap.ScopeWholeSubtree, goldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=groupOfNames)(member=%s))", goldap.EscapeFilter(userDN)),
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
