package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vaultviewer/vaultviewer/internal/model"
	"github.com/vaultviewer/vaultviewer/internal/storage/local"
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

func (f *fakeAudit) All() ([]model.AuditLog, error) { return f.entries, nil }

func newTestEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	root := t.TempDir()
	inner, err := local.New(root, &fakeAudit{})
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	eng, err := New(inner, root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return eng, root
}

func gitLog(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestBootstrapCommitsJustTheGitignore(t *testing.T) {
	eng, root := newTestEngine(t)
	_ = eng
	log := gitLog(t, root, "log", "--oneline")
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 bootstrap commit (.gitignore) on an empty vault, got %d:\n%s", len(lines), log)
	}
	files := gitLog(t, root, "show", "--stat", "--format=", "HEAD")
	if !strings.Contains(files, ".gitignore") {
		t.Errorf("expected the bootstrap commit to contain .gitignore, got:\n%s", files)
	}
}

func TestBootstrapIsIdempotentAcrossRestart(t *testing.T) {
	root := t.TempDir()
	inner, err := local.New(root, &fakeAudit{})
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	if _, err := New(inner, root); err != nil {
		t.Fatalf("New (first): %v", err)
	}
	// A second Engine over the same root — simulating a pod restart — must
	// not re-init or re-commit.
	if _, err := New(inner, root); err != nil {
		t.Fatalf("New (second): %v", err)
	}
	log := gitLog(t, root, "log", "--oneline")
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected bootstrap to still be exactly 1 commit after a second New(), got %d:\n%s", len(lines), log)
	}
}

func TestSaveCreatesACommit(t *testing.T) {
	eng, root := newTestEngine(t)
	if err := eng.Save("note.md", []byte("hello"), "alice", "first note"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	log := gitLog(t, root, "log", "--format=%an <%ae>|%s")
	if !strings.Contains(log, "alice <alice@vaultviewer.local>") {
		t.Errorf("expected commit authored by alice, got:\n%s", log)
	}
	if !strings.Contains(log, "save: note.md") {
		t.Errorf("expected commit message mentioning the path, got:\n%s", log)
	}

	body := gitLog(t, root, "log", "-1", "--format=%B")
	if !strings.Contains(body, "Reason: first note") {
		t.Errorf("expected reason in commit body, got:\n%s", body)
	}
}

func TestSaveWithIdenticalContentSkipsCommit(t *testing.T) {
	eng, root := newTestEngine(t)
	if err := eng.Save("note.md", []byte("hello"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := eng.Save("note.md", []byte("hello"), "alice", ""); err != nil {
		t.Fatalf("Save (identical): %v", err)
	}

	log := gitLog(t, root, "log", "--oneline")
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 2 { // bootstrap commit + the one real Save — the re-save adds nothing
		t.Fatalf("expected exactly 2 commits (re-save with no diff shouldn't add one), got %d:\n%s", len(lines), log)
	}
}

func TestDeleteCreatesACommit(t *testing.T) {
	eng, root := newTestEngine(t)
	if err := eng.Save("note.md", []byte("hello"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := eng.Delete("note.md", "bob"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	log := gitLog(t, root, "log", "--format=%an|%s")
	if !strings.Contains(log, "bob|delete: note.md") {
		t.Errorf("expected a delete commit authored by bob, got:\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(root, "note.md")); !os.IsNotExist(err) {
		t.Errorf("expected note.md to no longer exist on disk, stat err = %v", err)
	}
}

func TestRenameCreatesACommit(t *testing.T) {
	eng, root := newTestEngine(t)
	if err := eng.Save("old.md", []byte("hello"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := eng.Rename("old.md", "new.md", "carol", "typo fix"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	log := gitLog(t, root, "log", "--format=%an|%s")
	if !strings.Contains(log, "carol|rename: old.md -> new.md") {
		t.Errorf("expected a rename commit authored by carol, got:\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(root, "old.md")); !os.IsNotExist(err) {
		t.Errorf("expected old.md to no longer exist on disk, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.md")); err != nil {
		t.Errorf("expected new.md to exist on disk: %v", err)
	}

	status := gitLog(t, root, "status", "--porcelain=v1", "--", "old.md", "new.md")
	if strings.TrimSpace(status) != "" {
		t.Errorf("expected a clean working tree after the rename commit, got:\n%s", status)
	}
}

func TestAuditDotfilesAreIgnoredByGit(t *testing.T) {
	eng, root := newTestEngine(t)
	if err := eng.Save("note.md", []byte("hello"), "alice", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// .gitignore itself is committed during bootstrap; the audit/team
	// dotfiles aren't written by this test, but the ignore rule should
	// already be in place for when internal/audit or internal/teams later
	// write there.
	ignored := gitLog(t, root, "check-ignore", "-v", ".vaultviewer-audit.jsonl")
	if !strings.Contains(ignored, ".gitignore") {
		t.Errorf("expected .vaultviewer-audit.jsonl to be excluded by .gitignore, got:\n%s", ignored)
	}
}
