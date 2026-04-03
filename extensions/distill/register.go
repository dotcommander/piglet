// Package distill extracts reusable skills from completed sessions.
// Skills are written as markdown files to ~/.config/piglet/skills/.
package distill

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/extract-prompt.md
var defaultPrompt string

const (
	complexityThreshold = 20
	chatTimeout         = 30 * time.Second
)

// Register adds distill's event handler and /distill command to the extension.
func Register(e *sdk.Extension) {
	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "distill-auto",
		Priority: 200,
		Events:   []string{"EventAgentEnd"},
		Handle: func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
			var evt struct {
				Messages []json.RawMessage `json:"Messages"`
			}
			if err := json.Unmarshal(data, &evt); err != nil || len(evt.Messages) < 2 {
				return nil
			}

			score := scoreComplexity(evt.Messages)
			if score < complexityThreshold {
				return nil
			}

			messages := evt.Messages
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
				defer cancel()
				path, err := distillSession(ctx, e, messages)
				if err != nil {
					e.Log("debug", fmt.Sprintf("[distill] auto-distill skipped: %v", err))
					return
				}
				e.Log("info", fmt.Sprintf("[distill] skill written: %s", path))
			}()

			return nil
		},
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "distill",
		Description: "Extract a reusable skill from a session (current, list, or <session-id>)",
		Handler: func(ctx context.Context, args string) error {
			sub := strings.TrimSpace(args)
			switch {
			case sub == "" || sub == "current":
				return handleDistillCurrent(ctx, e)
			case sub == "list":
				return handleDistillList(e)
			default:
				return handleDistillSession(ctx, e, sub)
			}
		},
	})
}

func handleDistillCurrent(ctx context.Context, e *sdk.Extension) error {
	raw, err := e.ConversationMessages(ctx)
	if err != nil {
		e.ShowMessage("distill: failed to get messages: " + err.Error())
		return nil
	}

	var messages []json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		e.ShowMessage("distill: failed to parse messages: " + err.Error())
		return nil
	}

	if len(messages) < 2 {
		e.ShowMessage("distill: session too short to extract a skill")
		return nil
	}

	tctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	path, err := distillSession(tctx, e, messages)
	if err != nil {
		e.ShowMessage("distill: " + err.Error())
		return nil
	}
	e.ShowMessage(fmt.Sprintf("Skill written: %s", path))
	return nil
}

func handleDistillList(e *sdk.Extension) error {
	base, err := xdg.ConfigDir()
	if err != nil {
		e.ShowMessage("distill: " + err.Error())
		return nil
	}
	dir := filepath.Join(base, "skills")

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			e.ShowMessage("No distilled skills found.")
			return nil
		}
		e.ShowMessage("distill list: " + err.Error())
		return nil
	}

	var b strings.Builder
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if !isDistilledSkill(path) {
			continue
		}
		b.WriteString("  ")
		b.WriteString(entry.Name())
		b.WriteByte('\n')
		count++
	}

	if count == 0 {
		e.ShowMessage("No distilled skills found. Run /distill or /distill current to extract one.")
		return nil
	}
	e.ShowMessage(fmt.Sprintf("Distilled skills (%d):\n\n%s", count, b.String()))
	return nil
}

func handleDistillSession(ctx context.Context, e *sdk.Extension, sessionID string) error {
	sessions, err := e.Sessions(ctx)
	if err != nil {
		e.ShowMessage("distill: failed to list sessions: " + err.Error())
		return nil
	}

	var path string
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, sessionID) || s.ID == sessionID {
			path = s.Path
			break
		}
	}

	if path == "" {
		e.ShowMessage(fmt.Sprintf("distill: session %q not found", sessionID))
		return nil
	}

	messages, err := readSessionMessages(path)
	if err != nil {
		e.ShowMessage("distill: " + err.Error())
		return nil
	}

	if len(messages) < 2 {
		e.ShowMessage("distill: session too short to extract a skill")
		return nil
	}

	tctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	skillPath, err := distillSession(tctx, e, messages)
	if err != nil {
		e.ShowMessage("distill: " + err.Error())
		return nil
	}
	e.ShowMessage(fmt.Sprintf("Skill written: %s", skillPath))
	return nil
}

// isDistilledSkill checks if a skill file contains "source: distill" in its frontmatter.
func isDistilledSkill(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Frontmatter is always in the first 2KB — no need to read the full file.
	buf := make([]byte, 2048)
	n, _ := f.Read(buf)
	return strings.Contains(string(buf[:n]), "source: distill")
}
