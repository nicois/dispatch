//go:build !windows

package dispatch

import (
	"os/exec"
	"syscall"
)

func killProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL) // Unix-specific
}

func createNewProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
