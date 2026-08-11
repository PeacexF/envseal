package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"golang.org/x/term"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/identity"
	"github.com/PeacexF/envseal/internal/project"
)

// workspace is the project a command runs in. Config is nil when the directory
// has no .envseal.yaml yet, which is not an error: `encrypt` creates one.
type workspace struct {
	Project *project.Project
	Config  *config.Config
}

func (a *app) workspace() (*workspace, error) {
	p, err := project.FindFromWD()
	switch {
	case errors.Is(err, project.ErrNotFound):
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			return nil, errs.New(errs.CodeGeneral, "unable to determine the working directory").Wrap(wdErr)
		}
		return &workspace{Project: &project.Project{Root: wd}}, nil
	case err != nil:
		return nil, err
	}

	c, err := p.Config()
	if err != nil {
		return nil, err
	}
	return &workspace{Project: p, Config: c}, nil
}

// encryptedPath is the project's encrypted file.
func (w *workspace) encryptedPath() string {
	name := config.DefaultFile
	if w.Config != nil {
		name = w.Config.File
	}
	return w.Project.Path(name)
}

func (w *workspace) configPath() string {
	return w.Project.Path(config.Filename)
}

// warn reports non-fatal identity problems, such as loose file permissions.
func warn(w io.Writer, id *identity.Identity) {
	for _, msg := range id.Warnings {
		fmt.Fprintf(w, "Warning: %s\n", msg)
	}
}

// isTerminal reports whether w is an interactive terminal. A test buffer or a
// redirected stream is not, which is what makes secrets safe to write there.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// selfName labels the recipient created when bootstrapping a project.
func selfName() string {
	u, err := user.Current()
	if err != nil || u.Username == "" {
		return "me"
	}

	name := u.Username
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:] // DOMAIN\user on Windows
	}
	return name
}
