package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/identity"
)

const statusLong = `Show what envseal knows about this project.

Only public information is reported: file paths, recipient names, and public
keys. No secret value is ever displayed.`

// report is the machine-readable form of the project's state.
type report struct {
	Configuration     string   `json:"configuration"`
	ConfigurationOK   bool     `json:"configuration_found"`
	EncryptedFile     string   `json:"encrypted_file"`
	EncryptedFileOK   bool     `json:"encrypted_file_found"`
	Identity          string   `json:"identity"`
	IdentityAvailable bool     `json:"identity_available"`
	Recipients        int      `json:"recipients"`
	RecipientNames    []string `json:"recipient_names"`
	Decryptable       bool     `json:"decryptable"`
}

func newStatusCmd(a *app) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the project's encryption state",
		Long:  statusLong,
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := a.report()
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(r)
			}
			r.render(out, a.interactive)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable output")
	return cmd
}

func (a *app) report() (*report, error) {
	ws, err := a.workspace()
	if err != nil {
		return nil, err
	}

	r := &report{
		Configuration:  display(ws.configPath()),
		EncryptedFile:  display(ws.encryptedPath()),
		RecipientNames: []string{},
	}

	if ws.Config != nil {
		r.ConfigurationOK = true
		r.Recipients = len(ws.Config.Recipients)
		r.RecipientNames = names(ws.Config.Recipients)
	}

	ciphertext, readErr := os.ReadFile(ws.encryptedPath())
	r.EncryptedFileOK = readErr == nil

	id, idErr := identity.Resolve(a.identityPath)
	if idErr == nil {
		r.IdentityAvailable = true
		r.Identity = display(id.Source)

		if r.EncryptedFileOK {
			// The only honest way to report access is to try it.
			plaintext, err := crypto.Decrypt(ciphertext, id.Identities(), r.EncryptedFile)
			r.Decryptable = err == nil
			clear(plaintext)
		}
	} else {
		r.Identity = display(defaultIdentityPath())
	}

	return r, nil
}

func (r *report) render(w io.Writer, unicode bool) {
	fmt.Fprintf(w, "Project\n%s\n", strings.Repeat("─", 46))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Configuration\t%s\t%s\n", r.Configuration, mark(r.ConfigurationOK, unicode))
	fmt.Fprintf(tw, "Encrypted file\t%s\t%s\n", r.EncryptedFile, mark(r.EncryptedFileOK, unicode))
	fmt.Fprintf(tw, "Identity\t%s\t%s\n", r.Identity, mark(r.IdentityAvailable, unicode))
	fmt.Fprintf(tw, "Local access\t%s\t%s\n", access(r), mark(r.Decryptable, unicode))
	tw.Flush()

	if r.Recipients > 0 {
		fmt.Fprintf(w, "\nRecipients (%d)\n", r.Recipients)
		for _, name := range r.RecipientNames {
			fmt.Fprintf(w, "  %s\n", name)
		}
	}

	for _, hint := range r.hints() {
		fmt.Fprintf(w, "\n%s\n", hint)
	}
}

// hints turn a missing piece into the command that supplies it.
func (r *report) hints() []string {
	switch {
	case !r.IdentityAvailable:
		return []string{"Run `envseal init` to create your identity."}
	case !r.ConfigurationOK:
		return []string{"Run `envseal encrypt .env` to set this project up."}
	case r.Recipients == 0:
		return []string{"Run `envseal add <name> <public key>` to authorize someone."}
	case !r.EncryptedFileOK:
		return []string{"Run `envseal encrypt .env` to create the encrypted file."}
	case !r.Decryptable:
		return []string{"Your key is not a recipient of this file. Ask a current recipient to add it and run `envseal rotate`."}
	}
	return nil
}

func access(r *report) string {
	if r.Decryptable {
		return "you can decrypt this project"
	}
	return "not available"
}

func mark(ok, unicode bool) string {
	switch {
	case ok && unicode:
		return "✓"
	case ok:
		return "yes"
	case unicode:
		return "✗"
	default:
		return "no"
	}
}

func defaultIdentityPath() string {
	path, err := identity.DefaultPath()
	if err != nil {
		return "~/" + identity.Dir + "/" + identity.File
	}
	return path
}
