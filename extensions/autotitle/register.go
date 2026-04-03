// Package autotitle provides automatic session title generation for piglet.
package autotitle

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/prompt.md
var defaultPrompt string

const (
	maxTitleRunes = 50
	chatTimeout   = 10 * time.Second
	chatModel     = "small"
)

// Register adds autotitle's event handler to the extension.
func Register(e *sdk.Extension) {
	var fired atomic.Bool

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "autotitle",
		Priority: 100,
		Events:   []string{"EventAgentEnd"},
		Handle: func(_ context.Context, _ string, data json.RawMessage) (result *sdk.Action) {
			if !fired.CompareAndSwap(false, true) {
				return nil
			}
			defer func() {
				if result == nil {
					fired.Store(false)
				}
			}()

			p := xdg.LoadOrCreateExt("autotitle", "prompt.md", strings.TrimSpace(defaultPrompt))
			if p == "" {
				return nil
			}

			var evt struct {
				Messages []json.RawMessage `json:"Messages"`
			}
			if err := json.Unmarshal(data, &evt); err != nil || len(evt.Messages) < 2 {
				return nil
			}

			userText, assistantText := extractFirstExchange(evt.Messages)
			if userText == "" {
				return nil
			}

			content := "User: " + truncateTitle(userText, 200) + "\n\nAssistant: " + truncateTitle(assistantText, 200)

			ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
			defer cancel()
			resp, err := e.Chat(ctx, sdk.ChatRequest{
				System:    p,
				Messages:  []sdk.ChatMessage{{Role: "user", Content: content}},
				Model:     chatModel,
				MaxTokens: 30,
			})
			if err != nil || resp.Text == "" {
				return nil
			}

			title := strings.TrimSpace(resp.Text)
			title = truncateTitle(title, maxTitleRunes)
			if title != "" {
				return sdk.ActionSetSessionTitle(title)
			}
			return nil
		},
	})
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

// truncateTitle truncates s to limit runes.
func truncateTitle(s string, limit int) string {
	runes := []rune(s)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return s
}
