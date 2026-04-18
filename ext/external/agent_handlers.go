package external

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

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
