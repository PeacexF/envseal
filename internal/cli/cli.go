// Package cli assembles the envseal command tree and owns
// argument parsing, output streams, error rendering, and exit codes.
package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/version"
)

const rootLong = `Envseal encrypts .env files so they can be committed to Git.

Your private identity stays on your machine, in ~/.envseal/identity. The
encrypted file and the recipient list are safe to commit. Sharing the encrypted
file grants no access on its own: only an authorized private identity can
decrypt it.

Envseal needs no account, no server, and no network access.`

// Main runs envseal against the process streams and returns the exit code.
func Main(args []string) int {
	return Run(args, os.Stdout, os.Stderr)
}

func Run(args []string, stdout, stderr io.Writer) int {
	root := NewRoot()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err != nil {
		errs.Render(stderr, err)
	}
	return int(errs.CodeOf(err))
}

// app holds the global flags shared by every command.
type app struct {
	identityPath string
}

func NewRoot() *cobra.Command {
	a := &app{}

	root := &cobra.Command{
		Use:   "envseal",
		Short: "Git-friendly encrypted .env files",
		Long:  rootLong,

		Args:          cobra.NoArgs,
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.SetVersionTemplate("envseal {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true

	root.PersistentFlags().StringVar(&a.identityPath, "identity", "",
		"path to your private identity (default ~/.envseal/identity)")

	root.AddCommand(
		newInitCmd(a),
		newEncryptCmd(a),
		newDecryptCmd(a),
	)

	return root
}
