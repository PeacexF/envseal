package cli

import (
	"encoding/json"
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
)

const diffLong = `Show which variables changed, by name.

  envseal diff                    your .env against the encrypted file
  envseal diff --ref origin/main  the encrypted file against another revision

Only names are printed:

  + STRIPE_WEBHOOK_SECRET   added
  ~ DATABASE_URL            value changed
  - OLD_API_KEY             removed

That makes an encrypted change reviewable in a pull request without exposing a
single value.`

func newDiffCmd(a *app) *cobra.Command {
	var (
		ref      string
		asJSON   bool
		exitCode bool
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show which variables changed, by name",
		Long:  diffLong,
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := a.workspace()
			if err != nil {
				return err
			}
			sealed := ws.encryptedPath()

			id, err := identity.Resolve(a.identityPath)
			if err != nil {
				return err
			}
			warn(cmd.ErrOrStderr(), id)

			before, after, err := a.diffSides(ws, sealed, ref, id)
			if err != nil {
				return err
			}

			delta := dotenv.Compare(before, after)

			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(delta); err != nil {
					return errs.New(errs.CodeGeneral, "unable to write the report").Wrap(err)
				}
			} else {
				renderDelta(out, delta)
			}

			if exitCode && !delta.Empty() {
				return errs.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "", "compare the encrypted file against this git revision")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable output")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit 1 when anything differs")
	return cmd
}

// diffSides decrypts the two environments being compared. Without --ref the
// comparison is the sealed file against your working .env, which answers "what
// have I changed that is not sealed yet".
func (a *app) diffSides(ws *workspace, sealed, ref string, id *identity.Identity) (before, after *dotenv.File, err error) {
	current, err := a.unseal(sealed, id)
	if err != nil {
		return nil, nil, err
	}

	if ref == "" {
		plain, err := dotenv.Load(defaultPlaintext(sealed))
		if err != nil {
			return nil, nil, err
		}
		return current, plain, nil
	}

	repo, err := git.Open(ws.Project.Root)
	if err != nil {
		return nil, nil, err
	}
	path, err := repo.Relative(sealed)
	if err != nil {
		return nil, nil, errs.New(errs.CodeGit, "%s is outside the repository", display(sealed))
	}
	blob, err := repo.Show(ref, path)
	if err != nil {
		// A rejected revision explains itself; only git's own failure needs
		// translating into something actionable.
		if typed, ok := errors.AsType[*errs.Error](err); ok {
			return nil, nil, typed
		}
		return nil, nil, errs.New(errs.CodeGit, "%s does not exist at %s", path, ref).
			Check("check the revision name", "run `git fetch` if it is a remote branch")
	}

	previous, err := a.decryptEnv(blob, id, ref+":"+path)
	if err != nil {
		return nil, nil, err
	}
	return previous, current, nil
}

// unseal reads and decrypts an encrypted environment file.
func (a *app) unseal(path string, id *identity.Identity) (*dotenv.File, error) {
	ciphertext, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.New(errs.CodeConfig, "no encrypted file at %s", display(path)).
			Check("run `envseal encrypt .env` to create one")
	}
	return a.decryptEnv(ciphertext, id, display(path))
}

func (a *app) decryptEnv(ciphertext []byte, id *identity.Identity, source string) (*dotenv.File, error) {
	plaintext, err := crypto.Decrypt(ciphertext, id.Identities(), source)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)

	return dotenv.Parse(plaintext, source)
}

// defaultPlaintext is the plaintext counterpart of an encrypted file.
func defaultPlaintext(sealed string) string {
	if plain, err := defaultOutput(sealed); err == nil {
		return plain
	}
	return ".env"
}

func renderDelta(w io.Writer, d dotenv.Delta) {
	if d.Empty() {
		fmt.Fprintln(w, "No changes.")
		return
	}

	for _, key := range d.Added {
		fmt.Fprintf(w, "+ %s\n", key)
	}
	for _, key := range d.Changed {
		fmt.Fprintf(w, "~ %s\n", key)
	}
	for _, key := range d.Removed {
		fmt.Fprintf(w, "- %s\n", key)
	}

	fmt.Fprintf(w, "\n%s\n", plural(d.Len(), "change"))
}
