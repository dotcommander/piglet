package external

import (
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const reloadDebounce = 500 * time.Millisecond

// Watcher monitors extension directories for binary/manifest changes and
// triggers supervisor reloads. Debounces rapid changes (e.g. during build).
type Watcher struct {
	fsw    *fsnotify.Watcher
	done   chan struct{}
	doneWg sync.WaitGroup
}

// watchEntry maps an extension directory to its supervisor and watched filenames.
type watchEntry struct {
	sup   *Supervisor
	files map[string]bool // basenames that trigger reload
}

// startWatcher creates a filesystem watcher for all supervised extensions.
// It watches each extension's directory for changes to the binary or manifest,
// debounces events, and calls Supervisor.Reload() when files settle.
// Returns nil (no error) if fsnotify is unavailable — hot-reload is optional.
func startWatcher(supervisors []*Supervisor) *Watcher {
	if len(supervisors) == 0 {
		return nil
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("extension hot-reload unavailable", "err", err)
		return nil
	}

	entries := make(map[string]*watchEntry, len(supervisors))

	for _, s := range supervisors {
		dir := s.manifest.Dir
		files := map[string]bool{"manifest.yaml": true}

		// Add binary name for compiled extensions.
		switch s.manifest.Runtime {
		case "bun", "node", "deno", "python":
			// Script runtime — watch the entry file.
			if s.manifest.Entry != "" {
				files[s.manifest.Entry] = true
			}
		default:
			// Compiled binary — runtime field is the binary name.
			files[filepath.Base(s.manifest.Runtime)] = true
		}

		entries[dir] = &watchEntry{sup: s, files: files}
		if err := fsw.Add(dir); err != nil {
			slog.Debug("cannot watch extension dir", "dir", dir, "err", err)
		}
	}

	w := &Watcher{fsw: fsw, done: make(chan struct{})}
	w.doneWg.Add(1)
	go w.loop(entries)
	return w
}

// Stop shuts down the file watcher. Safe to call on nil.
func (w *Watcher) Stop() {
	if w == nil {
		return
	}
	close(w.done)
	w.fsw.Close()
	w.doneWg.Wait()
}

// loop processes fsnotify events, debouncing per-extension, and triggers reloads.
func (w *Watcher) loop(entries map[string]*watchEntry) {
	defer w.doneWg.Done()

	// Per-extension debounce timers.
	timers := make(map[string]*time.Timer)

	for {
		select {
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			dir := filepath.Dir(event.Name)
			e, ok := entries[dir]
			if !ok {
				continue
			}

			base := filepath.Base(event.Name)
			if !e.files[base] {
				continue
			}

			// Debounce: reset timer for this extension directory.
			if t, exists := timers[dir]; exists {
				t.Reset(reloadDebounce)
			} else {
				sup := e.sup
				timers[dir] = time.AfterFunc(reloadDebounce, func() {
					slog.Info("extension binary changed, reloading", "name", sup.Name())
					sup.Reload()
				})
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			slog.Debug("extension watcher error", "err", err)

		case <-w.done:
			// Stop pending timers.
			for _, t := range timers {
				t.Stop()
			}
			return
		}
	}
}
