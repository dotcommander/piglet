package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

const defaultClearTurns = 3
const clearSizeThreshold = 4096

// wireMsg wraps a message with a type discriminator for JSON transport.
// Matches the host's CompactMessage wire format used by ConversationMessages.
type wireMsg struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// wireToolResult is the wire representation of a ToolResultMessage.
type wireToolResult struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// clearerConfig holds the configurable turn threshold for the micro-compactor.
type clearerConfig struct {
	Turns int `yaml:"clear_turns"`
}

func defaultClearerConfig() clearerConfig {
	return clearerConfig{Turns: defaultClearTurns}
}

// loadClearerConfig reads the clearer config from the extension's namespaced directory.
func loadClearerConfig() clearerConfig {
	return xdg.LoadYAMLExt("memory", "memory-clearer.yaml", defaultClearerConfig())
}

// registerClearer registers an EventTurnEnd handler that clears old tool results
// exceeding the size threshold to save context space.
func registerClearer(x *sdk.Extension) {
	cfg := loadClearerConfig()

	x.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "memory-clearer",
		Priority: 60, // after memory-extractor (50)
		Events:   []string{"EventTurnEnd"},
		Handle: func(ctx context.Context, _ string, _ json.RawMessage) *sdk.Action {
			cleared := clearOldToolResults(ctx, x, cfg.Turns)
			if cleared > 0 {
				x.Log("debug", fmt.Sprintf("[memory-clearer] cleared %d tool result(s)", cleared))
			}
			return nil
		},
	})
}

// clearOldToolResults reads all messages, clears old large tool results,
// and writes the modified list back via the host.
func clearOldToolResults(ctx context.Context, x *sdk.Extension, turnThreshold int) int {
	rawMsgs, err := x.ConversationMessages(ctx)
	if err != nil {
		return 0
	}

	var messages []wireMsg
	if err := json.Unmarshal(rawMsgs, &messages); err != nil {
		return 0
	}

	if len(messages) <= turnThreshold+1 {
		return 0
	}

	cutoff := len(messages) - turnThreshold
	cleared := 0

	for i := range cutoff {
		msg := messages[i]
		if msg.Type != "tool_result" {
			continue
		}

		var tr wireToolResult
		if json.Unmarshal(msg.Data, &tr) != nil {
			continue
		}

		textSize := 0
		for _, c := range tr.Content {
			if c.Type == "text" {
				textSize += len(c.Text)
			}
		}
		if textSize <= clearSizeThreshold {
			continue
		}

		// Replace content with a compact placeholder
		placeholder := fmt.Sprintf(
			"[Old tool result content cleared to save context — %s]",
			tr.ToolName,
		)
		tr.Content = []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: placeholder}}

		data, err := json.Marshal(tr)
		if err != nil {
			continue
		}
		messages[i].Data = data
		cleared++
	}

	if cleared == 0 {
		return 0
	}

	updated, err := json.Marshal(messages)
	if err != nil {
		return 0
	}

	if err := x.SetConversationMessages(ctx, updated); err != nil {
		x.Log("debug", fmt.Sprintf("[memory-clearer] set messages failed: %v", err))
		return 0
	}

	return cleared
}
