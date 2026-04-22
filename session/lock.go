//go:build !windows

package session

import (
	"fmt"
	"os"
	"syscall"
)

// lockFd wraps a file holding an advisory exclusive flock.
// Acquire with acquireLock; release with release().
// The *os.File is kept alive so the Go runtime's fd lifecycle is respected —
// release() calls f.Close() which implicitly drops the flock and closes the fd.
type lockFd struct{ f *os.File }

// acquireLock opens the sidecar lock file at path and blocks until an exclusive
// flock is obtained. The sidecar avoids fd-aliasing with the JSONL append fd.
// path is typically "<session>.jsonl.lock".
func acquireLock(path string) (*lockFd, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock sidecar %s: %w", path, err)
	}
	// Blocking exclusive lock — second caller waits, does not fail.
	// f.Fd() switches to blocking I/O mode, which is correct here.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	return &lockFd{f: f}, nil
}

// release explicitly unlocks then closes. Called via defer — errors swallowed.
// flock(LOCK_UN) is belt-and-suspenders: closing also drops the lock, but
// explicit unlock makes the ordering visible and avoids any close-before-unlock race.
func (l *lockFd) release() {
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
}
