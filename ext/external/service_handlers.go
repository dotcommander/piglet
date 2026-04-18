package external

import (
	"context"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

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
