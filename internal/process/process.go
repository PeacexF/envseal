// Package process runs a child program with a supplied environment, forwarding
// signals to it and reporting its exit code.
package process

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"github.com/PeacexF/envseal/internal/errs"
)

type Options struct {
	Args   []string
	Env    []string
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Run executes the command and waits for it, returning its exit code. A child
// that fails is not an error here: its status is the caller's to propagate.
func Run(opts Options) (int, error) {
	if len(opts.Args) == 0 {
		return 0, errs.New(errs.CodeProcess, "no command to run")
	}

	cmd := exec.Command(opts.Args[0], opts.Args[1:]...)
	cmd.Env = opts.Env
	cmd.Dir = opts.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = opts.Stdin, opts.Stdout, opts.Stderr
	configure(cmd)

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return 0, errs.New(errs.CodeProcess, "cannot run %s", opts.Args[0]).
				Detailf("The command was not found in PATH.").
				Check("check the spelling", "confirm the program is installed")
		}
		return 0, errs.New(errs.CodeProcess, "cannot run %s", opts.Args[0]).Wrap(err)
	}

	stop := forward(cmd)
	defer stop()

	err := cmd.Wait()
	if err == nil {
		return 0, nil
	}

	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		if code := exit.ExitCode(); code >= 0 {
			return code, nil
		}
		return signalExitCode(exit), nil // killed by a signal
	}
	return 0, errs.New(errs.CodeProcess, "%s failed", opts.Args[0]).Wrap(err)
}

// forward relays signals to the child until the returned function is called.
// The child runs in its own process group, so it never receives a terminal
// signal directly: everything reaches it through here.
func forward(cmd *exec.Cmd) func() {
	incoming := make(chan os.Signal, 1)
	signal.Notify(incoming, forwarded...)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-incoming:
				signalChild(cmd, sig)
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(incoming)
		close(done)
	}
}
