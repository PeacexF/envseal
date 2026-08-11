package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
)

const env = "DATABASE_URL=postgres://localhost/app\nAPI_KEY=s3cretvalue\n"

// project sets up an isolated home with an identity and an empty project
// directory as the working directory.
func project(t *testing.T) string {
	t.Helper()
	isolate(t)

	dir := t.TempDir()
	t.Chdir(dir)

	if code, _, stderr := run(t, "init"); code != 0 {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}
	return dir
}

func writeEnv(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) = %v", path, err)
	}
	return string(data)
}

func TestEncryptBootstrapsProject(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	code, stdout, stderr := run(t, "encrypt", ".env")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}

	enc := read(t, filepath.Join(dir, config.DefaultFile))
	if !strings.HasPrefix(enc, "-----BEGIN AGE ENCRYPTED FILE-----") {
		t.Errorf("%s is not armored:\n%s", config.DefaultFile, enc)
	}

	cfg, err := config.Load(filepath.Join(dir, config.Filename))
	if err != nil {
		t.Fatalf("Load(%s) = %v", config.Filename, err)
	}
	if len(cfg.Recipients) != 1 {
		t.Errorf("recipients = %d, want 1", len(cfg.Recipients))
	}
	if !strings.Contains(stdout, "Created") {
		t.Errorf("stdout =\n%s\nwant it to report the created configuration", stdout)
	}
}

// Encrypting must never echo what it just protected.
func TestEncryptNeverPrintsSecrets(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	_, stdout, stderr := run(t, "encrypt", ".env")
	for name, out := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if strings.Contains(out, "s3cretvalue") {
			t.Errorf("%s contains a secret value:\n%s", name, out)
		}
	}
}

func TestEncryptDefaultsToDotEnv(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	if code, _, stderr := run(t, "encrypt"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DefaultFile)); err != nil {
		t.Errorf("encrypted file: %v", err)
	}
}

func TestEncryptOutputFlag(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	target := filepath.Join(dir, ".env.production.enc")
	if code, _, stderr := run(t, "encrypt", ".env", "-o", target); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("output file: %v", err)
	}
}

func TestEncryptExplicitRecipient(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	const key = "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p"
	if code, _, stderr := run(t, "encrypt", ".env", "--recipient", key); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}

	// An explicit recipient is a one-off and must not rewrite the project.
	if _, err := os.Stat(filepath.Join(dir, config.Filename)); err == nil {
		t.Error("--recipient wrote a configuration file")
	}
}

func TestEncryptInvalidRecipient(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	code, _, stderr := run(t, "encrypt", ".env", "--recipient", "nonsense")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "age1") {
		t.Errorf("stderr =\n%s\nwant it to describe a valid key", stderr)
	}
}

func TestEncryptWithoutRecipients(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, config.Filename, "version: 1\nrecipients: []\n")

	code, _, stderr := run(t, "encrypt", ".env")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "envseal add") {
		t.Errorf("stderr =\n%s\nwant it to suggest `envseal add`", stderr)
	}
}

func TestEncryptMissingFile(t *testing.T) {
	project(t)

	code, _, stderr := run(t, "encrypt", ".env")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, ".env") {
		t.Errorf("stderr =\n%s\nwant it to name the missing file", stderr)
	}
}

func TestEncryptRejectsMalformedEnv(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", "GOOD=1\nBROKEN LINE\n")

	code, _, stderr := run(t, "encrypt", ".env")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "Line 2") {
		t.Errorf("stderr =\n%s\nwant it to point at the bad line", stderr)
	}
}

func TestEncryptedFileIsCommittable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	info, err := os.Stat(filepath.Join(dir, config.DefaultFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("mode = %04o, want 0644", got)
	}
}

func TestEncryptFromSubdirectoryUsesProjectRoot(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	sub := filepath.Join(dir, "services", "api")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	writeEnv(t, sub, ".env", "LOCAL=1\n")

	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	// The input is relative to the working directory, the output to the project.
	if _, err := os.Stat(filepath.Join(sub, config.DefaultFile)); err == nil {
		t.Error("encrypted file was written to the subdirectory")
	}
	if _, err := os.Stat(filepath.Join(dir, config.DefaultFile)); err != nil {
		t.Errorf("project encrypted file: %v", err)
	}
}
