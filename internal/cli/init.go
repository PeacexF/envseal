package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/identity"
)

const initLong = `Create the private identity that decrypts your environments.

The identity is written to ~/.envseal/identity with owner-only permissions. It
never belongs in a repository. Share only the public key printed here: add it to
a project's recipients and re-encrypt to grant access.`

func newInitCmd(a *app) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a local identity",
		Long:  initLong,
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := identity.Destination(a.identityPath)
			if err != nil {
				return err
			}

			id, backup, err := identity.Create(path, force)
			if err != nil {
				return err
			}

			out := a.stdout(cmd)
			fmt.Fprint(out, "Identity created.\n\n")
			if backup != "" {
				fmt.Fprintf(out, "Previous identity kept at:\n  %s\n\n", display(backup))
			}
			fmt.Fprintf(out, "Private identity:\n  %s\n\n", display(path))
			fmt.Fprintf(out, "Public identity:\n  %s\n\n", id.PublicKey())
			fmt.Fprint(out, "Share the public identity freely. Never share or commit the private one.\n\n")
			fmt.Fprintf(out, "Authorize it in a project with:\n  envseal add <name> %s\n", id.PublicKey())
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "replace an existing identity, keeping a backup")
	return cmd
}
