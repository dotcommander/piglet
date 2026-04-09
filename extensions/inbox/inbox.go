// Package inbox provides a file-based message inbox for piglet.
// External processes drop JSON envelopes into a directory; the scanner
// picks them up and injects them into the agent loop.
package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Scanner watches the inbox directory and delivers messages.
type Scanner struct {
	inboxDir  string
	pid       int
	cwd       string
	deliverer Deliverer
	started   string

	mu        sync.Mutex
	stats     Stats
	seen      map[string]struct{}
	lastPrune time.Time

	cancel func()
	wg     sync.WaitGroup
}

// New creates a scanner. Call Start to begin scanning.
func New(inboxDir, cwd string, pid int, d Deliverer) *Scanner {
	return &Scanner{
		inboxDir:  inboxDir,
		pid:       pid,
		cwd:       cwd,
		deliverer: d,
		started:   time.Now().UTC().Format(time.RFC3339),
		stats:     Stats{StartedAt: time.Now()},
		seen:      make(map[string]struct{}, DedupCap),
	}
}

func (s *Scanner) processDir() string {
	return filepath.Join(s.inboxDir, strconv.Itoa(s.pid))
}

func (s *Scanner) acksDir() string {
	return filepath.Join(s.processDir(), "acks")
}

func (s *Scanner) registryDir() string {
	return filepath.Join(s.inboxDir, "registry")
}

// Start launches the scan and heartbeat goroutines.
func (s *Scanner) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	_ = os.MkdirAll(s.processDir(), 0750)
	_ = os.MkdirAll(s.acksDir(), 0750)
	_ = os.MkdirAll(s.registryDir(), 0750)

	s.wg.Add(2)
	go s.scanLoop(ctx)
	go s.heartbeatLoop(ctx)
}

// Stop cancels the scanner and waits for goroutines to finish.
func (s *Scanner) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	_ = os.Remove(filepath.Join(s.registryDir(), fmt.Sprintf("%d.json", s.pid)))
}

// Stats returns a copy of the current delivery statistics.
func (s *Scanner) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

func (s *Scanner) scanLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(DefaultScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scan()
		}
	}
}

func (s *Scanner) heartbeatLoop(ctx context.Context) {
	defer s.wg.Done()
	s.writeHeartbeat()
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.writeHeartbeat()
		}
	}
}

// scan performs one pass over the inbox directory.
func (s *Scanner) scan() {
	dir := s.processDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type fileEntry struct {
		name    string
		modTime time.Time
	}
	var files []fileEntry
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if strings.Contains(name, "..") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{name: name, modTime: info.ModTime()})
	}

	slices.SortFunc(files, func(a, b fileEntry) int {
		return a.modTime.Compare(b.modTime)
	})

	for _, f := range files {
		s.processFile(filepath.Join(dir, f.name))
	}

	s.pruneAcks()
}

func (s *Scanner) processFile(path string) {
	info, err := os.Lstat(path)
	if err != nil {
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(path)
		return
	}
	if info.Size() > MaxFileBytes {
		s.ackAndRemove(path, "", "failed", "file_too_large")
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		s.ackAndRemove(path, "", "failed", "invalid_json")
		return
	}

	ok, reason := Validate(&env)
	if !ok {
		s.ackAndRemove(path, env.ID, "failed", reason)
		return
	}

	if isExpired(&env, info.ModTime()) {
		s.mu.Lock()
		s.stats.Expired++
		s.mu.Unlock()
		s.ackAndRemove(path, env.ID, "failed", "expired")
		return
	}

	if s.isDuplicate(env.ID) {
		s.mu.Lock()
		s.stats.Duplicates++
		s.mu.Unlock()
		s.ackAndRemove(path, env.ID, "duplicate", "")
		return
	}

	s.deliver(&env)

	s.mu.Lock()
	s.stats.Delivered++
	if len(s.seen) >= DedupCap {
		clear(s.seen)
	}
	s.seen[env.ID] = struct{}{}
	s.mu.Unlock()

	s.writeAck(env.ID, "delivered", "")
	_ = os.Remove(path)
}

// deliver sends the envelope text to the agent loop via the appropriate mode.
func (s *Scanner) deliver(env *Envelope) {
	mode := env.Mode
	if mode == "" {
		mode = ModeQueue
	}
	source := env.Source
	if source == "" {
		source = "unknown"
	}

	switch mode {
	case ModeQueue:
		s.deliverer.SendMessage(env.Text)
	case ModeInterrupt:
		s.deliverer.Steer(env.Text)
	}

	s.deliverer.Notify(fmt.Sprintf("Inbox: message from %s", source))
}
