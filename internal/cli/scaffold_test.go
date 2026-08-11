package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
)

func TestInitScaffoldsTheProject(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	code, stdout, stderr := run(t, "init")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}

	c := loadConfig(t, dir)
	if len(c.Recipients) != 1 {
		t.Fatalf("recipients = %d, want your own key", len(c.Recipients))
	}

	example := read(t, filepath.Join(dir, ".env.example"))
	for _, key := range []string{"DATABASE_URL=", "API_KEY="} {
		if !strings.Contains(example, key) {
			t.Errorf(".env.example =\n%s\nwant %q", example, key)
		}
	}
	if strings.Contains(example, "s3cretvalue") {
		t.Errorf(".env.example leaked a value:\n%s", example)
	}

	ignore := read(t, filepath.Join(dir, ".gitignore"))
	for _, rule := range []string{".env", ".env.*", "!.env.example", "!*.enc"} {
		if !strings.Contains(ignore, rule) {
			t.Errorf(".gitignore =\n%s\nwant the rule %q", ignore, rule)
		}
	}

	if !strings.Contains(stdout, "envseal encrypt") {
		t.Errorf("stdout =\n%s\nwant the next step", stdout)
	}
}

// The ignore rules it writes must actually protect the plaintext.
func TestInitIgnoreRulesWork(t *testing.T) {
	dir := gitProject(t)
	if err := os.Remove(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	writeEnv(t, dir, ".env", env)

	if code, _, stderr := run(t, "init"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if code, _, stderr := run(t, "encrypt", ".env"); code != 0 {
		t.Fatalf("encrypt: exit = %d (stderr: %s)", code, stderr)
	}

	ignored := func(name string) bool {
		_, err := gitOutput(dir, "check-ignore", "-q", name)
		return err == nil
	}
	if !ignored(".env") {
		t.Error(".env is not ignored")
	}
	if ignored(config.DefaultFile) {
		t.Error(".env.enc is ignored, so it could never be committed")
	}
	if ignored(".env.example") {
		t.Error(".env.example is ignored")
	}
}

func TestInitIsRepeatable(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	if code, _, stderr := run(t, "init"); code != 0 {
		t.Fatalf("first init: exit = %d (stderr: %s)", code, stderr)
	}
	before := read(t, filepath.Join(dir, ".gitignore"))

	code, stdout, stderr := run(t, "init")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Kept") {
		t.Errorf("stdout =\n%s\nwant it to report existing files were kept", stdout)
	}
	if after := read(t, filepath.Join(dir, ".gitignore")); after != before {
		t.Errorf(".gitignore was rewritten:\n%s\nwas\n%s", after, before)
	}
}

// An existing .gitignore belongs to the user; only missing rules are added.
func TestInitAppendsToAnExistingGitignore(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, ".gitignore", "node_modules/\n.env\n")

	if code, _, stderr := run(t, "init"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}

	ignore := read(t, filepath.Join(dir, ".gitignore"))
	if !strings.HasPrefix(ignore, "node_modules/\n.env\n") {
		t.Errorf(".gitignore =\n%s\nwant the original content kept at the top", ignore)
	}
	if strings.Count(ignore, "\n.env\n") > 1 {
		t.Errorf(".gitignore =\n%s\nwant no duplicate rules", ignore)
	}
	if !strings.Contains(ignore, "!*.enc") {
		t.Errorf(".gitignore =\n%s\nwant the missing rule added", ignore)
	}
}

func TestInitKeepsAnExistingExample(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, ".env.example", "# hand written\nCUSTOM=\n")

	if code, _, stderr := run(t, "init"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got := read(t, filepath.Join(dir, ".env.example")); got != "# hand written\nCUSTOM=\n" {
		t.Errorf(".env.example was overwritten:\n%s", got)
	}
}

func TestInitWithoutDotEnv(t *testing.T) {
	dir := project(t)

	code, stdout, stderr := run(t, "init")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Skipped") {
		t.Errorf("stdout =\n%s\nwant it to report the skipped example", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, config.Filename)); err != nil {
		t.Errorf("configuration: %v", err)
	}
}

func TestInitNoExample(t *testing.T) {
	dir := project(t)
	writeEnv(t, dir, ".env", env)

	if code, _, stderr := run(t, "init", "--no-example"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.example")); err == nil {
		t.Error("--no-example wrote an example file")
	}
}

func TestInitWithoutIdentity(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	t.Chdir(dir)

	code, _, stderr := run(t, "init")
	if code != 4 {
		t.Errorf("exit = %d, want 4", code)
	}
	if !strings.Contains(stderr, "envseal keys generate") {
		t.Errorf("stderr =\n%s\nwant it to point at key generation", stderr)
	}
}

func TestInitKeepsAnExistingConfiguration(t *testing.T) {
	dir := sealed(t)

	before := read(t, filepath.Join(dir, config.Filename))
	if code, stdout, stderr := run(t, "init"); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	} else if !strings.Contains(stdout, "Kept") {
		t.Errorf("stdout =\n%s\nwant it to keep the configuration", stdout)
	}

	if after := read(t, filepath.Join(dir, config.Filename)); after != before {
		t.Error("the configuration was rewritten")
	}
}
