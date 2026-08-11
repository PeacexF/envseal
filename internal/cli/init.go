package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/safefile"
)

const initLong = `Set up envseal in this project.

Creates .envseal.yaml with your public key as the first recipient, writes a
.env.example listing the variables your .env defines (names only, no values),
and adds the ignore rules that keep plaintext out of Git.

Nothing is encrypted yet: run ` + "`envseal encrypt .env`" + ` or ` + "`envseal push`" + ` next.

Safe to run again — existing files are left alone.`

// ignoreRules keep plaintext out of Git while allowing the encrypted files in.
// The last rule matters: .env.* would otherwise swallow .env.enc.
var ignoreRules = []string{".env", ".env.*", "!.env.example", "!*.enc"}

func newInitCmd(a *app) *cobra.Command {
	var noExample bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up envseal in this project",
		Long:  initLong,
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := a.workspace()
			if err != nil {
				return err
			}

			id, err := identity.Resolve(a.identityPath)
			if err != nil {
				if errors.Is(err, identity.ErrNotFound) {
					return errs.New(errs.CodeIdentity, "no identity yet").
						Detailf("A project's recipients are public keys, so you need one first.").
						Check("run `envseal keys generate`")
				}
				return err
			}
			warn(cmd.ErrOrStderr(), id)

			out := a.stdout(cmd)

			if err := writeConfig(out, ws, id); err != nil {
				return err
			}
			if !noExample {
				if err := writeExample(out, ws); err != nil {
					return err
				}
			}
			if err := writeIgnoreRules(out, ws); err != nil {
				return err
			}

			fmt.Fprintf(out, "\nNext: `envseal encrypt .env` to seal your environment.\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&noExample, "no-example", false, "do not write .env.example")
	return cmd
}

func writeConfig(out io.Writer, ws *workspace, id *identity.Identity) error {
	path := ws.configPath()
	if ws.Config != nil {
		fmt.Fprintf(out, "Kept %s (%s)\n", display(path), plural(len(ws.Config.Recipients), "recipient"))
		return nil
	}

	c := config.New()
	c.Recipients = []config.Recipient{{Name: selfName(), Key: id.PublicKey()}}
	if err := c.Save(path); err != nil {
		return err
	}

	fmt.Fprintf(out, "Created %s with your key as the only recipient\n", display(path))
	return nil
}

// writeExample records which variables a project needs, without their values,
// so a newcomer knows what to expect before anyone grants them access.
func writeExample(out io.Writer, ws *workspace) error {
	path := ws.Project.Path(exampleFile)
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(out, "Kept %s\n", display(path))
		return nil
	}

	source := ws.Project.Path(".env")
	env, err := dotenv.Load(source)
	if err != nil {
		fmt.Fprintf(out, "Skipped %s (no .env to derive it from)\n", display(path))
		return nil
	}

	var b strings.Builder
	b.WriteString("# Variables this project expects. Values are deliberately empty.\n")
	for _, key := range env.Keys() {
		b.WriteString(key)
		b.WriteString("=\n")
	}

	if err := safefile.Write(path, []byte(b.String()), publicFileMode); err != nil {
		return errs.New(errs.CodeGeneral, "unable to write %s", display(path)).Wrap(err)
	}
	fmt.Fprintf(out, "Created %s from %s (%s, no values)\n",
		display(path), display(source), plural(env.Len(), "variable"))
	return nil
}

// writeIgnoreRules appends only the rules that are missing, so an existing
// .gitignore is never rewritten or reordered.
func writeIgnoreRules(out io.Writer, ws *workspace) error {
	path := ws.Project.Path(".gitignore")

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errs.New(errs.CodeGeneral, "unable to read %s", display(path)).Wrap(err)
	}

	present := make(map[string]bool)
	for line := range strings.SplitSeq(string(existing), "\n") {
		present[strings.TrimSpace(line)] = true
	}

	var missing []string
	for _, rule := range ignoreRules {
		if !present[rule] {
			missing = append(missing, rule)
		}
	}
	if len(missing) == 0 {
		fmt.Fprintf(out, "Kept %s (already correct)\n", display(path))
		return nil
	}

	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteString("\n")
	}
	if len(existing) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("# envseal: keep plaintext out of Git\n")
	b.WriteString(strings.Join(missing, "\n"))
	b.WriteString("\n")

	if err := safefile.Write(path, []byte(b.String()), publicFileMode); err != nil {
		return errs.New(errs.CodeGeneral, "unable to write %s", display(path)).Wrap(err)
	}

	verb := "Updated"
	if len(existing) == 0 {
		verb = "Created"
	}
	fmt.Fprintf(out, "%s %s (+%s)\n", verb, display(path), plural(len(missing), "rule"))
	return nil
}
