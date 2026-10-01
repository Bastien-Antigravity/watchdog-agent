//go:build !windows

package utils

// =============================================================================
// ESSENTIAL PROCESS:
// Unix-specific single-instance filesystem locking implementation using flock.
// Prevents duplicate watchdog-agent instances from executing concurrently.
//
// DATA FLOW:
// 1. Opens or creates .watchdog-agent.lock file descriptor.
// 2. Invokes syscall.Flock with LOCK_EX | LOCK_NB.
// 3. Returns acquired open file handle or error if lock is held.
//
// KEY PARAMETERS:
// - path: Absolute filesystem path to the lock file.
// =============================================================================

import (
	"os"
	"syscall"
)

// -----------------------------------------------------------------------------

// AcquireLock opens and locks the lock file exclusively on Unix-like systems.
func AcquireLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
