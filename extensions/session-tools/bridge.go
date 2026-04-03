package sessiontools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

// RegisterBridge adds the session context bridge:
// 1. On startup (OnInit): injects previous session's summary as a prompt section
// 2. On session end (EventAgentEnd): saves a compact summary for next session
func RegisterBridge(e *sdk.Extension) {
	e.OnInitAppend(func(x *sdk.Extension) {
		cwd := x.CWD()
		if cwd == "" {
			return
		}

		content, err := loadBridge(cwd)
		if err != nil || content == "" {
			return
		}

		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Previous Session Context",
			Content: content,
			Order:   90, // After memory (50), before session handoff (95)
		})
	})

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "session-bridge",
		Priority: 300, // After distill (200)
		Events:   []string{"EventAgentEnd"},
		Handle: func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
			var evt struct {
				Messages []json.RawMessage `json:"Messages"`
			}
			if err := json.Unmarshal(data, &evt); err != nil || len(evt.Messages) < 2 {
				return nil
			}

			// Find the last assistant message
			lastAssistant := extractLastAssistant(evt.Messages)
			if lastAssistant == "" {
				return nil
			}

			// Truncate to keep the bridge compact
			const maxLen = 2000
			summary := lastAssistant
			if len(summary) > maxLen {
				summary = summary[:maxLen] + "\n[truncated]"
			}

			// Save with timestamp
			bridge := fmt.Sprintf("Last session ended %s:\n\n%s",
				time.Now().Format("2006-01-02 15:04"),
				summary)

			cwdVal := e.CWD()
			if cwdVal != "" {
				_ = saveBridge(cwdVal, bridge)
			}

			return nil
		},
	})
}

// extractLastAssistant finds the last assistant message in a message list and
// returns its text content. Handles both plain string and content-block formats.
func extractLastAssistant(messages []json.RawMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(messages[i], &msg); err != nil {
			continue
		}
		if msg.Role != "assistant" {
			continue
		}

		text := extractTextContent(msg.Content)
		if text != "" {
			return text
		}
	}
	return ""
}

// extractTextContent pulls readable text out of a content field (string or []block).
func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try as plain string first
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}

	// Try as content blocks array
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}

	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// bridgePath returns the file path for the bridge summary for a given CWD.
func bridgePath(cwdVal string) (string, error) {
	dir, err := xdg.ExtensionDir("session-handoff")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(cwdVal))
	name := "bridge-" + hex.EncodeToString(sum[:])[:12] + ".md"
	return filepath.Join(dir, name), nil
}

func loadBridge(cwdVal string) (string, error) {
	path, err := bridgePath(cwdVal)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func saveBridge(cwdVal, content string) error {
	path, err := bridgePath(cwdVal)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
