package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/identity"
)

const (
	keysLong = `Manage the private identity that decrypts your environments.

An identity is created once per machine, not once per project. The same key
opens every project that lists your public key as a recipient.`

	generateLong = `Create the private identity that decrypts your environments.

The identity is written to ~/.envseal/identity with owner-only permissions. It
never belongs in a repository. Share only the public key printed here: someone
already on the project adds it as a recipient and re-encrypts.`

	publicLong = `Print your public key.

This is the shareable half of your identity. Send it to whoever manages a
project's recipients; it is not a secret.`
)

func newKeysCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage your identity",
		Long:  keysLong,
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}

	cmd.AddCommand(newKeysGenerateCmd(a), newKeysPublicCmd(a))
	return cmd
}

func newKeysGenerateCmd(a *app) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Create a local identity",
		Long:  generateLong,
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

func newKeysPublicCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "public",
		Aliases: []string{"export-public"},
		Short:   "Print your public key",
		Long:    publicLong,
		Args:    cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := identity.Resolve(a.identityPath)
			if err != nil {
				return err
			}
			warn(cmd.ErrOrStderr(), id)

			// The payload, not progress output: --quiet must not silence it.
			fmt.Fprintln(cmd.OutOrStdout(), id.PublicKey())
			return nil
		},
	}
}
