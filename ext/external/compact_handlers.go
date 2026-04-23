package external

import (
	"context"
	"time"
)

// HostTriggerCompactResult is the response payload for host/triggerCompact.
type HostTriggerCompactResult struct {
	Before int `json:"before"`
	After  int `json:"after"`
}

func (h *Host) handleHostHasCompactor(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	h.respond(*msg.ID, map[string]any{"present": h.app.Compactor() != nil})
}

func (h *Host) handleHostTriggerCompact(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	c := h.app.Compactor()
	if c == nil {
		h.respondError(*msg.ID, -32603, "no compactor registered")
		return
	}
	msgs := h.app.ConversationMessages()
	before := len(msgs)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	compacted, err := c.Compact(ctx, msgs)
	if err != nil {
		h.respondError(*msg.ID, -32603, "compact failed: "+err.Error())
		return
	}
	h.app.SetConversationMessages(compacted)
	h.respond(*msg.ID, HostTriggerCompactResult{Before: before, After: len(compacted)})
}
