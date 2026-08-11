package process_test

import (
	"bytes"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/PeacexF/envseal/internal/errs"
	"github.com/PeacexF/envseal/internal/process"
)

// shell wraps a one-liner for the platform's shell.
func shell(script string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", script}
	}
	return []string{"sh", "-c", script}
}

func echoVar(name string) []string {
	if runtime.GOOS == "windows" {
		return shell("echo %" + name + "%")
	}
	return shell("printf %s \"$" + name + "\"")
}

func TestRunExitCode(t *testing.T) {
	for _, want := range []int{0, 1, 7, 42} {
		code, err := process.Run(process.Options{Args: shell("exit " + strconv.Itoa(want))})
		if err != nil {
			t.Fatalf("Run() = %v", err)
		}
		if code != want {
			t.Errorf("exit = %d, want %d", code, want)
		}
	}
}

func TestRunPassesEnvironment(t *testing.T) {
	var out bytes.Buffer

	code, err := process.Run(process.Options{
		Args:   echoVar("ENVSEAL_TEST_VALUE"),
		Env:    append(os.Environ(), "ENVSEAL_TEST_VALUE=from-envseal"),
		Stdout: &out,
	})
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := strings.TrimSpace(out.String()); got != "from-envseal" {
		t.Errorf("child saw %q, want %q", got, "from-envseal")
	}
}

func TestRunConnectsStdio(t *testing.T) {
	var stdout, stderr bytes.Buffer

	script := "echo out; echo err 1>&2"
	if runtime.GOOS == "windows" {
		script = "echo out& echo err 1>&2"
	}

	if _, err := process.Run(process.Options{
		Args:   shell(script),
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	if !strings.Contains(stdout.String(), "out") {
		t.Errorf("stdout = %q, want the child's output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "err") {
		t.Errorf("stderr = %q, want the child's error output", stderr.String())
	}
}

func TestRunStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell differences")
	}
	var out bytes.Buffer

	if _, err := process.Run(process.Options{
		Args:   shell("cat"),
		Stdin:  strings.NewReader("piped input"),
		Stdout: &out,
	}); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if out.String() != "piped input" {
		t.Errorf("child read %q, want %q", out.String(), "piped input")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	_, err := process.Run(process.Options{Args: []string{"envseal-does-not-exist"}})
	if err == nil {
		t.Fatal("Run() = nil, want an error")
	}
	if got := errs.CodeOf(err); got != errs.CodeProcess {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeProcess)
	}
}

func TestRunNoCommand(t *testing.T) {
	if _, err := process.Run(process.Options{}); err == nil {
		t.Fatal("Run() = nil, want an error")
	}
}
