package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiffInSync(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "diff")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "No changes") {
		t.Errorf("stdout =\n%s\nwant it to report no changes", stdout)
	}
}

func TestDiffReportsChanges(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	// DATABASE_URL kept, API_KEY changed, DEBUG added.
	writeEnv(t, dir, ".env", "DATABASE_URL=postgres://localhost/app\nAPI_KEY=rotated\nDEBUG=true\n")

	code, stdout, stderr := run(t, "diff")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	for _, want := range []string{"+ DEBUG", "~ API_KEY", "2 changes"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout =\n%s\nwant %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "DATABASE_URL") {
		t.Errorf("stdout =\n%s\nwant unchanged variables omitted", stdout)
	}
}

func TestDiffReportsRemoval(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}
	writeEnv(t, dir, ".env", "DATABASE_URL=postgres://localhost/app\n")

	_, stdout, _ := run(t, "diff")
	if !strings.Contains(stdout, "- API_KEY") {
		t.Errorf("stdout =\n%s\nwant the removal", stdout)
	}
}

// The whole point: a change is reviewable without being readable.
func TestDiffNeverPrintsValues(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}
	writeEnv(t, dir, ".env", "DATABASE_URL=postgres://localhost/app\nAPI_KEY=rotatedsecret\n")

	for _, args := range [][]string{{"diff"}, {"diff", "--json"}} {
		_, stdout, stderr := run(t, args...)
		for name, out := range map[string]string{"stdout": stdout, "stderr": stderr} {
			for _, secret := range []string{"s3cretvalue", "rotatedsecret", "postgres://"} {
				if strings.Contains(out, secret) {
					t.Errorf("%v %s leaked %q:\n%s", args, name, secret, out)
				}
			}
		}
	}
}

func TestDiffJSON(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}
	writeEnv(t, dir, ".env", "DATABASE_URL=postgres://localhost/app\nDEBUG=true\n")

	_, stdout, _ := run(t, "diff", "--json")

	var delta struct {
		Added   []string `json:"added"`
		Changed []string `json:"changed"`
		Removed []string `json:"removed"`
	}
	if err := json.Unmarshal([]byte(stdout), &delta); err != nil {
		t.Fatalf("json.Unmarshal(%q) = %v", stdout, err)
	}
	if len(delta.Added) != 1 || delta.Added[0] != "DEBUG" {
		t.Errorf("added = %v, want [DEBUG]", delta.Added)
	}
	if len(delta.Removed) != 1 || delta.Removed[0] != "API_KEY" {
		t.Errorf("removed = %v, want [API_KEY]", delta.Removed)
	}
	if delta.Changed == nil {
		t.Error("changed = null, want an empty list")
	}
}

func TestDiffExitCode(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	if code, _, _ := run(t, "diff", "--exit-code"); code != 0 {
		t.Errorf("in sync: exit = %d, want 0", code)
	}

	writeEnv(t, dir, ".env", env+"EXTRA=1\n")
	if code, _, _ := run(t, "diff", "--exit-code"); code != 1 {
		t.Errorf("out of sync: exit = %d, want 1", code)
	}
}

func TestDiffAgainstGitRevision(t *testing.T) {
	dir, _ := gitRepo(t)
	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("push: exit = %d (stderr: %s)", code, stderr)
	}

	// Seal a change without committing it.
	writeEnv(t, dir, ".env", env+"ADDED_LATER=1\n")
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "diff", "--ref", "HEAD")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "+ ADDED_LATER") {
		t.Errorf("stdout =\n%s\nwant the added variable", stdout)
	}
}

func TestDiffUnknownRevision(t *testing.T) {
	_, _ = gitRepo(t)
	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("push: exit = %d (stderr: %s)", code, stderr)
	}

	code, _, stderr := run(t, "diff", "--ref", "no-such-branch")
	if code != 6 {
		t.Errorf("exit = %d, want 6", code)
	}
	if !strings.Contains(stderr, "no-such-branch") {
		t.Errorf("stderr =\n%s\nwant it to name the revision", stderr)
	}
}

func TestDiffWithoutPlaintext(t *testing.T) {
	sealed(t) // encrypts, then removes .env

	code, _, stderr := run(t, "diff")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, ".env") {
		t.Errorf("stderr =\n%s\nwant it to name the missing file", stderr)
	}
}

func TestDiffWrongIdentity(t *testing.T) {
	dir := sealed(t)
	writeEnv(t, dir, ".env", env)

	other := newIdentityOnly(t)
	if code, _, _ := run(t, "--identity", other, "diff"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}
