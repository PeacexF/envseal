package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
)

const (
	plaintextFileMode = 0o600 // secrets on disk stay owner-only

	decryptLong = `Decrypt an environment file back to plaintext.

Writing secrets to disk is the riskier path: prefer ` + "`envseal run`" + `, which keeps
the decrypted environment in memory and passes it to your program.

The output file is created with owner-only permissions and must not be
committed. An existing file is never overwritten without --force.`
)

func newDecryptCmd(a *app) *cobra.Command {
	var (
		output string
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "decrypt [file]",
		Short: "Decrypt an environment file",
		Long:  decryptLong,
		Args:  cobra.MaximumNArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.workspace()
			if err != nil {
				return err
			}

			source := ws.encryptedPath()
			if len(args) == 1 {
				source = args[0]
			}

			ciphertext, err := os.ReadFile(source)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return errs.New(errs.CodeConfig, "no encrypted file at %s", display(source)).
						Check("pass the path explicitly: `envseal decrypt <file>`",
							"run `envseal encrypt .env` to create one")
				}
				return errs.New(errs.CodeGeneral, "unable to read %s", display(source)).Wrap(err)
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

			if output == "-" {
				return writeToStdout(cmd, plaintext, force)
			}

			target := output
			if target == "" {
				if target, err = defaultOutput(source); err != nil {
					return err
				}
			}
			if err := refuseClobber(target, force); err != nil {
				return err
			}
			if err := safefile.Write(target, plaintext, plaintextFileMode); err != nil {
				return errs.New(errs.CodeGeneral, "unable to write %s", target).Wrap(err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Decrypted %s → %s\n", display(source), display(target))
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "write plaintext here, or - for standard output")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing file, or print to a terminal")
	return cmd
}

// defaultOutput drops the .enc suffix: .env.production.enc becomes
// .env.production.
func defaultOutput(source string) (string, error) {
	if trimmed := strings.TrimSuffix(source, ".enc"); trimmed != source && trimmed != "" {
		return trimmed, nil
	}
	return "", errs.New(errs.CodeGeneral, "cannot tell where to write the plaintext").
		Detailf("%s does not end in .enc, so there is no obvious output name.", display(source)).
		Check("pass -o <file>", "pass -o - to write to standard output")
}

func refuseClobber(target string, force bool) error {
	if _, err := os.Stat(target); err != nil || force {
		return nil
	}
	return errs.New(errs.CodeGeneral, "%s already exists", display(target)).
		Detailf("Overwriting it would discard whatever it currently holds.").
		Check("pass --force to overwrite it", "pass -o <file> to write somewhere else")
}

// writeToStdout guards against secrets landing in a terminal's scrollback,
// where they outlive the command in shell history and screen buffers.
func writeToStdout(cmd *cobra.Command, plaintext []byte, force bool) error {
	out := cmd.OutOrStdout()
	if isTerminal(out) && !force {
		return errs.New(errs.CodeGeneral, "refusing to print secrets to a terminal").
			Detailf("They would remain visible in your scrollback.").
			Check("redirect the output, as in `envseal decrypt -o - > .env`",
				"pass --force to print them anyway")
	}

	if _, err := out.Write(plaintext); err != nil {
		return errs.New(errs.CodeGeneral, "unable to write the plaintext").Wrap(err)
	}
	return nil
}
