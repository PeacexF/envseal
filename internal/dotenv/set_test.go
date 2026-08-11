package dotenv_test

import (
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/dotenv"
)

func set(t *testing.T, src, key, value string) string {
	t.Helper()

	out, err := parse(t, src).Set(key, value)
	if err != nil {
		t.Fatalf("Set(%q, %q) = %v", key, value, err)
	}
	return string(out)
}

// Everything except the one value must survive byte for byte.
func TestSetPreservesTheRestOfTheFile(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		key, value string
		want       string
	}{
		{
			"simple", "A=1\n", "A", "2", "A=2\n",
		},
		{
			"keeps other lines",
			"# top\nA=1\n\nB=2\n# end\n", "A", "changed",
			"# top\nA=changed\n\nB=2\n# end\n",
		},
		{
			"keeps the export prefix and spacing",
			"export A = old\n", "A", "new",
			"export A = new\n",
		},
		{
			"keeps an inline comment",
			"A=old # keep me\n", "A", "new",
			"A=new # keep me\n",
		},
		{
			"keeps double quotes",
			`A="old"` + "\n", "A", "new",
			`A="new"` + "\n",
		},
		{
			"keeps single quotes",
			"A='old'\n", "A", "new",
			"A='new'\n",
		},
		{
			"replaces a multiline value",
			"A=\"line1\nline2\"\nB=2\n", "A", "flat",
			"A=\"flat\"\nB=2\n",
		},
		{
			"empty value",
			"A=\n", "A", "filled",
			"A=filled\n",
		},
		{
			"becomes empty",
			"A=something\n", "A", "",
			"A=\n",
		},
		{
			"last assignment wins",
			"A=first\nA=second\n", "A", "third",
			"A=first\nA=third\n",
		},
		{
			"appends an unknown key",
			"A=1\n", "NEW", "value",
			"A=1\nNEW=value\n",
		},
		{
			"appends a newline first when the file lacks one",
			"A=1", "NEW", "value",
			"A=1\nNEW=value\n",
		},
		{
			"appends to an empty file",
			"", "NEW", "value",
			"NEW=value\n",
		},
		{
			"crlf lines are untouched",
			"A=1\r\nB=2\r\n", "B", "3",
			"A=1\r\nB=3\r\n",
		},
		{
			"value with spaces gets quoted",
			"A=old\n", "A", "two words",
			`A="two words"` + "\n",
		},
		{
			"value with a hash gets quoted",
			"A=old\n", "A", "p@ss#1",
			`A="p@ss#1"` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := set(t, tt.src, tt.key, tt.value); got != tt.want {
				t.Errorf("Set() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// Whatever goes in must come back out, however awkward the value.
func TestSetRoundTrip(t *testing.T) {
	values := []string{
		"simple",
		"",
		"two words",
		"trailing space ",
		" leading space",
		"has#hash",
		"has # spaced hash",
		`has"double`,
		"has'single",
		`has\backslash`,
		"has\nnewline",
		"has\ttab",
		"has\r\nwindows",
		"-----BEGIN KEY-----\nabc\n-----END KEY-----",
		"postgres://user:p@ss@host:5432/db?sslmode=require",
		"$NOT_INTERPOLATED",
		"${ALSO_NOT}",
		`{"json":"value"}`,
		"=leading equals",
		"#leading hash",
		`"quoted looking"`,
		"'single looking'",
		strings.Repeat("long", 500),
	}

	// Every starting style, since Set tries to preserve it.
	sources := map[string]string{
		"bare":   "TARGET=old\nOTHER=keep\n",
		"double": "TARGET=\"old\"\nOTHER=keep\n",
		"single": "TARGET='old'\nOTHER=keep\n",
		"absent": "OTHER=keep\n",
	}

	for style, src := range sources {
		for _, want := range values {
			t.Run(style+"/"+shortName(want), func(t *testing.T) {
				updated := set(t, src, "TARGET", want)

				reparsed, err := dotenv.Parse([]byte(updated), ".env")
				if err != nil {
					t.Fatalf("Parse(%q) = %v", updated, err)
				}

				got, ok := reparsed.Get("TARGET")
				if !ok {
					t.Fatalf("TARGET missing from %q", updated)
				}
				if got != want {
					t.Errorf("round trip = %q, want %q\nrendered as: %s", got, want, updated)
				}
				if other, _ := reparsed.Get("OTHER"); other != "keep" {
					t.Errorf("OTHER = %q, want it untouched", other)
				}
			})
		}
	}
}

func TestSetRejectsBadNames(t *testing.T) {
	for _, key := range []string{"", "has space", "has\ttab", "has\nnewline"} {
		if _, err := parse(t, "A=1\n").Set(key, "value"); err == nil {
			t.Errorf("Set(%q) = nil, want an error", key)
		}
	}
}

func TestSetKeepsByteOrderMark(t *testing.T) {
	got := set(t, "\ufeffA=1\n", "A", "2")

	if !strings.HasPrefix(got, "\ufeff") {
		t.Errorf("Set() = %q, want the byte order mark preserved", got)
	}
	if got != "\ufeffA=2\n" {
		t.Errorf("Set() = %q, want %q", got, "\ufeffA=2\n")
	}
}

func TestHas(t *testing.T) {
	f := parse(t, "A=1\n")

	if !f.Has("A") {
		t.Error("Has(A) = false, want true")
	}
	if f.Has("MISSING") {
		t.Error("Has(MISSING) = true, want false")
	}
}

func shortName(s string) string {
	name := strings.Map(func(r rune) rune {
		if r < 0x20 || r == ' ' || r == '/' {
			return '_'
		}
		return r
	}, s)
	if len(name) > 24 {
		name = name[:24]
	}
	if name == "" {
		name = "empty"
	}
	return name
}
