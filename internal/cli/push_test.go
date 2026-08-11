package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo sets up an isolated home, an identity, and a working clone of a bare
// "remote", so push has somewhere real to send commits.
func gitRepo(t *testing.T) (dir, remote string) {
	t.Helper()
	isolate(t)

	remote = filepath.Join(t.TempDir(), "origin.git")
	mustGit(t, "", "init", "--bare", "--initial-branch=main", remote)

	dir = t.TempDir()
	mustGit(t, "", "clone", "--quiet", remote, dir)
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test")

	t.Chdir(dir)
	if code, _, stderr := run(t, "keys", "generate"); code != 0 {
		t.Fatalf("keys generate: exit = %d (stderr: %s)", code, stderr)
	}

	writeEnv(t, dir, ".gitignore", ".env\n")
	mustGit(t, dir, "add", ".gitignore")
	mustGit(t, dir, "commit", "-m", "Initial commit")
	mustGit(t, dir, "push", "--quiet", "-u", "origin", "main")

	writeEnv(t, dir, ".env", env)
	return dir, remote
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOutput(dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// remoteLog reads the history the remote actually received.
func remoteLog(t *testing.T, remote string) string {
	t.Helper()
	return mustGit(t, "", "--git-dir", remote, "log", "--oneline", "--name-only", "main")
}

func TestPush(t *testing.T) {
	dir, remote := gitRepo(t)

	code, stdout, stderr := run(t, "push", "--yes", "-m", "Add environment")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	for _, want := range []string{"Encrypted .env", "Committed", "Pushed to origin/main"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout =\n%s\nwant it to mention %q", stdout, want)
		}
	}

	log := remoteLog(t, remote)
	if !strings.Contains(log, "Add environment") {
		t.Errorf("remote log =\n%s\nwant the commit", log)
	}
	if !strings.Contains(log, ".env.enc") {
		t.Errorf("remote log =\n%s\nwant .env.enc", log)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.enc")); err != nil {
		t.Errorf("encrypted file: %v", err)
	}
}

// The plaintext must never reach the remote, whatever else happens.
func TestPushNeverSendsPlaintext(t *testing.T) {
	_, remote := gitRepo(t)

	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	log := remoteLog(t, remote)
	if strings.Contains(log, "\n.env\n") {
		t.Errorf("remote received the plaintext file:\n%s", log)
	}

	blob, err := gitOutput("", "--git-dir", remote, "show", "main:.env.enc")
	if err != nil {
		t.Fatalf("reading .env.enc from the remote: %v", err)
	}
	if strings.Contains(blob, "s3cretvalue") {
		t.Error("the pushed file contains plaintext")
	}
	if !strings.HasPrefix(blob, "-----BEGIN AGE ENCRYPTED FILE-----") {
		t.Errorf("the pushed file is not armored ciphertext:\n%s", blob)
	}
}

// The disaster case: plaintext already tracked. Refuse before doing anything.
func TestPushRefusesWhenPlaintextIsTracked(t *testing.T) {
	dir, _ := gitRepo(t)

	mustGit(t, dir, "add", "--force", ".env")
	mustGit(t, dir, "commit", "-m", "Oops")

	code, _, stderr := run(t, "push", "--yes")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	for _, want := range []string{"tracked by git", "git rm --cached", "rotate"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr =\n%s\nwant it to mention %q", stderr, want)
		}
	}
}

func TestPushWarnsWhenPlaintextIsNotIgnored(t *testing.T) {
	dir, _ := gitRepo(t)
	writeEnv(t, dir, ".gitignore", "# nothing ignored\n")

	code, _, stderr := run(t, "push", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "not ignored") {
		t.Errorf("stderr =\n%s\nwant a warning", stderr)
	}
}

// Someone's half-staged work must not be swept into a secrets commit.
func TestPushCommitsOnlyItsOwnFiles(t *testing.T) {
	dir, remote := gitRepo(t)

	writeEnv(t, dir, "unrelated.txt", "work in progress\n")
	mustGit(t, dir, "add", "unrelated.txt")

	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	if log := remoteLog(t, remote); strings.Contains(log, "unrelated.txt") {
		t.Errorf("push committed an unrelated staged file:\n%s", log)
	}
	if status := mustGit(t, dir, "status", "--porcelain"); !strings.Contains(status, "unrelated.txt") {
		t.Errorf("the staged file was lost: %q", status)
	}
}

func TestPushWithoutChanges(t *testing.T) {
	_, _ = gitRepo(t)

	if code, _, stderr := run(t, "push", "--yes"); code != 0 {
		t.Fatalf("first push: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, stderr := run(t, "push", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Nothing to commit") {
		t.Errorf("stdout =\n%s\nwant a no-op message", stdout)
	}
}

func TestPushNoPush(t *testing.T) {
	dir, remote := gitRepo(t)

	code, stdout, stderr := run(t, "push", "--yes", "--no-push")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Not pushed") {
		t.Errorf("stdout =\n%s\nwant it to say nothing was pushed", stdout)
	}

	if log := remoteLog(t, remote); strings.Contains(log, ".env.enc") {
		t.Error("--no-push pushed anyway")
	}
	if log := mustGit(t, dir, "log", "--oneline", "--name-only"); !strings.Contains(log, ".env.enc") {
		t.Error("--no-push did not commit locally")
	}
}

func TestPushNoCommit(t *testing.T) {
	dir, _ := gitRepo(t)

	if code, _, stderr := run(t, "push", "--no-commit"); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}

	status := mustGit(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "A  .env.enc") {
		t.Errorf("status = %q, want .env.enc staged and uncommitted", status)
	}
}

// Without a terminal and without --yes, refuse rather than assume consent.
func TestPushRefusesToAssumeConsent(t *testing.T) {
	_, remote := gitRepo(t)

	code, _, stderr := run(t, "push")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Errorf("stderr =\n%s\nwant it to mention --yes", stderr)
	}
	if log := remoteLog(t, remote); strings.Contains(log, ".env.enc") {
		t.Error("pushed without confirmation")
	}
}

func TestPushWithoutUpstream(t *testing.T) {
	dir, _ := gitRepo(t)
	mustGit(t, dir, "checkout", "--quiet", "-b", "feature")

	code, _, stderr := run(t, "push", "--yes")
	if code != 6 {
		t.Errorf("exit = %d, want 6", code)
	}
	if !strings.Contains(stderr, "git push -u origin feature") {
		t.Errorf("stderr =\n%s\nwant the exact command to run", stderr)
	}
}

func TestPushDetachedHead(t *testing.T) {
	dir, _ := gitRepo(t)
	mustGit(t, dir, "checkout", "--quiet", "--detach", "HEAD")

	code, _, stderr := run(t, "push", "--yes", "--no-push")
	if code != 6 {
		t.Errorf("exit = %d, want 6", code)
	}
	if !strings.Contains(stderr, "not on a branch") {
		t.Errorf("stderr =\n%s\nwant it to explain detached HEAD", stderr)
	}
}

func TestPushOutsideARepository(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	code, stdout, stderr := run(t, "push", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Not a git repository") {
		t.Errorf("stdout =\n%s\nwant it to say so", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.enc")); err != nil {
		t.Errorf("it should still encrypt: %v", err)
	}
}

// git pushes whole branches, so unrelated commits ride along. That must be
// disclosed at the confirmation rather than happening quietly.
func TestPushDisclosesOtherUnpushedCommits(t *testing.T) {
	dir, _ := gitRepo(t)

	writeEnv(t, dir, "app.txt", "v2\n")
	mustGit(t, dir, "add", "app.txt")
	mustGit(t, dir, "commit", "-m", "Unrelated work in progress")

	// Answering "n" both proves the prompt appeared and stops the push.
	code, stdout, _ := runInput(t, "n\n", "push")
	if code != 1 {
		t.Errorf("exit = %d, want 1 for a declined push", code)
	}
	for _, want := range []string{"1 other commit", "Unrelated work in progress"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("prompt =\n%s\nwant it to disclose %q", stdout, want)
		}
	}
}

func TestPushConfirmationAccepted(t *testing.T) {
	_, remote := gitRepo(t)

	code, stdout, stderr := runInput(t, "y\n", "push")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Will commit") {
		t.Errorf("stdout =\n%s\nwant the confirmation prompt", stdout)
	}
	if log := remoteLog(t, remote); !strings.Contains(log, ".env.enc") {
		t.Error("confirming did not push")
	}
}
