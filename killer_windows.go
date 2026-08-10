//go:build windows

package dispatch

import (
	"os/exec" // Or use a dedicated library for process management on Windows
	"strconv"
)

func killProcess(pid int) error {
	// Implement Windows-specific process termination, e.g., using taskkill
	cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}

func createNewProcessGroup(cmd *exec.Cmd) {
}
