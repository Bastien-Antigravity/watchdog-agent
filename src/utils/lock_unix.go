//go:build !windows

package utils

import (
	"os"
	"syscall"
)

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
