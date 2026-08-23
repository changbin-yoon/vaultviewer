package storage

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/vaultviewer/vaultviewer/internal/model"
)

// binaryExtensions lists file types skipped during full-text search — they
// hold embedded media, not searchable text, mirroring how the frontend
// treats images/attachments in web/src/lib/markdown.ts.
var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".svg": true, ".ico": true, ".pdf": true, ".bmp": true,
}

// Searchable reports whether a file's name looks like text worth
// full-text-searching, based on its extension.
func Searchable(name string) bool {
	return !binaryExtensions[strings.ToLower(filepath.Ext(name))]
}

const snippetRadius = 40

// MatchSnippet case-insensitively searches content for query and, if
// found, returns a short excerpt of context centered on the first match.
func MatchSnippet(content []byte, query string) (string, bool) {
	if query == "" {
		return "", false
	}
	text := string(content)
	idx := strings.Index(strings.ToLower(text), strings.ToLower(query))
	if idx < 0 {
		return "", false
	}

	start := idx - snippetRadius
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + snippetRadius
	if end > len(text) {
		end = len(text)
	}
	// Never slice in the middle of a multi-byte rune.
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}

	snippet := strings.ReplaceAll(strings.TrimSpace(text[start:end]), "\n", " ")
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(text) {
		snippet = snippet + "…"
	}
	return snippet, true
}

// WalkAndSearch performs a generic full-text search over every searchable
// file reachable from engine, using only List and Read — so any backend
// can implement Search in one line by delegating here. A backend can
// still implement its own Search directly for a more efficient search
// (e.g. a future Git backend using `git grep`).
func WalkAndSearch(engine VaultStorageEngine, query string) ([]model.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var results []model.SearchResult
	var walk func(path string) error
	walk = func(path string) error {
		items, err := engine.List(path)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.IsDir {
				if err := walk(item.Path); err != nil {
					return err
				}
				continue
			}
			if !Searchable(item.Name) {
				continue
			}
			file, err := engine.Read(item.Path)
			if err != nil {
				// A single unreadable file shouldn't fail the whole search.
				continue
			}
			if snippet, ok := MatchSnippet(file.Content, query); ok {
				results = append(results, model.SearchResult{Path: item.Path, Snippet: snippet})
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return nil, err
	}
	return results, nil
}
