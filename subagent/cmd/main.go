// Subagent extension binary. Delegates tasks to independent sub-agents.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
//
// NOTE: The subagent extension creates full agent loops internally, requiring
// access to all registered tools and a StreamProvider. As an external binary,
// it creates its own provider and uses a limited tool set (read-only file tools).
// For full tool access, use the compiled-in version.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/provider"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

func main() {
	e := sdk.New("subagent", "0.1.0")

	prompt, _ := config.ReadExtensionConfig("subagent")

	e.RegisterTool(sdk.ToolDef{
		Name:        "dispatch",
		Description: "Delegate a task to an independent sub-agent that runs to completion and returns results. Use for research, analysis, or any task that benefits from focused execution with its own context.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":      map[string]any{"type": "string", "description": "Task instructions for the sub-agent"},
				"context":   map[string]any{"type": "string", "description": "Additional context to include in the sub-agent's system prompt"},
				"max_turns": map[string]any{"type": "integer", "description": "Maximum turns for the sub-agent"},
				"model":     map[string]any{"type": "string", "description": "Model override (e.g. anthropic/claude-haiku-4-5)"},
			},
			"required": []any{"task"},
		},
		PromptHint: "Delegate focused tasks to independent sub-agents for research, analysis, or exploration",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			task, _ := args["task"].(string)
			if task == "" {
				return sdk.ErrorResult("task is required"), nil
			}

			prov := createProvider(args)
			if prov == nil {
				return sdk.ErrorResult("no provider available — check auth.json and config"), nil
			}

			system := prompt
			if extra, _ := args["context"].(string); extra != "" {
				system = system + "\n\n" + extra
			}

			maxTurns := 10
			if mt, ok := args["max_turns"].(float64); ok && int(mt) > 0 {
				maxTurns = int(mt)
			}

			sub := core.NewAgent(core.AgentConfig{
				System:   system,
				Provider: prov,
				Tools:    nil, // External subagent runs without tools (text-only)
				MaxTurns: maxTurns,
			})

			ch := sub.Start(ctx, task)

			var result string
			var totalIn, totalOut, turns int
			for evt := range ch {
				if te, ok := evt.(core.EventTurnEnd); ok {
					turns++
					if te.Assistant != nil {
						totalIn += te.Assistant.Usage.InputTokens
						totalOut += te.Assistant.Usage.OutputTokens
						for _, c := range te.Assistant.Content {
							if tc, ok := c.(core.TextContent); ok {
								result = tc.Text
							}
						}
					}
				}
			}

			if result == "" {
				return sdk.TextResult("[sub-agent completed with no text output]"), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "[sub-agent: %d turns, %d in / %d out tokens]\n\n", turns, totalIn, totalOut)
			b.WriteString(result)
			return sdk.TextResult(b.String()), nil
		},
	})

	e.Run()
}

func createProvider(args map[string]any) core.StreamProvider {
	auth, err := config.NewAuthDefault()
	if err != nil {
		return nil
	}

	settings, err := config.Load()
	if err != nil {
		return nil
	}

	registry := provider.NewRegistry()

	modelQuery, _ := args["model"].(string)
	if modelQuery == "" {
		modelQuery = os.Getenv("PIGLET_DEFAULT_MODEL")
	}
	if modelQuery == "" {
		modelQuery = settings.DefaultModel
	}
	if modelQuery == "" {
		return nil
	}

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
