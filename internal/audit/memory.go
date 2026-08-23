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
	mu          sync.RWMutex
	entries     []model.AuditLog
	subscribers map[chan model.AuditLog]struct{}
}

var _ storage.AuditRecorder = (*MemoryRecorder)(nil)

func NewMemoryRecorder() *MemoryRecorder {
	return &MemoryRecorder{subscribers: make(map[chan model.AuditLog]struct{})}
}

func (r *MemoryRecorder) Record(entry model.AuditLog) error {
	r.mu.Lock()
	r.entries = append(r.entries, entry)
	subs := make([]chan model.AuditLog, 0, len(r.subscribers))
	for ch := range r.subscribers {
		subs = append(subs, ch)
	}
	r.mu.Unlock()

	// Fan out to live WebSocket subscribers outside the lock, and never
	// block Record (and therefore every Save/Delete caller) on a slow or
	// stuck consumer — a full channel just drops this event for that one
	// subscriber instead.
	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
	return nil
}

// Subscribe registers a listener for every future Record call, returning a
// channel of new entries and an unsubscribe function the caller must call
// exactly once when done (e.g. when its WebSocket connection closes).
func (r *MemoryRecorder) Subscribe() (<-chan model.AuditLog, func()) {
	ch := make(chan model.AuditLog, 16)
	r.mu.Lock()
	r.subscribers[ch] = struct{}{}
	r.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.subscribers, ch)
			r.mu.Unlock()
			close(ch)
		})
	}
	return ch, unsubscribe
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
