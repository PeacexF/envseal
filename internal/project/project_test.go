package project_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/project"
)

const validConfig = "version: 1\nfile: .env.enc\n"

// tree creates directories and files under a temporary root. A path ending in a
// separator is a directory; anything else is a file with placeholder content.
func tree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()

	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if len(p) > 0 && p[len(p)-1] == '/' {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(validConfig), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFindInSameDirectory(t *testing.T) {
	root := tree(t, config.Filename)

	p, err := project.Find(root)
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if p.Root != root {
		t.Errorf("Root = %q, want %q", p.Root, root)
	}
	if want := filepath.Join(root, config.Filename); p.ConfigPath != want {
		t.Errorf("ConfigPath = %q, want %q", p.ConfigPath, want)
	}
}

func TestFindWalksUp(t *testing.T) {
	root := tree(t, config.Filename, "api/internal/handlers/")

	p, err := project.Find(filepath.Join(root, "api", "internal", "handlers"))
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if p.Root != root {
		t.Errorf("Root = %q, want %q", p.Root, root)
	}
}

func TestFindPrefersNearest(t *testing.T) {
	root := tree(t, config.Filename, filepath.Join("api", config.Filename))

	p, err := project.Find(filepath.Join(root, "api"))
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if want := filepath.Join(root, "api"); p.Root != want {
		t.Errorf("Root = %q, want the nearest project %q", p.Root, want)
	}
}

func TestFindStopsAtRepositoryBoundary(t *testing.T) {
	// The configuration sits above a nested repository; the search must not
	// escape the inner repository to reach it.
	root := tree(t, config.Filename, "vendor/lib/.git/", "vendor/lib/src/")

	_, err := project.Find(filepath.Join(root, "vendor", "lib", "src"))
	if !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("Find() = %v, want ErrNotFound", err)
	}
}

func TestFindConfigAtRepositoryRoot(t *testing.T) {
	root := tree(t, config.Filename, ".git/", "cmd/")

	p, err := project.Find(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if p.Root != root {
		t.Errorf("Root = %q, want %q", p.Root, root)
	}
}

func TestFindNotFound(t *testing.T) {
	root := tree(t, ".git/", "cmd/")

	_, err := project.Find(filepath.Join(root, "cmd"))
	if !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("Find() = %v, want ErrNotFound", err)
	}
	if got := errs.CodeOf(err); got != errs.CodeConfig {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeConfig)
	}
}

func TestPath(t *testing.T) {
	root := tree(t, config.Filename)

	p, err := project.Find(root)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := p.Path(".env.enc"), filepath.Join(root, ".env.enc"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}

	abs := filepath.Join(t.TempDir(), "elsewhere.enc")
	if got := p.Path(abs); got != abs {
		t.Errorf("Path(absolute) = %q, want %q", got, abs)
	}
}

func TestProjectConfig(t *testing.T) {
	root := tree(t, config.Filename)

	p, err := project.Find(root)
	if err != nil {
		t.Fatal(err)
	}

	c, err := p.Config()
	if err != nil {
		t.Fatalf("Config() = %v", err)
	}
	if c.File != ".env.enc" {
		t.Errorf("File = %q, want %q", c.File, ".env.enc")
	}
}
