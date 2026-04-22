package session

import (
	"encoding/json"
	"fmt"

	"github.com/dotcommander/piglet/core"
)

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
		n.tokensBefore = entry.TokensBefore
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
