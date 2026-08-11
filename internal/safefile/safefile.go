// Package safefile writes files atomically: a reader never observes a partial
// file, and a crash never leaves one behind.
//
// Atomic replacement requires a temporary file, so decrypted content does
// briefly exist under a second name. That file is created 0600 in the
// destination directory, never in a shared location such as /tmp, and is
// removed on every error path. Only an uncatchable kill between the write and
// the rename can leave it behind, and it is owner-only even then.
package safefile

import (
	"os"
	"path/filepath"
)

// Write writes data to path via a temporary file in the same directory, then
// renames it into place. The temporary file is created 0600 and only widened to
// perm once written, so its contents are never briefly world-readable.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		f.Close()
		os.Remove(tmp)
	}()

	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Chmod(perm); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
