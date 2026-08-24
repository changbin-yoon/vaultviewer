package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/accesslens/accesslens/internal/model"
)

func TestSubscribeReceivesFutureEntries(t *testing.T) {
	r := NewMemoryRecorder()
	ch, unsubscribe := r.Subscribe()
	defer unsubscribe()

	entry := model.AuditLog{Path: "a.md", Action: "create", User: "alice", Timestamp: time.Now()}
	if err := r.Record(entry); err != nil {
		t.Fatalf("Record: %v", err)
	}

	select {
	case got := <-ch:
		if got.Path != entry.Path || got.User != entry.User {
			t.Errorf("got %+v, want %+v", got, entry)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribed entry")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	r := NewMemoryRecorder()
	ch, unsubscribe := r.Subscribe()
	unsubscribe()

	if err := r.Record(model.AuditLog{Path: "a.md", Action: "create", User: "alice"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Channel should be closed, not deliver a value.
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("expected closed channel, got a value")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel to close")
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	r := NewMemoryRecorder()
	_, unsubscribe := r.Subscribe()
	unsubscribe()
	unsubscribe() // must not panic on double-close
}

func TestRecordDoesNotBlockOnFullSubscriberChannel(t *testing.T) {
	r := NewMemoryRecorder()
	_, unsubscribe := r.Subscribe() // never drained
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 32; i++ { // well past the subscriber buffer size
			_ = r.Record(model.AuditLog{Path: "a.md", Action: "update", User: "alice"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Record blocked on a slow/full subscriber channel")
	}
}

func TestRecordStillAppendsToHistoryAndAll(t *testing.T) {
	r := NewMemoryRecorder()
	if err := r.Record(model.AuditLog{Path: "a.md", Action: "create", User: "alice"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	all, err := r.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(all))
	}
}

func TestFileRecorderPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	r1, err := NewMemoryRecorderWithFile(path)
	if err != nil {
		t.Fatalf("NewMemoryRecorderWithFile: %v", err)
	}
	entries := []model.AuditLog{
		{Path: "a.md", Action: "create", User: "alice", Timestamp: time.Now(), Reason: "initial"},
		{Path: "a.md", Action: "update", User: "bob", Timestamp: time.Now()},
		{Path: "b.md", Action: "delete", User: "alice", Timestamp: time.Now()},
	}
	for _, e := range entries {
		if err := r1.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	// A fresh recorder pointed at the same file — simulating a pod
	// restart — must see everything the first one wrote.
	r2, err := NewMemoryRecorderWithFile(path)
	if err != nil {
		t.Fatalf("NewMemoryRecorderWithFile (reload): %v", err)
	}
	all, err := r2.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != len(entries) {
		t.Fatalf("expected %d entries after reload, got %d: %+v", len(entries), len(all), all)
	}
	// All() returns newest-first.
	if all[0].Path != "b.md" || all[0].User != "alice" || all[0].Action != "delete" {
		t.Errorf("unexpected newest entry after reload: %+v", all[0])
	}

	history, err := r2.History("a.md")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 entries for a.md after reload, got %d: %+v", len(history), history)
	}
	if history[0].Reason != "initial" {
		t.Errorf("expected reason %q preserved across reload, got %q", "initial", history[0].Reason)
	}
}

func TestFileRecorderSkipsCorruptLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	content := `{"path":"a.md","action":"create","user":"alice"}
not valid json
{"path":"b.md","action":"update","user":"bob"}
`
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	r, err := NewMemoryRecorderWithFile(path)
	if err != nil {
		t.Fatalf("NewMemoryRecorderWithFile: %v", err)
	}
	all, err := r.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected the 2 valid entries to load (corrupt line skipped), got %d: %+v", len(all), all)
	}
}

func TestFileRecorderStartsEmptyWhenFileDoesNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist-yet.jsonl")
	r, err := NewMemoryRecorderWithFile(path)
	if err != nil {
		t.Fatalf("NewMemoryRecorderWithFile: %v", err)
	}
	all, err := r.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected no entries for a fresh file, got %d", len(all))
	}
	// The file itself should now exist (opened with O_CREATE) even though
	// nothing has been recorded yet.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected audit file to be created: %v", err)
	}
}
