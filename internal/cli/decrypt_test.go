package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/identity"
)

// sealed sets up a project with .env encrypted and the plaintext removed.
func sealed(t *testing.T) string {
	t.Helper()

	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}
	if err := os.Remove(filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDecryptRoundTrip(t *testing.T) {
	dir := sealed(t)

	code, stdout, stderr := run(t, "decrypt")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if got := read(t, filepath.Join(dir, ".env")); got != env {
		t.Errorf("decrypted =\n%q\nwant\n%q", got, env)
	}
	if !strings.Contains(stdout, "Decrypted") {
		t.Errorf("stdout =\n%s\nwant a confirmation", stdout)
	}
}

func TestDecryptDerivesOutputName(t *testing.T) {
	dir := sealed(t)

	source := filepath.Join(dir, ".env.production.enc")
	if err := os.Rename(filepath.Join(dir, config.DefaultFile), source); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := run(t, "decrypt", source); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.production")); err != nil {
		t.Errorf("derived output: %v", err)
	}
}

func TestDecryptRefusesToOverwrite(t *testing.T) {
	dir := sealed(t)
	writeEnv(t, dir, ".env", "EXISTING=keep me\n")

	code, _, stderr := run(t, "decrypt")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("stderr =\n%s\nwant it to mention --force", stderr)
	}
	if got := read(t, filepath.Join(dir, ".env")); got != "EXISTING=keep me\n" {
		t.Error("the existing file was overwritten")
	}
}

func TestDecryptForceOverwrites(t *testing.T) {
	dir := sealed(t)
	writeEnv(t, dir, ".env", "EXISTING=replace me\n")

	if code, _, stderr := run(t, "decrypt", "--force"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if got := read(t, filepath.Join(dir, ".env")); got != env {
		t.Errorf("decrypted =\n%q\nwant\n%q", got, env)
	}
}

// Plaintext on disk must not be readable by other users.
func TestDecryptedFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	dir := sealed(t)

	if code, _, stderr := run(t, "decrypt"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	info, err := os.Stat(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %04o, want 0600", got)
	}
}

// Atomic writes go through a temporary file; none may survive the command.
func TestDecryptLeavesNoTemporaryFiles(t *testing.T) {
	dir := sealed(t)

	if code, _, stderr := run(t, "decrypt"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("%s survived the write", e.Name())
		}
	}
}

func TestDecryptToStdout(t *testing.T) {
	sealed(t)

	code, stdout, stderr := run(t, "decrypt", "-o", "-")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stdout != env {
		t.Errorf("stdout =\n%q\nwant\n%q", stdout, env)
	}
}

func TestDecryptWrongIdentity(t *testing.T) {
	sealed(t)

	// A second identity that was never added as a recipient.
	other := filepath.Join(t.TempDir(), "identity")
	if code, _, stderr := run(t, "--identity", other, "keys", "generate"); code != 0 {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}

	code, _, stderr := run(t, "--identity", other, "decrypt")
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if !strings.Contains(stderr, "not encrypted for the identity") {
		t.Errorf("stderr =\n%s\nwant an explanation", stderr)
	}
}

func TestDecryptMissingFile(t *testing.T) {
	dir := sealed(t)
	if err := os.Remove(filepath.Join(dir, config.DefaultFile)); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := run(t, "decrypt")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, config.DefaultFile) {
		t.Errorf("stderr =\n%s\nwant it to name the missing file", stderr)
	}
}

func TestDecryptCorruptedFile(t *testing.T) {
	dir := sealed(t)

	path := filepath.Join(dir, config.DefaultFile)
	damaged := strings.Replace(read(t, path), "-----BEGIN AGE ENCRYPTED FILE-----", "-----BEGIN AGE ENCRYPTED FILE-----\ncorrupted", 1)
	if err := os.WriteFile(path, []byte(damaged), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := run(t, "decrypt")
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if !strings.Contains(stderr, "unable to decrypt") {
		t.Errorf("stderr =\n%s\nwant a decryption failure", stderr)
	}
}

func TestDecryptWithoutIdentity(t *testing.T) {
	dir := sealed(t)

	code, _, stderr := run(t, "--identity", filepath.Join(dir, "absent"), "decrypt")
	if code != 4 {
		t.Errorf("exit = %d, want 4", code)
	}
	if !strings.Contains(stderr, "envseal keys generate") {
		t.Errorf("stderr =\n%s\nwant it to suggest generating an identity", stderr)
	}
}

func TestDecryptCannotDeriveOutputName(t *testing.T) {
	dir := sealed(t)

	source := filepath.Join(dir, "secrets.sealed")
	if err := os.Rename(filepath.Join(dir, config.DefaultFile), source); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := run(t, "decrypt", source)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "-o") {
		t.Errorf("stderr =\n%s\nwant it to suggest -o", stderr)
	}
}

func TestDecryptWarnsAboutLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	sealed(t)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(home, identity.Dir, identity.File), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, stderr := run(t, "decrypt")
	if !strings.Contains(stderr, "chmod 600") {
		t.Errorf("stderr =\n%s\nwant a permissions warning", stderr)
	}
}
