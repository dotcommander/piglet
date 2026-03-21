package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

func registerTools(app *ext.App, store *Store) {
	app.RegisterTool(skillListTool(store))
	app.RegisterTool(skillLoadTool(store))
}

func skillListTool(store *Store) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "skill_list",
			Description: "List available skills with descriptions and trigger keywords.",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
		Execute: func(_ context.Context, _ string, _ map[string]any) (*core.ToolResult, error) {
			skills := store.List()
			if len(skills) == 0 {
				return ext.TextResult("No skills available."), nil
			}
			var b strings.Builder
			for _, sk := range skills {
				b.WriteString("- ")
				b.WriteString(sk.Name)
				if sk.Description != "" {
					b.WriteString(": ")
					b.WriteString(sk.Description)
				}
				if len(sk.Triggers) > 0 {
					b.WriteString(" (triggers: ")
					b.WriteString(strings.Join(sk.Triggers, ", "))
					b.WriteByte(')')
				}
				b.WriteByte('\n')
			}
			return ext.TextResult(b.String()), nil
		},
		BackgroundSafe: true,
	}
}

func skillLoadTool(store *Store) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "skill_load",
			Description: "Load a skill's full methodology and instructions by name.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "Skill name"},
				},
				"required": []string{"name"},
			},
		},
		Execute: func(_ context.Context, _ string, args map[string]any) (*core.ToolResult, error) {
			name := ext.StringArg(args, "name")
			if name == "" {
				return ext.TextResult("error: name is required"), nil
			}
			content, err := store.Load(name)
			if err != nil {
				return ext.TextResult(fmt.Sprintf("error: skill %q not found", name)), nil
			}
			return ext.TextResult(content), nil
		},
		BackgroundSafe: true,
	}
}

