package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic_WritesContentAndPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	if err := WriteFileAtomic(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("content: got %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm: want 0600, got %o", info.Mode().Perm())
	}
}

func TestWriteFileAtomic_OverwritesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	if err := WriteFileAtomic(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("content after overwrite: got %q", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("want exactly 1 file in dir, got %d", len(entries))
	}
}

func TestWriteFileAtomic_MissingDirFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope", "out.json")
	if err := WriteFileAtomic(path, []byte("x"), 0o600); err == nil {
		t.Fatal("expected error for missing directory")
	}
}
