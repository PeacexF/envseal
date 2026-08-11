package dotenv_test

import (
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/dotenv"
	"github.com/PeacexF/envseal/internal/errs"
)

// FuzzParse checks that arbitrary input either parses or fails cleanly, and
// that a parsed file never invents or mangles a variable name.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"", "KEY=value\n", "export KEY = value # note\n",
		`KEY="a\nb"`, "KEY='literal'\n", "KEY=\"multi\nline\"\n",
		"# comment\n\n", "KEY=\r\n", "\ufeffKEY=1", "A=1\nA=2\n",
		"KEY=", "=value", "KEY", `KEY="unterminated`, "KEY=a\x00b",
		"K==v", "K=#tag", "a.b-c_d=1", strings.Repeat("K=v\n", 50),
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := dotenv.Parse(data, ".env")
		if err != nil {
			if got := errs.CodeOf(err); got != errs.CodeConfig {
				t.Errorf("CodeOf() = %d, want %d", got, errs.CodeConfig)
			}
			return
		}

		for _, e := range file.Entries() {
			switch {
			case e.Key == "":
				t.Error("parsed an entry with an empty name")
			case strings.ContainsAny(e.Key, "= \t\n"):
				t.Errorf("parsed an unusable name %q", e.Key)
			case strings.ContainsRune(e.Value, 0):
				t.Errorf("parsed a NUL byte into the value of %q", e.Key)
			case e.Line < 1:
				t.Errorf("entry %q has line %d", e.Key, e.Line)
			}
		}

		// Environ must be usable by os/exec: one '=' separating name and value.
		for _, entry := range file.Environ() {
			name, _, ok := strings.Cut(entry, "=")
			if !ok || name == "" {
				t.Errorf("Environ() produced %q", entry)
			}
		}

		if string(file.Bytes()) != string(data) {
			t.Error("Bytes() does not return the source unchanged")
		}
	})
}
