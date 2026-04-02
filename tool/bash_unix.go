//go:build !windows

package tool

import (
	"os/exec"
	"syscall"
)

// configureSysProcAttr sets process-group isolation and a SIGKILL cancel func
// so that child processes spawned by the command are reliably killed on timeout.
func configureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
