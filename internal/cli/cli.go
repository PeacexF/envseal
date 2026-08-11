// Package cli assembles the envseal command tree and owns
// argument parsing, output streams, error rendering, and exit codes.
package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/version"
)

const rootLong = `Envseal encrypts .env files so they can be committed to Git.

Your private identity stays on your machine, in ~/.envseal/identity. The
encrypted file and the recipient list are safe to commit. Sharing the encrypted
file grants no access on its own: only an authorized private identity can
decrypt it.

Envseal needs no account, no server, and no network access.`

// Streams are what a command reads and writes.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer

	// Interactive reports whether a person is watching. It decides whether
	// envseal may ask a question, and whether printing a secret would leave it
	// in a terminal's scrollback.
	Interactive bool
}

// Main runs envseal against the process streams and returns the exit code.
func Main(args []string) int {
	return RunStreams(args, Streams{
		In:          os.Stdin,
		Out:         os.Stdout,
		Err:         os.Stderr,
		Interactive: isTerminal(os.Stdout),
	})
}

// Run executes with the given output streams, without a terminal.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunStreams(args, Streams{Out: stdout, Err: stderr})
}

func RunStreams(args []string, s Streams) int {
	a := &app{interactive: s.Interactive}

	root := newRoot(a)
	root.SetArgs(args)
	root.SetOut(s.Out)
	root.SetErr(s.Err)
	if s.In != nil {
		root.SetIn(s.In)
	}

	err := root.Execute()
	if err != nil {
		errs.Render(s.Err, err)
	}
	return int(errs.CodeOf(err))
}

// isTerminal reports whether w is an interactive terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// app holds the global flags shared by every command.
type app struct {
	identityPath string
	quiet        bool
	interactive  bool
}

// stdout is where a command writes its progress chatter, which --quiet
// discards. Payload output, such as decrypted content or JSON, always goes to
// the real stream instead.
func (a *app) stdout(cmd *cobra.Command) io.Writer {
	if a.quiet {
		return io.Discard
	}
	return cmd.OutOrStdout()
}

// NewRoot builds the command tree for a non-interactive invocation.
func NewRoot() *cobra.Command { return newRoot(&app{}) }

func newRoot(a *app) *cobra.Command {
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
	root.PersistentFlags().BoolVarP(&a.quiet, "quiet", "q", false,
		"suppress progress output, leaving only errors")

	root.AddCommand(
		newInitCmd(a),
		newEncryptCmd(a),
		newDecryptCmd(a),
		newPushCmd(a),
		newPullCmd(a),
		newRunCmd(a),
		newAddCmd(a),
		newRemoveCmd(a),
		newRotateCmd(a),
		newStatusCmd(a),
		newCompletionCmd(),
	)

	return root
}
