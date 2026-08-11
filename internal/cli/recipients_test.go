package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
)

const (
	keyAlice = "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p"
	keyBob   = "age1lggyhqrw2nlhcxprm67z43rta597azn8gknawjehu9d9dl0jq3yqqvfafg"
)

func loadConfig(t *testing.T, dir string) *config.Config {
	t.Helper()
	c, err := config.Load(filepath.Join(dir, config.Filename))
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	return c
}

func TestAdd(t *testing.T) {
	dir := sealed(t)

	code, stdout, stderr := run(t, "add", "alice", keyAlice)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "envseal rotate") {
		t.Errorf("stdout =\n%s\nwant a reminder to rotate", stdout)
	}

	c := loadConfig(t, dir)
	if i := c.Find("alice"); i < 0 || c.Recipients[i].Key != keyAlice {
		t.Errorf("recipients = %+v, want alice", c.Recipients)
	}
}

// Adding a recipient must not silently grant access to the existing file.
func TestAddDoesNotReEncrypt(t *testing.T) {
	dir := sealed(t)
	before := read(t, filepath.Join(dir, config.DefaultFile))

	if code, _, stderr := run(t, "add", "alice", keyAlice); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	if read(t, filepath.Join(dir, config.DefaultFile)) != before {
		t.Error("add rewrote the encrypted file")
	}
}

func TestAddInvalidKey(t *testing.T) {
	sealed(t)

	code, _, stderr := run(t, "add", "alice", "not-a-key")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "age1") {
		t.Errorf("stderr =\n%s\nwant it to describe a valid key", stderr)
	}
}

func TestAddDuplicates(t *testing.T) {
	sealed(t)
	if code, _, stderr := run(t, "add", "alice", keyAlice); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	t.Run("same name", func(t *testing.T) {
		code, _, stderr := run(t, "add", "alice", keyBob)
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(stderr, "already a recipient") {
			t.Errorf("stderr =\n%s", stderr)
		}
	})

	t.Run("same key", func(t *testing.T) {
		code, _, stderr := run(t, "add", "duplicate", keyAlice)
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(stderr, "already authorized") {
			t.Errorf("stderr =\n%s", stderr)
		}
	})
}

func TestAddCreatesConfiguration(t *testing.T) {
	dir := project(t)

	if code, _, stderr := run(t, "add", "alice", keyAlice); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if c := loadConfig(t, dir); len(c.Recipients) != 1 {
		t.Errorf("recipients = %d, want 1", len(c.Recipients))
	}
}

func TestRemove(t *testing.T) {
	dir := sealed(t)
	if code, _, stderr := run(t, "add", "alice", keyAlice); code != 0 {
		t.Fatalf("add: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "remove", "alice")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if c := loadConfig(t, dir); c.Find("alice") >= 0 {
		t.Error("alice is still a recipient")
	}
	if !strings.Contains(stdout, "older copy") {
		t.Errorf("stdout =\n%s\nwant a warning that old copies stay readable", stdout)
	}
}

func TestRemoveByKey(t *testing.T) {
	dir := sealed(t)
	if code, _, stderr := run(t, "add", "alice", keyAlice); code != 0 {
		t.Fatalf("add: exit = %d (stderr: %s)", code, stderr)
	}

	if code, _, stderr := run(t, "remove", keyAlice); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if c := loadConfig(t, dir); c.Find("alice") >= 0 {
		t.Error("alice is still a recipient")
	}
}

func TestRemoveUnknown(t *testing.T) {
	sealed(t)

	code, _, stderr := run(t, "remove", "nobody")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "envseal status") {
		t.Errorf("stderr =\n%s\nwant it to suggest `envseal status`", stderr)
	}
}

// Removing your own key costs you access at the next rotation; say so.
func TestRemoveOwnKeyWarns(t *testing.T) {
	dir := sealed(t)

	own := loadConfig(t, dir).Recipients[0].Name
	_, stdout, _ := run(t, "remove", own)
	if !strings.Contains(stdout, "your own key") {
		t.Errorf("stdout =\n%s\nwant a warning about removing yourself", stdout)
	}
	if !strings.Contains(stdout, "No recipients remain") {
		t.Errorf("stdout =\n%s\nwant a warning that none remain", stdout)
	}
}

func TestRotateGivesAccessToANewRecipient(t *testing.T) {
	sealed(t)

	// A second identity, added as a recipient but not yet able to read.
	second := newIdentity(t, "second")

	if code, _, _ := run(t, "--identity", second, "decrypt", "-o", "-"); code != 3 {
		t.Errorf("before rotate: exit = %d, want 3", code)
	}

	if code, stdout, stderr := run(t, "rotate"); code != 0 {
		t.Fatalf("rotate: exit = %d (stderr: %s)", code, stderr)
	} else if !strings.Contains(stdout, "2 recipient") {
		t.Errorf("stdout =\n%s\nwant the recipient count", stdout)
	}

	code, stdout, stderr := run(t, "--identity", second, "decrypt", "-o", "-")
	if code != 0 {
		t.Fatalf("after rotate: exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stdout != env {
		t.Errorf("decrypted =\n%q\nwant\n%q", stdout, env)
	}
}

func TestRotateRevokesAccess(t *testing.T) {
	sealed(t)
	second := newIdentity(t, "second")

	if code, _, stderr := run(t, "rotate"); code != 0 {
		t.Fatalf("rotate: exit = %d (stderr: %s)", code, stderr)
	}
	if code, _, stderr := run(t, "remove", "second"); code != 0 {
		t.Fatalf("remove: exit = %d (stderr: %s)", code, stderr)
	}

	// Still readable until the file is re-encrypted: that is why rotate exists.
	if code, _, _ := run(t, "--identity", second, "decrypt", "-o", "-"); code != 0 {
		t.Errorf("before rotate: exit = %d, want the old file to still be readable", code)
	}

	if code, _, stderr := run(t, "rotate"); code != 0 {
		t.Fatalf("rotate: exit = %d (stderr: %s)", code, stderr)
	}
	if code, _, _ := run(t, "--identity", second, "decrypt", "-o", "-"); code != 3 {
		t.Errorf("after rotate: exit = %d, want 3", code)
	}

	// The owner keeps access throughout.
	if code, _, stderr := run(t, "decrypt", "-o", "-"); code != 0 {
		t.Errorf("owner: exit = %d (stderr: %s)", code, stderr)
	}
}

func TestRotateWarnsWhenLockingYourselfOut(t *testing.T) {
	dir := sealed(t)

	// Replace the recipient list with a key we do not hold.
	writeEnv(t, dir, config.Filename,
		"version: 1\nfile: .env.enc\nrecipients:\n  - name: someone\n    key: "+keyAlice+"\n")

	code, stdout, stderr := run(t, "rotate")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "no longer decrypt") {
		t.Errorf("stdout =\n%s\nwant a lockout warning", stdout)
	}
}

func TestRotateWithoutEncryptedFile(t *testing.T) {
	dir := sealed(t)
	if err := os.Remove(filepath.Join(dir, config.DefaultFile)); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := run(t, "rotate")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "envseal encrypt") {
		t.Errorf("stderr =\n%s\nwant it to suggest `envseal encrypt`", stderr)
	}
}

func TestRotateOutsideAProject(t *testing.T) {
	project(t)

	code, _, stderr := run(t, "rotate")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, config.Filename) {
		t.Errorf("stderr =\n%s\nwant it to name the missing configuration", stderr)
	}
}

// newIdentityOnly creates an identity that is not a recipient of anything.
func newIdentityOnly(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "identity")
	if code, _, stderr := run(t, "--identity", path, "init"); code != 0 {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}
	return path
}

// newIdentity creates an identity elsewhere, authorizes it under name, and
// returns its path. The project is not re-encrypted: that is rotate's job.
func newIdentity(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "identity")
	code, stdout, stderr := run(t, "--identity", path, "init")
	if code != 0 {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}

	key := ""
	for _, field := range strings.Fields(stdout) {
		if strings.HasPrefix(field, "age1") {
			key = field
			break
		}
	}
	if key == "" {
		t.Fatalf("no public key in:\n%s", stdout)
	}

	if code, _, stderr := run(t, "add", name, key); code != 0 {
		t.Fatalf("add: exit = %d (stderr: %s)", code, stderr)
	}
	return path
}
