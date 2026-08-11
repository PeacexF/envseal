package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/process"
)

const runLong = `Run a program with the decrypted environment.

The environment is decrypted in memory and handed to the child process. No
plaintext is written to disk, which makes this the safest way to use a sealed
environment:

  envseal run -- ./server
  envseal run .env.production.enc -- docker compose up

Everything after -- is your command. The child's exit code becomes envseal's,
and signals are forwarded to it.`

// essential are the variables a program needs to function at all, kept when
// --isolated withholds the rest of the parent environment.
var essential = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL", "LANG", "LC_ALL", "TERM", "TMPDIR", "TZ",
	"SYSTEMROOT", "SYSTEMDRIVE", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP",
	"USERPROFILE", "APPDATA", "LOCALAPPDATA", "PROGRAMDATA", "NUMBER_OF_PROCESSORS", "OS",
}

func newRunCmd(a *app) *cobra.Command {
	var isolated bool

	cmd := &cobra.Command{
		Use:   "run [file] -- command [args...]",
		Short: "Run a command with the decrypted environment",
		Long:  runLong,

		RunE: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			if dash < 0 || dash >= len(args) {
				return errs.New(errs.CodeGeneral, "no command to run").
					Detailf("Separate envseal's arguments from your command with --.").
					Check("envseal run -- ./server",
						"envseal run .env.production.enc -- npm run dev")
			}

			before, command := args[:dash], args[dash:]
			if len(before) > 1 {
				return errs.New(errs.CodeGeneral, "too many arguments before --").
					Detailf("Expected at most one encrypted file, but got %d.", len(before)).
					Check("envseal run " + before[0] + " -- " + strings.Join(command, " "))
			}

			ws, err := a.workspace()
			if err != nil {
				return err
			}

			source := ws.encryptedPath()
			if len(before) == 1 {
				source = before[0]
			}

			ciphertext, err := os.ReadFile(source)
			if err != nil {
				return errs.New(errs.CodeConfig, "no encrypted file at %s", display(source)).
					Check("pass the path explicitly: `envseal run <file> -- ...`",
						"run `envseal encrypt .env` to create one")
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

			env, err := dotenv.Parse(plaintext, display(source))
			if err != nil {
				return err
			}
			childEnv := compose(env.Environ(), isolated)

			// Wipe the decrypted buffer now that the values have been copied
			// into the environment. The strings themselves are immutable and
			// live until the garbage collector reclaims them, so this narrows
			// the window rather than closing it.
			clear(plaintext)

			code, err := process.Run(process.Options{
				Args:   command,
				Env:    childEnv,
				Stdin:  cmd.InOrStdin(),
				Stdout: cmd.OutOrStdout(),
				Stderr: cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			if code != 0 {
				return errs.Exit(code)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&isolated, "isolated", false,
		"pass only the decrypted variables, withholding the parent environment")
	return cmd
}

// compose builds the child's environment. Decrypted variables come last so
// they win over any parent value of the same name.
func compose(decrypted []string, isolated bool) []string {
	parent := os.Environ()
	if isolated {
		parent = keepEssential(parent)
	}
	return append(parent, decrypted...)
}

func keepEssential(parent []string) []string {
	kept := make([]string, 0, len(essential))
	for _, entry := range parent {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		for _, allowed := range essential {
			if strings.EqualFold(name, allowed) {
				kept = append(kept, entry)
				break
			}
		}
	}
	return kept
}
