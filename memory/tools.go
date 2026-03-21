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
			key := ext.StringArg(args, "key")
			if key == "" {
				return ext.TextResult("error: key is required"), nil
			}
			value := ext.StringArg(args, "value")
			if value == "" {
				return ext.TextResult("error: value is required"), nil
			}
			category := ext.StringArg(args, "category")
			if err := store.Set(key, value, category); err != nil {
				return ext.TextResult(fmt.Sprintf("error: %v", err)), nil
			}
			return ext.TextResult("Saved: " + key), nil
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
			key := ext.StringArg(args, "key")
			fact, ok := store.Get(key)
			if !ok {
				return ext.TextResult("not found: " + key), nil
			}
			return ext.TextResult(fact.Value), nil
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
			category := ext.StringArg(args, "category")
			facts := store.List(category)
			if len(facts) == 0 {
				return ext.TextResult("No memories stored"), nil
			}
			var b strings.Builder
			for _, f := range facts {
				b.WriteString(f.Key)
				b.WriteString(": ")
				b.WriteString(f.Value)
				b.WriteByte('\n')
			}
			return ext.TextResult(strings.TrimRight(b.String(), "\n")), nil
		},
		PromptHint:   "List all project memory facts",
		PromptGuides: []string{"Use category to filter", "Returns key: value pairs"},
	}
}

