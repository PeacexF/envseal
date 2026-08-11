package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
)

const (
	publicFileMode = 0o644 // meant to be committed: ciphertext, config, examples

	encryptLong = `Encrypt an environment file for the project's recipients.

The result is ASCII-armored age ciphertext, so it diffs and merges in Git like
any other text file. It is safe to commit; only an authorized private identity
can read it.

With no recipients configured, envseal creates .envseal.yaml listing your own
public key, so you can decrypt what you just encrypted.`
)

func newEncryptCmd(a *app) *cobra.Command {
	var (
		output     string
		recipients []string
	)

	cmd := &cobra.Command{
		Use:   "encrypt [file]",
		Short: "Encrypt an environment file",
		Long:  encryptLong,
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

			chosen, created, err := resolveRecipients(a, ws, recipients)
			if err != nil {
				return err
			}

			keys, err := config.ParseRecipients(chosen)
			if err != nil {
				return err
			}

			ciphertext, err := crypto.Encrypt(env.Bytes(), keys)
			if err != nil {
				return err
			}

			target := output
			if target == "" {
				target = ws.encryptedPath()
			}
			if err := safefile.Write(target, ciphertext, publicFileMode); err != nil {
				return errs.New(errs.CodeGeneral, "unable to write %s", target).Wrap(err)
			}

			out := a.stdout(cmd)
			if created {
				fmt.Fprintf(out, "Created %s with your identity as the only recipient.\n\n", display(ws.configPath()))
			}
			fmt.Fprintf(out, "Encrypted %s → %s\n", source, display(target))
			fmt.Fprintf(out, "Recipients: %s\n", strings.Join(names(chosen), ", "))
			if created {
				fmt.Fprintf(out, "\nCommit %s and %s. Keep %s out of Git.\n",
					display(target), config.Filename, source)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "write the encrypted file here (default from .envseal.yaml)")
	cmd.Flags().StringArrayVar(&recipients, "recipient", nil, "public key to encrypt for (repeatable, overrides .envseal.yaml)")
	return cmd
}

// resolveRecipients picks who the file is encrypted for: explicit --recipient
// flags, then the project configuration, then the local identity for a project
// that has none yet. The last case writes .envseal.yaml and reports created.
func resolveRecipients(a *app, ws *workspace, flags []string) (chosen []config.Recipient, created bool, err error) {
	if len(flags) > 0 {
		for _, key := range flags {
			if err := config.ValidateKey(key); err != nil {
				return nil, false, errs.New(errs.CodeConfig, "invalid --recipient").
					Detailf("%s.", err).
					Check("public keys look like age1... and are printed by `envseal keys public`")
			}
			chosen = append(chosen, config.Recipient{Name: key, Key: key})
		}
		return chosen, false, nil
	}

	if ws.Config != nil {
		if len(ws.Config.Recipients) == 0 {
			return nil, false, errs.New(errs.CodeConfig, "no recipients in %s", config.Filename).
				Detailf("Encrypting for nobody would produce a file no one can read.").
				Check("run `envseal add <name> <public key>`",
					"pass --recipient age1... for a one-off")
		}
		return ws.Config.Recipients, false, nil
	}

	id, err := identity.Resolve(a.identityPath)
	if err != nil {
		return nil, false, err
	}

	chosen = []config.Recipient{{Name: selfName(), Key: id.PublicKey()}}

	c := config.New()
	c.Recipients = chosen
	if err := c.Save(ws.configPath()); err != nil {
		return nil, false, err
	}
	ws.Config = c
	return chosen, true, nil
}

func names(recipients []config.Recipient) []string {
	out := make([]string, len(recipients))
	for i, r := range recipients {
		out[i] = r.Name
	}
	return out
}
