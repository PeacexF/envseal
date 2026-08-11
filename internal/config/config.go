// Package config reads and writes .envseal.yaml, the project's public
// configuration. It must never hold secret values.
package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"gopkg.in/yaml.v3"

	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/safefile"
)

const (
	Filename    = ".envseal.yaml"
	Version     = 1
	DefaultFile = ".env.enc"

	fileMode = 0o644

	header = "# envseal project configuration\n" +
		"# Public information only. Never put secret values in this file.\n\n"
)

type Recipient struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

type Config struct {
	Version    int         `yaml:"version"`
	File       string      `yaml:"file"`
	Recipients []Recipient `yaml:"recipients"`
}

// New returns a configuration with defaults applied and no recipients.
func New() *Config {
	return &Config{Version: Version, File: DefaultFile}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errs.New(errs.CodeConfig, "no configuration at %s", path).
				Detailf("This project has not been initialized for envseal.").
				Check("run `envseal encrypt .env` to create one")
		}
		return nil, errs.New(errs.CodeConfig, "unable to read %s", path).Wrap(err)
	}
	return Parse(data, path)
}

// Parse decodes a configuration, rejecting unknown fields. source names the
// origin of the data for error messages.
func Parse(data []byte, source string) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	// An empty file decodes to io.EOF; let validation report the missing fields.
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return nil, errs.New(errs.CodeConfig, "invalid configuration in %s", source).
			Detailf("%s", yamlDetail(err))
	}
	if err := c.validate(source); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate(source string) error {
	fail := func(format string, args ...any) *errs.Error {
		return errs.New(errs.CodeConfig, "invalid configuration in %s", source).
			Detailf(format, args...)
	}

	switch {
	case c.Version == 0:
		return fail("Missing `version`. Add `version: %d` at the top of the file.", Version)
	case c.Version != Version:
		return fail("Unsupported version %d. This build of envseal understands version %d.", c.Version, Version)
	}

	if c.File == "" {
		c.File = DefaultFile
	}
	if filepath.IsAbs(c.File) || strings.Contains(filepath.ToSlash(c.File), "../") {
		return fail("`file` must be a path inside the project, but is %q.", c.File)
	}

	names := make(map[string]int, len(c.Recipients))
	keys := make(map[string]int, len(c.Recipients))
	for i, r := range c.Recipients {
		switch {
		case r.Name == "":
			return fail("Recipient %d has no `name`.", i+1)
		case r.Key == "":
			return fail("Recipient %q has no `key`.", r.Name)
		}

		if err := ValidateKey(r.Key); err != nil {
			return fail("Recipient %q has an invalid key: %s", r.Name, err)
		}

		lower := strings.ToLower(r.Name)
		if first, dup := names[lower]; dup {
			return fail("Recipients %d and %d share the name %q.", first+1, i+1, r.Name)
		}
		if first, dup := keys[r.Key]; dup {
			return fail("Recipients %q and %q share the same key.", c.Recipients[first].Name, r.Name)
		}
		names[lower] = i
		keys[r.Key] = i
	}

	return nil
}

// ValidateKey reports whether key is a public age recipient.
func ValidateKey(key string) error {
	if !strings.HasPrefix(key, "age1") {
		if strings.HasPrefix(key, "AGE-SECRET-KEY-") {
			return errors.New("that is a private identity, not a public key")
		}
		return errors.New("expected a public key starting with `age1`")
	}
	if _, err := age.ParseX25519Recipient(key); err != nil {
		return errors.New("not a valid age public key")
	}
	return nil
}

// Find returns the index of the recipient matching name or key, or -1.
func (c *Config) Find(nameOrKey string) int {
	for i, r := range c.Recipients {
		if strings.EqualFold(r.Name, nameOrKey) || r.Key == nameOrKey {
			return i
		}
	}
	return -1
}

func (c *Config) Save(path string) error {
	var b bytes.Buffer
	b.WriteString(header)

	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return errs.New(errs.CodeConfig, "unable to encode configuration").Wrap(err)
	}
	if err := enc.Close(); err != nil {
		return errs.New(errs.CodeConfig, "unable to encode configuration").Wrap(err)
	}

	if err := safefile.Write(path, b.Bytes(), fileMode); err != nil {
		return errs.New(errs.CodeConfig, "unable to write %s", path).Wrap(err)
	}
	return nil
}

// yamlDetail turns a yaml error into user-facing text, dropping Go type names.
func yamlDetail(err error) string {
	clean := func(s string) string {
		if i := strings.Index(s, " in type "); i >= 0 {
			s = s[:i]
		}
		return strings.TrimPrefix(s, "yaml: ")
	}

	var te *yaml.TypeError
	if errors.As(err, &te) {
		lines := make([]string, len(te.Errors))
		for i, e := range te.Errors {
			lines[i] = clean(e)
		}
		return strings.Join(lines, "\n")
	}
	return clean(err.Error())
}
