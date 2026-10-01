//go:build !windows

package utils

// =============================================================================
// ESSENTIAL PROCESS:
// Unix-specific process group management and signal dispatching for clean process
// tree termination.
//
// DATA FLOW:
// 1. Sets pgid on spawned child processes.
// 2. Transmits SIGKILL to the negative process group ID to terminate sub-trees.
//
// KEY PARAMETERS:
// - cmd: OS exec.Cmd handle representing target child process.
// =============================================================================

import (
	"os/exec"
	"syscall"
)

// -----------------------------------------------------------------------------

// SetSysProcAttrGroup sets the process group attributes for Unix-like systems
func SetSysProcAttrGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillProcessGroup forcefully terminates a process and all of its subprocesses (process group)
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		_ = cmd.Process.Kill()
	}
}

// KillProcessGroupByID forcefully terminates a process group using its PID
func KillProcessGroupByID(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
