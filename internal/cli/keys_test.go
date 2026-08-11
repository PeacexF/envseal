package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/identity"
)

func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(identity.EnvVar, "")
	return home
}

func TestKeysGenerate(t *testing.T) {
	home := isolate(t)

	code, stdout, stderr := run(t, "keys", "generate")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Identity created.") {
		t.Errorf("stdout =\n%s\nwant a confirmation", stdout)
	}
	if !strings.Contains(stdout, "age1") {
		t.Errorf("stdout =\n%s\nwant the public key", stdout)
	}

	if _, err := os.Stat(filepath.Join(home, identity.Dir, identity.File)); err != nil {
		t.Errorf("identity file: %v", err)
	}
}

// The private key must never reach a terminal, a log, or a CI transcript.
func TestKeysGenerateNeverPrintsPrivateKey(t *testing.T) {
	isolate(t)

	_, stdout, stderr := run(t, "keys", "generate")
	for name, out := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if strings.Contains(out, identity.KeyPrefix) {
			t.Errorf("%s contains private key material:\n%s", name, out)
		}
	}
}

func TestKeysGenerateRefusesExisting(t *testing.T) {
	isolate(t)

	if code, _, stderr := run(t, "keys", "generate"); code != 0 {
		t.Fatalf("first generate: exit = %d (stderr: %s)", code, stderr)
	}

	code, _, stderr := run(t, "keys", "generate")
	if code != 4 {
		t.Errorf("exit = %d, want 4", code)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("stderr =\n%s\nwant it to report the existing identity", stderr)
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("stderr =\n%s\nwant it to mention --force", stderr)
	}
}

func TestKeysGenerateForce(t *testing.T) {
	home := isolate(t)

	if code, _, stderr := run(t, "keys", "generate"); code != 0 {
		t.Fatalf("first generate: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "keys", "generate", "--force")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Previous identity kept at") {
		t.Errorf("stdout =\n%s\nwant the backup location", stdout)
	}

	entries, err := os.ReadDir(filepath.Join(home, identity.Dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("directory holds %d files, want the identity and one backup", len(entries))
	}
}

func TestKeysGenerateIdentityFlag(t *testing.T) {
	isolate(t)
	path := filepath.Join(t.TempDir(), "custom", "identity")

	code, _, stderr := run(t, "--identity", path, "keys", "generate")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("identity file: %v", err)
	}
}

func TestKeysGenerateAbbreviatesHomeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("~ abbreviation is a Unix convention")
	}
	home := isolate(t)

	_, stdout, _ := run(t, "keys", "generate")
	if !strings.Contains(stdout, "~/"+identity.Dir) {
		t.Errorf("stdout =\n%s\nwant the path abbreviated to ~", stdout)
	}
	if strings.Contains(stdout, home) {
		t.Errorf("stdout =\n%s\nwant the home directory abbreviated", stdout)
	}
}

func TestKeysGenerateRejectsEnvKeyMaterial(t *testing.T) {
	isolate(t)
	t.Setenv(identity.EnvVar, identity.KeyPrefix+"WHATEVER")

	code, _, stderr := run(t, "keys", "generate")
	if code != 4 {
		t.Errorf("exit = %d, want 4", code)
	}
	if !strings.Contains(stderr, identity.EnvVar) {
		t.Errorf("stderr =\n%s\nwant it to name "+identity.EnvVar, stderr)
	}
}

func TestKeysPublic(t *testing.T) {
	isolate(t)
	if code, _, stderr := run(t, "keys", "generate"); code != 0 {
		t.Fatalf("generate: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "keys", "public")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}

	key := strings.TrimSpace(stdout)
	if !strings.HasPrefix(key, "age1") {
		t.Errorf("stdout = %q, want a public key on its own line", stdout)
	}
	if strings.Contains(stdout, identity.KeyPrefix) {
		t.Errorf("stdout leaked the private key:\n%s", stdout)
	}

	// It is the payload, so --quiet must not silence it.
	if _, quiet, _ := run(t, "--quiet", "keys", "public"); strings.TrimSpace(quiet) != key {
		t.Errorf("--quiet output = %q, want the key", quiet)
	}
}

func TestKeysPublicWithoutIdentity(t *testing.T) {
	isolate(t)

	code, _, stderr := run(t, "keys", "public")
	if code != 4 {
		t.Errorf("exit = %d, want 4", code)
	}
	if !strings.Contains(stderr, "envseal keys generate") {
		t.Errorf("stderr =\n%s\nwant it to suggest generating one", stderr)
	}
}
