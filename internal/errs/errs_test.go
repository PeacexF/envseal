package errs_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/errs"
)

func TestCodeOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want errs.Code
	}{
		{"nil", nil, errs.CodeOK},
		{"plain", errors.New("boom"), errs.CodeGeneral},
		{"typed", errs.New(errs.CodeConfig, "bad config"), errs.CodeConfig},
		{"wrapped", fmt.Errorf("context: %w", errs.New(errs.CodeIdentity, "no identity")), errs.CodeIdentity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errs.CodeOf(tt.err); got != tt.want {
				t.Errorf("CodeOf() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	err := errs.New(errs.CodeCrypto, "unable to decrypt %s", ".env.enc").
		Detailf("The encrypted file was not encrypted for the current identity.").
		Check("~/.envseal/identity", ".envseal.yaml")

	var b strings.Builder
	errs.Render(&b, err)

	want := "Error: unable to decrypt .env.enc\n" +
		"\nThe encrypted file was not encrypted for the current identity.\n" +
		"\nCheck:\n" +
		"  • ~/.envseal/identity\n" +
		"  • .envseal.yaml\n"
	if got := b.String(); got != want {
		t.Errorf("Render() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderPlain(t *testing.T) {
	var b strings.Builder
	errs.Render(&b, errors.New("boom"))
	if got, want := b.String(), "Error: boom\n"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderNil(t *testing.T) {
	var b strings.Builder
	errs.Render(&b, nil)
	if b.Len() != 0 {
		t.Errorf("Render(nil) wrote %q, want nothing", b.String())
	}
}

func TestWrapUnwrap(t *testing.T) {
	cause := errors.New("permission denied")
	err := errs.New(errs.CodeIdentity, "unable to read identity").Wrap(cause)

	if !errors.Is(err, cause) {
		t.Error("errors.Is() = false, want true")
	}
	if got, want := err.Error(), "unable to read identity: permission denied"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
