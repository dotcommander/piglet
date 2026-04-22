package plan

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// planState holds mutable state shared across tool and command handlers.
type planState struct {
	store *Store
	git   *GitClient
	cwd   string
}

// Register wires the plan extension into a shared SDK extension.
func Register(e *sdk.Extension) {
	s := &planState{}

	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[plan] OnInit start")

		s.cwd = x.CWD()
		store, err := NewStore(s.cwd)
		if err != nil {
			x.Notify(fmt.Sprintf("plan: init failed: %v", err))
			x.Log("debug", fmt.Sprintf("[plan] OnInit complete — store init failed (%s)", time.Since(start)))
			return
		}
		s.store = store
		s.git = NewGitClient(s.cwd)

		active, _ := store.Active()
		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Active Plan",
			Content: FormatPrompt(active),
			Order:   55,
		})

		x.Log("debug", fmt.Sprintf("[plan] OnInit complete (%s)", time.Since(start)))
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "plan_create",
		Description: "Create a plan.md file in the project directory with structured steps. Human-readable, git-visible, session-surviving.",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string", "description": "Plan title"},
				"steps": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Step descriptions in order",
				},
				"checkpoints": map[string]any{"type": "boolean", "description": "Enable checkpoint commits (default: true in git repos)"},
			},
			"required": []string{"title", "steps"},
		},
		PromptHint: "Create a plan.md to track multi-step work — persists as a file in the project",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			return handlePlanCreate(s, args)
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "plan_update",
		Description: "Update a step in plan.md: change status, set notes, add a step, or remove a step. Checkpoint commits are created automatically when marking steps done/skipped/failed if git is enabled.",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step":       map[string]any{"type": "integer", "description": "Step ID to operate on"},
				"status":     map[string]any{"type": "string", "enum": []string{StatusPending, StatusInProgress, StatusDone, StatusSkipped, StatusFailed}, "description": "New status"},
				"notes":      map[string]any{"type": "string", "description": "Freeform notes on this step"},
				"add_after":  map[string]any{"type": "string", "description": "Add a new step after this step ID with this text"},
				"remove":     map[string]any{"type": "boolean", "description": "Remove this step"},
				"checkpoint": map[string]any{"type": "boolean", "description": "Force create checkpoint commit (default: auto on terminal status)"},
			},
			"required": []string{"step"},
		},
		PromptHint: "Update step status, notes, or structure in plan.md",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			return handlePlanUpdate(s, args)
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "plan_mode",
		Description: "Switch plan mode between propose (changes blocked, recorded as steps) and execute (changes allowed).",
		Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mode": map[string]any{"type": "string", "enum": []string{"propose", "execute"}, "description": "Mode to switch to"},
			},
			"required": []string{"mode"},
		},
		PromptHint: "Switch plan mode: propose (block changes, record as steps) or execute (allow changes)",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			return handlePlanMode(s, args)
		},
	})

	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "plan-mode",
		Priority: 1500,
		Before: func(_ context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			return interceptPlanPropose(s, toolName, args)
		},
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "plan",
		Description: "View, manage, or delete the project plan (plan.md)",
		Handler:     makePlanCommandHandler(e, s),
	})
}
