// Package session manages conversation persistence as tree-structured JSONL files.
// Each session is a single file with entries linked by ID/ParentID, enabling
// in-place branching without creating new files.
//
// Design draws from pi-mono's session format:
// https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/session.md
package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/google/uuid"
)

// Entry type constants.
const (
	entryTypeUser          = "user"
	entryTypeAssistant     = "assistant"
	entryTypeToolResult    = "tool_result"
	entryTypeMeta          = "meta"
	entryTypeCompact       = "compact"
	entryTypeBranchSummary = "branch_summary"
	entryTypeCustomMessage = "custom_message"
	entryTypeLabel         = "label"
)

// Entry is a single line in the JSONL session file.
type Entry struct {
	Type      string          `json:"type"`               // "user", "assistant", "tool_result", "meta", "compact", "branch_summary"
	ID        string          `json:"id,omitempty"`       // 8-char hex; empty for meta and legacy entries
	ParentID  string          `json:"parentId,omitempty"` // parent entry ID; empty for first entry
	Timestamp time.Time       `json:"ts"`
	Data      json.RawMessage `json:"data"`
}

// BranchSummaryData holds the data payload for a branch_summary entry.
type BranchSummaryData struct {
	Summary string `json:"summary"`
	FromID  string `json:"fromId"` // leaf ID of the abandoned branch
}

// CustomMessageData holds the data payload for a custom_message entry.
type CustomMessageData struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// LabelData holds the data payload for a label entry.
type LabelData struct {
	TargetID string `json:"targetId"` // entry ID being labeled
	Label    string `json:"label"`    // empty = clear label
}

// EntryInfo is a public view of a tree node for display and navigation.
type EntryInfo struct {
	ID        string
	ParentID  string
	Type      string
	Timestamp time.Time
	Children  int
}

// TreeNode is a full-tree view of a session entry for DAG rendering.
type TreeNode struct {
	ID           string
	ParentID     string
	Type         string
	Timestamp    time.Time
	Children     int
	OnActivePath bool
	Depth        int
	Preview      string // truncated content for user messages
	Label        string // user-assigned bookmark label (empty if none)
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

// node is an in-memory tree node.
type node struct {
	parentID string
	typ      string
	ts       time.Time
	message  core.Message   // non-nil for user/assistant/tool_result
	compact  []core.Message // non-nil for compact entries
}

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

// generateEntryID returns a random 8-character hex ID.
func generateEntryID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
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

// ---------------------------------------------------------------------------
// Mutation
// ---------------------------------------------------------------------------

// Append adds a message to the session at the current leaf and persists it.
func (s *Session) Append(msg core.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := marshalJSON(msg)
	if err != nil {
		return err
	}

	typ := messageEntryType(msg)
	_, err = s.commitNode(typ, data, &node{typ: typ, message: msg})
	return err
}

// AppendCompact writes a compact checkpoint at the current leaf. On context build,
// all ancestor messages before this entry are replaced by the compacted messages.
func (s *Session) AppendCompact(msgs []core.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	compactMsgs := make([]core.Message, len(msgs))
	copy(compactMsgs, msgs)

	_, err = s.commitNode(entryTypeCompact, data, &node{typ: entryTypeCompact, compact: compactMsgs})
	return err
}

// AppendCustom writes a custom extension entry to the session at the current leaf.
// The kind should be namespaced (e.g., "ext:memory:facts"). Data is JSON-marshaled.
// Custom entries are stored in the tree but do not appear in Messages().
func (s *Session) AppendCustom(kind string, data any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := marshalJSON(data)
	if err != nil {
		return err
	}

	// Custom entries have no message — they're metadata, like branch_summary.
	_, err = s.commitNode(kind, raw, &node{typ: kind})
	return err
}

// AppendCustomMessage writes a message entry that persists AND appears in Messages().
// Role must be "user" or "assistant". Used by extensions to inject durable context annotations.
func (s *Session) AppendCustomMessage(role, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cm := CustomMessageData{Role: role, Content: content}
	raw, err := marshalJSON(cm)
	if err != nil {
		return err
	}

	// message timestamp is set by commitNode; use zero time here and patch after commit.
	n := &node{typ: entryTypeCustomMessage}
	if _, err := s.commitNode(entryTypeCustomMessage, raw, n); err != nil {
		return err
	}
	n.message = customMessageToCore(role, content, n.ts)
	return nil
}

// customMessageToCore converts a custom_message role/content to a core.Message.
func customMessageToCore(role, content string, ts time.Time) core.Message {
	switch role {
	case "assistant":
		return &core.AssistantMessage{
			Content: []core.AssistantContent{core.TextContent{Text: content}},
		}
	default:
		return &core.UserMessage{Content: content, Timestamp: ts}
	}
}

// AppendLabel writes a label entry targeting a specific entry ID.
// The label is stored in the labels map (last-write-wins per target).
// Empty label clears an existing label.
func (s *Session) AppendLabel(targetID, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.nodes[targetID]; !ok {
		return fmt.Errorf("entry %s not found", targetID)
	}

	ld := LabelData{TargetID: targetID, Label: label}
	raw, err := marshalJSON(ld)
	if err != nil {
		return err
	}

	if _, err := s.commitNode(entryTypeLabel, raw, &node{typ: entryTypeLabel}); err != nil {
		return err
	}

	if label != "" {
		s.labels[targetID] = label
	} else {
		delete(s.labels, targetID)
	}

	return nil
}

// Label returns the label for an entry, or empty string.
func (s *Session) Label(entryID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.labels[entryID]
}

// Branch moves the leaf to an earlier entry, creating an in-place branch point.
// A branch_summary entry is written to persist the new leaf position across reloads.
func (s *Session) Branch(entryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[entryID]; !ok {
		return fmt.Errorf("entry %s not found", entryID)
	}
	return s.writeBranchEntry(entryID, "")
}

// BranchWithSummary moves the leaf to an earlier entry and writes a
// branch_summary entry capturing context about the abandoned branch.
// The summary entry becomes the new leaf.
func (s *Session) BranchWithSummary(entryID, summary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[entryID]; !ok {
		return fmt.Errorf("entry %s not found", entryID)
	}
	return s.writeBranchEntry(entryID, summary)
}

// writeBranchEntry writes a branch_summary entry and moves the leaf.
// Must be called with s.mu held.
func (s *Session) writeBranchEntry(parentID, summary string) error {
	bs := BranchSummaryData{Summary: summary, FromID: s.leafID}
	data, err := marshalJSON(bs)
	if err != nil {
		return err
	}

	// Branch target is parentID, not the current leaf. Temporarily set leafID
	// so that commitNode attaches the entry to the correct parent.
	prevLeaf := s.leafID
	s.leafID = parentID
	_, err = s.commitNode(entryTypeBranchSummary, data, &node{typ: entryTypeBranchSummary})
	if err != nil {
		s.leafID = prevLeaf // restore on failure
	}
	return err
}

// commitNode appends an entry, registers the node in the tree, and advances the leaf.
// Caller must hold s.mu.
func (s *Session) commitNode(typ string, data json.RawMessage, n *node) (string, error) {
	id := generateEntryID()
	entry := Entry{
		Type:      typ,
		ID:        id,
		ParentID:  s.leafID,
		Timestamp: time.Now(),
		Data:      data,
	}
	if err := s.appendEntry(entry); err != nil {
		return "", err
	}
	n.parentID = s.leafID
	n.ts = entry.Timestamp
	s.nodes[id] = n
	s.leafID = id
	return id, nil
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
		Type:      entryTypeMeta,
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
		Type:      entryTypeMeta,
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

// Fork creates a new session file with messages from the current branch.
// keepMessages limits how many messages to copy (0 = all).
func (s *Session) Fork(keepMessages int) (*Session, error) {
	s.mu.RLock()
	msgs := s.buildBranch()
	meta := s.meta
	s.mu.RUnlock()

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
		Type:      entryTypeMeta,
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

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

// EntryInfos returns info about all entries on the current branch (root to leaf).
func (s *Session) EntryInfos() []EntryInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.branchPath()

	// Compute children counts on the fly
	childCount := make(map[string]int, len(s.nodes))
	for _, n := range s.nodes {
		if n.parentID != "" {
			childCount[n.parentID]++
		}
	}

	infos := make([]EntryInfo, 0, len(path))
	for _, id := range path {
		n := s.nodes[id]
		if n == nil {
			continue
		}
		infos = append(infos, EntryInfo{
			ID:        id,
			ParentID:  n.parentID,
			Type:      n.typ,
			Timestamp: n.ts,
			Children:  childCount[id],
		})
	}
	return infos
}

// FullTree returns every entry in the session as a flat list ordered by DFS traversal.
// Active path entries are marked. Children are sorted with the active subtree first,
// then by timestamp (oldest first).
func (s *Session) FullTree() []TreeNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.nodes) == 0 {
		return nil
	}

	// Build parent → children map
	children := make(map[string][]string, len(s.nodes))
	var roots []string
	for id, n := range s.nodes {
		if n.parentID == "" {
			roots = append(roots, id)
		} else {
			children[n.parentID] = append(children[n.parentID], id)
		}
	}

	// Active path set
	activePath := make(map[string]bool, len(s.nodes))
	for _, id := range s.branchPath() {
		activePath[id] = true
	}

	// Sort children: active-subtree first, then oldest-first
	for parentID, kids := range children {
		slices.SortFunc(kids, func(a, b string) int {
			aActive := s.isInActiveSubtree(a, activePath, children)
			bActive := s.isInActiveSubtree(b, activePath, children)
			if aActive != bActive {
				if aActive {
					return -1
				}
				return 1
			}
			return s.nodes[a].ts.Compare(s.nodes[b].ts)
		})
		children[parentID] = kids
	}

	// Sort roots the same way
	slices.SortFunc(roots, func(a, b string) int {
		return s.nodes[a].ts.Compare(s.nodes[b].ts)
	})

	// DFS
	var result []TreeNode
	var dfs func(id string, depth int)
	dfs = func(id string, depth int) {
		n := s.nodes[id]
		if n == nil {
			return
		}
		result = append(result, TreeNode{
			ID:           id,
			ParentID:     n.parentID,
			Type:         n.typ,
			Timestamp:    n.ts,
			Children:     len(children[id]),
			OnActivePath: activePath[id],
			Depth:        depth,
			Preview:      s.nodePreview(n),
			Label:        s.labels[id],
		})
		for _, kid := range children[id] {
			dfs(kid, depth+1)
		}
	}
	for _, root := range roots {
		dfs(root, 0)
	}

	return result
}

// isInActiveSubtree returns true if id or any descendant is on the active path.
func (s *Session) isInActiveSubtree(id string, activePath map[string]bool, children map[string][]string) bool {
	if activePath[id] {
		return true
	}
	for _, kid := range children[id] {
		if s.isInActiveSubtree(kid, activePath, children) {
			return true
		}
	}
	return false
}

// nodePreview returns a short text preview for a node.
func (s *Session) nodePreview(n *node) string {
	if n.message == nil {
		return ""
	}
	switch m := n.message.(type) {
	case *core.UserMessage:
		return truncatePreview(m.Content, 60)
	case *core.AssistantMessage:
		for _, c := range m.Content {
			if tc, ok := c.(core.TextContent); ok {
				return truncatePreview(tc.Text, 60)
			}
		}
	}
	return ""
}

// truncatePreview truncates a string to n runes, appending "..." if truncated.
func truncatePreview(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

// ---------------------------------------------------------------------------
// List (package-level)
// ---------------------------------------------------------------------------

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
		summary, err := scanSummary(path)
		if err != nil {
			slog.Warn("session: scan summary failed", "path", path, "err", err)
			continue
		}
		if summary.ID != "" {
			summaries = append(summaries, summary)
		}
	}

	slices.SortFunc(summaries, func(a, b Summary) int {
		return b.CreatedAt.Compare(a.CreatedAt) // descending: newest first
	})

	return summaries, nil
}

func scanSummary(path string) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	s := Summary{Path: path}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			slog.Warn("session: skipping corrupt entry", "path", path, "err", err)
			continue
		}

		switch entry.Type {
		case entryTypeMeta:
			var meta Meta
			if err := json.Unmarshal(entry.Data, &meta); err == nil {
				s.ID = meta.ID
				s.Title = meta.Title
				s.Model = meta.Model
				s.CWD = meta.CWD
				s.CreatedAt = meta.CreatedAt
				s.ParentID = meta.ParentID
			}
		case entryTypeCompact:
			var entries []Entry
			if err := json.Unmarshal(entry.Data, &entries); err == nil {
				s.Messages = len(entries)
			}
		case entryTypeBranchSummary:
			// Not a conversation message; skip count
		case entryTypeUser, entryTypeAssistant, entryTypeToolResult, entryTypeCustomMessage:
			s.Messages++
			// default: skip unknown/custom types (e.g., "ext:*" entries)
		}
	}

	if err := scanner.Err(); err != nil {
		return Summary{}, fmt.Errorf("scan session %s: %w", path, err)
	}

	return s, nil
}

// ---------------------------------------------------------------------------
// Internal: tree operations
// ---------------------------------------------------------------------------

// buildBranch walks from leaf to root and builds the message list for the
// current branch. Compaction entries reset the message list.
// Must be called with s.mu held.
func (s *Session) buildBranch() []core.Message {
	path := s.branchPath()

	var msgs []core.Message
	for _, id := range path {
		n := s.nodes[id]
		if n == nil {
			continue
		}
		switch {
		case n.compact != nil:
			msgs = msgs[:0]
			msgs = append(msgs, n.compact...)
		case n.message != nil:
			msgs = append(msgs, n.message)
		}
	}
	return msgs
}

// branchPath returns entry IDs from root to leaf for the current branch.
// Must be called with s.mu held.
func (s *Session) branchPath() []string {
	if s.leafID == "" {
		return nil
	}
	var path []string
	current := s.leafID
	for current != "" {
		path = append(path, current)
		n := s.nodes[current]
		if n == nil {
			break
		}
		current = n.parentID
	}
	slices.Reverse(path)
	return path
}

// ---------------------------------------------------------------------------
// Internal: replay and persistence
// ---------------------------------------------------------------------------

func (s *Session) replayEntry(entry Entry) {
	if entry.Type == entryTypeMeta {
		var meta Meta
		if err := json.Unmarshal(entry.Data, &meta); err == nil {
			s.id = meta.ID
			s.meta = meta
		}
		return
	}

	// Legacy entries (pre-tree format): assign deterministic sequential IDs
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("L%d", s.legacySeq)
		s.legacySeq++
		if s.leafID != "" {
			entry.ParentID = s.leafID
		}
	}

	n := &node{parentID: entry.ParentID, typ: entry.Type, ts: entry.Timestamp}

	switch entry.Type {
	case entryTypeUser:
		var msg core.UserMessage
		if err := json.Unmarshal(entry.Data, &msg); err == nil {
			n.message = &msg
		}
	case entryTypeAssistant:
		var msg core.AssistantMessage
		if err := json.Unmarshal(entry.Data, &msg); err == nil {
			n.message = &msg
		}
	case entryTypeToolResult:
		var msg core.ToolResultMessage
		if err := json.Unmarshal(entry.Data, &msg); err == nil {
			n.message = &msg
		}
	case entryTypeCompact:
		var entries []Entry
		if err := json.Unmarshal(entry.Data, &entries); err == nil {
			n.compact = replayCompactEntries(entries)
		}
	case entryTypeCustomMessage:
		var cm CustomMessageData
		if err := json.Unmarshal(entry.Data, &cm); err == nil {
			n.message = customMessageToCore(cm.Role, cm.Content, entry.Timestamp)
		}
	case entryTypeLabel:
		var ld LabelData
		if err := json.Unmarshal(entry.Data, &ld); err == nil {
			if ld.Label != "" {
				s.labels[ld.TargetID] = ld.Label
			} else {
				delete(s.labels, ld.TargetID)
			}
		}
	}

	s.nodes[entry.ID] = n
	s.leafID = entry.ID
}

func replayCompactEntries(entries []Entry) []core.Message {
	var msgs []core.Message
	for _, sub := range entries {
		switch sub.Type {
		case entryTypeUser:
			var m core.UserMessage
			if json.Unmarshal(sub.Data, &m) == nil {
				msgs = append(msgs, &m)
			}
		case entryTypeAssistant:
			var m core.AssistantMessage
			if json.Unmarshal(sub.Data, &m) == nil {
				msgs = append(msgs, &m)
			}
		case entryTypeToolResult:
			var m core.ToolResultMessage
			if json.Unmarshal(sub.Data, &m) == nil {
				msgs = append(msgs, &m)
			}
		}
	}
	return msgs
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

func messageEntryType(msg core.Message) string {
	switch msg.(type) {
	case *core.UserMessage:
		return entryTypeUser
	case *core.AssistantMessage:
		return entryTypeAssistant
	case *core.ToolResultMessage:
		return entryTypeToolResult
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
