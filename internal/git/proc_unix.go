//go:build unix

package git

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// killGrace is how long a cancelled git has to exit after SIGTERM before its
// process group is killed. Git removes its lock files on SIGTERM; a SIGKILL
// in the middle of a ref update would leave them behind. It stays below
// waitDelay so that the group is killed before exec gives up on the pipes.
const killGrace = 2 * time.Second

// isolate starts git in its own session. On cancel it sends SIGTERM to the
// whole process group, so helpers such as ssh or credential prompts cannot
// outlive git, and SIGKILL to the group if it is still running after
// killGrace. The returned release must be called once the command has been
// waited for: it stops a kill that is still pending.
func isolate(cmd *exec.Cmd) (release func()) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	g := &procGroup{}
	cmd.Cancel = func() error { return g.terminate(cmd.Process.Pid) }

	return g.release
}

// procGroup stops the process group of one command.
type procGroup struct {
	mu sync.Mutex
	// kill is the pending SIGKILL; waited is set once the command was
	// waited for, after which the group is never signalled again.
	kill   *time.Timer
	waited bool
}

// terminate asks the group pgid to stop and schedules its kill.
func (g *procGroup) terminate(pgid int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.waited {
		return os.ErrProcessDone
	}

	if err := signalGroup(pgid, syscall.SIGTERM); err != nil {
		return err
	}

	g.kill = time.AfterFunc(killGrace, func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		if !g.waited {
			_ = signalGroup(pgid, syscall.SIGKILL) // the group may have exited meanwhile
		}
	})

	return nil
}

// release stops the pending kill once the command has exited.
func (g *procGroup) release() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.waited = true

	if g.kill != nil {
		g.kill.Stop()
	}
}

// signalGroup sends sig to the process group pgid. A group that no longer
// exists is reported as os.ErrProcessDone, so that exec does not turn a git
// that exited as the context ended into an error.
func signalGroup(pgid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pgid, sig); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}

		return err
	}

	return nil
}
