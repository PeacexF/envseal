package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
)

// sealedValue returns what the encrypted file currently holds for a key.
func sealedValue(t *testing.T, key string) string {
	t.Helper()

	code, stdout, stderr := run(t, "decrypt", "-o", "-")
	if code != 0 {
		t.Fatalf("decrypt: exit = %d (stderr: %s)", code, stderr)
	}
	for line := range strings.SplitSeq(stdout, "\n") {
		name, value, found := strings.Cut(line, "=")
		if found && name == key {
			return strings.Trim(value, `"`)
		}
	}
	t.Fatalf("%s not found in:\n%s", key, stdout)
	return ""
}

func TestRotateFromStdin(t *testing.T) {
	sealed(t)

	code, stdout, stderr := runInput(t, "brand-new-secret", "rotate", "API_KEY", "--stdin")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Rotated API_KEY") {
		t.Errorf("stdout =\n%s\nwant a confirmation", stdout)
	}
	if got := sealedValue(t, "API_KEY"); got != "brand-new-secret" {
		t.Errorf("sealed value = %q, want the new one", got)
	}
}

// A trailing newline from echo or a here-string is an artefact, not the value.
func TestRotateTrimsOneTrailingNewline(t *testing.T) {
	sealed(t)

	if code, _, stderr := runInput(t, "value\n", "rotate", "API_KEY", "--stdin"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got := sealedValue(t, "API_KEY"); got != "value" {
		t.Errorf("sealed value = %q, want %q", got, "value")
	}
}

// Everything else in the file must survive the edit untouched.
func TestRotatePreservesTheRestOfTheFile(t *testing.T) {
	dir := project(t)
	source := "# database settings\nDATABASE_URL=postgres://localhost/app  # primary\n\n# secrets\nAPI_KEY='old'\nDEBUG=false\n"
	writeEnv(t, dir, ".env", source)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}
	if err := os.Remove(filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := runInput(t, "new", "rotate", "API_KEY", "--stdin"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	_, got, _ := run(t, "decrypt", "-o", "-")
	want := strings.Replace(source, "API_KEY='old'", "API_KEY='new'", 1)
	if got != want {
		t.Errorf("decrypted =\n%q\nwant\n%q", got, want)
	}
}

func TestRotateGenerate(t *testing.T) {
	sealed(t)

	code, stdout, stderr := run(t, "rotate", "API_KEY", "--generate", "--length", "48")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Generated a value of 48 characters") {
		t.Errorf("stdout =\n%s\nwant the generated length", stdout)
	}

	got := sealedValue(t, "API_KEY")
	if len(got) != 48 {
		t.Errorf("value length = %d, want 48", len(got))
	}
	if got == "s3cretvalue" {
		t.Error("the value was not changed")
	}
	// Generated values must not need quoting.
	if strings.ContainsAny(got, " \t\n\"'#\\") {
		t.Errorf("generated value %q contains characters that need quoting", got)
	}
}

func TestRotateGenerateIsRandom(t *testing.T) {
	sealed(t)

	if code, _, stderr := run(t, "rotate", "API_KEY", "--generate"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	first := sealedValue(t, "API_KEY")

	if code, _, stderr := run(t, "rotate", "API_KEY", "--generate"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if second := sealedValue(t, "API_KEY"); second == first {
		t.Error("two generated values were identical")
	}
}

// The new value must not appear anywhere in the output.
func TestRotateNeverPrintsTheValue(t *testing.T) {
	sealed(t)

	_, stdout, stderr := runInput(t, "topsecretvalue", "rotate", "API_KEY", "--stdin")
	for name, out := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if strings.Contains(out, "topsecretvalue") {
			t.Errorf("%s leaked the new value:\n%s", name, out)
		}
	}
}

// Rotation happens in memory: no plaintext may be left behind.
func TestRotateWritesNoPlaintext(t *testing.T) {
	dir := sealed(t)

	tmp := t.TempDir()
	for _, name := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(name, tmp)
	}

	if code, _, stderr := runInput(t, "topsecretvalue", "rotate", "API_KEY", "--stdin"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	assertNoPlaintext(t, dir, "topsecretvalue")
	assertNoPlaintext(t, tmp, "topsecretvalue")
}

func TestRotateUnknownVariable(t *testing.T) {
	sealed(t)

	code, _, stderr := runInput(t, "value", "rotate", "TYPOED_NAME", "--stdin")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--add") {
		t.Errorf("stderr =\n%s\nwant it to mention --add", stderr)
	}
}

func TestRotateAdd(t *testing.T) {
	sealed(t)

	code, stdout, stderr := runInput(t, "value", "rotate", "BRAND_NEW", "--stdin", "--add")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Added BRAND_NEW") {
		t.Errorf("stdout =\n%s\nwant it to report an addition", stdout)
	}
	if got := sealedValue(t, "BRAND_NEW"); got != "value" {
		t.Errorf("value = %q, want %q", got, "value")
	}
}

// A stale .env would be re-encrypted by the next push, undoing the rotation.
func TestRotateUpdatesTheLocalPlaintext(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := runInput(t, "rotated-value", "rotate", "API_KEY", "--stdin")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Updated .env") {
		t.Errorf("stdout =\n%s\nwant it to report the local update", stdout)
	}

	local := read(t, filepath.Join(dir, ".env"))
	if !strings.Contains(local, "API_KEY=rotated-value") {
		t.Errorf(".env =\n%s\nwant the new value", local)
	}
}

func TestRotateWarnsWhenLocalPlaintextDiffers(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}
	writeEnv(t, dir, ".env", env+"LOCAL_EDIT=1\n")

	code, stdout, stderr := runInput(t, "rotated-value", "rotate", "API_KEY", "--stdin")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "local changes") {
		t.Errorf("stdout =\n%s\nwant a warning", stdout)
	}
	if local := read(t, filepath.Join(dir, ".env")); !strings.Contains(local, "LOCAL_EDIT=1") {
		t.Error("the local edit was discarded")
	}
}

func TestRotateWithoutATerminal(t *testing.T) {
	sealed(t)

	code, _, stderr := run(t, "rotate", "API_KEY")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	for _, want := range []string{"--stdin", "--generate"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr =\n%s\nwant it to suggest %s", stderr, want)
		}
	}
}

func TestRotateConflictingFlags(t *testing.T) {
	sealed(t)

	if code, _, _ := runInput(t, "x", "rotate", "API_KEY", "--stdin", "--generate"); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestRotateShortGenerateLength(t *testing.T) {
	sealed(t)

	code, _, stderr := run(t, "rotate", "API_KEY", "--generate", "--length", "4")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "at least 8") {
		t.Errorf("stderr =\n%s\nwant a minimum length", stderr)
	}
}

func TestRotateWithoutProject(t *testing.T) {
	project(t)

	code, _, stderr := runInput(t, "value", "rotate", "API_KEY", "--stdin")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, config.Filename) {
		t.Errorf("stderr =\n%s\nwant it to name the missing configuration", stderr)
	}
}

func TestRotateWrongIdentity(t *testing.T) {
	sealed(t)
	other := newIdentityOnly(t)

	if code, _, _ := runInput(t, "value", "--identity", other, "rotate", "API_KEY", "--stdin"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

// Everyone authorized must still be able to read the rotated file.
func TestRotateKeepsAllRecipients(t *testing.T) {
	sealed(t)
	second := newIdentity(t, "second")
	if code, _, stderr := run(t, "reseal"); code != 0 {
		t.Fatalf("reseal: exit = %d (stderr: %s)", code, stderr)
	}

	if code, _, stderr := runInput(t, "new-value", "rotate", "API_KEY", "--stdin"); code != 0 {
		t.Fatalf("rotate: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "--identity", second, "decrypt", "-o", "-")
	if code != 0 {
		t.Fatalf("second recipient: exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "API_KEY=new-value") {
		t.Errorf("second recipient sees:\n%s\nwant the rotated value", stdout)
	}
}
