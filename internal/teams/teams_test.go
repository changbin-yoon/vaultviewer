package teams

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryStoreSetAndGet(t *testing.T) {
	s := NewMemoryStore()
	if err := s.Set(map[string]string{"dt-bi-adm": "데이터플랫폼팀"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["dt-bi-adm"] != "데이터플랫폼팀" {
		t.Fatalf("got %+v", got)
	}
}

func TestMemoryStoreGetReturnsACopy(t *testing.T) {
	s := NewMemoryStore()
	_ = s.Set(map[string]string{"adm": "운영팀"})
	got, _ := s.Get()
	got["adm"] = "mutated"
	again, _ := s.Get()
	if again["adm"] != "운영팀" {
		t.Fatalf("mutating a Get() result affected the store: %+v", again)
	}
}

func TestFileStorePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group-teams.json")

	s1, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if err := s1.Set(map[string]string{"adm": "플랫폼운영팀", "dev": "백엔드개발팀"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// A fresh store pointed at the same file — simulating a restart — must
	// see what the first one wrote.
	s2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore (reload): %v", err)
	}
	got, err := s2.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["adm"] != "플랫폼운영팀" || got["dev"] != "백엔드개발팀" || len(got) != 2 {
		t.Fatalf("unexpected map after reload: %+v", got)
	}
}

func TestFileStoreStartsEmptyWhenFileDoesNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist-yet.json")
	s, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %+v", got)
	}
}

func TestFileStoreSetOverwritesPreviousContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group-teams.json")
	s, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if err := s.Set(map[string]string{"a": "1"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(map[string]string{"b": "2"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get()
	if len(got) != 1 || got["b"] != "2" {
		t.Fatalf("expected only the latest Set to survive, got %+v", got)
	}

	// No leftover .tmp file after a successful rename.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected no leftover .tmp file, stat err = %v", err)
	}
}
