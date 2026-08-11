package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/git"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
)

const (
	defaultMessage = "Update environment"

	pushLong = `Encrypt the environment and share it through Git.

This is the everyday way to publish a change:

  .env → .env.enc → git commit → git push

The plaintext file is never committed, never staged, and never leaves your
machine. Only the encrypted file and the recipient list are pushed.

Envseal shows what it is about to do and asks before committing. Only the files
it manages are committed, so unrelated staged work is left alone.`
)

func newPushCmd(a *app) *cobra.Command {
	var (
		message   string
		assumeYes bool
		noPush    bool
		noCommit  bool
	)

	cmd := &cobra.Command{
		Use:   "push [file]",
		Short: "Encrypt, commit, and push the environment",
		Long:  pushLong,
		Args:  cobra.MaximumNArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.workspace()
			if err != nil {
				return err
			}

			source := ".env"
			if len(args) == 1 {
				source = args[0]
			}

			env, err := dotenv.Load(source)
			if err != nil {
				return err
			}

			repo, err := git.Open(ws.Project.Root)
			if err != nil && !errors.Is(err, git.ErrNotRepo) {
				return err
			}

			out := a.stdout(cmd)

			// Refuse before encrypting: if the plaintext is already tracked,
			// nothing else matters until that is fixed.
			if repo != nil {
				if err := guardPlaintext(cmd, repo, source); err != nil {
					return err
				}
			}

			chosen, created, err := resolveRecipients(a, ws, nil)
			if err != nil {
				return err
			}
			keys, err := config.ParseRecipients(chosen)
			if err != nil {
				return err
			}

			target := ws.encryptedPath()

			if created {
				fmt.Fprintf(out, "Created %s with your identity as the only recipient.\n", display(ws.configPath()))
			}

			if a.sealedMatches(target, env.Bytes()) && !created && !recipientsPending(repo, ws) {
				fmt.Fprintf(out, "Environment unchanged; %s left as it is.\n", display(target))
			} else {
				ciphertext, err := crypto.Encrypt(env.Bytes(), keys)
				if err != nil {
					return err
				}
				if err := safefile.Write(target, ciphertext, publicFileMode); err != nil {
					return errs.New(errs.CodeGeneral, "unable to write %s", display(target)).Wrap(err)
				}
				fmt.Fprintf(out, "Encrypted %s → %s\n", source, display(target))
			}

			if repo == nil {
				fmt.Fprint(out, "\nNot a git repository, so nothing was committed.\n")
				return nil
			}
			if noCommit {
				if err := repo.Add(target, ws.configPath()); err != nil {
					return err
				}
				fmt.Fprint(out, "Staged. Commit when you are ready.\n")
				return nil
			}

			return publish(cmd, a, repo, ws, publishOptions{
				message:   message,
				assumeYes: assumeYes,
				noPush:    noPush,
			})
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", defaultMessage, "commit message")
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&noPush, "no-push", false, "commit without pushing")
	cmd.Flags().BoolVar(&noCommit, "no-commit", false, "encrypt and stage without committing")
	return cmd
}

type publishOptions struct {
	message   string
	assumeYes bool
	noPush    bool
}

func publish(cmd *cobra.Command, a *app, repo *git.Repo, ws *workspace, opts publishOptions) error {
	out := a.stdout(cmd)
	paths := []string{ws.encryptedPath(), ws.configPath()}

	changed, err := repo.Changed(paths...)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Fprint(out, "\nNothing to commit: the encrypted environment is unchanged.\n")
		return nil
	}

	conflicted, err := repo.Conflicted(paths...)
	if err != nil {
		return err
	}
	if conflicted {
		return errs.New(errs.CodeGit, "%s is in a merge conflict", display(ws.encryptedPath())).
			Detailf("Encrypted files cannot be merged line by line: every re-encryption changes the whole file.").
			Check("pick one side with `git checkout --ours` or `--theirs`",
				"decrypt it, re-apply the change, and encrypt again")
	}

	upstream := ""
	if !opts.noPush {
		if upstream, err = repo.Upstream(); err != nil {
			return err
		}
	} else if _, err := repo.Branch(); err != nil {
		return err
	}

	question := fmt.Sprintf("Will commit %s.", display(ws.encryptedPath()))
	if !opts.noPush {
		question = fmt.Sprintf("Will commit %s and push to %s.", display(ws.encryptedPath()), upstream)

		// Git pushes whole branches, so anything else waiting on this one goes
		// too. Say so rather than letting it happen quietly.
		if others, err := repo.Unpushed(upstream); err == nil && len(others) > 0 {
			question += fmt.Sprintf("\n\nThis also pushes %s already on this branch:", plural(len(others), "other commit"))
			for _, commit := range others {
				question += "\n  " + commit
			}
		}
	}

	if err := a.confirm(cmd, opts.assumeYes, question); err != nil {
		return err
	}

	if err := repo.Add(paths...); err != nil {
		return err
	}
	if err := repo.Commit(opts.message, paths...); err != nil {
		return err
	}
	fmt.Fprintf(out, "Committed %q\n", opts.message)

	if opts.noPush {
		fmt.Fprint(out, "Not pushed. Run `git push` when you are ready.\n")
		return nil
	}

	if err := repo.Push(out, cmd.ErrOrStderr()); err != nil {
		return err
	}
	fmt.Fprintf(out, "Pushed to %s\n", upstream)
	return nil
}

// plural renders a count with its noun, so messages read naturally.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// sealedMatches reports whether the existing ciphertext already holds exactly
// this plaintext.
//
// age randomizes every encryption, so re-encrypting unchanged input still
// produces a different file. Without this check, every push would commit a
// whole-file change that says nothing — churn in every pull request. Answering
// it requires decrypting, so a user who is not a recipient simply re-encrypts.
func (a *app) sealedMatches(target string, plaintext []byte) bool {
	existing, err := os.ReadFile(target)
	if err != nil {
		return false
	}

	id, err := identity.Resolve(a.identityPath)
	if err != nil {
		return false
	}

	current, err := crypto.Decrypt(existing, id.Identities(), target)
	if err != nil {
		return false
	}
	defer clear(current)

	return bytes.Equal(current, plaintext)
}

// recipientsPending reports an uncommitted change to the recipient list, which
// must be re-encrypted even when the environment itself is unchanged.
func recipientsPending(repo *git.Repo, ws *workspace) bool {
	if repo == nil {
		return false
	}
	changed, err := repo.Changed(ws.configPath())
	return err == nil && changed
}

// guardPlaintext refuses to continue while the plaintext environment is exposed
// to Git. Encrypting is pointless if the unencrypted file is already committed.
func guardPlaintext(cmd *cobra.Command, repo *git.Repo, source string) error {
	absolute, err := filepath.Abs(source)
	if err != nil {
		absolute = source
	}

	if repo.IsTracked(absolute) {
		return errs.New(errs.CodeGeneral, "%s is tracked by git", source).
			Detailf("Pushing would publish your secrets in plaintext. Encrypting is pointless until this is fixed.").
			Check("run `git rm --cached "+source+"` to untrack it",
				"add "+source+" to .gitignore",
				"treat the values as exposed and rotate them: they are in the repository history")
	}

	if !repo.IsIgnored(absolute) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: %s is not ignored by git. Add it to .gitignore before someone commits it.\n", source)
	}
	return nil
}
