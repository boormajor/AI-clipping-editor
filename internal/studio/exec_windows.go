//go:build windows

package studio

import (
	"os/exec"
	"strconv"
	"syscall"
)

// configureProcessGroup makes cancellation kill the full process tree with
// taskkill /T, which reaches the node and Chrome processes that Remotion
// starts underneath the CLI. The child is created without a console window
// so no stray terminals flash while rendering.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
