// Memory extension binary. Persistent per-project key-value memory.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/memory"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

var store *memory.Store

func main() {
	e := sdk.New("memory", "0.1.0")

	// Initialize store after host sends CWD
	e.OnInit(func(x *sdk.Extension) {
		s, err := memory.NewStore(x.CWD())
		if err != nil {
			return
		}
		store = s

		// Register prompt section with current memory contents
		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Project Memory",
			Content: buildMemoryPrompt(s),
			Order:   50,
		})
	})

	// Tools
	e.RegisterTool(sdk.ToolDef{
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
		PromptHint: "Save a fact to project memory",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			key, _ := args["key"].(string)
			value, _ := args["value"].(string)
			category, _ := args["category"].(string)
			if key == "" || value == "" {
				return sdk.ErrorResult("key and value are required"), nil
			}
			if err := store.Set(key, value, category); err != nil {
				return sdk.ErrorResult(fmt.Sprintf("error: %v", err)), nil
			}
			return sdk.TextResult("Saved: " + key), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "memory_get",
		Description: "Retrieve a fact from project memory by key.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{"type": "string", "description": "Memory key to retrieve"},
			},
			"required": []string{"key"},
		},
		PromptHint: "Retrieve a fact from project memory",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			key, _ := args["key"].(string)
			fact, ok := store.Get(key)
			if !ok {
				return sdk.TextResult("not found: " + key), nil
			}
			return sdk.TextResult(fact.Value), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "memory_list",
		Description: "List all facts in project memory, optionally filtered by category.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"category": map[string]any{"type": "string", "description": "Optional category filter"},
			},
			"required": []string{},
		},
		PromptHint: "List all project memory facts",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			category, _ := args["category"].(string)
			facts := store.List(category)
			if len(facts) == 0 {
				return sdk.TextResult("No memories stored"), nil
			}
			var b strings.Builder
			for _, f := range facts {
				b.WriteString(f.Key)
				b.WriteString(": ")
				b.WriteString(f.Value)
				b.WriteByte('\n')
			}
			return sdk.TextResult(strings.TrimRight(b.String(), "\n")), nil
		},
	})

	// Command
	e.RegisterCommand(sdk.CommandDef{
		Name:        "memory",
		Description: "List, delete, or clear project memories",
		Handler: func(_ context.Context, args string) error {
			if store == nil {
				e.ShowMessage("memory store not available")
				return nil
			}
			args = strings.TrimSpace(args)
			switch {
			case args == "":
				facts := store.List("")
				if len(facts) == 0 {
					e.ShowMessage("No project memories stored.")
					return nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "Project Memory:\n\n")
				for _, f := range facts {
					if f.Category != "" {
						fmt.Fprintf(&b, "  %s: %s (%s)\n", f.Key, f.Value, f.Category)
					} else {
						fmt.Fprintf(&b, "  %s: %s\n", f.Key, f.Value)
					}
				}
				fmt.Fprintf(&b, "\n%d fact(s) stored.", len(facts))
				e.ShowMessage(b.String())
			case args == "clear":
				if err := store.Clear(); err != nil {
					e.ShowMessage(fmt.Sprintf("error: %s", err))
					return nil
				}
				e.ShowMessage("Project memory cleared.")
			case strings.HasPrefix(args, "delete "):
				key := strings.TrimSpace(strings.TrimPrefix(args, "delete "))
				if err := store.Delete(key); err != nil {
					e.ShowMessage(fmt.Sprintf("error: %s", err))
					return nil
				}
				e.ShowMessage(fmt.Sprintf("Deleted: %s", key))
			default:
				e.ShowMessage("Usage: /memory [clear|delete <key>]")
			}
			return nil
		},
	})

	e.Run()
}

func buildMemoryPrompt(s *memory.Store) string {
	var b strings.Builder
	b.WriteString("Tools: memory_set (save), memory_get (retrieve by key), memory_list (browse all)\n\n")

	facts := s.List("")
	if len(facts) == 0 {
		b.WriteString("No memories stored yet.")
		return b.String()
	}

	b.WriteString("Current memories:\n")
	for _, f := range facts {
		if f.Category != "" {
			b.WriteString("- " + f.Key + ": " + f.Value + " (" + f.Category + ")\n")
		} else {
			b.WriteString("- " + f.Key + ": " + f.Value + "\n")
		}
	}
	return b.String()
}
