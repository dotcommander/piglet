package sessiontools

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// sessionToolsState holds mutable state shared across tool and command handlers.
type sessionToolsState struct {
	cwd string
	cfg Config
}

// Register registers the session-tools extension's commands, tools, and prompt
// section, and schedules OnInit work via OnInitAppend.
func Register(e *sdk.Extension) {
	st := &sessionToolsState{}

	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[session-tools] OnInit start")

		st.cwd = x.CWD()
		st.cfg = LoadConfig()

		content := LoadPromptContent()
		if content != "" {
			x.RegisterPromptSection(sdk.PromptSectionDef{
				Title:   "Session Handoff",
				Content: content,
				Order:   95,
			})
		}

		x.Log("debug", fmt.Sprintf("[session-tools] OnInit complete (%s)", time.Since(start)))
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "handoff",
		Description: "Transfer context to a new session with structured summary",
		Handler: func(ctx context.Context, args string) error {
			if !st.cfg.Enabled {
				e.ShowMessage("Session handoff is disabled in config.")
				return nil
			}
			focus := strings.TrimSpace(args)
			if err := Handoff(ctx, e, st.cwd, focus, st.cfg); err != nil {
				e.ShowMessage("Handoff failed: " + err.Error())
			}
			return nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "session_query",
		Description: "Search a session's JSONL file for content matching a keyword query. Use to recover specific details from a parent session after a handoff.",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_path": map[string]any{
					"type":        "string",
					"description": "Path to the session JSONL file (from SessionInfo.Path)",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Keyword or phrase to search for in session content",
				},
			},
			"required": []any{"session_path", "query"},
		},
		PromptHint: "Search a session file for specific content by keyword",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			path, _ := args["session_path"].(string)
			query, _ := args["query"].(string)

			if path == "" || query == "" {
				return sdk.ErrorResult("session_path and query are required"), nil
			}

			result, err := QuerySession(path, query, st.cfg.MaxQuerySize)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}

			return sdk.TextResult(result), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "handoff",
		Description: "Transfer context to a new session with a structured summary of current work. Use when explicitly asked to hand off, or when the session is very long and a fresh start would help.",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"focus": map[string]any{
					"type":        "string",
					"description": "Optional focus area or task for the new session",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Why handoff is needed (included in the summary)",
				},
			},
		},
		PromptHint: "Fork the session with a structured handoff summary",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if !st.cfg.Enabled {
				return sdk.ErrorResult("session handoff is disabled in config"), nil
			}

			focus, _ := args["focus"].(string)
			reason, _ := args["reason"].(string)
			if reason != "" && focus != "" {
				focus = focus + " (" + reason + ")"
			} else if reason != "" {
				focus = reason
			}

			if err := Handoff(ctx, e, st.cwd, focus, st.cfg); err != nil {
				return sdk.ErrorResult("handoff failed: " + err.Error()), nil
			}

			return sdk.TextResult("Handoff complete. Context transferred to new session."), nil
		},
	})

	RegisterBridge(e)
}
