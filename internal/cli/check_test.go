package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
)

// gitProject is a project inside a git repository, with no remote.
func gitProject(t *testing.T) string {
	t.Helper()

	dir := project(t)
	mustGit(t, dir, "init", "--initial-branch=main")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test")
	writeEnv(t, dir, ".gitignore", ".env\n!*.enc\n")
	return dir
}

func checkJSON(t *testing.T, args ...string) map[string]string {
	t.Helper()

	_, stdout, _ := run(t, append([]string{"check", "--json"}, args...)...)

	var report struct {
		Checks []struct {
			Check  string `json:"check"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("json.Unmarshal(%q) = %v", stdout, err)
	}

	status := make(map[string]string, len(report.Checks))
	for _, c := range report.Checks {
		status[c.Check] = c.Status
	}
	return status
}

func TestCheckPasses(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, ".env.example", "DATABASE_URL=\nAPI_KEY=\n")
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "check")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s stdout: %s)", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "No problems found") {
		t.Errorf("stdout =\n%s\nwant a clean report", stdout)
	}

	for name, want := range map[string]string{
		"configuration":  "ok",
		"encrypted file": "ok",
		"schema":         "ok",
		"plaintext":      "ok",
	} {
		if got := checkJSON(t)[name]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestCheckReportsMissingVariables(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, ".env.example", "DATABASE_URL=\nAPI_KEY=\nSTRIPE_KEY=\nSENTRY_DSN=\n")
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, _ := run(t, "check")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	for _, want := range []string{"STRIPE_KEY", "SENTRY_DSN", "2 variables missing"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout =\n%s\nwant %q", stdout, want)
		}
	}
	if got := checkJSON(t)["schema"]; got != "failed" {
		t.Errorf("schema = %q, want failed", got)
	}
}

// Extra variables are normal; only missing ones are a problem.
func TestCheckAllowsExtraVariables(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".env", env+"EXTRA=1\n")
	writeEnv(t, dir, ".env.example", "DATABASE_URL=\nAPI_KEY=\n")
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	if code, _, _ := run(t, "check"); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

// The failure the whole tool exists to prevent.
func TestCheckDetectsCommittedPlaintext(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	mustGit(t, dir, "add", "--force", ".env")
	mustGit(t, dir, "commit", "-m", "Oops")

	code, stdout, _ := run(t, "check")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, "exposed") || !strings.Contains(stdout, ".env") {
		t.Errorf("stdout =\n%s\nwant the exposed file reported", stdout)
	}
	if got := checkJSON(t)["plaintext"]; got != "failed" {
		t.Errorf("plaintext = %q, want failed", got)
	}
}

func TestCheckDetectsPlaintextInSubdirectory(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	if err := os.MkdirAll(filepath.Join(dir, "services", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeEnv(t, dir, filepath.Join("services", "api", ".env.production"), "SECRET=x\n")
	mustGit(t, dir, "add", "--force", filepath.Join("services", "api", ".env.production"))
	mustGit(t, dir, "commit", "-m", "Nested plaintext")

	code, stdout, _ := run(t, "check")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, ".env.production") {
		t.Errorf("stdout =\n%s\nwant the nested file reported", stdout)
	}
}

// A file that is merely unignored is one `git add .` from disaster.
func TestCheckWarnsAboutUnignoredPlaintext(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".gitignore", "# nothing ignored\n")
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, _ := run(t, "check")
	if code != 0 {
		t.Errorf("exit = %d, want 0: a warning is not a failure", code)
	}
	if !strings.Contains(stdout, "not ignored") {
		t.Errorf("stdout =\n%s\nwant a warning", stdout)
	}

	if code, _, _ := run(t, "check", "--strict"); code != 1 {
		t.Errorf("--strict: exit = %d, want 1", code)
	}
	if got := checkJSON(t, "--strict")["plaintext"]; got != "failed" {
		t.Errorf("--strict plaintext = %q, want failed", got)
	}
}

func TestCheckIgnoresEncryptedAndExampleFiles(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, ".env.example", "DATABASE_URL=\nAPI_KEY=\n")
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}
	mustGit(t, dir, "add", ".env.example", config.DefaultFile, config.Filename)
	mustGit(t, dir, "commit", "-m", "Commit the safe files")

	if code, stdout, _ := run(t, "check"); code != 0 {
		t.Errorf("exit = %d, want 0 (stdout: %s)", code, stdout)
	}
}

func TestCheckNeverPrintsValues(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, ".env.example", "DATABASE_URL=\n")
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}
	mustGit(t, dir, "add", "--force", ".env")
	mustGit(t, dir, "commit", "-m", "Oops")

	for _, args := range [][]string{{"check"}, {"check", "--json"}} {
		_, stdout, stderr := run(t, args...)
		for name, out := range map[string]string{"stdout": stdout, "stderr": stderr} {
			if strings.Contains(out, "s3cretvalue") || strings.Contains(out, "AGE-SECRET-KEY-") {
				t.Errorf("%v %s leaked a secret:\n%s", args, name, out)
			}
		}
	}
}

func TestCheckSkipsWhatItCannotRun(t *testing.T) {
	dir := project(t) // no git repository
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	status := checkJSON(t)
	if status["plaintext"] != "skipped" {
		t.Errorf("plaintext = %q, want skipped outside a repository", status["plaintext"])
	}
	if status["schema"] != "skipped" {
		t.Errorf("schema = %q, want skipped without an example file", status["schema"])
	}
	if code, _, _ := run(t, "check"); code != 0 {
		t.Errorf("exit = %d, want 0: skipped checks are not failures", code)
	}
}

func TestCheckWithoutIdentity(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	absent := filepath.Join(t.TempDir(), "identity")
	if got := checkJSON(t, "--identity", absent)["encrypted file"]; got != "skipped" {
		t.Errorf("encrypted file = %q, want skipped without an identity", got)
	}
}

func TestCheckWithoutConfiguration(t *testing.T) {
	project(t)

	code, stdout, _ := run(t, "check")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, config.Filename) {
		t.Errorf("stdout =\n%s\nwant it to name the missing configuration", stdout)
	}
}

func TestCheckCustomSchema(t *testing.T) {
	dir := gitProject(t)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, "env.schema", "DATABASE_URL=\nMISSING_ONE=\n")
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, _ := run(t, "check", "--schema", "env.schema")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, "MISSING_ONE") {
		t.Errorf("stdout =\n%s\nwant the missing variable", stdout)
	}
}
