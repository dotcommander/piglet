package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dotcommander/piglet/core"
	pigletprovider "github.com/dotcommander/piglet/provider"
	sdk "github.com/dotcommander/piglet/sdk"
)

// Register adds provider registrations and the stream handler to the extension.
// RegisterProvider sends notifications immediately, so calls are deferred to
// OnInitAppend to ensure the RPC pipe (FD 4) is open.
func Register(e *sdk.Extension) {
	cfg := loadProviderConfig()

	e.OnInitAppend(func(e *sdk.Extension) {
		e.RegisterProvider("openai")
		e.RegisterProvider("anthropic")
		e.RegisterProvider("google")

		// Register custom providers from config
		for name := range cfg.Custom {
			e.RegisterProvider(name)
		}
	})

	e.OnProviderStream(func(ctx context.Context, x *sdk.Extension, req sdk.ProviderStreamRequest) (*sdk.ProviderStreamResponse, error) {
		var model core.Model
		if err := json.Unmarshal(req.Model, &model); err != nil {
			return nil, fmt.Errorf("unmarshal model: %w", err)
		}

		// Apply config-driven overrides: proxy baseUrl, custom providers, compat quirks
		applyOverrides(cfg, &model)

		key, err := x.AuthGetKey(ctx, model.Provider)
		if err != nil {
			return nil, fmt.Errorf("get api key: %w", err)
		}

		// Fallback: if the host has no key, try key_command from provider config.
		// Supports three formats: "!shell-command", "ENV_VAR_NAME", or literal.
		// Shell command results are cached for the process lifetime.
		if key == "" {
			if cmd := resolveKeyCommand(cfg, model.Provider); cmd != "" {
				resolved, resolveErr := resolveKey(ctx, model.Provider, cmd)
				if resolveErr != nil {
					return nil, fmt.Errorf("key resolution for %q failed: %w", model.Provider, resolveErr)
				}
				key = resolved
			}
		}

		if key == "" {
			return nil, fmt.Errorf("no API key configured for provider %q", model.Provider)
		}

		apiKeyFn := func() string { return key }
		var prov core.StreamProvider
		switch model.API {
		case core.APIAnthropic:
			prov = NewAnthropic(model, apiKeyFn)
		case core.APIGoogle:
			prov = NewGoogle(model, apiKeyFn)
		default:
			// APIOpenAI and any OpenAI-compatible endpoints
			prov = pigletprovider.NewOpenAI(model, apiKeyFn)
		}

		streamReq, err := buildStreamRequest(req)
		if err != nil {
			return nil, fmt.Errorf("build stream request: %w", err)
		}

		var lastMessage *core.AssistantMessage
		for evt := range prov.Stream(ctx, streamReq) {
			switch evt.Type {
			case core.StreamTextDelta, core.StreamThinkingDelta, core.StreamToolCallDelta:
				x.SendProviderDelta(req.RequestID, evt.Type, evt.Index, evt.Delta, nil)
			case core.StreamToolCallEnd:
				if evt.Tool != nil {
					argsJSON, _ := json.Marshal(evt.Tool.Arguments)
					x.SendProviderDelta(req.RequestID, evt.Type, evt.Index, "", &sdk.ProviderToolCall{
						ID:        evt.Tool.ID,
						Name:      evt.Tool.Name,
						Arguments: string(argsJSON),
					})
				}
			case core.StreamDone:
				lastMessage = evt.Message
			case core.StreamError:
				return nil, evt.Error
			}
		}

		if lastMessage == nil {
			return nil, fmt.Errorf("stream ended without a final message")
		}

		msgJSON, err := json.Marshal(lastMessage)
		if err != nil {
			return nil, fmt.Errorf("marshal final message: %w", err)
		}

		return &sdk.ProviderStreamResponse{Message: msgJSON}, nil
	})
}

// buildStreamRequest reconstructs a core.StreamRequest from the wire format.
func buildStreamRequest(req sdk.ProviderStreamRequest) (core.StreamRequest, error) {
	var sr core.StreamRequest
	sr.System = req.System

	if len(req.Messages) > 0 {
		msgs, err := unmarshalMessages(req.Messages)
		if err != nil {
			return sr, fmt.Errorf("unmarshal messages: %w", err)
		}
		sr.Messages = msgs
	}

	if len(req.Tools) > 0 {
		if err := json.Unmarshal(req.Tools, &sr.Tools); err != nil {
			return sr, fmt.Errorf("unmarshal tools: %w", err)
		}
	}

	if len(req.Options) > 0 {
		var opts struct {
			Temperature *float64           `json:"temperature,omitempty"`
			MaxTokens   *int               `json:"maxTokens,omitempty"`
			Thinking    core.ThinkingLevel `json:"thinking,omitempty"`
			Headers     map[string]string  `json:"headers,omitempty"`
		}
		if err := json.Unmarshal(req.Options, &opts); err != nil {
			return sr, fmt.Errorf("unmarshal options: %w", err)
		}
		sr.Options.Temperature = opts.Temperature
		sr.Options.MaxTokens = opts.MaxTokens
		sr.Options.Thinking = opts.Thinking
		sr.Options.Headers = opts.Headers
	}

	return sr, nil
}

// unmarshalMessages deserializes a JSON array of core.Message values.
// Discrimination is done by probing unique fields on each concrete type:
//   - toolCallId present → *core.ToolResultMessage
//   - provider or model present → *core.AssistantMessage
//   - otherwise → *core.UserMessage
func unmarshalMessages(data json.RawMessage) ([]core.Message, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, err
	}

	msgs := make([]core.Message, 0, len(raws))
	for _, raw := range raws {
		var probe struct {
			ToolCallID string `json:"toolCallId"`
			Provider   string `json:"provider"`
			Model      string `json:"model"`
		}
		_ = json.Unmarshal(raw, &probe)

		switch {
		case probe.ToolCallID != "":
			var m core.ToolResultMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				return nil, fmt.Errorf("unmarshal tool result message: %w", err)
			}
			msgs = append(msgs, &m)
		case probe.Provider != "" || probe.Model != "":
			var m core.AssistantMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				return nil, fmt.Errorf("unmarshal assistant message: %w", err)
			}
			msgs = append(msgs, &m)
		default:
			var m core.UserMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				return nil, fmt.Errorf("unmarshal user message: %w", err)
			}
			msgs = append(msgs, &m)
		}
	}
	return msgs, nil
}
