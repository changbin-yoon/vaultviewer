package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameContent(t *testing.T) {
	cases := []struct {
		name           string
		localMD5, etag string
		want           bool
	}{
		{"exact match", "abc123", "abc123", true},
		{"case insensitive", "ABC123", "abc123", true},
		{"quoted etag", "abc123", `"abc123"`, true},
		{"different hash", "abc123", "def456", false},
		{"multipart etag always treated as different", "abc123-1", "abc123-1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sameContent(c.localMD5, c.etag); got != c.want {
				t.Errorf("sameContent(%q, %q) = %v, want %v", c.localMD5, c.etag, got, c.want)
			}
		})
	}
}

func TestMD5File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("hello vault"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	got, err := md5File(path)
	if err != nil {
		t.Fatalf("md5File: %v", err)
	}
	// MD5("hello vault"), computed independently via `md5sum`.
	const want = "065ee65f0aa49423cc75336292a8cb24"
	if got != want {
		t.Errorf("md5File() = %q, want %q", got, want)
	}
}
