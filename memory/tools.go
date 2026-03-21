package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

func registerTools(app *ext.App, store *Store) {
	app.RegisterTool(memorySetTool(store))
	app.RegisterTool(memoryGetTool(store))
	app.RegisterTool(memoryListTool(store))
}

func memorySetTool(store *Store) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "memory_set",
			Description: "Save a key-value fact to project memory, with an optional category.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":      map[string]any{"type": "string", "description": "Memory key"},
					"value":    map[string]any{"type": "string", "description": "Memory value"},
					"category": map[string]any{"type": "string", "description": "Optional category for grouping"},
				},
				"required": []string{"key", "value"},
			},
		},
		Execute: func(_ context.Context, _ string, args map[string]any) (*core.ToolResult, error) {
			key := stringArg(args, "key")
			if key == "" {
				return textResult("error: key is required"), nil
			}
			value := stringArg(args, "value")
			if value == "" {
				return textResult("error: value is required"), nil
			}
			category := stringArg(args, "category")
			if err := store.Set(key, value, category); err != nil {
				return textResult(fmt.Sprintf("error: %v", err)), nil
			}
			return textResult("Saved: " + key), nil
		},
		PromptHint: "Save a fact to project memory",
	}
}

func memoryGetTool(store *Store) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "memory_get",
			Description: "Retrieve a fact from project memory by key.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{"type": "string", "description": "Memory key to retrieve"},
				},
				"required": []string{"key"},
			},
		},
		Execute: func(_ context.Context, _ string, args map[string]any) (*core.ToolResult, error) {
			key := stringArg(args, "key")
			fact, ok := store.Get(key)
			if !ok {
				return textResult("not found: " + key), nil
			}
			return textResult(fact.Value), nil
		},
		PromptHint: "Retrieve a fact from project memory",
	}
}

func memoryListTool(store *Store) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "memory_list",
			Description: "List all facts in project memory, optionally filtered by category.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{"type": "string", "description": "Optional category filter"},
				},
				"required": []string{},
			},
		},
		Execute: func(_ context.Context, _ string, args map[string]any) (*core.ToolResult, error) {
			category := stringArg(args, "category")
			facts := store.List(category)
			if len(facts) == 0 {
				return textResult("No memories stored"), nil
			}
			var b strings.Builder
			for _, f := range facts {
				b.WriteString(f.Key)
				b.WriteString(": ")
				b.WriteString(f.Value)
				b.WriteByte('\n')
			}
			return textResult(strings.TrimRight(b.String(), "\n")), nil
		},
		PromptHint:   "List all project memory facts",
		PromptGuides: []string{"Use category to filter", "Returns key: value pairs"},
	}
}

func textResult(text string) *core.ToolResult {
	return &core.ToolResult{
		Content: []core.ContentBlock{core.TextContent{Text: text}},
	}
}

func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}
