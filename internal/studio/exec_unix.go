//go:build !windows

package studio

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup starts the child in its own process group and kills
// the whole group on cancellation, so ffmpeg or Chrome spawned by node die
// with their parent.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid addresses the process group.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
