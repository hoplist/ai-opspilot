package retention

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupDirKeepsNewestItems(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, string(rune('a'+i))+".json")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		ts := time.Now().Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	if err := CleanupDir(dir, Policy{MaxItems: 2, Extension: []string{".json"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.json")); !os.IsNotExist(err) {
		t.Fatalf("oldest file should be removed, err=%v", err)
	}
}

func TestCleanupDirRespectsMaxBytes(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, string(rune('a'+i))+".json")
		if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
			t.Fatal(err)
		}
		ts := time.Now().Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	if err := CleanupDir(dir, Policy{MaxBytes: 10, Extension: []string{".json"}}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d", len(entries))
	}
}
