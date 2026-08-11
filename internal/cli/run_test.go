package cli_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func shell(script string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", script}
	}
	return []string{"sh", "-c", script}
}

func runWith(t *testing.T, before []string, script string) (int, string, string) {
	t.Helper()
	args := append(append([]string{"run"}, before...), "--")
	return run(t, append(args, shell(script)...)...)
}

func echoVar(name string) string {
	if runtime.GOOS == "windows" {
		return "echo %" + name + "%"
	}
	return "printf %s \"$" + name + "\""
}

func TestRunInjectsEnvironment(t *testing.T) {
	sealed(t)

	code, stdout, stderr := runWith(t, nil, echoVar("API_KEY"))
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "s3cretvalue" {
		t.Errorf("child saw %q, want the decrypted value", got)
	}
}

func TestRunPropagatesExitCode(t *testing.T) {
	sealed(t)

	code, _, stderr := runWith(t, nil, "exit 7")
	if code != 7 {
		t.Errorf("exit = %d, want 7", code)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing: the child already spoke", stderr)
	}
}

func TestRunKeepsParentEnvironment(t *testing.T) {
	sealed(t)
	t.Setenv("ENVSEAL_PARENT_VALUE", "inherited")

	_, stdout, _ := runWith(t, nil, echoVar("ENVSEAL_PARENT_VALUE"))
	if got := strings.TrimSpace(stdout); got != "inherited" {
		t.Errorf("child saw %q, want the parent value", got)
	}
}

func TestRunIsolated(t *testing.T) {
	sealed(t)
	t.Setenv("ENVSEAL_PARENT_VALUE", "inherited")

	_, stdout, _ := runWith(t, []string{"--isolated"}, echoVar("ENVSEAL_PARENT_VALUE"))
	if got := strings.TrimSpace(stdout); got == "inherited" {
		t.Error("--isolated leaked a parent variable to the child")
	}

	// PATH must survive, or nothing can be executed at all.
	_, stdout, _ = runWith(t, []string{"--isolated"}, echoVar("PATH"))
	if strings.TrimSpace(stdout) == "" {
		t.Error("--isolated withheld PATH")
	}
}

func TestRunDecryptedWinsOverParent(t *testing.T) {
	sealed(t)
	t.Setenv("API_KEY", "from-parent")

	_, stdout, _ := runWith(t, nil, echoVar("API_KEY"))
	if got := strings.TrimSpace(stdout); got != "s3cretvalue" {
		t.Errorf("child saw %q, want the decrypted value to win", got)
	}
}

func TestRunExplicitFile(t *testing.T) {
	dir := sealed(t)

	other := filepath.Join(dir, ".env.production.enc")
	if err := os.Rename(filepath.Join(dir, ".env.enc"), other); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runWith(t, []string{other}, echoVar("API_KEY"))
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "s3cretvalue" {
		t.Errorf("child saw %q, want the decrypted value", got)
	}
}

func TestRunWithoutSeparator(t *testing.T) {
	sealed(t)

	code, _, stderr := run(t, "run", "./server")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--") {
		t.Errorf("stderr =\n%s\nwant it to explain the -- separator", stderr)
	}
}

func TestRunTooManyArguments(t *testing.T) {
	sealed(t)

	code, _, stderr := run(t, "run", "a.enc", "b.enc", "--", "true")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "too many arguments") {
		t.Errorf("stderr =\n%s\nwant a clear complaint", stderr)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	sealed(t)

	code, _, stderr := run(t, "run", "--", "envseal-does-not-exist")
	if code != 5 {
		t.Errorf("exit = %d, want 5", code)
	}
	if !strings.Contains(stderr, "PATH") {
		t.Errorf("stderr =\n%s\nwant it to mention PATH", stderr)
	}
}

func TestRunWrongIdentity(t *testing.T) {
	sealed(t)

	other := filepath.Join(t.TempDir(), "identity")
	if code, _, stderr := run(t, "--identity", other, "init"); code != 0 {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}

	code, _, _ := run(t, "--identity", other, "run", "--", "true")
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

// The whole point of run: secrets reach the child without touching the disk.
func TestRunWritesNoPlaintext(t *testing.T) {
	dir := sealed(t)

	tmp := t.TempDir()
	for _, name := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(name, tmp)
	}

	if code, _, stderr := runWith(t, nil, "exit 0"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	assertNoPlaintext(t, dir, "s3cretvalue")
	assertNoPlaintext(t, tmp, "s3cretvalue")
}

// assertNoPlaintext fails if any file under root contains the secret.
func assertNoPlaintext(t *testing.T, root, secret string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // unreadable entries are not our concern
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if bytes.Contains(data, []byte(secret)) {
			t.Errorf("%s contains plaintext", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
