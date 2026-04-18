package external

import (
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
