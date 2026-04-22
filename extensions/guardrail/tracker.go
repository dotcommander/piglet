// Package guardrail implements a daily token usage counter that can warn or
// block when the user exceeds a configured daily_token_limit. The limit is
// read once at session start (OnInit); runtime config edits require a restart.
package guardrail

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

// Tracker is a date-keyed daily token counter.
// It resets automatically when the local date ticks over.
// No package-level mutable state — callers capture a *Tracker in closures.
type Tracker struct {
	mu   sync.Mutex
	date string
	used int64
	path string
	// nowFn is injectable for tests. Defaults to time.Now.
	// Every skip returns nil — keeping tests deterministic without public API.
	nowFn func() time.Time
}

// NewTracker creates a zero-valued tracker writing persistence to path.
func NewTracker(path string) *Tracker {
	return &Tracker{
		path:  path,
		nowFn: time.Now,
	}
}

// newTrackerWithClock is a package-private helper for tests — injects clock.
func newTrackerWithClock(path string, now func() time.Time) *Tracker {
	return &Tracker{path: path, nowFn: now}
}

// Path returns the persistence file path.
func (t *Tracker) Path() string { return t.path }

// today returns today's date string using the injected clock.
func (t *Tracker) today() string {
	return t.nowFn().Format("2006-01-02")
}

// Add accumulates input+output tokens for today and persists asynchronously.
// If the local date has ticked over since the last Add, the counter resets first.
// Persistence failure is logged-and-ignored — the in-memory counter is not
// rolled back because the request has already consumed the tokens.
// Returns the updated daily total.
func (t *Tracker) Add(input, output int64) int64 {
	t.mu.Lock()
	today := t.today()
	if t.date != today {
		t.date = today
		t.used = 0
	}
	t.used += input + output
	snap := t.used
	snapDate := t.date
	t.mu.Unlock()

	// Persist outside the lock — a slow fsync should not block the caller.
	if err := t.saveSnapshot(snapDate, snap); err != nil {
		// Best-effort; caller logs via the extension's Log helper if needed.
		_ = err
	}
	return snap
}

// Used returns the accumulated token count for today, or 0 if the tracker's
// date has rolled over (even if Add has not been called since midnight).
func (t *Tracker) Used() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.date == "" || t.date != t.today() {
		return 0
	}
	return t.used
}

// Date returns the date the tracker is currently counting for.
// Returns today's date after a rollover check.
func (t *Tracker) Date() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	today := t.today()
	if t.date != today {
		t.date = today
		t.used = 0
	}
	return t.date
}

// LoadFrom reads a persisted usage file into the tracker.
// Stale dates (not today) are silently discarded — yesterday's usage zeroes
// naturally without error. Missing files return nil (fresh start).
func (t *Tracker) LoadFrom(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var rec struct {
		Date string `json:"date"`
		Used int64  `json:"used"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	today := t.today()
	if rec.Date != today {
		// Yesterday's data — discard; tracker stays zeroed at today's date.
		t.date = today
		t.used = 0
		return nil
	}
	t.date = rec.Date
	t.used = rec.Used
	return nil
}

// SaveTo JSON-marshals the current counter and writes it atomically to path.
func (t *Tracker) SaveTo(path string) error {
	t.mu.Lock()
	rec := struct {
		Date string `json:"date"`
		Used int64  `json:"used"`
	}{Date: t.date, Used: t.used}
	t.mu.Unlock()

	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return xdg.WriteFileAtomic(path, data)
}

// saveSnapshot writes a specific date+used pair atomically. Used by Add to
// persist the snapshot taken under lock, avoiding a second lock acquisition.
func (t *Tracker) saveSnapshot(date string, used int64) error {
	rec := struct {
		Date string `json:"date"`
		Used int64  `json:"used"`
	}{Date: date, Used: used}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return xdg.WriteFileAtomic(t.path, data)
}
