package external

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/dotcommander/piglet/ext"
)

// UndoSnapshotsFn retrieves undo snapshots (injected to avoid importing tool/).
type UndoSnapshotsFn func() (map[string][]byte, error)

// Supervisor wraps a Host with crash detection and automatic restart.
// When the extension process exits unexpectedly, the supervisor creates a new
// Host from the same Manifest, re-runs the handshake, and re-bridges registrations.
// Reload() triggers an intentional restart (e.g. binary changed on disk).
type Supervisor struct {
	manifest        *Manifest
	cwd             string
	app             *ext.App
	resolverFn      providerResolverFn
	undoSnapshotsFn UndoSnapshotsFn

	mu   sync.Mutex
	host *Host // current live host; nil between crash and restart

	reloadCh chan struct{} // signals intentional reload (buffered 1)
	stopped  chan struct{}
	stopOnce sync.Once
}

// NewSupervisor creates a supervisor for the given manifest.
func NewSupervisor(m *Manifest, cwd string, app *ext.App, resolver providerResolverFn, undoFn UndoSnapshotsFn) *Supervisor {
	return &Supervisor{
		manifest:        m,
		cwd:             cwd,
		app:             app,
		resolverFn:      resolver,
		undoSnapshotsFn: undoFn,
		reloadCh:        make(chan struct{}, 1),
		stopped:         make(chan struct{}),
	}
}

// Start creates the initial host, performs the handshake, bridges registrations,
// and starts the crash-watch goroutine.
func (s *Supervisor) Start(ctx context.Context) error {
	h := NewHost(s.manifest, s.cwd)
	if err := h.Start(ctx); err != nil {
		return err
	}
	h.SetApp(s.app)
	h.SetProviderResolver(s.resolverFn)
	h.SetUndoSnapshots(s.undoSnapshotsFn)

	s.mu.Lock()
	s.host = h
	s.mu.Unlock()

	bridge(s.app, h)

	go s.watch(ctx)
	return nil
}

// Stop terminates the supervised extension and disables auto-restart.
func (s *Supervisor) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopped)
		s.mu.Lock()
		h := s.host
		s.host = nil
		s.mu.Unlock()
		if h != nil {
			h.Stop()
		}
	})
}

func (s *Supervisor) Name() string { return s.manifest.Name }

// Reload triggers a graceful restart of the extension process.
// The watch goroutine handles the actual restart — this method just signals it.
// Safe for concurrent use; no-op if a reload is already pending.
func (s *Supervisor) Reload() {
	select {
	case s.reloadCh <- struct{}{}:
	default: // already pending
	}
}

const (
	maxRestarts    = 5
	initialBackoff = time.Second
	maxBackoff     = 16 * time.Second
	stableAfter    = 30 * time.Second // reset failure count after this uptime
)

// watch monitors the current host and restarts on crash or intentional reload.
func (s *Supervisor) watch(ctx context.Context) {
	backoff := initialBackoff
	failures := 0

	for {
		s.mu.Lock()
		h := s.host
		s.mu.Unlock()

		var crash bool
		if h == nil {
			// No live host (previous restart failed) — wait for reload signal.
			select {
			case <-s.reloadCh:
			case <-s.stopped:
				return
			case <-ctx.Done():
				return
			}
		} else {
			startedAt := time.Now()

			select {
			case <-h.Closed():
				// Stop() closes s.stopped then calls h.Stop() which closes h.closed.
				// Both may fire simultaneously — check s.stopped to distinguish.
				select {
				case <-s.stopped:
					return
				default:
				}
				crash = true
			case <-s.reloadCh:
				s.mu.Lock()
				s.host = nil
				s.mu.Unlock()
				h.Stop()
			case <-s.stopped:
				return
			case <-ctx.Done():
				return
			}

			if crash {
				if time.Since(startedAt) >= stableAfter {
					failures = 0
					backoff = initialBackoff
				}
				failures++
				if failures > maxRestarts {
					slog.Error("extension restart limit reached",
						"name", s.manifest.Name,
						"attempts", failures)
					s.app.Notify(s.manifest.Name + " crashed and could not be restarted")
					return
				}
				slog.Warn("extension crashed, restarting",
					"name", s.manifest.Name,
					"attempt", failures,
					"backoff", backoff)
				s.app.Notify(s.manifest.Name + " crashed, restarting...")
			} else {
				slog.Info("extension reloading", "name", s.manifest.Name)
				s.app.Notify("Reloading " + s.manifest.Name + "...")
				failures = 0
				backoff = initialBackoff
			}
		}

		s.app.UnregisterExtension(s.manifest.Name)

		if crash {
			t := time.NewTimer(backoff)
			select {
			case <-t.C:
			case <-s.stopped:
				t.Stop()
				return
			case <-ctx.Done():
				t.Stop()
				return
			}
			backoff = min(backoff*2, maxBackoff)
		}

		if err := s.restart(ctx, crash); err != nil {
			if !crash {
				s.app.Notify(s.manifest.Name + " reload failed: " + err.Error())
			}
			s.mu.Lock()
			s.host = nil
			s.mu.Unlock()
			continue
		}

		if crash {
			slog.Info("extension restarted successfully", "name", s.manifest.Name)
		} else {
			slog.Info("extension reloaded", "name", s.manifest.Name)
			s.app.Notify(s.manifest.Name + " reloaded")
		}
	}
}

// restart creates a new Host, starts it, wires it into the app, and sets it as
// the live host. crash is false for intentional reloads (used only for error logging).
func (s *Supervisor) restart(ctx context.Context, crash bool) error {
	newHost := NewHost(s.manifest, s.cwd)
	if err := newHost.Start(ctx); err != nil {
		slog.Error("extension restart failed",
			"name", s.manifest.Name,
			"err", err)
		return err
	}
	newHost.SetApp(s.app)
	newHost.SetProviderResolver(s.resolverFn)
	newHost.SetUndoSnapshots(s.undoSnapshotsFn)

	s.mu.Lock()
	s.host = newHost
	s.mu.Unlock()

	bridge(s.app, newHost)
	return nil
}
