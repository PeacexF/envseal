package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// teammate is a second clone of the same remote, with its own identity already
// authorized, so it can decrypt what the first clone pushes.
func teammate(t *testing.T, remote string) (otherDir, identityPath string) {
	t.Helper()

	otherDir = filepath.Join(t.TempDir(), "clone")
	mustGit(t, "", "clone", "--quiet", remote, otherDir)
	mustGit(t, otherDir, "config", "user.email", "other@example.com")
	mustGit(t, otherDir, "config", "user.name", "Other")

	identityPath = filepath.Join(t.TempDir(), "identity")
	code, stdout, stderr := run(t, "--identity", identityPath, "keys", "generate")
	if code != 0 {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}

	key := ""
	for field := range strings.FieldsSeq(stdout) {
		if strings.HasPrefix(field, "age1") {
			key = field
			break
		}
	}

	if code, _, stderr := run(t, "add", "teammate", key); code != 0 {
		t.Fatalf("add: exit = %d (stderr: %s)", code, stderr)
	}
	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("push: exit = %d (stderr: %s)", code, stderr)
	}

	t.Chdir(otherDir)
	return otherDir, identityPath
}

func TestPullFetchesAndDecrypts(t *testing.T) {
	_, remote := gitRepo(t)
	_, identityPath := teammate(t, remote)

	code, stdout, stderr := run(t, "--identity", identityPath, "pull")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Decrypted") {
		t.Errorf("stdout =\n%s\nwant a confirmation", stdout)
	}

	local, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf(".env: %v", err)
	}
	if string(local) != env {
		t.Errorf("decrypted =\n%q\nwant\n%q", local, env)
	}
}

// The summary names what changed without revealing any value.
func TestPullSummarizesChangesByNameOnly(t *testing.T) {
	dir, remote := gitRepo(t)
	other, identityPath := teammate(t, remote)

	if code, _, stderr := run(t, "--identity", identityPath, "pull"); code != 0 {
		t.Fatalf("first pull: exit = %d (stderr: %s)", code, stderr)
	}

	// Back in the first clone, change the environment and push it.
	t.Chdir(dir)
	writeEnv(t, dir, ".env", "DATABASE_URL=postgres://localhost/app\nAPI_KEY=rotatedvalue\nNEW_FLAG=1\n")
	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("push: exit = %d (stderr: %s)", code, stderr)
	}
	t.Chdir(other)

	code, stdout, stderr := run(t, "--identity", identityPath, "pull")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}

	for _, want := range []string{"+ NEW_FLAG", "~ API_KEY"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout =\n%s\nwant it to report %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "rotatedvalue") {
		t.Errorf("the summary leaked a value:\n%s", stdout)
	}
}

func TestPullWhenAlreadyUpToDate(t *testing.T) {
	_, remote := gitRepo(t)
	_, identityPath := teammate(t, remote)

	if code, _, stderr := run(t, "--identity", identityPath, "pull"); code != 0 {
		t.Fatalf("first pull: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "--identity", identityPath, "pull")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Already up to date") {
		t.Errorf("stdout =\n%s\nwant a no-op message", stdout)
	}
}

// Local edits are work in progress; pull must not silently discard them.
func TestPullRefusesToDiscardLocalEdits(t *testing.T) {
	_, remote := gitRepo(t)
	_, identityPath := teammate(t, remote)

	if code, _, stderr := run(t, "--identity", identityPath, "pull"); code != 0 {
		t.Fatalf("first pull: exit = %d (stderr: %s)", code, stderr)
	}

	// Edit locally, after the ciphertext was written.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(".env", []byte("API_KEY=my-local-experiment\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := run(t, "--identity", identityPath, "pull")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "local changes") {
		t.Errorf("stderr =\n%s\nwant it to report local changes", stderr)
	}

	kept, err := os.ReadFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kept), "my-local-experiment") {
		t.Error("the local edit was discarded")
	}
}

func TestPullForceDiscardsLocalEdits(t *testing.T) {
	_, remote := gitRepo(t)
	_, identityPath := teammate(t, remote)

	if code, _, stderr := run(t, "--identity", identityPath, "pull"); code != 0 {
		t.Fatalf("first pull: exit = %d (stderr: %s)", code, stderr)
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(".env", []byte("API_KEY=my-local-experiment\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := run(t, "--identity", identityPath, "pull", "--force"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}

	restored, err := os.ReadFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != env {
		t.Errorf("content =\n%q\nwant the shared environment", restored)
	}
}

func TestPullOutsideARepository(t *testing.T) {
	sealed(t)

	code, stdout, stderr := run(t, "pull")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Not a git repository") {
		t.Errorf("stdout =\n%s\nwant it to say so", stdout)
	}
	if _, err := os.Stat(".env"); err != nil {
		t.Errorf("it should still decrypt: %v", err)
	}
}

func TestPullNoGit(t *testing.T) {
	sealed(t)

	if code, _, stderr := run(t, "pull", "--no-git"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(".env"); err != nil {
		t.Errorf(".env: %v", err)
	}
}

func TestPullWrongIdentity(t *testing.T) {
	sealed(t)

	other := filepath.Join(t.TempDir(), "identity")
	if code, _, stderr := run(t, "--identity", other, "keys", "generate"); code != 0 {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}

	if code, _, _ := run(t, "--identity", other, "pull", "--no-git"); code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

// Pull must not touch the repository: no branch movement, no working-tree
// changes to anything, not even envseal's own files.
func TestPullLeavesTheRepositoryUntouched(t *testing.T) {
	dir, remote := gitRepo(t)
	other, identityPath := teammate(t, remote)

	// A teammate changes unrelated files and the environment upstream.
	t.Chdir(dir)
	writeEnv(t, dir, "app.txt", "v2\n")
	mustGit(t, dir, "add", "app.txt")
	mustGit(t, dir, "commit", "-m", "Unrelated upstream change")
	writeEnv(t, dir, ".env", env+"NEW_ONE=1\n")
	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("push: exit = %d (stderr: %s)", code, stderr)
	}

	t.Chdir(other)
	headBefore := mustGit(t, other, "rev-parse", "HEAD")
	statusBefore := mustGit(t, other, "status", "--porcelain")

	if code, stdout, stderr := run(t, "--identity", identityPath, "pull"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	} else if !strings.Contains(stdout, "+ NEW_ONE") {
		t.Errorf("stdout =\n%s\nwant the new variable", stdout)
	}

	if head := mustGit(t, other, "rev-parse", "HEAD"); head != headBefore {
		t.Error("pull moved the branch")
	}
	if status := mustGit(t, other, "status", "--porcelain"); status != statusBefore {
		t.Errorf("pull changed the working tree: %q, was %q", status, statusBefore)
	}
	if _, err := os.Stat(filepath.Join(other, "app.txt")); err == nil {
		t.Error("pull brought in an unrelated file")
	}
}

// Fetching from a ref works regardless of local state, unlike `git pull`.
func TestPullWorksWithADirtyWorkingTree(t *testing.T) {
	dir, remote := gitRepo(t)
	other, identityPath := teammate(t, remote)

	t.Chdir(dir)
	writeEnv(t, dir, ".env", env+"ANOTHER=1\n")
	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("push: exit = %d (stderr: %s)", code, stderr)
	}

	t.Chdir(other)
	writeEnv(t, other, "app.txt", "uncommitted work in progress\n")

	if code, _, stderr := run(t, "--identity", identityPath, "pull"); code != 0 {
		t.Fatalf("exit = %d, want 0 with a dirty tree (stderr: %s)", code, stderr)
	}
	if got := read(t, filepath.Join(other, "app.txt")); got != "uncommitted work in progress\n" {
		t.Errorf("app.txt = %q, want the local work preserved", got)
	}
}

// Taking the upstream copy must not silently undo an unpushed change.
func TestPullRefusesToDiscardUnpushedEnvironment(t *testing.T) {
	_, _ = gitRepo(t)

	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("push: exit = %d (stderr: %s)", code, stderr)
	}
	// Commit an environment change locally without pushing it.
	writeEnv(t, ".", ".env", env+"LOCAL_ONLY=1\n")
	if code, _, stderr := run(t, "push", "--yes", "--no-push"); code != 0 {
		t.Fatalf("push --no-push: exit = %d (stderr: %s)", code, stderr)
	}

	code, _, stderr := run(t, "pull")
	if code != 6 {
		t.Errorf("exit = %d, want 6", code)
	}
	if !strings.Contains(stderr, "unpushed") {
		t.Errorf("stderr =\n%s\nwant it to report unpushed changes", stderr)
	}
}
