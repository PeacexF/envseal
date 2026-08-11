package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
	"github.com/PeacexF/envseal/internal/syncstate"
)

const rotateLong = `Replace one variable's value and re-encrypt.

  envseal rotate STRIPE_SECRET_KEY              type the new value, hidden
  envseal rotate SESSION_SECRET --generate      generate a random one
  pass show api | envseal rotate API_KEY --stdin

The environment is decrypted in memory, the single value is replaced, and the
file is encrypted again. No plaintext is written to disk, which makes this safer
than decrypting, editing, and encrypting by hand.

Every other line of the file survives exactly: comments, ordering, and the
quoting of other variables.

There is deliberately no --value flag: a secret on the command line lands in
your shell history and is visible to anyone who can run ps.`

func newRotateCmd(a *app) *cobra.Command {
	var (
		fromStdin bool
		generate  bool
		length    int
		add       bool
	)

	cmd := &cobra.Command{
		Use:   "rotate <VARIABLE>",
		Short: "Replace one variable's value and re-encrypt",
		Long:  rotateLong,
		Args:  cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			ws, err := a.workspace()
			if err != nil {
				return err
			}
			if ws.Config == nil {
				return errs.New(errs.CodeConfig, "no %s in this project", config.Filename).
					Check("run `envseal init` to set the project up")
			}

			sealed := ws.encryptedPath()
			ciphertext, err := os.ReadFile(sealed)
			if err != nil {
				return errs.New(errs.CodeConfig, "no encrypted file at %s", display(sealed)).
					Check("run `envseal encrypt .env` first")
			}

			id, err := identity.Resolve(a.identityPath)
			if err != nil {
				return err
			}
			warn(cmd.ErrOrStderr(), id)

			plaintext, err := crypto.Decrypt(ciphertext, id.Identities(), display(sealed))
			if err != nil {
				return err
			}
			defer clear(plaintext)

			env, err := dotenv.Parse(plaintext, display(sealed))
			if err != nil {
				return err
			}
			if !env.Has(key) && !add {
				return errs.New(errs.CodeConfig, "%s is not in %s", key, display(sealed)).
					Detailf("Rotating a name that does not exist is usually a typo.").
					Check("run `envseal diff` or `envseal check` to see the variable names",
						"pass --add to create it")
			}

			value, err := a.newValue(cmd, key, valueSource{
				stdin:    fromStdin,
				generate: generate,
				length:   length,
			})
			if err != nil {
				return err
			}

			updated, err := env.Set(key, value)
			if err != nil {
				return err
			}
			defer clear(updated)

			keys, err := config.ParseRecipients(ws.Config.Recipients)
			if err != nil {
				return err
			}
			resealed, err := crypto.Encrypt(updated, keys)
			if err != nil {
				return err
			}
			if err := safefile.Write(sealed, resealed, publicFileMode); err != nil {
				return errs.New(errs.CodeGeneral, "unable to write %s", display(sealed)).Wrap(err)
			}

			out := a.stdout(cmd)
			action := "Rotated"
			if !env.Has(key) {
				action = "Added"
			}
			fmt.Fprintf(out, "%s %s in %s for %s.\n",
				action, key, display(sealed), plural(len(ws.Config.Recipients), "recipient"))

			return a.syncPlaintext(out, sealed, plaintext, updated)
		},
	}

	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read the new value from standard input")
	cmd.Flags().BoolVar(&generate, "generate", false, "use a randomly generated value")
	cmd.Flags().IntVar(&length, "length", 32, "characters to generate with --generate")
	cmd.Flags().BoolVar(&add, "add", false, "create the variable if it does not exist")
	return cmd
}

type valueSource struct {
	stdin    bool
	generate bool
	length   int
}

// newValue obtains the replacement without it ever reaching the command line,
// the shell history, or the process list.
func (a *app) newValue(cmd *cobra.Command, key string, src valueSource) (string, error) {
	switch {
	case src.generate && src.stdin:
		return "", errs.New(errs.CodeGeneral, "--generate and --stdin cannot both be used")

	case src.generate:
		value, err := randomValue(src.length)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(a.stdout(cmd), "Generated a value of %s.\n", plural(len(value), "character"))
		return value, nil

	case src.stdin:
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", errs.New(errs.CodeGeneral, "unable to read the value from standard input").Wrap(err)
		}
		// One trailing newline is an artefact of echo and here-strings; more
		// than that is deliberate and kept.
		return strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"), nil
	}

	return a.promptValue(cmd, key)
}

// promptValue reads a value from the terminal without echoing it.
func (a *app) promptValue(cmd *cobra.Command, key string) (string, error) {
	in, ok := cmd.InOrStdin().(*os.File)
	if !a.interactive || !ok || !term.IsTerminal(int(in.Fd())) {
		return "", errs.New(errs.CodeGeneral, "no terminal to type the value on").
			Check("pipe it in: `printf %s \"$NEW\" | envseal rotate "+key+" --stdin`",
				"or generate one: `envseal rotate "+key+" --generate`")
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "New value for %s: ", key)

	typed, err := term.ReadPassword(int(in.Fd()))
	defer clear(typed)
	fmt.Fprintln(out)
	if err != nil {
		return "", errs.New(errs.CodeGeneral, "unable to read the value").Wrap(err)
	}
	if len(typed) == 0 {
		return "", errs.New(errs.CodeGeneral, "no value entered")
	}

	// Nothing is echoed, so confirm the length instead: it catches a slipped
	// keystroke without putting the secret on screen.
	fmt.Fprintf(a.stdout(cmd), "Read %s.\n", plural(len(typed), "character"))
	return string(typed), nil
}

// randomValue produces a value made only of characters that need no quoting.
func randomValue(length int) (string, error) {
	if length < 8 {
		return "", errs.New(errs.CodeGeneral, "--length must be at least 8")
	}

	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", errs.New(errs.CodeGeneral, "unable to generate a random value").Wrap(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)[:length], nil
}

// syncPlaintext keeps a local .env in step with the rotation. Leaving it stale
// would be worse than writing to it: the next push would encrypt the old value
// and quietly undo the rotation.
func (a *app) syncPlaintext(out io.Writer, sealed string, before, after []byte) error {
	target, err := defaultOutput(sealed)
	if err != nil {
		return nil
	}

	current, err := os.ReadFile(target)
	if err != nil {
		return nil // no local plaintext to keep in step
	}

	if !bytes.Equal(current, before) {
		fmt.Fprintf(out, "\nWarning: %s has local changes and now differs from the sealed environment.\n",
			display(target))
		fmt.Fprintf(out, "Run `envseal pull --force` to take the sealed version.\n")
		return nil
	}

	if err := safefile.Write(target, after, plaintextFileMode); err != nil {
		return errs.New(errs.CodeGeneral, "unable to update %s", display(target)).Wrap(err)
	}
	syncstate.Record(target, after)

	fmt.Fprintf(out, "Updated %s to match.\n", display(target))
	return nil
}
