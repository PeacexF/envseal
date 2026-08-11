// Package project locates the envseal project a command is running in.
package project

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/errs"
)

// ErrNotFound is reported when no configuration exists at or above the starting
// directory. Callers that can proceed without one should match it with
// errors.Is rather than treating the error as fatal.
var ErrNotFound = errors.New("no " + config.Filename + " found")

type Project struct {
	Root       string
	ConfigPath string
}

// Find walks up from dir looking for a configuration file, stopping at a
// repository boundary so a search never escapes into an unrelated project.
func Find(dir string) (*Project, error) {
	start, err := filepath.Abs(dir)
	if err != nil {
		return nil, errs.New(errs.CodeGeneral, "unable to resolve %s", dir).Wrap(err)
	}

	for cur := start; ; {
		path := filepath.Join(cur, config.Filename)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return &Project{Root: cur, ConfigPath: path}, nil
		}

		parent := filepath.Dir(cur)
		if parent == cur || isRepoRoot(cur) {
			break
		}
		cur = parent
	}

	return nil, errs.New(errs.CodeConfig, "not an envseal project").
		Detailf("No %s was found in %s or any parent directory.", config.Filename, start).
		Check("run `envseal encrypt .env` in your project root to create one").
		Wrap(ErrNotFound)
}

// FindFromWD is Find starting at the working directory.
func FindFromWD() (*Project, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, errs.New(errs.CodeGeneral, "unable to determine the working directory").Wrap(err)
	}
	return Find(dir)
}

// Path resolves name against the project root. Absolute paths are returned
// unchanged.
func (p *Project) Path(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(p.Root, name)
}

func (p *Project) Config() (*config.Config, error) {
	return config.Load(p.ConfigPath)
}

func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}
