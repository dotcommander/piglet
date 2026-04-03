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

// Deliverer injects messages into the agent loop.
// *sdk.Extension satisfies this interface.
type Deliverer interface {
	SendMessage(content string)
	Steer(content string)
	Notify(msg string)
}

// Envelope is an inbound message from an external process.
type Envelope struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Text    string `json:"text"`
	Mode    string `json:"mode,omitzero"`
	Created string `json:"created,omitzero"`
	TTL     int    `json:"ttl,omitzero"`
	Source  string `json:"source,omitzero"`
}

// Ack is written after processing an envelope.
type Ack struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitzero"`
	Ts     string `json:"ts"`
}

// Heartbeat is written periodically to the registry.
type Heartbeat struct {
	PID       int    `json:"pid"`
	CWD       string `json:"cwd"`
	Started   string `json:"started"`
	Heartbeat string `json:"heartbeat"`
}

// Stats tracks delivery counts for the current session.
type Stats struct {
	Delivered  int       `json:"delivered"`
	Failed     int       `json:"failed"`
	Duplicates int       `json:"duplicates"`
	Expired    int       `json:"expired"`
	StartedAt  time.Time `json:"startedAt"`
}

const (
	DefaultScanInterval = 750 * time.Millisecond
	HeartbeatInterval   = 2 * time.Second
	MaxFileBytes        = 32 * 1024
	MaxTextRunes        = 8000
	DedupCap            = 1000
	AckMaxAge           = time.Hour
	PruneInterval       = time.Minute

	ModeQueue     = "queue"
	ModeInterrupt = "interrupt"
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

	if env.TTL > 0 {
		// Use Created field if present, otherwise fall back to file mtime.
		created := info.ModTime()
		if env.Created != "" {
			if t, err := time.Parse(time.RFC3339, env.Created); err == nil {
				created = t
			}
		}
		if time.Now().After(created.Add(time.Duration(env.TTL) * time.Second)) {
			s.mu.Lock()
			s.stats.Expired++
			s.mu.Unlock()
			s.ackAndRemove(path, env.ID, "failed", "expired")
			return
		}
	}

	if s.isDuplicate(env.ID) {
		s.mu.Lock()
		s.stats.Duplicates++
		s.mu.Unlock()
		s.ackAndRemove(path, env.ID, "duplicate", "")
		return
	}

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

// Validate checks an envelope for structural validity.
func Validate(env *Envelope) (bool, string) {
	if env.Version != 1 {
		return false, "unsupported_version"
	}
	if env.ID == "" {
		return false, "missing_id"
	}
	if env.Text == "" {
		return false, "missing_text"
	}
	if len([]rune(env.Text)) > MaxTextRunes {
		return false, "text_too_long"
	}
	switch env.Mode {
	case "", ModeQueue, ModeInterrupt:
		// valid
	default:
		return false, "invalid_mode"
	}
	return true, ""
}

func (s *Scanner) isDuplicate(id string) bool {
	s.mu.Lock()
	_, inMem := s.seen[id]
	s.mu.Unlock()
	if inMem {
		return true
	}
	ackPath := filepath.Join(s.acksDir(), id+".json")
	_, err := os.Stat(ackPath)
	return err == nil
}

func (s *Scanner) ackAndRemove(path, id, status, reason string) {
	if id != "" {
		s.writeAck(id, status, reason)
	}
	if status == "failed" {
		s.mu.Lock()
		s.stats.Failed++
		s.mu.Unlock()
	}
	_ = os.Remove(path)
}

func (s *Scanner) writeAck(id, status, reason string) {
	ack := Ack{
		ID:     id,
		Status: status,
		Reason: reason,
		Ts:     time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(ack)
	if err != nil {
		return
	}
	dest := filepath.Join(s.acksDir(), id+".json")
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0640); err != nil {
		return
	}
	_ = os.Rename(tmp, dest)
}

func (s *Scanner) writeHeartbeat() {
	hb := Heartbeat{
		PID:       s.pid,
		CWD:       s.cwd,
		Started:   s.started,
		Heartbeat: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(hb)
	if err != nil {
		return
	}
	dest := filepath.Join(s.registryDir(), fmt.Sprintf("%d.json", s.pid))
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0640); err != nil {
		return
	}
	_ = os.Rename(tmp, dest)
}

func (s *Scanner) pruneAcks() {
	if time.Since(s.lastPrune) < PruneInterval {
		return
	}
	s.lastPrune = time.Now()
	dir := s.acksDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > AckMaxAge {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
