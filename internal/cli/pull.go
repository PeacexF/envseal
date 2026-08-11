package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/git"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
)

const pullLong = `Fetch the shared environment and decrypt it locally.

  git pull → .env.enc → .env

Envseal summarizes which variables changed by name, never by value, so you can
see what a teammate altered without exposing anything.

A local .env you have edited since the last sync is not overwritten without
--force.`

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

			if !noGit {
				repo, err := git.Open(ws.Project.Root)
				switch {
				case errors.Is(err, git.ErrNotRepo):
					fmt.Fprint(out, "Not a git repository, so nothing was fetched.\n")
				case err != nil:
					return err
				default:
					if err := repo.Pull(out, cmd.ErrOrStderr()); err != nil {
						return err
					}
					// Reload: the pull may have changed the recipient list.
					if ws, err = a.workspace(); err != nil {
						return err
					}
				}
			}

			source := ws.encryptedPath()
			if len(args) == 1 {
				source = args[0]
			}
			target, err := defaultOutput(source)
			if err != nil {
				return err
			}

			ciphertext, err := os.ReadFile(source)
			if err != nil {
				return errs.New(errs.CodeConfig, "no encrypted file at %s", display(source)).
					Check("run `envseal push` to create one")
			}

			id, err := identity.Resolve(a.identityPath)
			if err != nil {
				return err
			}
			warn(cmd.ErrOrStderr(), id)

			plaintext, err := crypto.Decrypt(ciphertext, id.Identities(), display(source))
			if err != nil {
				return err
			}
			defer clear(plaintext)

			current, hadLocal := readLocal(target)
			if hadLocal {
				if slices.Equal(current, plaintext) {
					fmt.Fprintf(out, "Already up to date: %s matches %s\n", display(target), display(source))
					return nil
				}
				if err := guardLocalEdits(target, source, force); err != nil {
					return err
				}
			}

			if err := safefile.Write(target, plaintext, plaintextFileMode); err != nil {
				return errs.New(errs.CodeGeneral, "unable to write %s", display(target)).Wrap(err)
			}

			fmt.Fprintf(out, "Decrypted %s → %s\n", display(source), display(target))
			summarize(out, current, plaintext, display(source))
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite a locally modified environment file")
	cmd.Flags().BoolVar(&noGit, "no-git", false, "decrypt without fetching first")
	return cmd
}

func readLocal(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	return data, err == nil
}

// guardLocalEdits refuses to discard local work. Whether the file was edited is
// judged by modification time, which is a heuristic: git refreshes .env.enc when
// it pulls, so a .env that is newer than the ciphertext was almost certainly
// changed by hand afterwards.
func guardLocalEdits(target, source string, force bool) error {
	if force {
		return nil
	}

	local, err := os.Stat(target)
	if err != nil {
		return nil
	}
	sealed, err := os.Stat(source)
	if err != nil || !local.ModTime().After(sealed.ModTime()) {
		return nil
	}

	return errs.New(errs.CodeGeneral, "%s has local changes", display(target)).
		Detailf("It was modified after %s, so overwriting it would discard your edits.", display(source)).
		Check("run `envseal push` to share your changes first",
			"pass --force to discard them")
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
