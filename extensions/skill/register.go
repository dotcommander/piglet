package skill

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

var store *Store

// Register registers the skill extension's tools, command, message hook,
// and schedules OnInit work via OnInitAppend.
func Register(e *sdk.Extension) {
	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[skill] OnInit start")

		base, err := xdg.ConfigDir()
		if err != nil {
			x.Log("debug", fmt.Sprintf("[skill] OnInit complete — config dir error (%s)", time.Since(start)))
			return
		}
		store = NewStore(filepath.Join(base, "skills"))
		if len(store.List()) == 0 {
			store = nil
			x.Log("debug", fmt.Sprintf("[skill] OnInit complete — no skills found (%s)", time.Since(start)))
			return
		}

		// Prompt section listing available skills
		var b strings.Builder
		b.WriteString("Available skills (call skill_load to use):\n")
		for _, sk := range store.List() {
			b.WriteString("- ")
			b.WriteString(sk.Name)
			if sk.Description != "" {
				b.WriteString(": ")
				b.WriteString(sk.Description)
			}
			b.WriteByte('\n')
		}
		e.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Skills",
			Content: b.String(),
			Order:   25,
		})

		x.Log("debug", fmt.Sprintf("[skill] OnInit complete — %d skill(s) registered (%s)", len(store.List()), time.Since(start)))
	})

	// Tools
	e.RegisterTool(sdk.ToolDef{
		Name:        "skill_list",
		Description: "List available skills with descriptions and trigger keywords.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Execute: func(_ context.Context, _ map[string]any) (*sdk.ToolResult, error) {
			if store == nil {
				return sdk.TextResult("No skills available."), nil
			}
			skills := store.List()
			if len(skills) == 0 {
				return sdk.TextResult("No skills available."), nil
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
			return sdk.TextResult(b.String()), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "skill_load",
		Description: "Load a skill's full methodology and instructions by name.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Skill name"},
			},
			"required": []string{"name"},
		},
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if store == nil {
				return sdk.ErrorResult("no skills available"), nil
			}
			name, _ := args["name"].(string)
			if name == "" {
				return sdk.ErrorResult("name is required"), nil
			}
			content, err := store.Load(name)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("skill %q not found", name)), nil
			}
			return sdk.TextResult(content), nil
		},
	})

	// Command
	e.RegisterCommand(sdk.CommandDef{
		Name:        "skill",
		Description: "List or load a skill",
		Handler: func(_ context.Context, args string) error {
			if store == nil {
				e.ShowMessage("No skills available.")
				return nil
			}
			arg := strings.TrimSpace(args)
			if arg == "" || arg == "list" {
				skills := store.List()
				if len(skills) == 0 {
					e.ShowMessage("No skills found in " + store.Dir())
					return nil
				}
				var b strings.Builder
				b.WriteString("Available skills:\n")
				for _, sk := range skills {
					b.WriteString("  ")
					b.WriteString(sk.Name)
					if sk.Description != "" {
						b.WriteString(" — ")
						b.WriteString(sk.Description)
					}
					b.WriteByte('\n')
				}
				e.ShowMessage(b.String())
				return nil
			}
			content, err := store.Load(arg)
			if err != nil {
				e.ShowMessage(fmt.Sprintf("Skill %q not found. Run /skill list to see available skills.", arg))
				return nil
			}
			e.ShowMessage(fmt.Sprintf("# Skill: %s\n\n%s", arg, content))
			return nil
		},
	})

	// Message hook for auto-triggering skills
	e.RegisterMessageHook(sdk.MessageHookDef{
		Name:     "skill-trigger",
		Priority: 500,
		OnMessage: func(_ context.Context, msg string) (string, error) {
			if store == nil {
				return "", nil
			}
			matches := store.Match(msg)
			if len(matches) == 0 {
				return "", nil
			}
			content, err := store.Load(matches[0].Name)
			if err != nil {
				return "", nil
			}
			return "# Skill: " + matches[0].Name + "\n\n" + content, nil
		},
	})
}
