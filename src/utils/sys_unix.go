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
	"time"
)

// -----------------------------------------------------------------------------

// SetSysProcAttrGroup sets the process group attributes for Unix-like systems
func SetSysProcAttrGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillProcessGroup terminates a process and all of its subprocesses (process group),
// attempting graceful SIGTERM first before sending SIGKILL [WD-04].
func KillProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		// Give process tree a brief window to flush buffers and exit cleanly
		done := make(chan struct{})
		go func() {
			for i := 0; i < 20; i++ {
				time.Sleep(100 * time.Millisecond)
				if err := syscall.Kill(-pgid, 0); err != nil {
					close(done)
					return
				}
			}
			close(done)
		}()
		<-done
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond)
		_ = cmd.Process.Kill()
	}
}

// KillProcessGroupByID terminates a process group using its PID, trying SIGTERM before SIGKILL
func KillProcessGroupByID(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(300 * time.Millisecond)
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
