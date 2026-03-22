// Memory extension binary. Persistent per-project key-value memory.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/memory"
	"github.com/dotcommander/piglet/provider"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

var (
	store     *memory.Store
	extractor *memory.Extractor
)

func main() {
	e := sdk.New("memory", "0.1.0")

	// Initialize store after host sends CWD
	e.OnInit(func(x *sdk.Extension) {
		s, err := memory.NewStore(x.CWD())
		if err != nil {
			return
		}
		store = s
		extractor = memory.NewExtractor(s)

		// Register prompt section with current memory contents
		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Project Memory",
			Content: memory.BuildMemoryPrompt(s),
			Order:   50,
		})

		// Register compactor (uses memory store + optional LLM refinement)
		settings, _ := config.Load()
		prov := createProvider(settings)
		threshold := config.IntOr(settings.Agent.CompactAt, 0)
		x.RegisterCompactor(sdk.CompactorDef{
			Name:      "rolling-memory",
			Threshold: threshold,
			Compact:   makeCompactHandler(s, prov),
		})
	})

	// EventAgentStart handler — clear stale context facts on new session
	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "memory-context-reset",
		Priority: 10,
		Events:   []string{"EventAgentStart"},
		Handle: func(_ context.Context, _ string, _ json.RawMessage) *sdk.Action {
			if store != nil {
				facts := store.List("_context")
				for _, f := range facts {
					_ = store.Delete(f.Key)
				}
			}
			return nil
		},
	})

	// EventTurnEnd handler — deterministic fact extraction
	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "memory-extractor",
		Priority: 50,
		Events:   []string{"EventTurnEnd"},
		Handle: func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
			if extractor != nil {
				_ = extractor.Extract(data)
			}
			return nil
		},
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
			case args == "clear context":
				facts := store.List("_context")
				for _, f := range facts {
					_ = store.Delete(f.Key)
				}
				e.ShowMessage(fmt.Sprintf("Cleared %d context fact(s).", len(facts)))
			case strings.HasPrefix(args, "delete "):
				key := strings.TrimSpace(strings.TrimPrefix(args, "delete "))
				if err := store.Delete(key); err != nil {
					e.ShowMessage(fmt.Sprintf("error: %s", err))
					return nil
				}
				e.ShowMessage(fmt.Sprintf("Deleted: %s", key))
			default:
				e.ShowMessage("Usage: /memory [clear|clear context|delete <key>]")
			}
			return nil
		},
	})

	e.Run()
}

// createProvider creates a lightweight LLM provider for summary refinement.
func createProvider(settings config.Settings) core.StreamProvider {
	auth, err := config.NewAuthDefault()
	if err != nil {
		return nil
	}

	modelQuery := settings.ResolveSmallModel()
	if modelQuery == "" {
		return nil
	}

	registry := provider.NewRegistry()
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

// makeCompactHandler returns the SDK compact handler that bridges JSON messages
// to the memory.CompactFn logic.
func makeCompactHandler(s *memory.Store, prov core.StreamProvider) func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	compactFn := memory.CompactFn(s, prov)

	return func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		// Parse incoming messages
		var params struct {
			Messages []struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("unmarshal compact params: %w", err)
		}

		// Deserialize to core.Message
		msgs := make([]core.Message, 0, len(params.Messages))
		for _, cm := range params.Messages {
			switch cm.Type {
			case "user":
				var msg core.UserMessage
				if json.Unmarshal(cm.Data, &msg) == nil {
					msgs = append(msgs, &msg)
				}
			case "assistant":
				var msg core.AssistantMessage
				if json.Unmarshal(cm.Data, &msg) == nil {
					msgs = append(msgs, &msg)
				}
			case "tool_result":
				var msg core.ToolResultMessage
				if json.Unmarshal(cm.Data, &msg) == nil {
					msgs = append(msgs, &msg)
				}
			}
		}

		// Run compaction
		compacted, err := compactFn(ctx, msgs)
		if err != nil {
			return nil, err
		}

		// Serialize result
		type wireMsg struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		wire := make([]wireMsg, 0, len(compacted))
		for _, m := range compacted {
			var msgType string
			switch m.(type) {
			case *core.UserMessage:
				msgType = "user"
			case *core.AssistantMessage:
				msgType = "assistant"
			case *core.ToolResultMessage:
				msgType = "tool_result"
			default:
				continue
			}
			data, err := json.Marshal(m)
			if err != nil {
				continue
			}
			wire = append(wire, wireMsg{Type: msgType, Data: data})
		}

		return json.Marshal(map[string]any{"messages": wire})
	}
}

