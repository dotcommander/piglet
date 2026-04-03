package memory

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/compact-system.md
var defaultCompactSystem string

var (
	store     *Store
	extractor *Extractor
)

// Register registers the memory extension's tools, commands, and event handlers
// onto e, and schedules OnInit work via OnInitAppend.
func Register(e *sdk.Extension) {
	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[memory] OnInit start")

		s, err := NewStore(x.CWD())
		if err != nil {
			x.Log("debug", fmt.Sprintf("[memory] OnInit complete — store init failed (%s)", time.Since(start)))
			return
		}
		store = s
		extractor = NewExtractor(s)

		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Project Memory",
			Content: BuildMemoryPrompt(s),
			Order:   50,
		})

		x.RegisterCompactor(sdk.CompactorDef{
			Name:      "rolling-memory",
			Threshold: 50000,
			Compact:   makeCompactHandler(x, s),
		})

		x.Log("debug", fmt.Sprintf("[memory] OnInit complete (%s)", time.Since(start)))
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

	// EventTurnEnd handler — micro-compact old tool results (priority 60, after extractor)
	registerClearer(e)

	// After interceptor — persist large tool results to disk (priority 30, before sift)
	registerOverflow(e)

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

	e.RegisterTool(sdk.ToolDef{
		Name:        "memory_relate",
		Description: "Create a bidirectional relation between two memory facts. Both keys must exist.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key_a": map[string]any{"type": "string", "description": "First fact key"},
				"key_b": map[string]any{"type": "string", "description": "Second fact key"},
			},
			"required": []string{"key_a", "key_b"},
		},
		PromptHint: "Link two related facts in memory",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			keyA, _ := args["key_a"].(string)
			keyB, _ := args["key_b"].(string)
			if keyA == "" || keyB == "" {
				return sdk.ErrorResult("key_a and key_b are required"), nil
			}
			if err := store.Relate(keyA, keyB); err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			return sdk.TextResult(fmt.Sprintf("Linked: %s ↔ %s", keyA, keyB)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "memory_related",
		Description: "Find all facts related to a key by traversing memory graph edges. Returns facts within the specified depth.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":       map[string]any{"type": "string", "description": "Starting fact key"},
				"max_depth": map[string]any{"type": "integer", "description": "Maximum traversal depth (default: 3)"},
			},
			"required": []string{"key"},
		},
		PromptHint: "Find related facts by traversing memory graph",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			key, _ := args["key"].(string)
			if key == "" {
				return sdk.ErrorResult("key is required"), nil
			}
			maxDepth := 3
			if md, ok := args["max_depth"].(float64); ok && int(md) > 0 {
				maxDepth = int(md)
			}
			facts := Related(store, key, maxDepth)
			if len(facts) == 0 {
				return sdk.TextResult("No related facts found for: " + key), nil
			}
			var b strings.Builder
			for _, f := range facts {
				b.WriteString(f.Key)
				b.WriteString(": ")
				b.WriteString(f.Value)
				if len(f.Relations) > 0 {
					fmt.Fprintf(&b, " [→ %s]", strings.Join(f.Relations, ", "))
				}
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
			case strings.HasPrefix(args, "related "):
				key := strings.TrimSpace(strings.TrimPrefix(args, "related "))
				facts := Related(store, key, 3)
				if len(facts) == 0 {
					e.ShowMessage(fmt.Sprintf("No facts related to %q", key))
					return nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "Facts related to %q:\n\n", key)
				for _, f := range facts {
					if len(f.Relations) > 0 {
						fmt.Fprintf(&b, "  %s: %s [→ %s]\n", f.Key, f.Value, strings.Join(f.Relations, ", "))
					} else {
						fmt.Fprintf(&b, "  %s: %s\n", f.Key, f.Value)
					}
				}
				e.ShowMessage(b.String())
			default:
				e.ShowMessage("Usage: /memory [clear|clear context|delete <key>|related <key>]")
			}
			return nil
		},
	})
}

// wireMsg wraps a message with a type discriminator for JSON transport.
// Matches the host's CompactMessage wire format used by ConversationMessages.
type wireMsg struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// compactConfig holds configurable parameters for the compaction handler.
type compactConfig struct {
	KeepRecent         int `yaml:"keep_recent"`
	TruncateToolResult int `yaml:"truncate_tool_result"`
}

func defaultCompactConfig() compactConfig {
	return compactConfig{
		KeepRecent:         6,
		TruncateToolResult: 2000,
	}
}

// makeCompactHandler returns the SDK compact handler that works with raw JSON messages.
// It reads facts from the store, optionally refines with an LLM call, and keeps
// the last keepRecent messages prepended with a summary reference message.
func makeCompactHandler(ext *sdk.Extension, s *Store) func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	cfg := xdg.LoadYAMLExt("memory", "compact.yaml", defaultCompactConfig())

	return func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		// Parse incoming messages
		var params struct {
			Messages []wireMsg `json:"messages"`
		}
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("unmarshal compact params: %w", err)
		}

		if len(params.Messages) <= cfg.KeepRecent+1 {
			// Not enough to compact — return as-is
			return raw, nil
		}

		// Truncate large tool results in messages being compacted to prevent
		// bash/read output from dominating the summarizer's budget.
		truncateToolResults(params.Messages[:len(params.Messages)-cfg.KeepRecent], cfg.TruncateToolResult)

		// Extract prior compaction file lists for cumulative tracking.
		priorRead, priorModified := extractPriorFileLists(params.Messages)

		// Get fact summary from store
		result := Compact(s)
		summary := result.Summary

		// Merge prior compaction file lists into the current summary
		// so cumulative tracking survives across compaction boundaries.
		summary = mergeFileLists(summary, priorRead, priorModified)

		// Try to refine with LLM if we have facts
		if summary != "" {
			resp, err := ext.Chat(ctx, sdk.ChatRequest{
				System:   strings.TrimSpace(defaultCompactSystem),
				Messages: []sdk.ChatMessage{{Role: "user", Content: summary}},
				Model:    "small",
			})
			if err == nil && resp.Text != "" {
				summary = resp.Text
			}
		}

		// Write summary back to store
		WriteSummary(s, summary)

		// Build reference message content
		var ref strings.Builder
		ref.WriteString("[Context compacted — session memory updated]\n\n")
		ref.WriteString("Use memory_list category=_context to see accumulated context.\n")
		ref.WriteString("Use memory_get to retrieve specific facts.\n")
		if summary != "" {
			ref.WriteString("\nSummary: ")
			ref.WriteString(summary)
		}

		// Build summary user message as wire format
		summaryData, err := json.Marshal(map[string]any{
			"role":    "user",
			"content": ref.String(),
		})
		if err != nil {
			return nil, fmt.Errorf("marshal summary message: %w", err)
		}

		// Keep last keepRecent messages, prepend summary
		kept := params.Messages[len(params.Messages)-cfg.KeepRecent:]
		wire := make([]wireMsg, 0, len(kept)+2)
		wire = append(wire, wireMsg{Type: "user", Data: summaryData})

		// Post-compact context re-injection
		items := gatherCriticalContext(s)
		reinjectMsg := buildReinjectMessage(items)
		if reinjectMsg != "" {
			reinjectData, err := json.Marshal(map[string]any{
				"role":    "user",
				"content": reinjectMsg,
			})
			if err == nil {
				wire = append(wire, wireMsg{Type: "user", Data: reinjectData})
			}
		}

		wire = append(wire, kept...)

		return json.Marshal(map[string]any{"messages": wire})
	}
}
