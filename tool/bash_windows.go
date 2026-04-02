//go:build windows

package tool

import "os/exec"

// configureSysProcAttr is a no-op on Windows.
// Process group isolation (Setpgid) is not supported on this platform.
func configureSysProcAttr(_ *exec.Cmd) {}
