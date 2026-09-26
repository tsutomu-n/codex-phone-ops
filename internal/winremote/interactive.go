package winremote

import (
	"github.com/tsutomu-n/codex-phone-ops/internal/control"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// The local SSH process owns the terminal during handoff. Signals never trigger
// a remote repair or retry. Restore termios even after abnormal child exit.
func interactive(cmd *exec.Cmd, t *control.Terminal) (error, error) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	if e := cmd.Start(); e != nil {
		return e, t.Restore()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case e := <-done:
			return e, t.Restore()
		case s := <-signals:
			_ = cmd.Process.Signal(s)
		}
	}
}
