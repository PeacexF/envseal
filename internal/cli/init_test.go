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

func TestInit(t *testing.T) {
	home := isolate(t)

	code, stdout, stderr := run(t, "init")
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
func TestInitNeverPrintsPrivateKey(t *testing.T) {
	isolate(t)

	_, stdout, stderr := run(t, "init")
	for name, out := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if strings.Contains(out, identity.KeyPrefix) {
			t.Errorf("%s contains private key material:\n%s", name, out)
		}
	}
}

func TestInitRefusesExisting(t *testing.T) {
	isolate(t)

	if code, _, stderr := run(t, "init"); code != 0 {
		t.Fatalf("first init: exit = %d (stderr: %s)", code, stderr)
	}

	code, _, stderr := run(t, "init")
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

func TestInitForce(t *testing.T) {
	home := isolate(t)

	if code, _, stderr := run(t, "init"); code != 0 {
		t.Fatalf("first init: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "init", "--force")
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

func TestInitIdentityFlag(t *testing.T) {
	isolate(t)
	path := filepath.Join(t.TempDir(), "custom", "identity")

	code, _, stderr := run(t, "--identity", path, "init")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("identity file: %v", err)
	}
}

func TestInitAbbreviatesHomeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("~ abbreviation is a Unix convention")
	}
	home := isolate(t)

	_, stdout, _ := run(t, "init")
	if !strings.Contains(stdout, "~/"+identity.Dir) {
		t.Errorf("stdout =\n%s\nwant the path abbreviated to ~", stdout)
	}
	if strings.Contains(stdout, home) {
		t.Errorf("stdout =\n%s\nwant the home directory abbreviated", stdout)
	}
}

func TestInitRejectsEnvKeyMaterial(t *testing.T) {
	isolate(t)
	t.Setenv(identity.EnvVar, identity.KeyPrefix+"WHATEVER")

	code, _, stderr := run(t, "init")
	if code != 4 {
		t.Errorf("exit = %d, want 4", code)
	}
	if !strings.Contains(stderr, identity.EnvVar) {
		t.Errorf("stderr =\n%s\nwant it to name "+identity.EnvVar, stderr)
	}
}
