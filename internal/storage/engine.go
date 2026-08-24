// Package storage defines the unified interface that all vault backends
// (local filesystem, Kubernetes, and git — see internal/storage/git) must
// implement.
package storage

import "github.com/accesslens/accesslens/internal/model"

// VaultStorageEngine is the interface every storage backend must satisfy so
// that controllers stay backend-agnostic.
type VaultStorageEngine interface {
	List(path string) ([]model.FileItem, error)
	Read(path string) (*model.VaultFile, error)
	Save(path string, content []byte, user, reason string) error // adm, dev
	Delete(path string, user string) error                       // adm only
	GetHistory(path string) ([]model.AuditLog, error)            // Git / Local History
	// Rename moves a single note to a new path within the same directory
	// (everything before the last path segment must match) — renaming
	// into a different directory/namespace is out of scope. Fails if
	// oldPath doesn't exist or newPath already does.
	Rename(oldPath, newPath, user, reason string) error // adm, dev
	// CreateNamespace creates an empty grouping node at path (a directory
	// in local mode). Backends with no concept of an empty namespace (e.g.
	// Kubernetes Secrets, where a namespace only exists once it holds a
	// secret) return an error explaining the limitation.
	CreateNamespace(path string, user string) error // adm, dev
	// Search performs a case-insensitive full-text search across every
	// searchable file in the vault, returning a short snippet of context
	// around each match. Backends without a more efficient option can
	// implement this with one line via WalkAndSearch.
	Search(query string) ([]model.SearchResult, error)
}

// AuditRecorder is the dependency storage backends use to record and
// retrieve modification history, keeping storage decoupled from the audit
// package's persistence/streaming implementation.
type AuditRecorder interface {
	Record(entry model.AuditLog) error
	History(path string) ([]model.AuditLog, error)
	All() ([]model.AuditLog, error)
}
