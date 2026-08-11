package dotenv_test

import (
	"testing"

	"github.com/PeacexF/envseal/internal/dotenv"
)

// FuzzSet checks the editing path the way rotate uses it: whatever the file
// looks like, replacing one value must store exactly that value and leave every
// other variable alone.
func FuzzSet(f *testing.F) {
	seeds := []struct{ src, value string }{
		{"TARGET=old\n", "new"},
		{"TARGET=\"old\"\nOTHER=1\n", "with spaces"},
		{"TARGET='old'\n", "has'quote"},
		{"# comment\nTARGET=old # note\n", "p@ss#1"},
		{"TARGET=\"multi\nline\"\n", "flat"},
		{"OTHER=1\n", "appended"},
		{"", ""},
		{"TARGET=old\r\n", "crlf"},
		{"TARGET=old", `back\slash`},
		{"A=1\nTARGET=x\nA=2\n", "\n"},
	}
	for _, s := range seeds {
		f.Add([]byte(s.src), s.value)
	}

	f.Fuzz(func(t *testing.T, data []byte, value string) {
		before, err := dotenv.Parse(data, ".env")
		if err != nil {
			return // malformed input is rejected elsewhere
		}

		// Record the other variables so we can prove they survive.
		untouched := map[string]string{}
		for key, v := range before.Map() {
			if key != "TARGET" {
				untouched[key] = v
			}
		}

		updated, err := before.Set("TARGET", value)
		if err != nil {
			return // an unusable name is rejected, which is allowed
		}

		after, err := dotenv.Parse(updated, ".env")
		if err != nil {
			t.Fatalf("Set produced a file that no longer parses: %q\nerror: %v", updated, err)
		}

		got, ok := after.Get("TARGET")
		if !ok {
			t.Fatalf("TARGET missing after Set: %q", updated)
		}
		if got != value {
			t.Errorf("round trip = %q, want %q\nrendered: %q", got, value, updated)
		}

		for key, want := range untouched {
			if got, ok := after.Get(key); !ok || got != want {
				t.Errorf("%s = %q (present %v), want %q\nrendered: %q", key, got, ok, want, updated)
			}
		}

		// Setting the same value again must be a no-op.
		again, err := after.Set("TARGET", value)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(updated) {
			t.Errorf("Set is not idempotent:\nfirst  %q\nsecond %q", updated, again)
		}
	})
}
