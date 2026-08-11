//go:build !windows

package process_test

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/PeacexF/envseal/internal/process"
)

// An interrupt must reach the child, and its death must be reported as the
// shell does: 128 plus the signal number.
func TestRunForwardsInterrupt(t *testing.T) {
	// Keep the test binary alive when the signal arrives.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGINT)
	defer signal.Stop(guard)

	done := make(chan int, 1)
	go func() {
		code, err := process.Run(process.Options{Args: shell("sleep 30")})
		if err != nil {
			t.Error(err)
		}
		done <- code
	}()

	time.Sleep(500 * time.Millisecond)
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}

	select {
	case code := <-done:
		if code != 128+int(syscall.SIGINT) {
			t.Errorf("exit = %d, want %d", code, 128+int(syscall.SIGINT))
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the child was never interrupted")
	}
}
