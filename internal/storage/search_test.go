package storage

import (
	"errors"
	"testing"

	"github.com/vaultviewer/vaultviewer/internal/model"
)

func TestSearchable(t *testing.T) {
	cases := map[string]bool{
		"note.md":     true,
		"config.yaml": true,
		"README":      true,
		"photo.png":   false,
		"photo.JPG":   false,
		"scan.pdf":    false,
		"icon.svg":    false,
	}
	for name, want := range cases {
		if got := Searchable(name); got != want {
			t.Errorf("Searchable(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestMatchSnippet(t *testing.T) {
	content := []byte("이 노트는 쿠버네티스 클러스터 운영에 대한 내용을 담고 있습니다.")

	snippet, ok := MatchSnippet(content, "클러스터")
	if !ok {
		t.Fatalf("expected a match")
	}
	if snippet == "" {
		t.Errorf("expected non-empty snippet")
	}

	// Case-insensitive.
	if _, ok := MatchSnippet([]byte("Hello World"), "world"); !ok {
		t.Errorf("expected case-insensitive match")
	}

	if _, ok := MatchSnippet(content, "존재하지않는단어"); ok {
		t.Errorf("expected no match for absent query")
	}

	if _, ok := MatchSnippet(content, ""); ok {
		t.Errorf("expected no match for empty query")
	}
}

func TestMatchSnippetLongContentTruncatesWithEllipsis(t *testing.T) {
	long := make([]byte, 0, 300)
	for i := 0; i < 300; i++ {
		long = append(long, 'a')
	}
	copy(long[150:], "NEEDLE")

	snippet, ok := MatchSnippet(long, "needle")
	if !ok {
		t.Fatalf("expected a match")
	}
	if snippet[0] != '\xe2' { // first byte of "…" in UTF-8
		t.Errorf("expected snippet to start with an ellipsis, got %q", snippet)
	}
	if len(snippet) >= len(long) {
		t.Errorf("expected snippet to be truncated, got length %d", len(snippet))
	}
}

// fakeEngine is a minimal in-memory VaultStorageEngine for exercising
// WalkAndSearch without touching a real backend.
type fakeEngine struct {
	files map[string][]byte // path -> content, directories implied by "/"
}

func (f *fakeEngine) List(path string) ([]model.FileItem, error) {
	seen := map[string]bool{}
	var items []model.FileItem
	prefix := path
	if prefix != "" {
		prefix += "/"
	}
	for p := range f.files {
		if prefix != "" && len(p) <= len(prefix) {
			continue
		}
		if prefix != "" && p[:len(prefix)] != prefix {
			continue
		}
		rest := p[len(prefix):]
		if prefix == "" {
			rest = p
		}
		var name string
		isDir := false
		if idx := indexOfSlash(rest); idx >= 0 {
			name = rest[:idx]
			isDir = true
		} else {
			name = rest
		}
		itemPath := prefix + name
		if seen[itemPath] {
			continue
		}
		seen[itemPath] = true
		items = append(items, model.FileItem{Path: itemPath, Name: name, IsDir: isDir})
	}
	return items, nil
}

func indexOfSlash(s string) int {
	for i, c := range s {
		if c == '/' {
			return i
		}
	}
	return -1
}

func (f *fakeEngine) Read(path string) (*model.VaultFile, error) {
	content, ok := f.files[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return &model.VaultFile{Path: path, Content: content}, nil
}

func (f *fakeEngine) Save(path string, content []byte, user, reason string) error { return nil }
func (f *fakeEngine) Delete(path string, user string) error                       { return nil }
func (f *fakeEngine) GetHistory(path string) ([]model.AuditLog, error)            { return nil, nil }
func (f *fakeEngine) CreateNamespace(path string, user string) error              { return nil }
func (f *fakeEngine) Search(query string) ([]model.SearchResult, error) {
	return WalkAndSearch(f, query)
}

var _ VaultStorageEngine = (*fakeEngine)(nil)

func TestWalkAndSearch(t *testing.T) {
	eng := &fakeEngine{files: map[string][]byte{
		"00-홈.md":                    []byte("쿠버네티스 클러스터 운영 노트"),
		"01-예제/기능-둘러보기.md":           []byte("콜아웃과 표를 설명하는 노트"),
		"01-예제/두번째-노트.md":            []byte("아무 내용도 없는 노트"),
		"attachments/screenshot.png": []byte("binary garbage that happens to contain 쿠버네티스"),
	}}

	results, err := WalkAndSearch(eng, "쿠버네티스")
	if err != nil {
		t.Fatalf("WalkAndSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 match (image should be skipped), got %d: %+v", len(results), results)
	}
	if results[0].Path != "00-홈.md" {
		t.Errorf("expected match in 00-홈.md, got %q", results[0].Path)
	}

	results, err = WalkAndSearch(eng, "")
	if err != nil {
		t.Fatalf("WalkAndSearch empty query: %v", err)
	}
	if results != nil {
		t.Errorf("expected no results for empty query, got %+v", results)
	}

	results, err = WalkAndSearch(eng, "존재하지않음")
	if err != nil {
		t.Fatalf("WalkAndSearch no-match query: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results, got %+v", results)
	}
}
