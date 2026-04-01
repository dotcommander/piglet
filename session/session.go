// Package session manages conversation persistence as linear JSONL files.
// Each session is a single file; fork copies the file and optionally truncates.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/dotcommander/piglet/core"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Entry is a single line in the JSONL session file.
type Entry struct {
	Type      string          `json:"type"` // "user", "assistant", "tool_result", "meta"
	Timestamp time.Time       `json:"ts"`
	Data      json.RawMessage `json:"data"`
}

// Meta holds session metadata.
type Meta struct {
	ID        string    `json:"id"`
	CWD       string    `json:"cwd"`
	Model     string    `json:"model,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Title     string    `json:"title,omitempty"`
	ParentID  string    `json:"parentId,omitzero"`
	ForkPoint int       `json:"forkPoint,omitzero"`
}

// Summary is returned by List.
type Summary struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	CWD       string    `json:"cwd"`
	CreatedAt time.Time `json:"createdAt"`
	Messages  int       `json:"messages"`
	ParentID  string    `json:"parentId,omitzero"`
}

// Session manages a single conversation.
type Session struct {
	mu       sync.Mutex
	dir      string
	id       string
	path     string
	file     *os.File
	meta     Meta
	messages []core.Message
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
		dir:  dir,
		id:   id,
		path: path,
		file: f,
		meta: meta,
	}

	data, err := marshalJSON(meta)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := s.appendEntry(Entry{
		Type:      "meta",
		Timestamp: meta.CreatedAt,
		Data:      data,
	}); err != nil {
		_ = f.Close()
		return nil, err
	}

	return s, nil
}

// Open loads an existing session from a JSONL file.
func Open(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer f.Close()

	s := &Session{
		dir:  filepath.Dir(path),
		path: path,
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		s.replayEntry(entry)
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

// ID returns the session ID.
func (s *Session) ID() string { return s.id }

// Path returns the session file path.
func (s *Session) Path() string { return s.path }

// Meta returns session metadata.
func (s *Session) Meta() Meta { return s.meta }

// Messages returns all messages in the session.
func (s *Session) Messages() []core.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]core.Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// Append adds a message to the session and persists it.
func (s *Session) Append(msg core.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = append(s.messages, msg)

	data, err := marshalJSON(msg)
	if err != nil {
		return err
	}
	return s.appendEntry(Entry{
		Type:      messageEntryType(msg),
		Timestamp: time.Now(),
		Data:      data,
	})
}

// AppendCompact writes a compact checkpoint to the session. On reload, all
// entries before this checkpoint are replaced by the compacted messages.
// The in-memory message list is also replaced.
func (s *Session) AppendCompact(msgs []core.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = make([]core.Message, len(msgs))
	copy(s.messages, msgs)

	entries := make([]Entry, 0, len(msgs))
	for _, msg := range msgs {
		data, err := marshalJSON(msg)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{Type: messageEntryType(msg), Data: data})
	}

	data, err := marshalJSON(entries)
	if err != nil {
		return err
	}
	return s.appendEntry(Entry{
		Type:      "compact",
		Timestamp: time.Now(),
		Data:      data,
	})
}

// SetTitle updates the session title.
func (s *Session) SetTitle(title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.meta.Title = title
	data, err := marshalJSON(s.meta)
	if err != nil {
		return err
	}
	return s.appendEntry(Entry{
		Type:      "meta",
		Timestamp: time.Now(),
		Data:      data,
	})
}

// SetModel updates the model in metadata.
func (s *Session) SetModel(model string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.meta.Model = model
	data, err := marshalJSON(s.meta)
	if err != nil {
		return err
	}
	return s.appendEntry(Entry{
		Type:      "meta",
		Timestamp: time.Now(),
		Data:      data,
	})
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

// Fork creates a copy of this session, optionally keeping only the first n messages.
func (s *Session) Fork(keepMessages int) (*Session, error) {
	s.mu.Lock()
	msgs := make([]core.Message, len(s.messages))
	copy(msgs, s.messages)
	meta := s.meta
	s.mu.Unlock()

	limit := len(msgs)
	if keepMessages > 0 && keepMessages < limit {
		limit = keepMessages
	}

	newSess, err := New(s.dir, meta.CWD)
	if err != nil {
		return nil, err
	}

	// Set branch metadata and rewrite the initial meta entry so there's only one.
	newSess.meta.Model = meta.Model
	newSess.meta.ParentID = meta.ID
	newSess.meta.ForkPoint = limit
	metaData, err := marshalJSON(newSess.meta)
	if err != nil {
		_ = newSess.Close()
		return nil, err
	}
	if err := newSess.appendEntry(Entry{
		Type:      "meta",
		Timestamp: time.Now(),
		Data:      metaData,
	}); err != nil {
		_ = newSess.Close()
		return nil, fmt.Errorf("fork: write metadata: %w", err)
	}

	for _, msg := range msgs[:limit] {
		if err := newSess.Append(msg); err != nil {
			_ = newSess.Close()
			return nil, err
		}
	}

	return newSess, nil
}

// List returns summaries of all sessions in a directory, newest first.
func List(dir string) ([]Summary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var summaries []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		summary := scanSummary(path)
		if summary.ID != "" {
			summaries = append(summaries, summary)
		}
	}

	slices.SortFunc(summaries, func(a, b Summary) int {
		return b.CreatedAt.Compare(a.CreatedAt) // descending: newest first
	})

	return summaries, nil
}

func scanSummary(path string) Summary {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}
	}
	defer f.Close()

	s := Summary{Path: path}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "meta":
			var meta Meta
			if err := json.Unmarshal(entry.Data, &meta); err == nil {
				s.ID = meta.ID
				s.Title = meta.Title
				s.Model = meta.Model
				s.CWD = meta.CWD
				s.CreatedAt = meta.CreatedAt
				s.ParentID = meta.ParentID
			}
		case "compact":
			var entries []Entry
			if err := json.Unmarshal(entry.Data, &entries); err == nil {
				s.Messages = len(entries)
			}
		default:
			s.Messages++
		}
	}

	return s
}

func (s *Session) appendEntry(entry Entry) error {
	if s.file == nil {
		return nil
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	data = append(data, '\n')
	_, err = s.file.Write(data)
	return err
}

func (s *Session) replayEntry(entry Entry) {
	switch entry.Type {
	case "meta":
		var meta Meta
		if err := json.Unmarshal(entry.Data, &meta); err == nil {
			s.id = meta.ID
			s.meta = meta
		}
	case "user":
		var msg core.UserMessage
		if err := json.Unmarshal(entry.Data, &msg); err == nil {
			s.messages = append(s.messages, &msg)
		}
	case "assistant":
		var msg core.AssistantMessage
		if err := json.Unmarshal(entry.Data, &msg); err == nil {
			s.messages = append(s.messages, &msg)
		}
	case "tool_result":
		var msg core.ToolResultMessage
		if err := json.Unmarshal(entry.Data, &msg); err == nil {
			s.messages = append(s.messages, &msg)
		}
	case "compact":
		var entries []Entry
		if err := json.Unmarshal(entry.Data, &entries); err != nil {
			return
		}
		s.messages = s.messages[:0]
		for _, sub := range entries {
			s.replayEntry(sub)
		}
	}
}

func messageEntryType(msg core.Message) string {
	switch msg.(type) {
	case *core.UserMessage:
		return "user"
	case *core.AssistantMessage:
		return "assistant"
	case *core.ToolResultMessage:
		return "tool_result"
	default:
		return "unknown"
	}
}

func marshalJSON(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal entry: %w", err)
	}
	return data, nil
}
