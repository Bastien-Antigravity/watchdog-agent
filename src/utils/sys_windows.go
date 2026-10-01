//go:build windows

package utils

// =============================================================================
// ESSENTIAL PROCESS:
// Windows-specific process tree management and taskkill termination.
//
// DATA FLOW:
// 1. Sets CREATE_NEW_PROCESS_GROUP creation flags on spawned processes.
// 2. Invokes taskkill /F /T /PID to tear down entire Windows process hierarchy.
//
// KEY PARAMETERS:
// - cmd: OS exec.Cmd handle representing target child process.
// =============================================================================

import (
	"fmt"
	"os/exec"
	"syscall"
)

// -----------------------------------------------------------------------------

// SetSysProcAttrGroup sets the process group attributes for Windows systems
func SetSysProcAttrGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// KillProcessGroup forcefully terminates a process and all of its subprocesses (process tree)
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	KillProcessGroupByID(cmd.Process.Pid)
}

// KillProcessGroupByID forcefully terminates a process group using its PID
func KillProcessGroupByID(pid int) {
	killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	_ = killCmd.Run()
}
