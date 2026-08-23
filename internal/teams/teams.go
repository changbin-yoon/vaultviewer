// Package teams manages the admin-editable mapping from an LDAP group CN
// to a display-friendly team name, shown as a user's "소속" (affiliation)
// in the UI. Kept separate from auth.Config's GroupRoleMap (which grants
// RBAC roles and is fixed at deploy time via env vars) — this mapping is
// purely cosmetic and, since there's no database, is administered at
// runtime from the Settings page and persisted as a plain file.
package teams

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Store holds the current group-to-team-name mapping.
type Store interface {
	Get() (map[string]string, error)
	Set(map[string]string) error
}

// MemoryStore is a process-local, in-memory Store — edits are lost on
// restart. Used for cluster mode, which has no natural persistent location
// for this without adding new infrastructure — the same call made for the
// audit log (see internal/audit) — so it's accepted as an asymmetry rather
// than solved with e.g. a ConfigMap write.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[string]string{}}
}

func (s *MemoryStore) Get() (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMap(s.data), nil
}

func (s *MemoryStore) Set(m map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = cloneMap(m)
	return nil
}

var _ Store = (*MemoryStore)(nil)

// FileStore persists the mapping as JSON at path. Reads always serve from
// an in-memory cache (fast); writes replace the file's full contents via a
// temp-file-then-rename so a crash mid-write can't leave truncated/corrupt
// JSON behind. Intended for local mode, which already has a persistent
// volume to put this on.
type FileStore struct {
	mu   sync.RWMutex
	path string
	data map[string]string
}

// NewFileStore creates a FileStore backed by path, loading any existing
// mapping (a missing file just starts empty).
func NewFileStore(path string) (*FileStore, error) {
	data, err := loadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load group-team map %q: %w", path, err)
	}
	return &FileStore{path: path, data: data}, nil
}

func loadFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return map[string]string{}, nil
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *FileStore) Get() (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMap(s.data), nil
}

func (s *FileStore) Set(m map[string]string) error {
	clean := cloneMap(m)
	b, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}

	s.mu.Lock()
	s.data = clean
	s.mu.Unlock()
	return nil
}

var _ Store = (*FileStore)(nil)

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
