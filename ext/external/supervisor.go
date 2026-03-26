package external

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/dotcommander/piglet/ext"
)

// Supervisor wraps a Host with crash detection and automatic restart.
// When the extension process exits unexpectedly, the supervisor creates a new
// Host from the same Manifest, re-runs the handshake, and re-bridges registrations.
type Supervisor struct {
	manifest   *Manifest
	cwd        string
	app        *ext.App
	resolverFn providerResolverFn

	mu   sync.Mutex
	host *Host // current live host; nil between crash and restart

	stopped  chan struct{}
	stopOnce sync.Once
}

// NewSupervisor creates a supervisor for the given manifest.
func NewSupervisor(m *Manifest, cwd string, app *ext.App, resolver providerResolverFn) *Supervisor {
	return &Supervisor{
		manifest:   m,
		cwd:        cwd,
		app:        app,
		resolverFn: resolver,
		stopped:    make(chan struct{}),
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

const (
	maxRestarts    = 5
	initialBackoff = time.Second
	maxBackoff     = 16 * time.Second
	stableAfter    = 30 * time.Second // reset failure count after this uptime
)

// watch monitors the current host and restarts on crash.
func (s *Supervisor) watch(ctx context.Context) {
	backoff := initialBackoff
	failures := 0

	for {
		s.mu.Lock()
		h := s.host
		s.mu.Unlock()
		if h == nil {
			return
		}

		startedAt := time.Now()

		// Block until crash, manual stop, or context cancellation.
		select {
		case <-h.Closed():
			// Stop() closes s.stopped then calls h.Stop() which closes h.closed.
			// Both may fire simultaneously — check s.stopped to distinguish
			// a manual stop from an unexpected crash.
			select {
			case <-s.stopped:
				return
			default:
			}
		case <-s.stopped:
			return
		case <-ctx.Done():
			return
		}

		// If the host was stable for long enough, reset failure tracking.
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

		// Remove stale registrations from the crashed host.
		s.app.UnregisterExtension(s.manifest.Name)

		// Backoff before restart.
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

		// Attempt restart.
		newHost := NewHost(s.manifest, s.cwd)
		if err := newHost.Start(ctx); err != nil {
			slog.Error("extension restart handshake failed",
				"name", s.manifest.Name,
				"err", err)
			s.mu.Lock()
			s.host = nil
			s.mu.Unlock()
			continue // loops back, host is nil, returns at top
		}
		newHost.SetApp(s.app)
		newHost.SetProviderResolver(s.resolverFn)

		s.mu.Lock()
		s.host = newHost
		s.mu.Unlock()

		bridge(s.app, newHost)

		slog.Info("extension restarted successfully", "name", s.manifest.Name)
	}
}
