//go:build darwin

package cron

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func lockPath() string {
	return filepath.Join(os.TempDir(), "piglet-cron.lock")
}

// Lock represents a file lock.
type Lock struct {
	file *os.File
}

// Acquire attempts to get an exclusive lock. Returns error if already locked.
func Acquire() (*Lock, error) {
	f, err := os.OpenFile(lockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// Non-blocking exclusive lock.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("piglet-cron already running (lock held)")
	}

	// Write PID for debugging.
	f.Truncate(0)
	fmt.Fprintf(f, "%d\n", os.Getpid())

	return &Lock{file: f}, nil
}

// Release releases the file lock.
func (l *Lock) Release() {
	if l.file != nil {
		syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
		l.file.Close()
		os.Remove(lockPath())
	}
}
