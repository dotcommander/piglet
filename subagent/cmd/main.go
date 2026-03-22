// Subagent extension binary. Delegates tasks to independent sub-agents.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
//
// Uses host/listTools and host/executeTool to give sub-agents access to the
// host's registered tools (Read, Edit, Grep, Bash, etc.) without needing
// direct Go function references.
package main

import (
	"context"
	"fmt"

	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/provider"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

func main() {
	ext := sdk.New("subagent", "0.1.0")

	prompt, _ := config.ReadExtensionConfig("subagent")

	ext.RegisterTool(sdk.ToolDef{
		Name:        "dispatch",
		Description: "Delegate a task to an independent sub-agent that runs to completion and returns results. Use for research, analysis, or any task that benefits from focused execution with its own context.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":      map[string]any{"type": "string", "description": "Task instructions for the sub-agent"},
				"context":   map[string]any{"type": "string", "description": "Additional context to include in the sub-agent's system prompt"},
				"tools":     map[string]any{"type": "string", "enum": []any{"read_only", "all"}, "description": "Tool access level (default: read_only)"},
				"max_turns": map[string]any{"type": "integer", "description": "Maximum turns for the sub-agent"},
				"model":     map[string]any{"type": "string", "description": "Model override (e.g. anthropic/claude-haiku-4-5)"},
				"prefer":    map[string]any{"type": "string", "enum": []any{"default", "small"}, "description": "Model preference: default (main model) or small (cheaper model for background tasks)"},
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

			// Get host tools via the protocol
			filter := "background_safe"
			if access, _ := args["tools"].(string); access == "all" {
				filter = "all"
			}
			tools := resolveHostTools(ctx, ext, filter)

			sub := core.NewAgent(core.AgentConfig{
				System:   system,
				Provider: prov,
				Tools:    tools,
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
			fmt.Fprintf(&b, "[sub-agent: %d turns, %dk in / %dk out tokens]\n\n", turns, totalIn/1000, totalOut/1000)
			b.WriteString(result)
			return sdk.TextResult(b.String()), nil
		},
	})

	ext.Run()
}

// Tool caches — host tools don't change during a session.
var (
	cachedAllTools []core.Tool
	cachedBgTools  []core.Tool
)

// resolveHostTools returns host tool proxies, caching after first query per filter.
func resolveHostTools(ctx context.Context, ext *sdk.Extension, filter string) []core.Tool {
	if filter == "all" && cachedAllTools != nil {
		return cachedAllTools
	}
	if filter == "background_safe" && cachedBgTools != nil {
		return cachedBgTools
	}

	infos, err := ext.ListHostTools(ctx, filter)
	if err != nil || len(infos) == 0 {
		return nil
	}

	tools := make([]core.Tool, len(infos))
	for i, info := range infos {
		name := info.Name
		tools[i] = core.Tool{
			ToolSchema: core.ToolSchema{
				Name:        info.Name,
				Description: info.Description,
				Parameters:  info.Parameters,
			},
			Execute: func(ctx context.Context, _ string, args map[string]any) (*core.ToolResult, error) {
				result, err := ext.CallHostTool(ctx, name, args)
				if err != nil {
					return nil, err
				}
				blocks := make([]core.ContentBlock, len(result.Content))
				for j, b := range result.Content {
					switch b.Type {
					case "image":
						blocks[j] = core.ImageContent{Data: b.Data, MimeType: b.Mime}
					default:
						blocks[j] = core.TextContent{Text: b.Text}
					}
				}
				return &core.ToolResult{Content: blocks}, nil
			},
		}
	}

	if filter == "all" {
		cachedAllTools = tools
	} else {
		cachedBgTools = tools
	}
	return tools
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
		prefer, _ := args["prefer"].(string)
		if prefer == "small" {
			modelQuery = settings.ResolveSmallModel()
		} else {
			modelQuery = settings.ResolveDefaultModel()
		}
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
