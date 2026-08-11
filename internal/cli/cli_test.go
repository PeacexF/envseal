package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/cli"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = cli.Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// runInput drives a command that asks for confirmation, with a terminal-like
// stdin supplying the answer.
func runInput(t *testing.T, input string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = cli.RunStreams(args, cli.Streams{In: strings.NewReader(input), Out: &out, Err: &errOut, Interactive: true})
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	code, stdout, stderr := run(t, "--version")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.HasPrefix(stdout, "envseal ") {
		t.Errorf("stdout = %q, want it to start with %q", stdout, "envseal ")
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {}} {
		code, stdout, stderr := run(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit = %d, want 0 (stderr: %s)", args, code, stderr)
		}
		if !strings.Contains(stdout, "Usage:") {
			t.Errorf("%v: stdout = %q, want usage", args, stdout)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, stderr := run(t, "nope")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr, "Error: ") {
		t.Errorf("stderr = %q, want a rendered error", stderr)
	}
}
