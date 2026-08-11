package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/config"
	"github.com/PeacexF/envseal/internal/errs"
)

const (
	keyAlice = "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p"
	keyBob   = "age1lggyhqrw2nlhcxprm67z43rta597azn8gknawjehu9d9dl0jq3yqqvfafg"
)

func parse(t *testing.T, yaml string) (*config.Config, error) {
	t.Helper()
	return config.Parse([]byte(yaml), config.Filename)
}

func mustParse(t *testing.T, yaml string) *config.Config {
	t.Helper()
	c, err := parse(t, yaml)
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	return c
}

func TestParse(t *testing.T) {
	c := mustParse(t, `
version: 1
file: .env.enc
recipients:
  - name: alice
    key: `+keyAlice+`
  - name: bob
    key: `+keyBob+`
`)

	if c.Version != 1 {
		t.Errorf("Version = %d, want 1", c.Version)
	}
	if c.File != ".env.enc" {
		t.Errorf("File = %q, want %q", c.File, ".env.enc")
	}
	if len(c.Recipients) != 2 {
		t.Fatalf("len(Recipients) = %d, want 2", len(c.Recipients))
	}
	if c.Recipients[0].Name != "alice" || c.Recipients[0].Key != keyAlice {
		t.Errorf("Recipients[0] = %+v", c.Recipients[0])
	}
}

func TestParseDefaultsFile(t *testing.T) {
	c := mustParse(t, "version: 1\n")
	if c.File != config.DefaultFile {
		t.Errorf("File = %q, want %q", c.File, config.DefaultFile)
	}
}

func TestParseAllowsNoRecipients(t *testing.T) {
	c := mustParse(t, "version: 1\nrecipients: []\n")
	if len(c.Recipients) != 0 {
		t.Errorf("len(Recipients) = %d, want 0", len(c.Recipients))
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string // substring of the rendered error
	}{
		{"empty", "", "version"},
		{"missing version", "file: .env.enc\n", "version"},
		{"unsupported version", "version: 99\n", "version 99"},
		{"unknown field", "version: 1\nrecipient: x\n", "recipient"},
		{"not a mapping", "- 1\n- 2\n", "invalid configuration"},
		{"absolute file", "version: 1\nfile: /etc/passwd\n", "inside the project"},
		{"windows absolute file", "version: 1\nfile: C:\\secrets.enc\n", "inside the project"},
		{"windows unc file", "version: 1\nfile: \\\\host\\share\\x.enc\n", "inside the project"},
		{"escaping file", "version: 1\nfile: ../../secrets.enc\n", "inside the project"},
		{"escaping file mid-path", "version: 1\nfile: a/../../secrets.enc\n", "inside the project"},
		{"escaping file trailing", "version: 1\nfile: a/..\n", "inside the project"},
		{"recipient without name", "version: 1\nrecipients:\n  - key: " + keyAlice + "\n", "no `name`"},
		{"recipient without key", "version: 1\nrecipients:\n  - name: alice\n", "no `key`"},
		{"bare string recipient", "version: 1\nrecipients:\n  - " + keyAlice + "\n", "invalid configuration"},
		{"bad key", "version: 1\nrecipients:\n  - name: alice\n    key: nonsense\n", "age1"},
		{"malformed age key", "version: 1\nrecipients:\n  - name: alice\n    key: age1nope\n", "not a valid age public key"},
		{
			"duplicate name",
			"version: 1\nrecipients:\n  - name: alice\n    key: " + keyAlice + "\n  - name: Alice\n    key: " + keyBob + "\n",
			"share the name",
		},
		{
			"duplicate key",
			"version: 1\nrecipients:\n  - name: alice\n    key: " + keyAlice + "\n  - name: bob\n    key: " + keyAlice + "\n",
			"share the same key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parse(t, tt.yaml)
			if err == nil {
				t.Fatalf("Parse() = %+v, want an error", c)
			}
			if got := errs.CodeOf(err); got != errs.CodeConfig {
				t.Errorf("CodeOf() = %d, want %d", got, errs.CodeConfig)
			}

			var b strings.Builder
			errs.Render(&b, err)
			if !strings.Contains(b.String(), tt.want) {
				t.Errorf("error =\n%s\nwant it to mention %q", b.String(), tt.want)
			}
		})
	}
}

func TestParseRejectsPrivateIdentity(t *testing.T) {
	yaml := "version: 1\nrecipients:\n  - name: alice\n    key: AGE-SECRET-KEY-1QQQQ\n"

	_, err := parse(t, yaml)
	if err == nil {
		t.Fatal("Parse() = nil, want an error")
	}

	var b strings.Builder
	errs.Render(&b, err)
	if !strings.Contains(b.String(), "private identity") {
		t.Errorf("error =\n%s\nwant it to say the key is a private identity", b.String())
	}
}

func TestValidateKey(t *testing.T) {
	if err := config.ValidateKey(keyAlice); err != nil {
		t.Errorf("ValidateKey(valid) = %v", err)
	}
	for _, key := range []string{"", "nonsense", "age1nope", "AGE-SECRET-KEY-1QQQQ"} {
		if err := config.ValidateKey(key); err == nil {
			t.Errorf("ValidateKey(%q) = nil, want an error", key)
		}
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), config.Filename))
	if err == nil {
		t.Fatal("Load() = nil, want an error")
	}
	if got := errs.CodeOf(err); got != errs.CodeConfig {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeConfig)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.Filename)

	want := config.New()
	want.Recipients = []config.Recipient{{Name: "alice", Key: keyAlice}}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got.Version != want.Version || got.File != want.File || len(got.Recipients) != 1 {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if got.Recipients[0] != want.Recipients[0] {
		t.Errorf("Recipients[0] = %+v, want %+v", got.Recipients[0], want.Recipients[0])
	}
}

func TestSaveWritesWarningHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.Filename)
	if err := config.New().Save(path); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# envseal") {
		t.Errorf("file starts with %q, want a header comment", string(data))
	}
	if !strings.Contains(string(data), "Never put secret values") {
		t.Errorf("file =\n%s\nwant a warning about secret values", data)
	}
}

func TestFind(t *testing.T) {
	c := mustParse(t, "version: 1\nrecipients:\n  - name: alice\n    key: "+keyAlice+"\n")

	tests := []struct {
		arg  string
		want int
	}{
		{"alice", 0},
		{"ALICE", 0},
		{keyAlice, 0},
		{"bob", -1},
		{"", -1},
	}
	for _, tt := range tests {
		if got := c.Find(tt.arg); got != tt.want {
			t.Errorf("Find(%q) = %d, want %d", tt.arg, got, tt.want)
		}
	}
}
