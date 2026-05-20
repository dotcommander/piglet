//go:build darwin || linux

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
	f, err := os.OpenFile(lockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// Non-blocking exclusive lock. Fd() returns uintptr; on darwin/linux a real
	// file descriptor is a small non-negative int that fits in int — the
	// conversion cannot overflow in practice.
	fd := int(f.Fd()) //nolint:gosec // G115: fd is a small non-negative int from the kernel
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("piglet-cron already running (lock held)")
	}

	// Write PID for debugging. Truncation/write failures are not fatal — the
	// lock is held regardless of PID file contents.
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())

	return &Lock{file: f}, nil
}

// Release releases the file lock.
func (l *Lock) Release() {
	if l.file != nil {
		fd := int(l.file.Fd()) //nolint:gosec // G115: fd is a small non-negative int from the kernel
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = l.file.Close()
		_ = os.Remove(lockPath())
	}
}
