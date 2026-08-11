package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
)

const (
	addLong = `Authorize a public key to decrypt this project.

Adding a recipient changes the configuration only. The encrypted file still
holds the old recipient list until you re-encrypt it with ` + "`envseal rotate`" + `.`

	removeLong = `Withdraw a recipient's access.

The encrypted file is unchanged until you run ` + "`envseal rotate`" + `. Anyone who
already has a copy of the old file can still read it with their key, so treat
any secret they held as compromised and rotate its value at the source.`

	rotateLong = `Re-encrypt the environment for the current recipient list.

Run this after adding or removing a recipient. Rotation decrypts with your
identity and re-encrypts in memory; the plaintext never touches the disk.`
)

func newAddCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <public key>",
		Short: "Authorize a recipient",
		Long:  addLong,
		Args:  cobra.ExactArgs(2),

		RunE: func(cmd *cobra.Command, args []string) error {
			name, key := args[0], args[1]

			if err := config.ValidateKey(key); err != nil {
				return errs.New(errs.CodeConfig, "invalid public key for %q", name).
					Detailf("%s.", err).
					Check("public keys look like age1... and are printed by `envseal init`")
			}

			ws, err := a.workspace()
			if err != nil {
				return err
			}

			c := ws.Config
			if c == nil {
				c = config.New()
			}
			if i := c.Find(name); i >= 0 {
				return errs.New(errs.CodeConfig, "%q is already a recipient", name).
					Check("use a different name",
						"run `envseal remove "+name+"` first to replace their key")
			}
			if i := c.Find(key); i >= 0 {
				return errs.New(errs.CodeConfig, "that key is already authorized as %q", c.Recipients[i].Name)
			}

			c.Recipients = append(c.Recipients, config.Recipient{Name: name, Key: key})
			if err := c.Save(ws.configPath()); err != nil {
				return err
			}

			out := a.stdout(cmd)
			fmt.Fprintf(out, "Added %s to %s\n", name, display(ws.configPath()))
			fmt.Fprintf(out, "\nRun `envseal rotate` to give them access to the current secrets.\n")
			return nil
		},
	}
}

func newRemoveCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name or public key>",
		Aliases: []string{"rm"},
		Short:   "Withdraw a recipient",
		Long:    removeLong,
		Args:    cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.workspace()
			if err != nil {
				return err
			}
			if ws.Config == nil {
				return errs.New(errs.CodeConfig, "no %s in this project", config.Filename).
					Check("run `envseal encrypt .env` to create one")
			}

			c := ws.Config
			i := c.Find(args[0])
			if i < 0 {
				return errs.New(errs.CodeConfig, "no recipient named %q", args[0]).
					Check("run `envseal status` to list the current recipients")
			}

			removed := c.Recipients[i]
			c.Recipients = append(c.Recipients[:i], c.Recipients[i+1:]...)
			if err := c.Save(ws.configPath()); err != nil {
				return err
			}

			out := a.stdout(cmd)
			fmt.Fprintf(out, "Removed %s from %s\n", removed.Name, display(ws.configPath()))

			if own, err := identity.Resolve(a.identityPath); err == nil && own.PublicKey() == removed.Key {
				fmt.Fprint(out, "\nThat was your own key. After rotating you will no longer be able to decrypt this project.\n")
			}
			if len(c.Recipients) == 0 {
				fmt.Fprint(out, "\nNo recipients remain. Add one before encrypting again.\n")
			} else {
				fmt.Fprint(out, "\nRun `envseal rotate` to re-encrypt without them.\n")
			}
			fmt.Fprint(out, "Anyone holding an older copy of the encrypted file can still read it, so rotate the secrets themselves.\n")
			return nil
		},
	}
}

func newRotateCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "rotate",
		Short: "Re-encrypt for the current recipients",
		Long:  rotateLong,
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := a.workspace()
			if err != nil {
				return err
			}
			if ws.Config == nil {
				return errs.New(errs.CodeConfig, "no %s in this project", config.Filename).
					Check("run `envseal encrypt .env` to create one")
			}
			if len(ws.Config.Recipients) == 0 {
				return errs.New(errs.CodeConfig, "no recipients in %s", config.Filename).
					Detailf("Re-encrypting for nobody would produce a file no one can read.").
					Check("run `envseal add <name> <public key>`")
			}

			source := ws.encryptedPath()
			ciphertext, err := os.ReadFile(source)
			if err != nil {
				return errs.New(errs.CodeConfig, "no encrypted file at %s", display(source)).
					Check("run `envseal encrypt .env` first")
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

			keys, err := config.ParseRecipients(ws.Config.Recipients)
			if err != nil {
				return err
			}

			rotated, err := crypto.Encrypt(plaintext, keys)
			if err != nil {
				return err
			}
			if err := safefile.Write(source, rotated, encryptedFileMode); err != nil {
				return errs.New(errs.CodeGeneral, "unable to write %s", display(source)).Wrap(err)
			}

			out := a.stdout(cmd)
			fmt.Fprintf(out, "Re-encrypted %s for %d recipient(s): %s\n",
				display(source), len(ws.Config.Recipients), join(names(ws.Config.Recipients)))
			warnIfLockedOut(out, ws.Config, id)
			return nil
		},
	}
}

// warnIfLockedOut reports the one mistake rotation cannot undo: re-encrypting
// for a set that excludes you, leaving the file unreadable on this machine.
func warnIfLockedOut(out io.Writer, c *config.Config, id *identity.Identity) {
	if c.Find(id.PublicKey()) < 0 {
		fmt.Fprint(out, "\nWarning: your own key is not a recipient, so you can no longer decrypt this file.\n")
	}
}

func join(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	out := items[0]
	for _, s := range items[1:] {
		out += ", " + s
	}
	return out
}
