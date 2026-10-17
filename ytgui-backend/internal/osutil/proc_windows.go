//go:build windows

package osutil

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

// Background makes a child process run without its own console window.
func Background(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// KillTree kills the process and every child it started (yt-dlp -> ffmpeg).
func KillTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	Background(kill)
	if err := kill.Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
