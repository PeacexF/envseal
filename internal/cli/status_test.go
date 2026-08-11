package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
)

func statusJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()

	code, stdout, stderr := run(t, append([]string{"status", "--json"}, args...)...)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}

	var report map[string]any
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("json.Unmarshal(%q) = %v", stdout, err)
	}
	return report
}

func TestStatus(t *testing.T) {
	sealed(t)

	code, stdout, stderr := run(t, "status")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	for _, want := range []string{"Configuration", "Encrypted file", "Identity", "Recipients (1)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout =\n%s\nwant it to mention %q", stdout, want)
		}
	}
}

// Status reports on secrets without ever showing one.
func TestStatusShowsNoSecrets(t *testing.T) {
	sealed(t)

	_, stdout, stderr := run(t, "status")
	for name, out := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if strings.Contains(out, "s3cretvalue") || strings.Contains(out, "AGE-SECRET-KEY-") {
			t.Errorf("%s leaks secret material:\n%s", name, out)
		}
	}
}

func TestStatusJSON(t *testing.T) {
	sealed(t)
	report := statusJSON(t)

	for key, want := range map[string]any{
		"configuration_found":  true,
		"encrypted_file_found": true,
		"identity_available":   true,
		"decryptable":          true,
		"recipients":           float64(1),
	} {
		if report[key] != want {
			t.Errorf("%s = %v, want %v", key, report[key], want)
		}
	}
	if report["encrypted_file"] != config.DefaultFile {
		t.Errorf("encrypted_file = %v, want %q", report["encrypted_file"], config.DefaultFile)
	}
}

func TestStatusJSONShowsNoSecrets(t *testing.T) {
	sealed(t)

	_, stdout, _ := run(t, "status", "--json")
	if strings.Contains(stdout, "s3cretvalue") || strings.Contains(stdout, "AGE-SECRET-KEY-") {
		t.Errorf("JSON leaks secret material:\n%s", stdout)
	}
}

// Decryptability is reported by trying, not by assuming.
func TestStatusReportsLostAccess(t *testing.T) {
	sealed(t)
	other := filepath.Join(t.TempDir(), "identity")
	if code, _, stderr := run(t, "--identity", other, "keys", "generate"); code != 0 {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}

	report := statusJSON(t, "--identity", other)
	if report["identity_available"] != true {
		t.Error("identity_available = false, want true")
	}
	if report["decryptable"] != false {
		t.Error("decryptable = true, want false for a key that is not a recipient")
	}

	_, stdout, _ := run(t, "--identity", other, "status")
	if !strings.Contains(stdout, "envseal push") {
		t.Errorf("stdout =\n%s\nwant advice on regaining access", stdout)
	}
}

func TestStatusWithoutProject(t *testing.T) {
	project(t)

	code, stdout, stderr := run(t, "status")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "envseal init") {
		t.Errorf("stdout =\n%s\nwant it to explain how to start", stdout)
	}

	report := statusJSON(t)
	if report["configuration_found"] != false {
		t.Error("configuration_found = true, want false")
	}
}

func TestStatusWithoutIdentity(t *testing.T) {
	sealed(t)

	absent := filepath.Join(t.TempDir(), "identity")
	report := statusJSON(t, "--identity", absent)
	if report["identity_available"] != false {
		t.Error("identity_available = true, want false")
	}

	_, stdout, _ := run(t, "--identity", absent, "status")
	if !strings.Contains(stdout, "envseal keys generate") {
		t.Errorf("stdout =\n%s\nwant it to suggest generating an identity", stdout)
	}
}

func TestStatusWithoutEncryptedFile(t *testing.T) {
	dir := sealed(t)
	if err := os.Remove(filepath.Join(dir, config.DefaultFile)); err != nil {
		t.Fatal(err)
	}

	report := statusJSON(t)
	if report["encrypted_file_found"] != false {
		t.Error("encrypted_file_found = true, want false")
	}
	if report["decryptable"] != false {
		t.Error("decryptable = true, want false without a file")
	}
}

func TestStatusMalformedConfiguration(t *testing.T) {
	dir := sealed(t)
	writeEnv(t, dir, config.Filename, "version: 99\n")

	code, _, stderr := run(t, "status")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "version") {
		t.Errorf("stderr =\n%s\nwant it to name the problem", stderr)
	}
}

func TestQuietSuppressesProgressOutput(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	code, stdout, stderr := run(t, "--quiet", "encrypt", ".env")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing under --quiet", stdout)
	}

	// The payload of a command is not progress output: -o - still writes.
	if code, stdout, _ := run(t, "--quiet", "decrypt", "-o", "-"); code != 0 || stdout != env {
		t.Errorf("decrypt -o - = %q (exit %d), want the plaintext", stdout, code)
	}
}

func TestCompletion(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(sh, func(t *testing.T) {
			code, stdout, stderr := run(t, "completion", sh)
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
			}
			if !strings.Contains(stdout, "envseal") {
				t.Errorf("stdout does not look like a completion script:\n%s", stdout)
			}
		})
	}
}

func TestCompletionUnknownShell(t *testing.T) {
	if code, _, _ := run(t, "completion", "csh"); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}
