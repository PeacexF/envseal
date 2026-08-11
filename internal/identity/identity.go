// Package identity manages the local age identity:
// the private key that decrypts a project's environment.
//
// Private key material never appears in errors, warnings, or String output.
// Callers can print anything this package returns.
package identity

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/safefile"
)

const (
	Dir       = ".envseal"
	File      = "identity"
	EnvVar    = "ENVSEAL_IDENTITY"
	KeyPrefix = "AGE-SECRET-KEY-"

	fileMode = 0o600
	dirMode  = 0o700
)

var ErrNotFound = errors.New("no identity found")

type Identity struct {
	// Source describes where the identity came from, for display.
	Source string
	// Warnings are non-fatal problems worth telling the user about.
	Warnings []string

	primary *age.X25519Identity
	all     []age.Identity
}

// PublicKey returns the shareable age recipient string.
func (i *Identity) PublicKey() string { return i.primary.Recipient().String() }

// Recipient returns the identity's own public key, for encrypting to self.
func (i *Identity) Recipient() *age.X25519Recipient { return i.primary.Recipient() }

// Identities returns every key in the source, for attempting decryption.
func (i *Identity) Identities() []age.Identity { return i.all }

// String returns the public key, so formatting an Identity cannot leak the private key.
func (i *Identity) String() string { return i.PublicKey() }

// DefaultPath is ~/.envseal/identity.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errs.New(errs.CodeIdentity, "unable to determine your home directory").
			Wrap(err).
			Check("set "+EnvVar+" to the path of your identity file", "pass --identity <path>")
	}
	return filepath.Join(home, Dir, File), nil
}

func Generate() (*Identity, error) {
	key, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, errs.New(errs.CodeIdentity, "unable to generate an identity").Wrap(err)
	}
	return &Identity{Source: "generated", primary: key, all: []age.Identity{key}}, nil
}

// Create generates an identity and writes it to path with 0600 permissions. An
// existing identity is only replaced when force is set, and is then moved to a
// timestamped backup rather than destroyed: losing a private key means losing
// access to everything encrypted for it. The backup path is returned when one
// was made.
func Create(path string, force bool) (*Identity, string, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil && !force:
		return nil, "", errs.New(errs.CodeIdentity, "an identity already exists at %s", path).
			Detailf("Replacing it would permanently lose access to everything encrypted for it.").
			Check("run `envseal keys generate --force` to replace it, keeping a backup of the old one",
				"pass --identity <path> to create a second identity elsewhere")
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return nil, "", errs.New(errs.CodeIdentity, "unable to inspect %s", path).Wrap(err)
	}

	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return nil, "", errs.New(errs.CodeIdentity, "unable to create %s", filepath.Dir(path)).Wrap(err)
	}

	var backup string
	if err == nil {
		backup = fmt.Sprintf("%s.%s.bak", path, time.Now().UTC().Format("20060102T150405Z"))
		if err := os.Rename(path, backup); err != nil {
			return nil, "", errs.New(errs.CodeIdentity, "unable to back up the existing identity").Wrap(err)
		}
	}

	id, err := Generate()
	if err != nil {
		return nil, "", err
	}
	id.Source = path

	if err := safefile.Write(path, id.marshal(), fileMode); err != nil {
		return nil, "", errs.New(errs.CodeIdentity, "unable to write %s", path).Wrap(err)
	}
	return id, backup, nil
}

// marshal renders the age-keygen file format.
func (i *Identity) marshal() []byte {
	return fmt.Appendf(nil, "# created: %s\n# public key: %s\n%s\n",
		time.Now().UTC().Format(time.RFC3339), i.PublicKey(), i.primary)
}

func Load(path string) (*Identity, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errs.New(errs.CodeIdentity, "no identity at %s", path).
				Detailf("An identity is the private key that decrypts your environment.").
				Check("run `envseal keys generate` to create one",
					"set "+EnvVar+" if your identity lives elsewhere").
				Wrap(ErrNotFound)
		}
		return nil, errs.New(errs.CodeIdentity, "unable to read %s", path).Wrap(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.New(errs.CodeIdentity, "unable to read %s", path).Wrap(err)
	}

	id, err := Parse(data, path)
	if err != nil {
		return nil, err
	}
	if w := permissionWarning(path, info.Mode()); w != "" {
		id.Warnings = append(id.Warnings, w)
	}
	return id, nil
}

// Parse reads identities from age key file content. source names the origin for
// error messages; the content itself is never included in an error, because it
// is private key material.
func Parse(data []byte, source string) (*Identity, error) {
	invalid := func() error {
		return errs.New(errs.CodeIdentity, "no usable identity in %s", source).
			Detailf("Expected an age private key beginning with %s.", KeyPrefix).
			Check("regenerate it with `envseal keys generate`",
				"confirm you copied the private identity, not the public key")
	}

	keys, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil {
		return nil, invalid()
	}

	id := &Identity{Source: source, all: keys}
	for _, k := range keys {
		if x, ok := k.(*age.X25519Identity); ok {
			id.primary = x
			break
		}
	}
	if id.primary == nil {
		return nil, invalid()
	}
	return id, nil
}

// Resolve loads the identity to use: the --identity flag, then ENVSEAL_IDENTITY
// (a path, or key material for CI), then ~/.envseal/identity.
func Resolve(flagPath string) (*Identity, error) {
	if flagPath != "" {
		return Load(flagPath)
	}

	if v, ok := lookupEnv(); ok {
		if isKeyMaterial(v) {
			return Parse([]byte(v), "$"+EnvVar)
		}
		return Load(v)
	}

	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Load(path)
}

// Destination reports where `envseal keys generate` should write a new identity.
func Destination(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}

	if v, ok := lookupEnv(); ok {
		if isKeyMaterial(v) {
			return "", errs.New(errs.CodeIdentity, "%s already holds an identity", EnvVar).
				Detailf("It contains key material rather than a path, so a newly created file would be ignored.").
				Check("unset "+EnvVar+" to create an identity on disk",
					"pass --identity <path> to choose where to write it")
		}
		return v, nil
	}
	return DefaultPath()
}

func lookupEnv() (string, bool) {
	v := strings.TrimSpace(os.Getenv(EnvVar))
	return v, v != ""
}

func isKeyMaterial(v string) bool {
	return strings.HasPrefix(strings.ToUpper(v), KeyPrefix)
}

func permissionWarning(path string, mode fs.FileMode) string {
	if runtime.GOOS == "windows" || mode.Perm()&0o077 == 0 {
		return ""
	}
	return fmt.Sprintf("%s is readable by other users (mode %04o); run `chmod 600 %s`",
		path, mode.Perm(), path)
}
