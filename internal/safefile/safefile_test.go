package safefile_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PeacexF/envseal/internal/safefile"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.enc")

	if err := safefile.Write(path, []byte("payload"), 0o600); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("content = %q, want %q", got, "payload")
	}
}

func TestWritePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}

	dir := t.TempDir()
	for _, perm := range []os.FileMode{0o600, 0o644} {
		path := filepath.Join(dir, perm.String())
		if err := safefile.Write(path, []byte("x"), perm); err != nil {
			t.Fatalf("Write() = %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() = %v", err)
		}
		if info.Mode().Perm() != perm {
			t.Errorf("mode = %v, want %v", info.Mode().Perm(), perm)
		}
	}
}

func TestWriteOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")

	if err := os.WriteFile(path, []byte("old and much longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := safefile.Write(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

func TestWriteLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()

	if err := safefile.Write(filepath.Join(dir, "f"), []byte("x"), 0o600); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "f" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory contains %v, want only [f]", names)
	}
}

func TestWriteFailureLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()

	if err := safefile.Write(filepath.Join(dir, "missing", "f"), []byte("x"), 0o600); err == nil {
		t.Fatal("Write() = nil, want an error for a missing directory")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("directory is not empty after a failed write: %v", entries)
	}
}
