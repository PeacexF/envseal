package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/git"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
	"github.com/PeacexF/envseal/internal/syncstate"
)

const pullLong = `Fetch the shared environment and decrypt it locally.

Only your .env is written. Envseal fetches and reads the encrypted file straight
out of the upstream branch, so your own branch, working tree, and staged changes
are left exactly as they were — this works even with uncommitted work in
progress. Run git pull yourself when you want the rest of the repository.

Changed variables are summarized by name, never by value.`

func newPullCmd(a *app) *cobra.Command {
	var (
		force bool
		noGit bool
	)

	cmd := &cobra.Command{
		Use:   "pull [file]",
		Short: "Fetch and decrypt the shared environment",
		Long:  pullLong,
		Args:  cobra.MaximumNArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.workspace()
			if err != nil {
				return err
			}
			out := a.stdout(cmd)

			source := ws.encryptedPath()
			if len(args) == 1 {
				source = args[0]
			}
			target, err := defaultOutput(source)
			if err != nil {
				return err
			}

			ciphertext, origin, err := a.fetchSealed(cmd, ws, source, noGit, force)
			if err != nil {
				return err
			}

			id, err := identity.Resolve(a.identityPath)
			if err != nil {
				return err
			}
			warn(cmd.ErrOrStderr(), id)

			plaintext, err := crypto.Decrypt(ciphertext, id.Identities(), origin)
			if err != nil {
				return err
			}
			defer clear(plaintext)

			current, hadLocal := readLocal(target)
			if hadLocal {
				if bytes.Equal(current, plaintext) {
					fmt.Fprintf(out, "Already up to date: %s matches %s\n", display(target), origin)
					return nil
				}
				if err := a.guardLocalEdits(target, source, current, force); err != nil {
					return err
				}
			}

			if err := safefile.Write(target, plaintext, plaintextFileMode); err != nil {
				return errs.New(errs.CodeGeneral, "unable to write %s", display(target)).Wrap(err)
			}

			syncstate.Record(target, plaintext)

			fmt.Fprintf(out, "Decrypted %s → %s\n", origin, display(target))
			summarize(out, current, plaintext, origin)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite a locally modified environment file")
	cmd.Flags().BoolVar(&noGit, "no-git", false, "decrypt the local file without fetching")
	return cmd
}

// fetchSealed returns the ciphertext to decrypt and a label describing where it
// came from. It reads from the upstream branch without checking anything out,
// so nothing in the repository is modified.
func (a *app) fetchSealed(cmd *cobra.Command, ws *workspace, source string, noGit, force bool) ([]byte, string, error) {
	local := func() ([]byte, string, error) {
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, "", errs.New(errs.CodeConfig, "no encrypted file at %s", display(source)).
				Check("run `envseal push` to create one")
		}
		return data, display(source), nil
	}

	if noGit {
		return local()
	}

	out := a.stdout(cmd)

	repo, err := git.Open(ws.Project.Root)
	if errors.Is(err, git.ErrNotRepo) {
		fmt.Fprint(out, "Not a git repository, so nothing was fetched.\n")
		return local()
	}
	if err != nil {
		return nil, "", err
	}

	if err := repo.Fetch(out, cmd.ErrOrStderr()); err != nil {
		return nil, "", err
	}

	upstream, err := repo.Upstream()
	if err != nil {
		fmt.Fprint(out, "No upstream branch, so the local file was used.\n")
		return local()
	}

	path, err := repo.Relative(source)
	if err != nil {
		return local()
	}

	sealed, err := repo.Show(upstream, path)
	if err != nil {
		fmt.Fprintf(out, "%s does not exist on %s yet, so the local file was used.\n", path, upstream)
		return local()
	}

	// Taking the upstream copy would undo an environment change you have
	// committed but not yet pushed.
	if !force {
		unpushed, err := repo.Unpushed(upstream, source)
		if err == nil && len(unpushed) > 0 {
			return nil, "", errs.New(errs.CodeGit, "you have unpushed changes to %s", path).
				Detailf("Taking the copy from %s would discard them.", upstream).
				Check("run `envseal push` to share your version first",
					"pass --force to take the upstream version anyway")
		}
	}

	return sealed, upstream + ":" + path, nil
}

func readLocal(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	return data, err == nil
}

// guardLocalEdits refuses to discard hand edits.
//
// A file is safe to replace when it is exactly what envseal last wrote: either
// according to the recorded sync, or because it still matches what the local
// encrypted file decrypts to. Anything else has been edited since.
func (a *app) guardLocalEdits(target, sealed string, current []byte, force bool) error {
	if force || syncstate.Matches(target, current) || a.matchesSealed(sealed, current) {
		return nil
	}

	return errs.New(errs.CodeGeneral, "%s has local changes", display(target)).
		Detailf("It is not what envseal last wrote, so overwriting it would discard your edits.").
		Check("run `envseal push` to share your changes first",
			"pass --force to discard them")
}

// matchesSealed reports whether content is what the local encrypted file holds.
func (a *app) matchesSealed(sealed string, content []byte) bool {
	existing, err := os.ReadFile(sealed)
	if err != nil {
		return false
	}
	id, err := identity.Resolve(a.identityPath)
	if err != nil {
		return false
	}
	previous, err := crypto.Decrypt(existing, id.Identities(), sealed)
	if err != nil {
		return false
	}
	defer clear(previous)

	return bytes.Equal(previous, content)
}

// summarize reports which variables changed, by name only. Values are compared
// but never shown: the point is to make a change reviewable without exposing it.
func summarize(w io.Writer, before, after []byte, source string) {
	old, err := dotenv.Parse(before, source)
	if err != nil {
		return
	}
	updated, err := dotenv.Parse(after, source)
	if err != nil {
		return
	}

	oldValues, newValues := old.Map(), updated.Map()

	var added, changed, removed []string
	for _, key := range updated.Keys() {
		previous, existed := oldValues[key]
		switch {
		case !existed:
			added = append(added, key)
		case previous != newValues[key]:
			changed = append(changed, key)
		}
	}
	for _, key := range old.Keys() {
		if _, still := newValues[key]; !still {
			removed = append(removed, key)
		}
	}

	if len(added)+len(changed)+len(removed) == 0 {
		return
	}

	fmt.Fprintln(w)
	for _, key := range added {
		fmt.Fprintf(w, "+ %s\n", key)
	}
	for _, key := range changed {
		fmt.Fprintf(w, "~ %s\n", key)
	}
	for _, key := range removed {
		fmt.Fprintf(w, "- %s\n", key)
	}
}
