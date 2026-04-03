package export

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dotcommander/piglet/sdk"
)

// Register registers the export extension's commands.
func Register(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "export",
		Description: "Export conversation",
		Handler: func(ctx context.Context, args string) error {
			raw, err := e.ConversationMessages(ctx)
			if err != nil {
				e.ShowMessage("Export failed: " + err.Error())
				return nil
			}

			var msgs []json.RawMessage
			if err := json.Unmarshal(raw, &msgs); err != nil || len(msgs) == 0 {
				e.ShowMessage("No messages to export")
				return nil
			}

			path := fmt.Sprintf("piglet-export-%s.md", time.Now().Format("20060102-150405"))
			if err := exportMarkdown(msgs, path); err != nil {
				e.ShowMessage("Export failed: " + err.Error())
				return nil
			}
			e.ShowMessage("Exported to " + path)
			return nil
		},
	})
}

type wireMessage struct {
	Role     string            `json:"role"`
	Content  json.RawMessage   `json:"content"`
	ToolName string            `json:"toolName,omitempty"`
	Blocks   []json.RawMessage `json:"blocks,omitempty"`
}

func exportMarkdown(msgs []json.RawMessage, path string) error {
	var b strings.Builder
	b.WriteString("# Piglet Conversation\n\n")

	for _, raw := range msgs {
		var msg wireMessage
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}

		switch msg.Role {
		case "user":
			b.WriteString("## User\n\n")
			var text string
			if json.Unmarshal(msg.Content, &text) == nil {
				b.WriteString(text)
			}
			b.WriteString("\n\n")

		case "assistant":
			b.WriteString("## Assistant\n\n")
			var blocks []json.RawMessage
			if json.Unmarshal(msg.Content, &blocks) == nil {
				for _, block := range blocks {
					var cb struct {
						Type     string `json:"type"`
						Text     string `json:"text"`
						Thinking string `json:"thinking"`
					}
					if json.Unmarshal(block, &cb) != nil {
						continue
					}
					switch cb.Type {
					case "text":
						b.WriteString(cb.Text)
					case "thinking":
						b.WriteString("<details><summary>Thinking</summary>\n\n")
						b.WriteString(cb.Thinking)
						b.WriteString("\n</details>")
					}
				}
			}
			b.WriteString("\n\n")

		case "tool_result":
			fmt.Fprintf(&b, "### Tool: %s\n\n", msg.ToolName)
			var blocks []json.RawMessage
			if json.Unmarshal(msg.Content, &blocks) == nil {
				for _, block := range blocks {
					var cb struct {
						Type string `json:"type"`
						Text string `json:"text"`
					}
					if json.Unmarshal(block, &cb) == nil && cb.Type == "text" {
						b.WriteString(cb.Text)
					}
				}
			}
			b.WriteString("\n\n")
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}
