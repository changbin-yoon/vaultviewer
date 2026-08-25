package model

import "time"

// Role represents an RBAC role mapped from an LDAP group.
type Role string

const (
	RoleAdmin Role = "adm"  // Read, Create, Update, Delete
	RoleDev   Role = "dev"  // Read, Create, Update
	RoleView  Role = "view" // Read only
)

// CanDelete reports whether the role is permitted to delete resources.
func (r Role) CanDelete() bool {
	return r == RoleAdmin
}

// CanWrite reports whether the role is permitted to create/update resources.
func (r Role) CanWrite() bool {
	return r == RoleAdmin || r == RoleDev
}

// IsAdmin reports whether the role is permitted admin-only actions (viewing
// the audit trail, changing settings).
func (r Role) IsAdmin() bool {
	return r == RoleAdmin
}

// TeamGrant is one LDAP group membership that grants a role scoped to a
// named team — parsed from a group CN of the form "<team>-<role>" or
// "<team>_<role>" (e.g. "bi-adm" -> {Team: "bi", Role: RoleAdmin}; see
// auth.ResolveTeams). Purely informational for the UI ("which team(s) grant
// me what") — separate from the single overall Role above, which
// auth.ResolveRole computes as the highest-precedence role across every
// matching group (team-scoped or not) and is what actually gates RBAC.
type TeamGrant struct {
	Team string
	Role Role
}

// User represents an authenticated LDAP user and their resolved role.
type User struct {
	Username string
	Role     Role
	// Department is the LDAP "o" (organizationName) attribute, shown in the
	// UI as the user's affiliation. Empty if the directory entry doesn't set it.
	Department string
	// Teams is every team-scoped role grant found among the user's LDAP
	// groups (see TeamGrant). Empty for accounts whose groups don't follow
	// the "<team>-<role>" naming convention (e.g. a bare "adm" group).
	Teams []TeamGrant
}

// FileItem represents a single node (file or directory) in the vault tree.
type FileItem struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

// VaultFile represents the contents of a single secret/vault file.
type VaultFile struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
}

// SearchResult represents one full-text match found while searching the
// vault, with a short excerpt of context around the match.
type SearchResult struct {
	Path    string `json:"path"`
	Snippet string `json:"snippet"`
}

// AuditLog represents a single recorded modification event.
type AuditLog struct {
	Path      string    `json:"path"`
	Action    string    `json:"action"` // create, update, delete, rename
	User      string    `json:"user"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	// PreviousPath is set only for Action "rename", holding the path this
	// note was renamed from.
	PreviousPath string `json:"previousPath,omitempty"`
}
