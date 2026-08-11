package process

import (
	"os"
	"os/exec"
)

var forwarded = []os.Signal{os.Interrupt}

func configure(*exec.Cmd) {}

// signalChild kills the child: Windows has no signal to forward, and leaving a
// program running after envseal has been interrupted would strand it with the
// decrypted environment.
func signalChild(cmd *exec.Cmd, _ os.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func signalExitCode(*exec.ExitError) int { return 1 }
