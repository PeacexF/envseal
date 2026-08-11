// Package git drives the user's own git binary.
//
// Shelling out rather than embedding a Git library is deliberate: the user's
// git already knows their credentials, SSH agent, signing keys, hooks, and
// proxy settings. Reimplementing that would be a large surface to get subtly
// wrong, and secrets are involved.
//
// Nothing in this package ever passes a secret to git. Only paths, branch
// names, and commit messages.
package git

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/PeacexF/envseal/internal/errs"
)

// ErrNotRepo reports that a directory is not inside a git repository. It is not
// fatal on its own: commands fall back to plain encryption.
var ErrNotRepo = errors.New("not a git repository")

type Repo struct {
	Root string
}

// Open finds the repository containing dir.
func Open(dir string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, errs.New(errs.CodeGit, "git is not installed").
			Detailf("push and pull drive git directly.").
			Check("install git", "use `envseal encrypt` and `envseal decrypt` instead").
			Wrap(ErrNotRepo)
	}

	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, errs.New(errs.CodeGit, "not a git repository").
			Detailf("push and pull synchronize through git.").
			Check("run this inside a repository",
				"use `envseal encrypt` and `envseal decrypt` instead").
			Wrap(ErrNotRepo)
	}
	return &Repo{Root: out}, nil
}

// IsTracked reports whether git has the file in its index. A tracked plaintext
// environment file is the failure this whole tool exists to prevent.
func (r *Repo) IsTracked(path string) bool {
	_, err := run(r.Root, "ls-files", "--error-unmatch", "--", path)
	return err == nil
}

func (r *Repo) IsIgnored(path string) bool {
	_, err := run(r.Root, "check-ignore", "-q", "--", path)
	return err == nil
}

// Changed reports whether any of paths differ from the index or HEAD.
func (r *Repo) Changed(paths ...string) (bool, error) {
	args := append([]string{"status", "--porcelain", "--"}, paths...)
	out, err := run(r.Root, args...)
	if err != nil {
		return false, gitError("check the working tree", err)
	}
	return out != "", nil
}

// Conflicted reports whether any of paths are in a merge conflict.
func (r *Repo) Conflicted(paths ...string) (bool, error) {
	args := append([]string{"status", "--porcelain", "--"}, paths...)
	out, err := run(r.Root, args...)
	if err != nil {
		return false, gitError("check the working tree", err)
	}
	for line := range strings.SplitSeq(out, "\n") {
		if len(line) >= 2 && isConflictStatus(line[:2]) {
			return true, nil
		}
	}
	return false, nil
}

// isConflictStatus recognises git's unmerged porcelain codes.
func isConflictStatus(code string) bool {
	switch code {
	case "UU", "AA", "DD", "AU", "UA", "DU", "UD":
		return true
	}
	return false
}

// Branch returns the checked-out branch, or an error when HEAD is detached.
func (r *Repo) Branch() (string, error) {
	out, err := run(r.Root, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", errs.New(errs.CodeGit, "HEAD is not on a branch").
			Detailf("Committing from a detached HEAD would leave the change unreachable.").
			Check("switch to a branch with `git switch <branch>`")
	}
	return out, nil
}

// Upstream returns the tracking branch, such as origin/main.
func (r *Repo) Upstream() (string, error) {
	out, err := run(r.Root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		branch, berr := r.Branch()
		if berr != nil {
			return "", berr
		}
		return "", errs.New(errs.CodeGit, "%s has no upstream branch", branch).
			Detailf("There is nowhere to push to yet.").
			Check("run `git push -u origin "+branch+"` once",
				"or pass --no-push to commit without pushing")
	}
	return out, nil
}

func (r *Repo) Add(paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	if _, err := run(r.Root, args...); err != nil {
		return gitError("stage the changes", err)
	}
	return nil
}

// Commit records only the given paths, so unrelated staged work is never swept
// into a commit the user did not intend to make.
func (r *Repo) Commit(message string, paths ...string) error {
	args := append([]string{"commit", "-m", message, "--"}, paths...)
	if _, err := run(r.Root, args...); err != nil {
		return gitError("create the commit", err)
	}
	return nil
}

// Push streams git's own output, so credential prompts and progress reach the
// user unchanged.
func (r *Repo) Push(stdout, stderr io.Writer) error {
	if err := stream(r.Root, stdout, stderr, "push"); err != nil {
		return errs.New(errs.CodeGit, "unable to push").
			Detailf("The commit was created locally; only the push failed.").
			Check("check your network and credentials",
				"run `git push` yourself once the problem is fixed")
	}
	return nil
}

// Fetch updates remote-tracking refs without touching the working tree or the
// current branch. Reading the shared environment must not move someone's work.
func (r *Repo) Fetch(stdout, stderr io.Writer) error {
	if err := stream(r.Root, stdout, stderr, "fetch"); err != nil {
		return errs.New(errs.CodeGit, "unable to fetch").
			Check("check your network and credentials",
				"pass --no-git to decrypt the file you already have")
	}
	return nil
}

// Show reads a file's contents at a revision, without checking anything out.
// path must be relative to the repository root.
func (r *Repo) Show(ref, path string) ([]byte, error) {
	cmd := exec.Command("git", "show", ref+":"+path)
	cmd.Dir = r.Root

	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut

	if err := cmd.Run(); err != nil {
		return nil, errors.New(strings.TrimSpace(errOut.String()))
	}
	return out.Bytes(), nil
}

// Relative expresses an absolute path the way git refers to it.
//
// Symlinks are resolved on both sides first: git reports its root through the
// real filesystem, while a working directory may be reached through a link —
// on macOS /var is /private/var — and comparing the two directly yields a
// path full of "..".
func (r *Repo) Relative(path string) (string, error) {
	root := resolve(r.Root)

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// The file itself may not exist yet, so resolve its directory.
	abs = filepath.Join(resolve(filepath.Dir(abs)), filepath.Base(abs))

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path is outside the repository")
	}
	return filepath.ToSlash(rel), nil
}

func resolve(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// Tracked lists every file in the index, relative to the repository root.
func (r *Repo) Tracked() ([]string, error) {
	out, err := run(r.Root, "ls-files")
	if err != nil {
		return nil, gitError("list tracked files", err)
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// Unpushed lists the commits on the current branch that the upstream lacks.
func (r *Repo) Unpushed(upstream string, paths ...string) ([]string, error) {
	args := append([]string{"log", "--oneline", "--no-decorate", upstream + "..HEAD", "--"}, paths...)
	out, err := run(r.Root, args...)
	if err != nil {
		return nil, gitError("inspect unpushed commits", err)
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut

	if err := cmd.Run(); err != nil {
		return "", errors.New(strings.TrimSpace(errOut.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func stream(dir string, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

func gitError(action string, err error) error {
	return errs.New(errs.CodeGit, "unable to %s", action).Wrap(err)
}
