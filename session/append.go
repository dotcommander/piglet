package session

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dotcommander/piglet/core"
)

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
// tokensBefore is the token count in context immediately before compaction (0 = unknown).
func (s *Session) AppendCompact(msgs []core.Message, tokensBefore int) error {
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

	n := &node{typ: entryTypeCompact, compact: compactMsgs, tokensBefore: tokensBefore}
	_, err = s.commitNode(entryTypeCompact, data, n)
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

// commitNode appends an entry, registers the node in the tree, and advances the leaf.
// Caller must hold s.mu.
func (s *Session) commitNode(typ string, data json.RawMessage, n *node) (string, error) {
	id := generateEntryID()
	entry := Entry{
		Type:         typ,
		ID:           id,
		ParentID:     s.leafID,
		Timestamp:    time.Now(),
		Data:         data,
		TokensBefore: n.tokensBefore, // zero for non-compact entries; omitempty keeps JSON clean
	}
	if err := s.withFileLock(func() error { return s.appendEntry(entry) }); err != nil {
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
	return s.withFileLock(func() error {
		return s.appendEntry(Entry{
			Type:      entryTypeMeta,
			Timestamp: time.Now(),
			Data:      data,
		})
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
	return s.withFileLock(func() error {
		return s.appendEntry(Entry{
			Type:      entryTypeMeta,
			Timestamp: time.Now(),
			Data:      data,
		})
	})
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

// withFileLock acquires the inter-process write lock and calls fn.
// s.mu must be held by the caller before invoking withFileLock.
// Lock ordering: in-process mutex → file lock (coarse to fine).
func (s *Session) withFileLock(fn func() error) error {
	lk, err := acquireLock(s.lockPath)
	if err != nil {
		return fmt.Errorf("session lock: %w", err)
	}
	defer lk.release()
	return fn()
}
