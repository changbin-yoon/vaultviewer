package k8s

import (
	"testing"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/vaultviewer/vaultviewer/internal/model"
)

type fakeAudit struct {
	entries []model.AuditLog
}

func (f *fakeAudit) Record(e model.AuditLog) error {
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeAudit) History(path string) ([]model.AuditLog, error) {
	var out []model.AuditLog
	for _, e := range f.entries {
		if e.Path == path {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeAudit) All() ([]model.AuditLog, error) {
	return f.entries, nil
}

func TestEngineSaveReadListDelete(t *testing.T) {
	audit := &fakeAudit{}
	eng := New(fake.NewSimpleClientset(), "default", audit)

	if err := eng.Save("db-credentials/password", []byte("hunter2"), "alice", "initial provision"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	file, err := eng.Read("db-credentials/password")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(file.Content) != "hunter2" {
		t.Fatalf("unexpected content: %s", file.Content)
	}

	secrets, err := eng.List("")
	if err != nil {
		t.Fatalf("List (root): %v", err)
	}
	if len(secrets) != 1 || secrets[0].Name != "db-credentials" || !secrets[0].IsDir {
		t.Fatalf("unexpected root list: %+v", secrets)
	}

	keys, err := eng.List("db-credentials")
	if err != nil {
		t.Fatalf("List (secret): %v", err)
	}
	if len(keys) != 1 || keys[0].Name != "password" {
		t.Fatalf("unexpected key list: %+v", keys)
	}

	if err := eng.Delete("db-credentials/password", "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := eng.Read("db-credentials/password"); err == nil {
		t.Fatalf("expected error reading deleted key")
	}

	history, err := eng.GetHistory("db-credentials/password")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 2 || history[0].Action != "create" || history[1].Action != "delete" {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestEngineSearch(t *testing.T) {
	eng := New(fake.NewSimpleClientset(), "default", &fakeAudit{})
	if err := eng.Save("db-credentials/password", []byte("hunter2-super-secret"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := eng.Save("api-tokens/key", []byte("unrelated value"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := eng.Search("super-secret")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Path != "db-credentials/password" {
		t.Fatalf("unexpected search results: %+v", results)
	}
}
