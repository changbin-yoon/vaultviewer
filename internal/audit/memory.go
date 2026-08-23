// Package audit records and serves modification history for storage
// backends. MemoryRecorder is a process-local implementation, optionally
// backed by a file for durability across restarts.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/vaultviewer/vaultviewer/internal/model"
	"github.com/vaultviewer/vaultviewer/internal/storage"
)

// MemoryRecorder implements storage.AuditRecorder, holding every entry in
// memory for fast reads (History/All always serve from there). Constructed
// via NewMemoryRecorder, it's purely in-memory — history is lost on
// restart. Constructed via NewMemoryRecorderWithFile, it also appends each
// entry to a JSON-Lines file and reloads from it on startup, so history
// survives a restart.
type MemoryRecorder struct {
	mu          sync.RWMutex
	entries     []model.AuditLog
	subscribers map[chan model.AuditLog]struct{}
	file        *os.File // nil unless persistence is enabled
}

var _ storage.AuditRecorder = (*MemoryRecorder)(nil)

// NewMemoryRecorder creates an in-memory-only recorder.
func NewMemoryRecorder() *MemoryRecorder {
	return &MemoryRecorder{subscribers: make(map[chan model.AuditLog]struct{})}
}

// NewMemoryRecorderWithFile creates a MemoryRecorder backed by a
// JSON-Lines file at path: existing entries are loaded from it (if it
// exists) before returning, and every future Record() is appended to it.
// Intended for local mode, which already has a persistent volume to put
// this on — cluster mode has no equivalent persistent location and uses
// NewMemoryRecorder instead.
func NewMemoryRecorderWithFile(path string) (*MemoryRecorder, error) {
	entries, err := loadEntries(path)
	if err != nil {
		return nil, fmt.Errorf("load audit log %q: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("open audit log %q: %w", path, err)
	}
	return &MemoryRecorder{
		entries:     entries,
		subscribers: make(map[chan model.AuditLog]struct{}),
		file:        f,
	}, nil
}

func loadEntries(path string) ([]model.AuditLog, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []model.AuditLog
	scanner := bufio.NewScanner(f)
	// Audit reasons can be long; raise the scanner's line-buffer ceiling
	// well past anything a UI text field would realistically produce.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry model.AuditLog
		if err := json.Unmarshal(line, &entry); err != nil {
			// One corrupt line (e.g. a truncated write from an unclean
			// shutdown) shouldn't take down the whole history.
			log.Printf("audit: skipping corrupt log line: %v", err)
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *MemoryRecorder) Record(entry model.AuditLog) error {
	r.mu.Lock()
	if r.file != nil {
		if err := r.appendToFile(entry); err != nil {
			// A disk hiccup persisting the audit trail must never fail (or
			// even just look like it failed) the create/update/delete
			// it's describing — that already succeeded against the real
			// storage. Best-effort: log and keep going with the in-memory
			// copy, which still serves reads and the WebSocket stream.
			log.Printf("audit: failed to persist entry to disk: %v", err)
		}
	}
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

// appendToFile must be called with r.mu held.
func (r *MemoryRecorder) appendToFile(entry model.AuditLog) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := r.file.Write(append(line, '\n')); err != nil {
		return err
	}
	// Sync rather than relying on the OS's write-back cache: this log
	// exists specifically to survive a restart, so minimize the window
	// where a written entry only exists in a buffer an unclean pod kill
	// could lose.
	return r.file.Sync()
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
