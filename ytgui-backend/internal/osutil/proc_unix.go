//go:build !windows

package osutil

import (
	"os/exec"
	"syscall"
	"time"
)

// Background puts the child in its own process group so KillTree can
// signal it together with anything it spawns.
func Background(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillTree sends SIGTERM to the process group, then SIGKILL after 3 s.
func KillTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pgid := -cmd.Process.Pid
	if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil {
		return cmd.Process.Kill()
	}
	go func() {
		time.Sleep(3 * time.Second)
		_ = syscall.Kill(pgid, syscall.SIGKILL)
	}()
	return nil
}
