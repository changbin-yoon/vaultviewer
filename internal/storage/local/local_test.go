package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/accesslens/accesslens/internal/model"
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

func TestEngineRename(t *testing.T) {
	audit := &fakeAudit{}
	eng, err := New(t.TempDir(), audit)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.Save("notes/old.md", []byte("hello"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := eng.Rename("notes/old.md", "notes/new.md", "alice", "typo fix"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := eng.Read("notes/old.md"); err == nil {
		t.Fatalf("expected old path to no longer exist")
	}
	file, err := eng.Read("notes/new.md")
	if err != nil {
		t.Fatalf("Read new path: %v", err)
	}
	if string(file.Content) != "hello" {
		t.Fatalf("unexpected content after rename: %s", file.Content)
	}

	history, err := audit.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(history) != 2 || history[1].Action != "rename" || history[1].Path != "notes/new.md" || history[1].PreviousPath != "notes/old.md" {
		t.Fatalf("unexpected audit history: %+v", history)
	}
}

func TestEngineRenameFailsIfSourceMissing(t *testing.T) {
	eng, err := New(t.TempDir(), &fakeAudit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.Rename("notes/missing.md", "notes/new.md", "alice", ""); err == nil {
		t.Fatalf("expected error renaming a nonexistent source")
	}
}

func TestEngineRenameFailsIfTargetExists(t *testing.T) {
	eng, err := New(t.TempDir(), &fakeAudit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.Save("notes/a.md", []byte("a"), "alice", ""); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	if err := eng.Save("notes/b.md", []byte("b"), "alice", ""); err != nil {
		t.Fatalf("Save b: %v", err)
	}
	if err := eng.Rename("notes/a.md", "notes/b.md", "alice", ""); err == nil {
		t.Fatalf("expected error renaming onto an existing note")
	}
}

func TestEngineRenameRejectsDirectoryChange(t *testing.T) {
	eng, err := New(t.TempDir(), &fakeAudit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.Save("a/note.md", []byte("hi"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := eng.Rename("a/note.md", "b/note.md", "alice", ""); err == nil {
		t.Fatalf("expected error renaming into a different directory")
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

func TestEngineSearch(t *testing.T) {
	eng, err := New(t.TempDir(), &fakeAudit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.Save("00-홈.md", []byte("쿠버네티스 클러스터 운영 노트"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := eng.Save("01-예제/노트.md", []byte("아무 관련 없는 내용"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := eng.Search("클러스터")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Path != "00-홈.md" {
		t.Fatalf("unexpected search results: %+v", results)
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
