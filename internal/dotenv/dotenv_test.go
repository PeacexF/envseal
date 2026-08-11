package dotenv_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
)

func parse(t *testing.T, src string) *dotenv.File {
	t.Helper()
	f, err := dotenv.Parse([]byte(src), ".env")
	if err != nil {
		t.Fatalf("Parse(%q) = %v", src, err)
	}
	return f
}

func TestParseValues(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want map[string]string
	}{
		{"simple", "KEY=value\n", map[string]string{"KEY": "value"}},
		{"no trailing newline", "KEY=value", map[string]string{"KEY": "value"}},
		{"empty value", "KEY=\n", map[string]string{"KEY": ""}},
		{"export prefix", "export KEY=value\n", map[string]string{"KEY": "value"}},
		{"spaces around equals", "KEY = value\n", map[string]string{"KEY": "value"}},
		{"trailing spaces", "KEY=value   \n", map[string]string{"KEY": "value"}},
		{"value with equals", "DSN=key=val;x=y\n", map[string]string{"DSN": "key=val;x=y"}},
		{"value with spaces", "MSG=hello world\n", map[string]string{"MSG": "hello world"}},
		{"double quoted", `KEY="value"` + "\n", map[string]string{"KEY": "value"}},
		{"single quoted", "KEY='value'\n", map[string]string{"KEY": "value"}},
		{"quotes preserve spaces", `KEY="  padded  "` + "\n", map[string]string{"KEY": "  padded  "}},
		{"quoted empty", `KEY=""` + "\n", map[string]string{"KEY": ""}},
		{"escapes in double quotes", `KEY="a\nb\tc\\d\"e"`, map[string]string{"KEY": "a\nb\tc\\d\"e"}},
		{"no escapes in single quotes", `KEY='a\nb'`, map[string]string{"KEY": `a\nb`}},
		{"dollar is literal", `KEY="$HOME/bin"`, map[string]string{"KEY": "$HOME/bin"}},
		{"no interpolation", "A=1\nB=${A}\n", map[string]string{"A": "1", "B": "${A}"}},
		{"hash without space is a value", "KEY=#tag\n", map[string]string{"KEY": "#tag"}},
		{"inline comment", "KEY=value # note\n", map[string]string{"KEY": "value"}},
		{"inline comment after quotes", `KEY="a # b" # note`, map[string]string{"KEY": "a # b"}},
		{"only a comment after equals", "KEY= # note\n", map[string]string{"KEY": ""}},
		{"comments and blanks", "# top\n\nKEY=value\n\n  # indented\n", map[string]string{"KEY": "value"}},
		{"crlf", "A=1\r\nB=2\r\n", map[string]string{"A": "1", "B": "2"}},
		{"byte order mark", "\ufeffKEY=value\n", map[string]string{"KEY": "value"}},
		{"multiline double quoted", "KEY=\"line1\nline2\"\nNEXT=2\n", map[string]string{"KEY": "line1\nline2", "NEXT": "2"}},
		{"multiline single quoted", "KEY='line1\nline2'\nNEXT=2\n", map[string]string{"KEY": "line1\nline2", "NEXT": "2"}},
		{"private key block", "K=\"-----BEGIN KEY-----\nabc\n-----END KEY-----\"\n", map[string]string{"K": "-----BEGIN KEY-----\nabc\n-----END KEY-----"}},
		{"dotted name", "app.key=1\n", map[string]string{"app.key": "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parse(t, tt.src).Map()
			if len(got) != len(tt.want) {
				t.Fatalf("Map() = %#v, want %#v", got, tt.want)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("Map()[%q] = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestParsePreservesOrder(t *testing.T) {
	f := parse(t, "ZED=1\nALPHA=2\nMID=3\n")

	if got := f.Keys(); !slices.Equal(got, []string{"ZED", "ALPHA", "MID"}) {
		t.Errorf("Keys() = %v, want source order", got)
	}
}

func TestParseLineNumbers(t *testing.T) {
	f := parse(t, "# comment\n\nA=1\nB=\"multi\nline\"\nC=3\n")

	want := []int{3, 4, 6}
	for i, e := range f.Entries() {
		if e.Line != want[i] {
			t.Errorf("Entries()[%d].Line = %d, want %d", i, e.Line, want[i])
		}
	}
}

func TestParseDuplicateKeys(t *testing.T) {
	f := parse(t, "KEY=first\nKEY=second\n")

	if f.Len() != 2 {
		t.Errorf("Len() = %d, want both assignments kept", f.Len())
	}
	if got, _ := f.Get("KEY"); got != "second" {
		t.Errorf("Get() = %q, want the last assignment", got)
	}
	if got := f.Keys(); !slices.Equal(got, []string{"KEY"}) {
		t.Errorf("Keys() = %v, want one entry", got)
	}
}

func TestBytesArePreservedExactly(t *testing.T) {
	// Encryption uses these bytes, so a parsed file must reproduce its source.
	src := []byte("# comment\nexport A = 1  # note\n\nB='x'\n\r\nC=\"multi\nline\"\n")

	f, err := dotenv.Parse(src, ".env")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.Bytes(), src) {
		t.Errorf("Bytes() = %q, want the source unchanged", f.Bytes())
	}
}

func TestEnviron(t *testing.T) {
	f := parse(t, "A=1\nB=two words\nA=3\n")

	want := []string{"A=3", "B=two words"}
	if got := f.Environ(); !slices.Equal(got, want) {
		t.Errorf("Environ() = %v, want %v", got, want)
	}
}

func TestGetMissing(t *testing.T) {
	f := parse(t, "A=1\n")

	if v, ok := f.Get("MISSING"); ok || v != "" {
		t.Errorf("Get() = %q, %v, want \"\", false", v, ok)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"no equals", "KEY value\n", "expected KEY=value"},
		{"no name", "=value\n", "missing a name"},
		{"name with space", "MY KEY=value\n", "contains a space"},
		{"unterminated double quote", "KEY=\"open\n", "unterminated"},
		{"unterminated single quote", "KEY='open\n", "unterminated"},
		{"text after quoted value", `KEY="a"b` + "\n", "unexpected text"},
		{"nul byte", "KEY=a\x00b\n", "NUL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dotenv.Parse([]byte(tt.src), ".env")
			if err == nil {
				t.Fatal("Parse() = nil, want an error")
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

func TestParseErrorReportsTheRightLine(t *testing.T) {
	_, err := dotenv.Parse([]byte("A=1\n\n# note\nBROKEN\n"), ".env")
	if err == nil {
		t.Fatal("Parse() = nil, want an error")
	}

	var b strings.Builder
	errs.Render(&b, err)
	if !strings.Contains(b.String(), "Line 4") {
		t.Errorf("error =\n%s\nwant it to point at line 4", b.String())
	}
}

// A malformed line may still contain a secret, so errors must not quote it.
func TestParseErrorDoesNotEchoValues(t *testing.T) {
	const secret = "hunter2SUPERSECRET"

	for _, src := range []string{
		"API_KEY " + secret + "\n",
		"API KEY=" + secret + "\n",
		"API_KEY=\"" + secret + "\n",
	} {
		_, err := dotenv.Parse([]byte(src), ".env")
		if err == nil {
			t.Fatalf("Parse(%q) = nil, want an error", src)
		}

		var b strings.Builder
		errs.Render(&b, err)
		if strings.Contains(b.String(), secret) {
			t.Errorf("error echoes the value:\n%s", b.String())
		}
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("KEY=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := dotenv.Load(path)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if got, _ := f.Get("KEY"); got != "value" {
		t.Errorf("Get() = %q, want %q", got, "value")
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := dotenv.Load(filepath.Join(t.TempDir(), ".env"))
	if err == nil {
		t.Fatal("Load() = nil, want an error")
	}
	if got := errs.CodeOf(err); got != errs.CodeConfig {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeConfig)
	}
}

func TestParseEmpty(t *testing.T) {
	f := parse(t, "")

	if f.Len() != 0 {
		t.Errorf("Len() = %d, want 0", f.Len())
	}
	if len(f.Environ()) != 0 {
		t.Errorf("Environ() = %v, want empty", f.Environ())
	}
}
