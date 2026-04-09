package memory

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/compact-system.md
var defaultCompactSystem string

// memoryState holds mutable state shared across tool and command handlers.
type memoryState struct {
	store     *Store
	extractor *Extractor
}

// Register registers the memory extension's tools, commands, and event handlers
// onto e, and schedules OnInit work via OnInitAppend.
func Register(e *sdk.Extension) {
	s := &memoryState{}

	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[memory] OnInit start")

		store, err := NewStore(x.CWD())
		if err != nil {
			x.Log("debug", fmt.Sprintf("[memory] OnInit complete — store init failed (%s)", time.Since(start)))
			return
		}
		s.store = store
		s.extractor = NewExtractor(s.store)

		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Project Memory",
			Content: BuildMemoryPrompt(s.store),
			Order:   50,
		})

		x.RegisterCompactor(sdk.CompactorDef{
			Name:      "rolling-memory",
			Threshold: 50000,
			Compact:   makeCompactHandler(x, s.store),
		})

		x.Log("debug", fmt.Sprintf("[memory] OnInit complete (%s)", time.Since(start)))
	})

	// EventAgentStart handler — clear stale context facts on new session
	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "memory-context-reset",
		Priority: 10,
		Events:   []string{"EventAgentStart"},
		Handle: func(_ context.Context, _ string, _ json.RawMessage) *sdk.Action {
			if s.store != nil {
				facts := s.store.List("_context")
				for _, f := range facts {
					_ = s.store.Delete(f.Key)
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
			if s.extractor != nil {
				_ = s.extractor.Extract(data)
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
		Deferred:    true,
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
			if s.store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			key, _ := args["key"].(string)
			value, _ := args["value"].(string)
			category, _ := args["category"].(string)
			if key == "" || value == "" {
				return sdk.ErrorResult("key and value are required"), nil
			}
			if err := s.store.Set(key, value, category); err != nil {
				return sdk.ErrorResult(fmt.Sprintf("error: %v", err)), nil
			}
			return sdk.TextResult("Saved: " + key), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "memory_get",
		Description: "Retrieve a fact from project memory by key.",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{"type": "string", "description": "Memory key to retrieve"},
			},
			"required": []string{"key"},
		},
		PromptHint: "Retrieve a fact from project memory",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if s.store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			key, _ := args["key"].(string)
			fact, ok := s.store.Get(key)
			if !ok {
				return sdk.TextResult("not found: " + key), nil
			}
			return sdk.TextResult(fact.Value), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "memory_list",
		Description: "List all facts in project memory, optionally filtered by category.",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"category": map[string]any{"type": "string", "description": "Optional category filter"},
			},
			"required": []string{},
		},
		PromptHint: "List all project memory facts",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if s.store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			category, _ := args["category"].(string)
			facts := s.store.List(category)
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
		Deferred:    true,
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
			if s.store == nil {
				return sdk.ErrorResult("memory store not available"), nil
			}
			keyA, _ := args["key_a"].(string)
			keyB, _ := args["key_b"].(string)
			if keyA == "" || keyB == "" {
				return sdk.ErrorResult("key_a and key_b are required"), nil
			}
			if err := s.store.Relate(keyA, keyB); err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			return sdk.TextResult(fmt.Sprintf("Linked: %s ↔ %s", keyA, keyB)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "memory_related",
		Description: "Find all facts related to a key by traversing memory graph edges. Returns facts within the specified depth.",
		Deferred:    true,
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
			if s.store == nil {
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
			facts := Related(s.store, key, maxDepth)
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
		Handler: func(ctx context.Context, args string) error {
			return handleMemoryCommand(e, s, ctx, args)
		},
	})
}
