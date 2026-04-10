package external

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// hostRequestTimeout caps how long an inbound extension request (tool call, chat,
// agent run) may run before cancellation.
const hostRequestTimeout = 5 * time.Minute

// decodeParams unmarshals msg.Params into dst, responding with error on failure.
func (h *Host) decodeParams(msg *Message, dst any) bool {
	if err := json.Unmarshal(msg.Params, dst); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return false
	}
	return true
}

// requireApp checks h.app is bound, responding with error if nil.
func (h *Host) requireApp(msg *Message) bool {
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return false
	}
	return true
}

// handleAppCall decodes params, requires app, calls fn, and responds.
// fn receives the decoded params and the bound app.
// If fn returns an error, responds with error. Otherwise responds with fn's result.
func handleAppCall[P any, R any](h *Host, msg *Message, fn func(*ext.App, P) (R, error)) {
	var params P
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}
	result, err := fn(h.app, params)
	if err != nil {
		h.respondError(*msg.ID, -32603, err.Error())
		return
	}
	h.respond(*msg.ID, result)
}

// resolveProvider resolves a StreamProvider for the given model, responding
// with error on failure. Returns (provider, true) on success.
func (h *Host) resolveProvider(msg *Message, model string) (core.StreamProvider, bool) {
	if h.resolveProviderFn == nil {
		h.respondError(*msg.ID, -32603, "provider resolver not available")
		return nil, false
	}
	prov, err := h.resolveProviderFn(model)
	if err != nil {
		h.respondError(*msg.ID, -32603, "resolve provider: "+err.Error())
		return nil, false
	}
	return prov, true
}

// requireUndoSnapshots checks h.undoSnapshotsFn is set, responding with error if nil.
func (h *Host) requireUndoSnapshots(msg *Message) bool {
	if h.undoSnapshotsFn == nil {
		h.respondError(*msg.ID, -32603, "undo snapshots not available")
		return false
	}
	return true
}

// handleHostListTools returns the list of available host tools with their schemas.
func (h *Host) handleHostListTools(msg *Message) {
	var params HostListToolsParams
	if !h.decodeParams(msg, &params) {
		return
	}

	if h.app == nil {
		h.respond(*msg.ID, HostListToolsResult{})
		return
	}

	var defs []*ext.ToolDef
	if params.Filter == FilterBackgroundSafe {
		for _, td := range h.app.ToolDefs() {
			if td.BackgroundSafe {
				defs = append(defs, td)
			}
		}
	} else {
		defs = h.app.ToolDefs()
	}

	infos := make([]HostToolInfo, len(defs))
	for i, td := range defs {
		infos[i] = HostToolInfo{
			Name:        td.Name,
			Description: td.Description,
			Parameters:  td.Parameters,
			Deferred:    td.Deferred,
		}
	}
	h.respond(*msg.ID, HostListToolsResult{Tools: infos})
}

// handleHostExecuteTool executes a host-registered tool on behalf of the extension.
func (h *Host) handleHostExecuteTool(msg *Message) {
	ctx, cancel := context.WithTimeout(h.ctx, hostRequestTimeout)
	defer cancel()

	var params HostExecuteToolParams
	if !h.decodeParams(msg, &params) {
		return
	}

	if !h.requireApp(msg) {
		return
	}

	// Look up the tool in the host registry
	tool := h.app.FindTool(params.Name)
	if tool == nil {
		h.respondError(*msg.ID, -32604, "unknown tool: "+params.Name)
		return
	}

	// Execute the tool with per-request context
	result, err := tool.Execute(ctx, params.CallID, params.Args)
	if err != nil {
		if ctx.Err() != nil {
			h.respondError(*msg.ID, -32603, "tool execution cancelled: "+ctx.Err().Error())
			return
		}
		h.respond(*msg.ID, HostExecuteToolResult{
			Content: []ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
		return
	}

	h.respond(*msg.ID, HostExecuteToolResult{Content: coreToWire(result.Content)})
}

func (h *Host) handleHostConfigGet(msg *Message) {
	var params HostConfigGetParams
	if !h.decodeParams(msg, &params) {
		return
	}

	settings, err := config.Load()
	if err != nil {
		h.respondError(*msg.ID, -32603, "load config: "+err.Error())
		return
	}

	values := make(map[string]any, len(params.Keys))
	for _, key := range params.Keys {
		switch key {
		case "defaultModel":
			values[key] = settings.ResolveDefaultModel()
		case "smallModel":
			values[key] = settings.ResolveSmallModel()
		case "agent.compactAt":
			values[key] = config.IntOr(settings.Agent.CompactAt, 0)
		case "agent.maxTurns":
			values[key] = config.IntOr(settings.Agent.MaxTurns, config.DefaultMaxTurns)
		}
	}
	h.respond(*msg.ID, HostConfigGetResult{Values: values})
}

func (h *Host) handleHostConfigReadExt(msg *Message) {
	var params HostConfigReadExtParams
	if !h.decodeParams(msg, &params) {
		return
	}

	content, err := config.ReadExtensionConfig(params.Name)
	if err != nil {
		h.respondError(*msg.ID, -32603, "read extension config: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostConfigReadExtResult{Content: content})
}

func (h *Host) handleHostAuthGetKey(msg *Message) {
	var params HostAuthGetKeyParams
	if !h.decodeParams(msg, &params) {
		return
	}

	auth, err := config.NewAuthDefault()
	if err != nil {
		h.respondError(*msg.ID, -32603, "load auth: "+err.Error())
		return
	}

	key := auth.GetAPIKey(params.Provider)
	h.respond(*msg.ID, HostAuthGetKeyResult{Key: key})
}

func (h *Host) handleHostChat(msg *Message) {
	ctx, cancel := context.WithTimeout(h.ctx, hostRequestTimeout)
	defer cancel()

	var params HostChatParams
	if !h.decodeParams(msg, &params) {
		return
	}

	prov, ok := h.resolveProvider(msg, params.Model)
	if !ok {
		return
	}

	msgs := make([]core.Message, len(params.Messages))
	for i, m := range params.Messages {
		switch m.Role {
		case "assistant":
			msgs[i] = &core.AssistantMessage{
				Content: []core.AssistantContent{core.TextContent{Text: m.Content}},
			}
		default:
			msgs[i] = &core.UserMessage{Content: m.Content}
		}
	}

	req := core.StreamRequest{
		System:   params.System,
		Messages: msgs,
	}
	if params.MaxTokens > 0 {
		req.Options.MaxTokens = &params.MaxTokens
	}

	ch := prov.Stream(ctx, req)

	var text strings.Builder
	var usage HostTokenUsage
	for evt := range ch {
		switch evt.Type {
		case core.StreamTextDelta:
			text.WriteString(evt.Delta)
		case core.StreamDone:
			if evt.Message != nil {
				usage.Input += evt.Message.Usage.InputTokens
				usage.Output += evt.Message.Usage.OutputTokens
			}
		case core.StreamError:
			if ctx.Err() != nil {
				h.respondError(*msg.ID, -32603, "chat cancelled: "+ctx.Err().Error())
			} else {
				h.respondError(*msg.ID, -32603, "chat error: "+evt.Error.Error())
			}
			return
		}
	}

	h.respond(*msg.ID, HostChatResult{Text: text.String(), Usage: usage})
}

func (h *Host) handleHostAgentRun(msg *Message) {
	ctx, cancel := context.WithTimeout(h.ctx, hostRequestTimeout)
	defer cancel()

	var params HostAgentRunParams
	if !h.decodeParams(msg, &params) {
		return
	}

	if !h.requireApp(msg) {
		return
	}

	prov, ok := h.resolveProvider(msg, params.Model)
	if !ok {
		return
	}

	var tools []core.Tool
	if params.Tools == "all" {
		tools = h.app.CoreTools()
	} else {
		tools = h.app.BackgroundSafeTools()
	}

	maxTurns := params.MaxTurns
	if maxTurns <= 0 {
		maxTurns = config.DefaultMaxTurns
	}

	sub := core.NewAgent(core.AgentConfig{
		System:   params.System,
		Provider: prov,
		Tools:    tools,
		MaxTurns: maxTurns,
	})

	ch := sub.Start(ctx, params.Task)

	var resultBuilder strings.Builder
	var totalIn, totalOut, turns int
	for evt := range ch {
		if te, ok := evt.(core.EventTurnEnd); ok {
			turns++
			if te.Assistant != nil {
				totalIn += te.Assistant.Usage.InputTokens
				totalOut += te.Assistant.Usage.OutputTokens
				for _, c := range te.Assistant.Content {
					if tc, ok := c.(core.TextContent); ok {
						if resultBuilder.Len() > 0 {
							resultBuilder.WriteByte('\n')
						}
						resultBuilder.WriteString(tc.Text)
					}
				}
			}
		}
	}

	h.respond(*msg.ID, HostAgentRunResult{
		Text:  resultBuilder.String(),
		Turns: turns,
		Usage: HostTokenUsage{Input: totalIn, Output: totalOut},
	})
}

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

func (h *Host) handleHostSyncModels(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	updated, err := h.app.SyncModels()
	if err != nil {
		h.respondError(*msg.ID, -32603, "sync models: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostSyncModelsResult{Updated: updated})
}

func (h *Host) handleHostWriteModels(msg *Message) {
	var params HostWriteModelsParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}
	overrides := make(map[string]ext.ModelOverride, len(params.Overrides))
	for k, v := range params.Overrides {
		overrides[k] = ext.ModelOverride{
			Name:          v.Name,
			ContextWindow: v.ContextWindow,
			MaxTokens:     v.MaxTokens,
		}
	}
	n, err := h.app.WriteModelsWithOverrides(overrides)
	if err != nil {
		h.respondError(*msg.ID, -32603, "write models: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostWriteModelsResult{ModelsWritten: n})
}

func (h *Host) handleHostRunBackground(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostRunBackgroundParams) (struct{}, error) {
		return struct{}{}, fmt.Errorf("run background: %w", app.RunBackground(p.Prompt))
	})
}

func (h *Host) handleHostCancelBackground(msg *Message) {
	if h.app != nil {
		h.app.CancelBackground()
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostIsBackgroundRunning(msg *Message) {
	running := h.app != nil && h.app.IsBackgroundRunning()
	h.respond(*msg.ID, HostIsBackgroundRunningResult{Running: running})
}

func (h *Host) handleHostExtInfos(msg *Message) {
	if h.app == nil {
		h.respond(*msg.ID, HostExtInfosResult{})
		return
	}
	infos := h.app.ExtInfos()
	wires := make([]WireExtInfo, len(infos))
	for i, info := range infos {
		wires[i] = WireExtInfo{
			Name:          info.Name,
			Version:       info.Version,
			Kind:          info.Kind,
			Runtime:       info.Runtime,
			Tools:         info.Tools,
			Commands:      info.Commands,
			Interceptors:  info.Interceptors,
			EventHandlers: info.EventHandlers,
			Shortcuts:     info.Shortcuts,
			MessageHooks:  info.MessageHooks,
			Compactor:     info.Compactor,
		}
	}
	h.respond(*msg.ID, HostExtInfosResult{Extensions: wires})
}

func (h *Host) handleHostExtensionsDir(msg *Message) {
	dir, err := ExtensionsDir()
	if err != nil {
		h.respondError(*msg.ID, -32603, "extensions dir: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostExtensionsDirResult{Path: dir})
}

func (h *Host) handleHostUndoSnapshots(msg *Message) {
	if !h.requireUndoSnapshots(msg) {
		return
	}
	snapshots, err := h.undoSnapshotsFn()
	if err != nil {
		h.respondError(*msg.ID, -32603, "undo snapshots: "+err.Error())
		return
	}
	sizes := make(map[string]int, len(snapshots))
	for path, data := range snapshots {
		sizes[path] = len(data)
	}
	h.respond(*msg.ID, HostUndoSnapshotsResult{Snapshots: sizes})
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

func (h *Host) handleHostPublish(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostPublishParams) (struct{}, error) {
		app.Publish(p.Topic, p.Data)
		return struct{}{}, nil
	})
}

func (h *Host) handleHostActivateTool(msg *Message) {
	var params HostActivateToolParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}
	if !h.app.ActivateTool(params.Name) {
		h.respondError(*msg.ID, -32604, "tool not found or not deferred: "+params.Name)
		return
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostSetToolFilter(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostSetToolFilterParams) (struct{}, error) {
		app.SetToolFilterByName(p.Names)
		return struct{}{}, nil
	})
}

func (h *Host) handleHostSubscribe(msg *Message) {
	var params HostSubscribeParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}

	// Generate subscription ID
	subID := int(h.nextID.Add(1))

	// Subscribe on the event bus and forward events as notifications to the extension
	unsub := h.app.Subscribe(params.Topic, func(data any) {
		dataJSON, _ := json.Marshal(data)
		h.sendNotification(MethodHostEventBusEvent, EventBusEventParams{
			Topic:          params.Topic,
			SubscriptionID: subID,
			Data:           dataJSON,
		})
	})

	// Track unsubscribe function for cleanup
	h.subsMu.Lock()
	h.subscriptions[subID] = unsub
	h.subsMu.Unlock()

	h.respond(*msg.ID, HostSubscribeResult{SubscriptionID: subID})
}

func (h *Host) handleHostUndoRestore(msg *Message) {
	var params HostUndoRestoreParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireUndoSnapshots(msg) {
		return
	}
	snapshots, err := h.undoSnapshotsFn()
	if err != nil {
		h.respondError(*msg.ID, -32603, "undo snapshots: "+err.Error())
		return
	}
	data, ok := snapshots[params.Path]
	if !ok {
		h.respondError(*msg.ID, -32604, "no snapshot for path: "+params.Path)
		return
	}
	if err := config.AtomicWrite(params.Path, data, 0600); err != nil {
		h.respondError(*msg.ID, -32603, "restore file: "+err.Error())
		return
	}
	h.respond(*msg.ID, struct{}{})
}
