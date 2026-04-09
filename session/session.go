// Package session manages conversation persistence as tree-structured JSONL files.
// Each session is a single file with entries linked by ID/ParentID, enabling
// in-place branching without creating new files.
//
// Design draws from pi-mono's session format:
// https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/session.md
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/google/uuid"
)

// Session manages a single conversation as a tree-structured JSONL file.
// Entries form a tree via ID/ParentID linking. A leaf pointer tracks the current
// position; Messages() walks from leaf to root to build the active branch.
type Session struct {
	mu   sync.RWMutex
	dir  string
	id   string
	path string
	file *os.File
	meta Meta

	nodes  map[string]*node  // id -> node
	labels map[string]string // targetID -> label (last-write-wins)
	leafID string            // current position

	// Counter for assigning deterministic IDs to legacy entries (no id field)
	legacySeq int
}

// New creates a new session in the given directory.
func New(dir, cwd string) (*Session, error) {
	id := uuid.New().String()
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	path := filepath.Join(dir, id+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("create session file: %w", err)
	}

	meta := Meta{
		ID:        id,
		CWD:       cwd,
		CreatedAt: time.Now(),
	}

	s := &Session{
		dir:    dir,
		id:     id,
		path:   path,
		file:   f,
		meta:   meta,
		nodes:  make(map[string]*node),
		labels: make(map[string]string),
	}

	data, err := marshalJSON(meta)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := s.appendEntry(Entry{
		Type:      entryTypeMeta,
		Timestamp: meta.CreatedAt,
		Data:      data,
	}); err != nil {
		_ = f.Close()
		return nil, err
	}

	return s, nil
}

// Open loads an existing session from a JSONL file.
// Legacy sessions (entries without IDs) are transparently upgraded in memory.
func Open(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer f.Close()

	s := &Session{
		dir:    filepath.Dir(path),
		path:   path,
		nodes:  make(map[string]*node),
		labels: make(map[string]string),
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			slog.Warn("session: skipping corrupt entry", "path", path, "err", err)
			continue
		}
		s.replayEntry(entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session %s: %w", path, err)
	}

	if s.id == "" {
		return nil, fmt.Errorf("session file has no metadata")
	}

	// Reopen for appending
	s.file, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("reopen session for append: %w", err)
	}

	return s, nil
}

// ---------------------------------------------------------------------------
// Accessors
// ---------------------------------------------------------------------------

// ID returns the session ID.
func (s *Session) ID() string { return s.id }

// Path returns the session file path.
func (s *Session) Path() string { return s.path }

// Meta returns session metadata.
func (s *Session) Meta() Meta { return s.meta }

// LeafID returns the current leaf entry ID.
func (s *Session) LeafID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.leafID
}

// Messages returns all messages on the current branch (leaf to root walk).
func (s *Session) Messages() []core.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buildBranch()
}

// Close closes the session file.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}
