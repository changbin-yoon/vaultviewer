// Package storage defines the unified interface that all vault backends
// (local filesystem, Kubernetes, and future Git integration) must implement.
package storage

import "github.com/vaultviewer/vaultviewer/internal/model"

// VaultStorageEngine is the interface every storage backend must satisfy so
// that controllers stay backend-agnostic.
type VaultStorageEngine interface {
	List(path string) ([]model.FileItem, error)
	Read(path string) (*model.VaultFile, error)
	Save(path string, content []byte, user, reason string) error // adm, dev
	Delete(path string, user string) error                       // adm only
	GetHistory(path string) ([]model.AuditLog, error)            // Git / Local History
	// CreateNamespace creates an empty grouping node at path (a directory
	// in local mode). Backends with no concept of an empty namespace (e.g.
	// Kubernetes Secrets, where a namespace only exists once it holds a
	// secret) return an error explaining the limitation.
	CreateNamespace(path string, user string) error // adm, dev
}

// AuditRecorder is the dependency storage backends use to record and
// retrieve modification history, keeping storage decoupled from the audit
// package's persistence/streaming implementation.
type AuditRecorder interface {
	Record(entry model.AuditLog) error
	History(path string) ([]model.AuditLog, error)
	All() ([]model.AuditLog, error)
}
