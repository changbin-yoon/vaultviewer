package audit

import (
	"testing"
	"time"

	"github.com/vaultviewer/vaultviewer/internal/model"
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
