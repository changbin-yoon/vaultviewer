// Package local implements storage.VaultStorageEngine over a local
// directory tree — used for Local Mode, where a host path is mapped onto a
// container volume path.
package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vaultviewer/vaultviewer/internal/model"
	"github.com/vaultviewer/vaultviewer/internal/storage"
)

// Engine implements storage.VaultStorageEngine rooted at a fixed local
// directory. All paths passed to its methods are logical vault paths
// relative to that root.
type Engine struct {
	root  string
	audit storage.AuditRecorder
}

var _ storage.VaultStorageEngine = (*Engine)(nil)

// New creates a local Engine rooted at root, which must already exist as a
// directory (e.g. the mounted volume path).
func New(root string, audit storage.AuditRecorder) (*Engine, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat root path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path %q is not a directory", abs)
	}
	return &Engine{root: abs, audit: audit}, nil
}

// resolve maps a logical vault path onto an absolute filesystem path,
// rejecting any attempt to escape the configured root.
func (e *Engine) resolve(path string) (string, error) {
	cleaned := filepath.Clean(string(filepath.Separator) + path)
	full := filepath.Join(e.root, cleaned)
	if full != e.root && !strings.HasPrefix(full, e.root+string(filepath.Separator)) {
		return "", errors.New("path escapes storage root")
	}
	return full, nil
}

func (e *Engine) List(path string) ([]model.FileItem, error) {
	full, err := e.resolve(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	items := make([]model.FileItem, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		// A freshly provisioned ext4 volume (the common case for a new
		// PVC) always has this reserved directory at its root.
		if path == "" && entry.Name() == "lost+found" {
			continue
		}
		items = append(items, model.FileItem{
			Path:  filepath.Join(path, entry.Name()),
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
		})
	}
	return items, nil
}

func (e *Engine) Read(path string) (*model.VaultFile, error) {
	full, err := e.resolve(path)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return &model.VaultFile{Path: path, Content: content}, nil
}

func (e *Engine) Save(path string, content []byte, user, reason string) error {
	full, err := e.resolve(path)
	if err != nil {
		return err
	}
	action := "update"
	if _, statErr := os.Stat(full); errors.Is(statErr, os.ErrNotExist) {
		action = "create"
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(full, content, 0o640); err != nil {
		return err
	}
	return e.audit.Record(model.AuditLog{
		Path:      path,
		Action:    action,
		User:      user,
		Reason:    reason,
		Timestamp: time.Now(),
	})
}

func (e *Engine) Delete(path string, user string) error {
	full, err := e.resolve(path)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		return err
	}
	return e.audit.Record(model.AuditLog{
		Path:      path,
		Action:    "delete",
		User:      user,
		Timestamp: time.Now(),
	})
}

func (e *Engine) GetHistory(path string) ([]model.AuditLog, error) {
	return e.audit.History(path)
}

func (e *Engine) CreateNamespace(path string, user string) error {
	full, err := e.resolve(path)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(full); statErr == nil {
		return fmt.Errorf("path %q already exists", path)
	}
	if err := os.MkdirAll(full, 0o750); err != nil {
		return err
	}
	return e.audit.Record(model.AuditLog{
		Path:      path,
		Action:    "create",
		User:      user,
		Timestamp: time.Now(),
	})
}
