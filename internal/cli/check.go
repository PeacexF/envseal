package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/git"
	"github.com/PeacexF/envseal/internal/identity"
)

const (
	exampleFile = ".env.example"

	checkLong = `Validate the project without exposing any value.

Four things are checked:

  • the configuration parses and has recipients
  • the encrypted file decrypts with your identity
  • every variable in ` + exampleFile + ` is present in the environment
  • no plaintext environment file is tracked by git or left unignored

Checks that cannot run — no identity in this environment, no git repository —
are reported as skipped rather than failed. Exits non-zero when something is
wrong, so CI can gate on it.`
)

// Status is how a single check turned out.
const (
	statusOK      = "ok"
	statusFailed  = "failed"
	statusWarning = "warning"
	statusSkipped = "skipped"
)

type finding struct {
	Check   string   `json:"check"`
	Status  string   `json:"status"`
	Detail  string   `json:"detail,omitempty"`
	Items   []string `json:"items,omitempty"`
	problem bool
}

type checkReport struct {
	Findings []finding `json:"checks"`
	Problems int       `json:"problems"`
}

func (r *checkReport) add(f finding) {
	if f.problem {
		r.Problems++
	}
	r.Findings = append(r.Findings, f)
}

func newCheckCmd(a *app) *cobra.Command {
	var (
		asJSON bool
		strict bool
		schema string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate the project and detect exposed plaintext",
		Long:  checkLong,
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error {
			report := a.check(schema, strict)

			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return errs.New(errs.CodeGeneral, "unable to write the report").Wrap(err)
				}
			} else {
				report.render(out, a.interactive)
			}

			if report.Problems > 0 {
				return errs.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable output")
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as failures")
	cmd.Flags().StringVar(&schema, "schema", exampleFile, "file listing the expected variables")
	return cmd
}

func (a *app) check(schema string, strict bool) *checkReport {
	report := &checkReport{Findings: []finding{}}

	ws, err := a.workspace()
	if err != nil {
		report.add(finding{
			Check:   "configuration",
			Status:  statusFailed,
			Detail:  err.Error(),
			problem: true,
		})
		return report
	}

	report.add(a.checkConfiguration(ws))

	env, sealedFinding := a.checkSealed(ws)
	report.add(sealedFinding)

	report.add(checkSchema(ws, env, schema))
	report.addAll(checkPlaintext(ws), strict)

	return report
}

func (r *checkReport) addAll(findings []finding, strict bool) {
	for _, f := range findings {
		if strict && f.Status == statusWarning {
			f.Status, f.problem = statusFailed, true
		}
		r.add(f)
	}
}

func (a *app) checkConfiguration(ws *workspace) finding {
	switch {
	case ws.Config == nil:
		return finding{
			Check:   "configuration",
			Status:  statusFailed,
			Detail:  "no " + config.Filename + " in this project",
			problem: true,
		}
	case len(ws.Config.Recipients) == 0:
		return finding{
			Check:   "configuration",
			Status:  statusFailed,
			Detail:  "no recipients, so nothing can be encrypted",
			problem: true,
		}
	}
	return finding{
		Check:  "configuration",
		Status: statusOK,
		Detail: plural(len(ws.Config.Recipients), "recipient"),
	}
}

// checkSealed confirms the encrypted file exists and opens, returning the
// environment for the schema check.
func (a *app) checkSealed(ws *workspace) (*dotenv.File, finding) {
	sealed := ws.encryptedPath()
	if _, err := os.Stat(sealed); err != nil {
		return nil, finding{
			Check:   "encrypted file",
			Status:  statusFailed,
			Detail:  "missing: " + display(sealed),
			problem: true,
		}
	}

	id, err := identity.Resolve(a.identityPath)
	if err != nil {
		return nil, finding{
			Check:  "encrypted file",
			Status: statusSkipped,
			Detail: "no identity available to open it",
		}
	}

	env, err := a.unseal(sealed, id)
	if err != nil {
		return nil, finding{
			Check:   "encrypted file",
			Status:  statusFailed,
			Detail:  "cannot be decrypted with this identity",
			problem: true,
		}
	}
	return env, finding{
		Check:  "encrypted file",
		Status: statusOK,
		Detail: plural(env.Len(), "variable"),
	}
}

// checkSchema compares variable names against the example file. Only names are
// read from either side.
func checkSchema(ws *workspace, env *dotenv.File, schema string) finding {
	path := schema
	if !filepath.IsAbs(path) {
		path = ws.Project.Path(schema)
	}

	// A missing or unreadable example file means there is no schema to check
	// against, which is a skip rather than a failure.
	expected, err := dotenv.Load(path)
	if err != nil {
		return finding{Check: "schema", Status: statusSkipped, Detail: "no " + schema}
	}
	if env == nil {
		return finding{Check: "schema", Status: statusSkipped, Detail: "the environment could not be read"}
	}

	present := env.Map()
	var missing []string
	for _, key := range expected.Keys() {
		if _, ok := present[key]; !ok {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		slices.Sort(missing)
		return finding{
			Check:   "schema",
			Status:  statusFailed,
			Detail:  fmt.Sprintf("%s missing from the environment", plural(len(missing), "variable")),
			Items:   missing,
			problem: true,
		}
	}
	return finding{
		Check:  "schema",
		Status: statusOK,
		Detail: fmt.Sprintf("every variable in %s is present", schema),
	}
}

// checkPlaintext looks for unencrypted environment files that git can see. A
// tracked one is a failure; one that is merely unignored is a warning, because
// it is one `git add .` away from becoming a failure.
func checkPlaintext(ws *workspace) []finding {
	repo, err := git.Open(ws.Project.Root)
	if err != nil {
		return []finding{{Check: "plaintext", Status: statusSkipped, Detail: "not a git repository"}}
	}

	tracked, err := repo.Tracked()
	if err != nil {
		return []finding{{Check: "plaintext", Status: statusSkipped, Detail: "unable to list tracked files"}}
	}

	var exposed []string
	for _, name := range tracked {
		if isPlaintextEnv(name) {
			exposed = append(exposed, name)
		}
	}
	if len(exposed) > 0 {
		slices.Sort(exposed)
		return []finding{{
			Check:   "plaintext",
			Status:  statusFailed,
			Detail:  "committed to the repository, so the values are exposed",
			Items:   exposed,
			problem: true,
		}}
	}

	var unignored []string
	for _, name := range plaintextOnDisk(ws.Project.Root) {
		if !repo.IsIgnored(name) {
			if rel, err := repo.Relative(name); err == nil {
				unignored = append(unignored, rel)
			}
		}
	}
	if len(unignored) > 0 {
		slices.Sort(unignored)
		return []finding{{
			Check:  "plaintext",
			Status: statusWarning,
			Detail: "not ignored by git, so one `git add .` would commit them",
			Items:  unignored,
		}}
	}

	return []finding{{Check: "plaintext", Status: statusOK, Detail: "no exposed environment files"}}
}

// skipDirs are never worth walking and would dominate the time if they were.
var skipDirs = []string{".git", "node_modules", "vendor", "dist", "build", "target", ".venv", "venv"}

func plaintextOnDisk(root string) []string {
	var found []string

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable directory is not a finding
		}
		if d.IsDir() {
			if path != root && slices.Contains(skipDirs, d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if isPlaintextEnv(d.Name()) {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// isPlaintextEnv recognises a file that holds unencrypted environment values.
func isPlaintextEnv(name string) bool {
	base := filepath.Base(name)
	switch base {
	case ".env":
		return true
	case exampleFile, ".env.sample", ".env.template", ".env.dist":
		return false
	}
	return strings.HasPrefix(base, ".env.") && !strings.HasSuffix(base, ".enc")
}

func (r *checkReport) render(w io.Writer, unicode bool) {
	for _, f := range r.Findings {
		fmt.Fprintf(w, "%s %-15s %s\n", symbol(f.Status, unicode), f.Check, f.Detail)
		for _, item := range f.Items {
			fmt.Fprintf(w, "    %s\n", item)
		}
	}

	fmt.Fprintln(w)
	if r.Problems == 0 {
		fmt.Fprintln(w, "No problems found.")
		return
	}
	fmt.Fprintf(w, "%s found.\n", plural(r.Problems, "problem"))
}

func symbol(status string, unicode bool) string {
	if !unicode {
		switch status {
		case statusOK:
			return "ok  "
		case statusFailed:
			return "FAIL"
		case statusWarning:
			return "warn"
		default:
			return "skip"
		}
	}

	switch status {
	case statusOK:
		return "✓"
	case statusFailed:
		return "✗"
	case statusWarning:
		return "!"
	default:
		return "-"
	}
}
