// Package git decorates internal/storage/local's Engine so that every
// Save/Delete also commits the change into a real git repository rooted at
// the same directory — giving diffs, blame, and external git tooling on
// top of local mode's PVC, without changing the existing audit-log
// timeline (internal/audit) at all; the two run side by side.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/accesslens/accesslens/internal/storage"
	"github.com/accesslens/accesslens/internal/storage/local"
)

const gitignoreContent = `.vaultviewer-audit.jsonl
.vaultviewer-audit.jsonl.tmp
.vaultviewer-group-teams.json
.vaultviewer-group-teams.json.tmp
`

// serviceUserName/Email is the fixed git committer identity for every
// commit this package makes — the actual acting user goes in the commit's
// *author* field instead (see commitChange), so `git log` still shows who
// really made each change.
const (
	serviceUserName  = "AccessLens"
	serviceUserEmail = "accesslens@local"
)

// Engine wraps a *local.Engine, embedding it so List/Read/GetHistory/Search
// pass through unchanged — only Save and Delete are overridden to also
// commit. CreateNamespace is deliberately *not* overridden: git can't
// represent an empty directory, so a bare namespace creation has nothing
// to commit until its first file is saved.
type Engine struct {
	*local.Engine
	root string
	mu   sync.Mutex // serializes git commands against the one working tree
}

var _ storage.VaultStorageEngine = (*Engine)(nil)

// New wraps inner (rooted at root) with git-backed versioning, bootstrapping
// a repository at root if one doesn't already exist there.
func New(inner *local.Engine, root string) (*Engine, error) {
	e := &Engine{Engine: inner, root: root}
	if err := e.bootstrap(); err != nil {
		return nil, err
	}
	return e, nil
}

// bootstrap makes root a git repository if it isn't already one (checked
// via the presence of root/.git, so this is a no-op — and safe to call
// again — on every restart after the first). Any pre-existing content
// (e.g. from before git mode was turned on) is captured in one initial
// commit.
func (e *Engine) bootstrap() error {
	// PVC-mounted directories are commonly created by the CSI driver as
	// root, with only the *group* adjusted to the pod's fsGroup — the pod
	// itself runs as a non-root UID that owns none of it. Git's "dubious
	// ownership" protection (CVE-2022-24765) then refuses every command
	// except `init` on such a directory. We genuinely own and control this
	// PVC end-to-end, so it's safe to allowlist — and since $HOME isn't
	// itself persisted across restarts, this must run on every startup,
	// not just the first (hence before the already-a-repo check below).
	if _, err := e.run("config", "--global", "--add", "safe.directory", e.root); err != nil {
		return fmt.Errorf("git config safe.directory: %w", err)
	}

	if _, err := os.Stat(filepath.Join(e.root, ".git")); err == nil {
		return nil
	}
	if _, err := e.run("init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if _, err := e.run("config", "user.name", serviceUserName); err != nil {
		return fmt.Errorf("git config user.name: %w", err)
	}
	if _, err := e.run("config", "user.email", serviceUserEmail); err != nil {
		return fmt.Errorf("git config user.email: %w", err)
	}
	if err := os.WriteFile(filepath.Join(e.root, ".gitignore"), []byte(gitignoreContent), 0o640); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	if _, err := e.run("add", "-A"); err != nil {
		return fmt.Errorf("git add -A: %w", err)
	}
	status, err := e.run("status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if strings.TrimSpace(status) == "" {
		return nil // freshly provisioned, empty volume — nothing to commit yet
	}
	if err := e.commit(serviceUserName, "initial commit"); err != nil {
		return fmt.Errorf("git initial commit: %w", err)
	}
	return nil
}

func (e *Engine) Save(path string, content []byte, user, reason string) error {
	if err := e.Engine.Save(path, content, user, reason); err != nil {
		return err
	}
	e.commitChange([]string{path}, "save: "+path, user, reason)
	return nil
}

func (e *Engine) Delete(path string, user string) error {
	if err := e.Engine.Delete(path, user); err != nil {
		return err
	}
	e.commitChange([]string{path}, "delete: "+path, user, "")
	return nil
}

func (e *Engine) Rename(oldPath, newPath, user, reason string) error {
	if err := e.Engine.Rename(oldPath, newPath, user, reason); err != nil {
		return err
	}
	// Staging both the vanished old path and the new one in the same
	// commit lets git recognize it as a rename (shows as "renamed:" in
	// status, preserved across `git log --follow`) rather than an
	// unrelated delete+add pair.
	e.commitChange([]string{oldPath, newPath}, fmt.Sprintf("rename: %s -> %s", oldPath, newPath), user, reason)
	return nil
}

// commitChange stages and commits only the paths this operation touched
// (never a blanket "add everything", so an unrelated concurrent write
// can't get swept into this commit). Best-effort: any git failure is
// logged and swallowed, never returned to the caller — the underlying
// file write (the real source of truth) already succeeded, matching how
// internal/audit treats its own persistence failures.
func (e *Engine) commitChange(paths []string, subject, user, reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	args := append([]string{"add", "-A", "--"}, paths...)
	if _, err := e.run(args...); err != nil {
		log.Printf("git: failed to stage %v: %v", paths, err)
		return
	}
	dirty, err := e.hasStagedDiff(paths)
	if err != nil {
		log.Printf("git: failed to check staged diff for %v: %v", paths, err)
		return
	}
	if !dirty {
		// Re-saving identical content, or deleting/renaming a path git
		// never tracked (e.g. it only ever held now-ignored dotfiles) —
		// nothing to commit, and that's not an error.
		return
	}
	message := subject
	if reason != "" {
		message += "\n\nReason: " + reason
	}
	if err := e.commit(user, message); err != nil {
		log.Printf("git: failed to commit %v: %v", paths, err)
	}
}

// hasStagedDiff reports whether the index currently differs from HEAD for
// any of paths. `git diff --cached --quiet` exits 1 when there IS a
// difference and 0 when there isn't — that exit code is the answer, not a
// failure.
func (e *Engine) hasStagedDiff(paths []string) (bool, error) {
	args := append([]string{"diff", "--cached", "--quiet", "--"}, paths...)
	cmd := exec.Command("git", append([]string{"-C", e.root}, args...)...)
	err := cmd.Run()
	if err == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}

// commit runs `git commit`, crediting user as the commit's author while
// the repository-wide committer identity (set once in bootstrap) stays
// fixed — so `git log` shows who really made the change.
func (e *Engine) commit(user, message string) error {
	author := fmt.Sprintf("%s <%s@accesslens.local>", user, user)
	_, err := e.run("commit", "--author", author, "-m", message)
	return err
}

func (e *Engine) run(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", e.root}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(out.String()))
	}
	return out.String(), nil
}
