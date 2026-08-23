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

// User represents an authenticated LDAP user and their resolved role.
type User struct {
	Username string
	Role     Role
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
	Action    string    `json:"action"` // create, update, delete
	User      string    `json:"user"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
