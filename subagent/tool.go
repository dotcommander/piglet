package subagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// Tool access level constants.
const (
	AccessReadOnly = "read_only"
	AccessAll      = "all"
)

func registerTool(app *ext.App, cfg Config, cachedPrompt string) {
	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "dispatch",
			Description: "Delegate a task to an independent sub-agent that runs to completion and returns results. Use for research, analysis, or any task that benefits from focused execution with its own context.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task":      map[string]any{"type": "string", "description": "Task instructions for the sub-agent"},
					"context":   map[string]any{"type": "string", "description": "Additional context to include in the sub-agent's system prompt"},
					"tools":     map[string]any{"type": "string", "enum": []any{AccessReadOnly, AccessAll}, "description": "Tool access level (default: read_only)"},
					"max_turns": map[string]any{"type": "integer", "description": "Maximum turns for the sub-agent"},
					"model":     map[string]any{"type": "string", "description": "Model override (e.g. anthropic/claude-haiku-4-5)"},
				},
				"required": []any{"task"},
			},
		},
		Execute: func(ctx context.Context, _ string, args map[string]any) (*core.ToolResult, error) {
			task := ext.StringArg(args, "task")
			if task == "" {
				return ext.TextResult("error: task is required"), nil
			}

			// Select tools based on access level
			var tools []core.Tool
			if ext.StringArg(args, "tools") == AccessAll {
				tools = app.CoreTools()
			} else {
				tools = app.BackgroundSafeTools()
			}
			if len(tools) == 0 {
				return ext.TextResult("error: no tools available for sub-agent"), nil
			}

			// Resolve provider — use model override or current provider
			prov := app.Provider()
			if modelID := ext.StringArg(args, "model"); modelID != "" {
				_, switchedProv, err := app.ResolveModel(modelID)
				if err != nil {
					return ext.TextResult(fmt.Sprintf("error resolving model %q: %v", modelID, err)), nil
				}
				prov = switchedProv
			}
			if prov == nil {
				return ext.TextResult("error: no provider available"), nil
			}

			// Build system prompt from config file + optional context
			system := cachedPrompt
			if extra := ext.StringArg(args, "context"); extra != "" {
				system = system + "\n\n" + extra
			}

			// Resolve max turns
			maxTurns := cfg.MaxTurns
			if mt, ok := args["max_turns"].(float64); ok && int(mt) > 0 {
				maxTurns = int(mt)
			}

			// Create and run sub-agent
			sub := core.NewAgent(core.AgentConfig{
				System:   system,
				Provider: prov,
				Tools:    tools,
				MaxTurns: maxTurns,
			})

			ch := sub.Start(ctx, task)

			var result string
			var totalIn, totalOut int
			var turns int

			for evt := range ch {
				if te, ok := evt.(core.EventTurnEnd); ok {
					turns++
					if te.Assistant != nil {
						totalIn += te.Assistant.Usage.InputTokens
						totalOut += te.Assistant.Usage.OutputTokens
						// Keep last assistant text
						for _, c := range te.Assistant.Content {
							if tc, ok := c.(core.TextContent); ok {
								result = tc.Text
							}
						}
					}
				}
			}

			if result == "" {
				return ext.TextResult("[sub-agent completed with no text output]"), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "[sub-agent: %d turns, %d in / %d out tokens]\n\n", turns, totalIn, totalOut)
			b.WriteString(result)
			return ext.TextResult(b.String()), nil
		},
		PromptHint: "Delegate focused tasks to independent sub-agents for research, analysis, or exploration",
	})
}

// loadPrompt reads the sub-agent system prompt from ~/.config/piglet/subagent.md.
// Returns empty string if the file doesn't exist.
func loadPrompt() string {
	s, _ := config.ReadExtensionConfig("subagent")
	return s
}
