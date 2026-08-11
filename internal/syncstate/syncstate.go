// Package syncstate remembers what envseal last wrote to a plaintext file, so
// a later command can tell an untouched file from one you edited by hand.
//
// The record lives under the home directory rather than in the project, so a
// repository is never modified to hold envseal's bookkeeping. It stores a
// SHA-256 of the content, never the content itself: enough to recognise a file
// envseal produced, useless for recovering what was in it.
package syncstate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
)

const (
	dirName  = "sync"
	fileMode = 0o600
	dirMode  = 0o700
)

// Record notes that content is what envseal wrote to path. A failure to record
// is not fatal: the caller simply loses the ability to detect later edits.
func Record(path string, content []byte) {
	target, err := recordPath(path)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return
	}

	sum := fingerprint(content)
	_ = safefile.Write(target, []byte(sum), fileMode)
}

// Matches reports whether content is exactly what envseal last wrote to path.
// An unknown file is not a match: absence of a record proves nothing.
func Matches(path string, content []byte) bool {
	target, err := recordPath(path)
	if err != nil {
		return false
	}
	recorded, err := os.ReadFile(target)
	if err != nil {
		return false
	}
	return string(recorded) == fingerprint(content)
}

// Forget removes the record for path.
func Forget(path string) {
	if target, err := recordPath(path); err == nil {
		_ = os.Remove(target)
	}
}

func fingerprint(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// recordPath names the record for a file, keyed by its resolved location so
// that two projects never collide.
func recordPath(path string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(real, filepath.Base(abs))
	}

	key := sha256.Sum256([]byte(abs))
	return filepath.Join(home, identity.Dir, dirName, hex.EncodeToString(key[:])), nil
}
