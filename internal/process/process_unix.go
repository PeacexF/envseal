//go:build !windows

package process

import (
	"os"
	"os/exec"
	"syscall"
)

var forwarded = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}

// configure puts the child in its own process group, so a signal reaches the
// whole tree it spawns rather than only the program envseal started.
func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalChild(cmd *exec.Cmd, sig os.Signal) {
	s, ok := sig.(syscall.Signal)
	if !ok || cmd.Process == nil {
		return
	}
	// A negative pid addresses the process group.
	if err := syscall.Kill(-cmd.Process.Pid, s); err != nil {
		_ = cmd.Process.Signal(sig)
	}
}

// signalExitCode follows the shell convention of 128 plus the signal number.
func signalExitCode(exit *exec.ExitError) int {
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return 1
}
