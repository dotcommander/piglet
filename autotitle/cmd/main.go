// Autotitle extension binary. Generates session titles after first exchange.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
// Creates its own LLM provider to generate titles independently.
package main

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/dotcommander/piglet/autotitle"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/provider"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

func main() {
	e := sdk.New("autotitle", "0.1.0")

	prompt := autotitle.LoadPrompt()
	if prompt == "" {
		// No prompt file — run as no-op
		e.Run()
		return
	}

	var fired atomic.Bool

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "autotitle",
		Priority: 100,
		Events:   []string{"EventAgentEnd"},
		Handle: func(_ context.Context, eventType string, data json.RawMessage) *sdk.Action {
			if !fired.CompareAndSwap(false, true) {
				return nil
			}

			// Parse EventAgentEnd to get conversation messages
			var evt struct {
				Messages []json.RawMessage `json:"Messages"`
			}
			if err := json.Unmarshal(data, &evt); err != nil || len(evt.Messages) < 2 {
				fired.Store(false)
				return nil
			}

			// Extract first user and assistant text from messages
			userText, assistantText := extractFirstExchange(evt.Messages)
			if userText == "" {
				fired.Store(false)
				return nil
			}

			// Create a lightweight provider for title generation
			prov := createProvider()
			if prov == nil {
				fired.Store(false)
				return nil
			}

			title := autotitle.GenerateTitle(context.Background(), prov, buildMessages(userText, assistantText), prompt)
			if title != "" {
				return sdk.ActionSetSessionTitle(title)
			}
			return nil
		},
	})

	e.Run()
}

func createProvider() core.StreamProvider {
	auth, err := config.NewAuthDefault()
	if err != nil {
		return nil
	}

	settings, err := config.Load()
	if err != nil {
		return nil
	}

	modelQuery := settings.ResolveSmallModel()
	if modelQuery == "" {
		return nil
	}

	registry := provider.NewRegistry()
	model, ok := registry.Resolve(modelQuery)
	if !ok {
		return nil
	}

	prov, err := registry.Create(model, func() string {
		return auth.GetAPIKey(model.Provider)
	})
	if err != nil {
		return nil
	}
	return prov
}

func extractFirstExchange(messages []json.RawMessage) (userText, assistantText string) {
	for _, raw := range messages {
		var msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if msg.Role == "user" && userText == "" {
			userText = msg.Content
		}
		if msg.Role == "assistant" && assistantText == "" {
			assistantText = msg.Content
		}
		if userText != "" && assistantText != "" {
			break
		}
	}
	return
}

func buildMessages(userText, assistantText string) []core.Message {
	msgs := []core.Message{
		&core.UserMessage{Content: userText},
	}
	if assistantText != "" {
		msgs = append(msgs, &core.AssistantMessage{
			Content: []core.AssistantContent{core.TextContent{Text: assistantText}},
		})
	}
	return msgs
}
