//go:build windows

package utils

// =============================================================================
// ESSENTIAL PROCESS:
// Windows-specific single-instance filesystem locking implementation using CreateFile.
// Prevents duplicate watchdog-agent instances from executing concurrently.
//
// DATA FLOW:
// 1. Opens or creates .watchdog-agent.lock handle with zero share mode.
// 2. Returns acquired open file handle or error if file is already locked.
//
// KEY PARAMETERS:
// - path: Absolute filesystem path to the lock file.
// =============================================================================

import (
	"os"
	"syscall"
)

// -----------------------------------------------------------------------------

// AcquireLock opens and locks the lock file exclusively on Windows using CreateFile.
func AcquireLock(path string) (*os.File, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // 0 share mode = exclusive lock
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
