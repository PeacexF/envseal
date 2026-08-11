package identity_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/identity"
)

// isolate points the identity search at a private home directory with no
// environment override, so tests never touch the developer's real identity.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(identity.EnvVar, "")
	return home
}

func rendered(err error) string {
	var b strings.Builder
	errs.Render(&b, err)
	return b.String()
}

func create(t *testing.T, path string) *identity.Identity {
	t.Helper()
	id, _, err := identity.Create(path, false)
	if err != nil {
		t.Fatalf("Create() = %v", err)
	}
	return id
}

func TestGenerate(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("Generate() = %v", err)
	}
	if !strings.HasPrefix(id.PublicKey(), "age1") {
		t.Errorf("PublicKey() = %q, want an age1 key", id.PublicKey())
	}
	if len(id.Identities()) != 1 {
		t.Errorf("len(Identities()) = %d, want 1", len(id.Identities()))
	}
}

// Formatting an Identity must never expose the private key.
func TestStringDoesNotLeakPrivateKey(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(id.String(), identity.KeyPrefix) {
		t.Error("String() contains private key material")
	}
	if id.String() != id.PublicKey() {
		t.Errorf("String() = %q, want the public key", id.String())
	}
}

func TestCreate(t *testing.T) {
	home := isolate(t)
	path := filepath.Join(home, identity.Dir, identity.File)

	id, backup, err := identity.Create(path, false)
	if err != nil {
		t.Fatalf("Create() = %v", err)
	}
	if backup != "" {
		t.Errorf("backup = %q, want none", backup)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	if !strings.Contains(string(data), identity.KeyPrefix) {
		t.Error("file contains no private key")
	}
	if !strings.Contains(string(data), id.PublicKey()) {
		t.Error("file does not record the public key")
	}
}

func TestCreatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	home := isolate(t)
	path := filepath.Join(home, identity.Dir, identity.File)
	create(t, path)

	file, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := file.Mode().Perm(); got != 0o600 {
		t.Errorf("identity mode = %04o, want 0600", got)
	}

	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dir.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode = %04o, want 0700", got)
	}
}

func TestCreateRefusesExisting(t *testing.T) {
	home := isolate(t)
	path := filepath.Join(home, identity.Dir, identity.File)
	before := create(t, path)

	_, _, err := identity.Create(path, false)
	if err == nil {
		t.Fatal("Create() = nil, want an error")
	}
	if got := errs.CodeOf(err); got != errs.CodeIdentity {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeIdentity)
	}

	after, err := identity.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.PublicKey() != before.PublicKey() {
		t.Error("the existing identity was replaced")
	}
}

func TestCreateForceKeepsBackup(t *testing.T) {
	home := isolate(t)
	path := filepath.Join(home, identity.Dir, identity.File)
	before := create(t, path)

	after, backup, err := identity.Create(path, true)
	if err != nil {
		t.Fatalf("Create(force) = %v", err)
	}
	if backup == "" {
		t.Fatal("backup = \"\", want the old identity preserved")
	}
	if after.PublicKey() == before.PublicKey() {
		t.Error("Create(force) reused the old key")
	}

	restored, err := identity.Load(backup)
	if err != nil {
		t.Fatalf("Load(backup) = %v", err)
	}
	if restored.PublicKey() != before.PublicKey() {
		t.Error("backup does not hold the previous identity")
	}
}

func TestLoadRoundTrip(t *testing.T) {
	home := isolate(t)
	path := filepath.Join(home, identity.Dir, identity.File)
	created := create(t, path)

	loaded, err := identity.Load(path)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if loaded.PublicKey() != created.PublicKey() {
		t.Errorf("PublicKey() = %q, want %q", loaded.PublicKey(), created.PublicKey())
	}
	if len(loaded.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", loaded.Warnings)
	}
}

func TestLoadMissing(t *testing.T) {
	home := isolate(t)

	_, err := identity.Load(filepath.Join(home, identity.Dir, identity.File))
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("Load() = %v, want ErrNotFound", err)
	}
	if got := errs.CodeOf(err); got != errs.CodeIdentity {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeIdentity)
	}
	if !strings.Contains(rendered(err), "envseal keys generate") {
		t.Errorf("error =\n%s\nwant it to suggest generating an identity", rendered(err))
	}
}

func TestLoadWarnsOnLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	home := isolate(t)
	path := filepath.Join(home, identity.Dir, identity.File)
	create(t, path)

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	id, err := identity.Load(path)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if len(id.Warnings) == 0 {
		t.Fatal("Warnings = none, want a warning about permissions")
	}
	if !strings.Contains(id.Warnings[0], "chmod 600") {
		t.Errorf("Warnings[0] = %q, want it to suggest chmod 600", id.Warnings[0])
	}
}

func TestParseInvalid(t *testing.T) {
	for _, data := range []string{"", "not a key", "age1notanidentity", "# only a comment\n"} {
		_, err := identity.Parse([]byte(data), "test")
		if err == nil {
			t.Errorf("Parse(%q) = nil, want an error", data)
			continue
		}
		if got := errs.CodeOf(err); got != errs.CodeIdentity {
			t.Errorf("Parse(%q): CodeOf() = %d, want %d", data, got, errs.CodeIdentity)
		}
	}
}

// A malformed identity must not be echoed back: it is private key material.
func TestParseErrorDoesNotEchoKeyMaterial(t *testing.T) {
	const marker = "S3CRETMATERIAL"

	_, err := identity.Parse([]byte(identity.KeyPrefix+marker), "$"+identity.EnvVar)
	if err == nil {
		t.Fatal("Parse() = nil, want an error")
	}
	if strings.Contains(rendered(err), marker) {
		t.Errorf("error echoes key material:\n%s", rendered(err))
	}
}

func TestParseIgnoresComments(t *testing.T) {
	home := isolate(t)
	path := filepath.Join(home, identity.Dir, identity.File)
	created := create(t, path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := identity.Parse(data, "test")
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if parsed.PublicKey() != created.PublicKey() {
		t.Error("Parse() did not recover the identity written by Create()")
	}
}

func TestResolveDefaultPath(t *testing.T) {
	home := isolate(t)
	path := filepath.Join(home, identity.Dir, identity.File)
	created := create(t, path)

	resolved, err := identity.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}
	if resolved.PublicKey() != created.PublicKey() {
		t.Error("Resolve() did not find the default identity")
	}
}

func TestResolveEnvPath(t *testing.T) {
	isolate(t)
	elsewhere := filepath.Join(t.TempDir(), "ci-identity")
	created := create(t, elsewhere)
	t.Setenv(identity.EnvVar, elsewhere)

	resolved, err := identity.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}
	if resolved.PublicKey() != created.PublicKey() {
		t.Error("Resolve() ignored " + identity.EnvVar)
	}
}

// CI systems hand the identity over as a secret value rather than a file.
func TestResolveEnvKeyMaterial(t *testing.T) {
	home := isolate(t)
	created := create(t, filepath.Join(home, identity.Dir, identity.File))

	data, err := os.ReadFile(filepath.Join(home, identity.Dir, identity.File))
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, identity.KeyPrefix) {
			key = line
		}
	}
	t.Setenv(identity.EnvVar, "\n"+key+"\n")

	resolved, err := identity.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}
	if resolved.PublicKey() != created.PublicKey() {
		t.Error("Resolve() did not accept key material from " + identity.EnvVar)
	}
	if resolved.Source != "$"+identity.EnvVar {
		t.Errorf("Source = %q, want %q", resolved.Source, "$"+identity.EnvVar)
	}
}

func TestResolveFlagWins(t *testing.T) {
	home := isolate(t)
	create(t, filepath.Join(home, identity.Dir, identity.File))

	envID := filepath.Join(t.TempDir(), "env-identity")
	create(t, envID)
	t.Setenv(identity.EnvVar, envID)

	flagID := filepath.Join(t.TempDir(), "flag-identity")
	wanted := create(t, flagID)

	resolved, err := identity.Resolve(flagID)
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}
	if resolved.PublicKey() != wanted.PublicKey() {
		t.Error("--identity did not take precedence over " + identity.EnvVar)
	}
}

func TestDestination(t *testing.T) {
	home := isolate(t)

	got, err := identity.Destination("")
	if err != nil {
		t.Fatalf("Destination() = %v", err)
	}
	if want := filepath.Join(home, identity.Dir, identity.File); got != want {
		t.Errorf("Destination() = %q, want %q", got, want)
	}

	t.Setenv(identity.EnvVar, "/tmp/ci-identity")
	if got, err = identity.Destination(""); err != nil || got != "/tmp/ci-identity" {
		t.Errorf("Destination() = %q, %v, want the path from "+identity.EnvVar, got, err)
	}

	if got, err = identity.Destination("/tmp/flag"); err != nil || got != "/tmp/flag" {
		t.Errorf("Destination(flag) = %q, %v, want the flag to win", got, err)
	}
}

// Writing a file while the environment supplies key material would silently
// produce an identity that is never used.
func TestDestinationRejectsEnvKeyMaterial(t *testing.T) {
	isolate(t)
	t.Setenv(identity.EnvVar, identity.KeyPrefix+"WHATEVER")

	_, err := identity.Destination("")
	if err == nil {
		t.Fatal("Destination() = nil, want an error")
	}
	if !strings.Contains(rendered(err), identity.EnvVar) {
		t.Errorf("error =\n%s\nwant it to name "+identity.EnvVar, rendered(err))
	}
}
