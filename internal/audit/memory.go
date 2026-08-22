// Package audit records and serves modification history for storage
// backends. MemoryRecorder is a process-local implementation used to boot
// the server before persistent storage and WebSocket streaming are added.
package audit

import (
	"sync"

	"github.com/vaultviewer/vaultviewer/internal/model"
	"github.com/vaultviewer/vaultviewer/internal/storage"
)

// MemoryRecorder implements storage.AuditRecorder in process memory. It
// does not persist across restarts.
type MemoryRecorder struct {
	mu      sync.RWMutex
	entries []model.AuditLog
}

var _ storage.AuditRecorder = (*MemoryRecorder)(nil)

func NewMemoryRecorder() *MemoryRecorder {
	return &MemoryRecorder{}
}

func (r *MemoryRecorder) Record(entry model.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	return nil
}

func (r *MemoryRecorder) History(path string) ([]model.AuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []model.AuditLog
	for _, e := range r.entries {
		if e.Path == path {
			out = append(out, e)
		}
	}
	return out, nil
}

// All returns every recorded entry, most recent first, for the global
// audit log stream.
func (r *MemoryRecorder) All() ([]model.AuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.AuditLog, len(r.entries))
	for i, e := range r.entries {
		out[len(r.entries)-1-i] = e
	}
	return out, nil
}
