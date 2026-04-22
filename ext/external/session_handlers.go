package external

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dotcommander/piglet/ext"
)

func (h *Host) handleHostConversationMessages(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	msgs := h.app.ConversationMessages()
	data, err := json.Marshal(msgs)
	if err != nil {
		h.respondError(*msg.ID, -32603, "marshal messages: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostConversationMessagesResult{Messages: data})
}

func (h *Host) handleHostLLMSnapshot(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	snap := h.app.LLMSnapshot()
	msgsData, err := json.Marshal(snap.Messages)
	if err != nil {
		h.respondError(*msg.ID, -32603, "marshal snapshot messages: "+err.Error())
		return
	}
	toolsData, err := json.Marshal(snap.Tools)
	if err != nil {
		h.respondError(*msg.ID, -32603, "marshal snapshot tools: "+err.Error())
		return
	}
	h.respond(*msg.ID, map[string]any{
		"system":   snap.System,
		"messages": json.RawMessage(msgsData),
		"tools":    json.RawMessage(toolsData),
	})
}

func (h *Host) handleHostSetConversationMessages(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	var params HostSetConversationMessagesParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		h.respondError(*msg.ID, -32600, "invalid params: "+err.Error())
		return
	}
	coreMsgs := compactWireToCore(params.Messages)
	h.app.SetConversationMessages(coreMsgs)
	h.respond(*msg.ID, map[string]string{})
}

func (h *Host) handleHostSessions(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	summaries, err := h.app.Sessions()
	if err != nil {
		h.respondError(*msg.ID, -32603, "sessions: "+err.Error())
		return
	}
	infos := make([]WireSessionInfo, len(summaries))
	for i, s := range summaries {
		infos[i] = WireSessionInfo{
			ID:        s.ID,
			Title:     s.Title,
			CWD:       s.CWD,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			ParentID:  s.ParentID,
			Path:      s.Path,
		}
	}
	h.respond(*msg.ID, HostSessionsResult{Sessions: infos})
}

func (h *Host) handleHostLoadSession(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostLoadSessionParams) (struct{}, error) {
		return struct{}{}, fmt.Errorf("load session: %w", app.LoadSession(p.Path))
	})
}

func (h *Host) handleHostForkSession(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	parentID, count, err := h.app.ForkSession()
	if err != nil {
		h.respondError(*msg.ID, -32603, "fork session: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostForkSessionResult{ParentID: parentID, MessageCount: count})
}

func (h *Host) handleHostSetSessionTitle(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostSetSessionTitleParams) (struct{}, error) {
		return struct{}{}, fmt.Errorf("set session title: %w", app.SetSessionTitle(p.Title))
	})
}

func (h *Host) handleHostLastAssistantText(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	h.respond(*msg.ID, HostLastAssistantTextResult{Text: h.app.LastAssistantText()})
}

func (h *Host) handleHostAppendSessionEntry(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostAppendSessionEntryParams) (struct{}, error) {
		return struct{}{}, fmt.Errorf("append session entry: %w", app.AppendSessionEntry(p.Kind, p.Data))
	})
}

func (h *Host) handleHostAppendCustomMessage(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostAppendCustomMessageParams) (struct{}, error) {
		return struct{}{}, fmt.Errorf("append custom message: %w", app.AppendCustomMessage(p.Role, p.Content))
	})
}

func (h *Host) handleHostSetLabel(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostSetLabelParams) (struct{}, error) {
		return struct{}{}, fmt.Errorf("set label: %w", app.SetSessionLabel(p.TargetID, p.Label))
	})
}

func (h *Host) handleHostBranchSession(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostBranchSessionParams) (struct{}, error) {
		return struct{}{}, fmt.Errorf("branch session: %w", app.BranchSession(p.EntryID))
	})
}

func (h *Host) handleHostBranchSessionSummary(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostBranchSessionSummaryParams) (struct{}, error) {
		return struct{}{}, fmt.Errorf("branch session: %w", app.BranchSessionWithSummary(p.EntryID, p.Summary))
	})
}

func (h *Host) handleHostResetSessionLeaf(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, _ HostResetSessionLeafParams) (struct{}, error) {
		if err := app.ResetSessionLeaf(); err != nil {
			return struct{}{}, fmt.Errorf("reset session leaf: %w", err)
		}
		return struct{}{}, nil
	})
}

func (h *Host) handleHostSessionEntryInfos(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	infos := h.app.SessionEntryInfos()
	wire := make([]WireEntryInfo, len(infos))
	for i, e := range infos {
		wire[i] = WireEntryInfo{
			ID:        e.ID,
			ParentID:  e.ParentID,
			Type:      e.Type,
			Timestamp: e.Timestamp.Format(time.RFC3339),
			Children:  e.Children,
		}
	}
	h.respond(*msg.ID, HostSessionEntryInfosResult{Entries: wire})
}

func (h *Host) handleHostSessionFullTree(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	nodes := h.app.SessionFullTree()
	wire := make([]WireTreeNode, len(nodes))
	for i, n := range nodes {
		wire[i] = WireTreeNode{
			ID:           n.ID,
			ParentID:     n.ParentID,
			Type:         n.Type,
			Timestamp:    n.Timestamp.Format(time.RFC3339),
			Children:     n.Children,
			OnActivePath: n.OnActivePath,
			Depth:        n.Depth,
			Preview:      n.Preview,
			Label:        n.Label,
			TokensBefore: n.TokensBefore,
		}
	}
	h.respond(*msg.ID, HostSessionFullTreeResult{Nodes: wire})
}

func (h *Host) handleHostSessionTitle(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	h.respond(*msg.ID, HostSessionTitleResult{Title: h.app.SessionTitle()})
}

func (h *Host) handleHostSessionStats(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	h.respond(*msg.ID, h.app.SessionStats())
}

func (h *Host) handleHostToggleStepMode(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	on := h.app.ToggleStepMode()
	h.respond(*msg.ID, map[string]any{"on": on})
}

func (h *Host) handleHostRequestQuit(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	h.app.RequestQuit()
	h.respond(*msg.ID, map[string]string{})
}

func (h *Host) handleHostHasCompactor(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	h.respond(*msg.ID, map[string]any{"present": h.app.Compactor() != nil})
}

// HostTriggerCompactResult is the response payload for host/triggerCompact.
type HostTriggerCompactResult struct {
	Before int `json:"before"`
	After  int `json:"after"`
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

// handleHostWaitForIdle blocks until the agent is idle or the host context is
// cancelled. No host-side timeout — the extension's request ctx governs; if the
// extension wants a deadline, it passes one on its side. Wrapping with
// hostRequestTimeout (5m) would cap legitimate long turns at 5 minutes.
func (h *Host) handleHostWaitForIdle(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	if err := h.app.WaitForIdle(h.ctx); err != nil {
		h.respondError(*msg.ID, -32603, "wait for idle: "+err.Error())
		return
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostShowPicker(msg *Message) {
	var params HostShowPickerParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}
	items := make([]ext.PickerItem, len(params.Items))
	for i, it := range params.Items {
		items[i] = ext.PickerItem{ID: it.ID, Label: it.Label, Desc: it.Desc}
	}
	// Capture msg.ID by value — the closure may fire on a different goroutine
	// after the TUI processes the picker action.
	id := *msg.ID
	h.app.ShowPicker(params.Title, items, func(selected ext.PickerItem) {
		h.respond(id, HostShowPickerResult{Selected: selected.ID})
	})
	// Do NOT call respond here — the callback will send the response when the
	// user selects. If no selection occurs within hostRequestTimeout, the
	// extension's request will time out and the orphaned callback becomes
	// a no-op (h.respond writes to the already-closed or ignored response path).
}
