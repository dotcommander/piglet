//go:build windows

package session

// Windows gap: syscall.Flock is not available on Windows. Session file locking
// is a no-op on this platform. Concurrent multi-process writes to the same
// session file are unsafe on Windows. LockFileEx-based implementation deferred —
// Windows is not a primary piglet platform.

type lockFd struct{}

func acquireLock(_ string) (*lockFd, error) { return &lockFd{}, nil }
func (l *lockFd) release()                  {}
