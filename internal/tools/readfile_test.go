package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if want := "hello\nworld\n"; got != want {
		t.Errorf("ReadFile = %q, want %q", got, want)
	}
}

func TestReadFileMissing(t *testing.T) {
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Error("ReadFile on a missing file returned no error")
	}
}
