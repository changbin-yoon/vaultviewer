package local

import (
	"os"
	"path/filepath"
	"testing"

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
	eng, err := New(t.TempDir(), audit)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := eng.Save("secrets/db.yaml", []byte("password: hunter2"), "alice", "initial provision"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	file, err := eng.Read("secrets/db.yaml")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(file.Content) != "password: hunter2" {
		t.Fatalf("unexpected content: %s", file.Content)
	}

	items, err := eng.List("secrets")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "db.yaml" {
		t.Fatalf("unexpected list result: %+v", items)
	}

	if err := eng.Save("secrets/db.yaml", []byte("password: changed"), "alice", "rotate password"); err != nil {
		t.Fatalf("Save (update): %v", err)
	}

	if err := eng.Delete("secrets/db.yaml", "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := eng.Read("secrets/db.yaml"); err == nil {
		t.Fatalf("expected error reading deleted file")
	}

	history, err := eng.GetHistory("secrets/db.yaml")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 3 || history[0].Action != "create" || history[1].Action != "update" || history[2].Action != "delete" {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestEngineCreateNamespace(t *testing.T) {
	eng, err := New(t.TempDir(), &fakeAudit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.CreateNamespace("prod", "alice"); err != nil {
		t.Fatalf("CreateNamespace: %v", err)
	}
	items, err := eng.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "prod" || !items[0].IsDir {
		t.Fatalf("expected empty namespace %q, got: %+v", "prod", items)
	}
	if err := eng.CreateNamespace("prod", "alice"); err == nil {
		t.Fatalf("expected error creating an already-existing namespace")
	}
}

func TestEngineListSkipsDotfiles(t *testing.T) {
	root := t.TempDir()
	eng, err := New(root, &fakeAudit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.Save("note.md", []byte("# hi"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := eng.Save(".obsidian/config.json", []byte("{}"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "lost+found"), 0o750); err != nil {
		t.Fatalf("mkdir lost+found: %v", err)
	}

	items, err := eng.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Name != "note.md" {
		t.Fatalf("expected only note.md, got: %+v", items)
	}
}

func TestEnginePathTraversalRejected(t *testing.T) {
	eng, err := New(t.TempDir(), &fakeAudit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := eng.Read("../../etc/passwd"); err == nil {
		t.Fatalf("expected path traversal to be rejected")
	}
}
