// Plan extension binary. Persistent structured task tracking.
// Communicates with piglet host via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/plan"
	sdk "github.com/dotcommander/piglet/sdk/go"
)

var store *plan.Store

func main() {
	e := sdk.New("plan", "0.1.0")

	e.OnInit(func(x *sdk.Extension) {
		s, err := plan.NewStore(x.CWD())
		if err != nil {
			x.Notify(fmt.Sprintf("plan: init failed: %v", err))
			return
		}
		store = s

		active, _ := s.Active()
		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Active Plan",
			Content: plan.FormatPrompt(active),
			Order:   55,
		})
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "plan_create",
		Description: "Create a new structured plan with steps. Deactivates any existing active plan.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string", "description": "Plan title"},
				"steps": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Step descriptions in order",
				},
			},
			"required": []string{"title", "steps"},
		},
		PromptHint: "Create a structured plan to track multi-step work",
		Execute:    handleCreate,
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "plan_update",
		Description: "Update a step in the active plan: change status, set notes, add a step after, or remove a step.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step":      map[string]any{"type": "integer", "description": "Step ID to operate on"},
				"status":    map[string]any{"type": "string", "enum": []string{plan.StatusPending, plan.StatusInProgress, plan.StatusDone, plan.StatusSkipped, plan.StatusFailed}, "description": "New status"},
				"notes":     map[string]any{"type": "string", "description": "Freeform notes on this step"},
				"add_after": map[string]any{"type": "string", "description": "Add a new step after this step ID with this text"},
				"remove":    map[string]any{"type": "boolean", "description": "Remove this step"},
			},
			"required": []string{"step"},
		},
		PromptHint: "Update step status, notes, or structure in the active plan",
		Execute:    handleUpdate,
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "plan",
		Description: "View, list, switch, archive, clear, or delete plans",
		Handler:     makeCommandHandler(e),
	})

	e.Run()
}

func handleCreate(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
	if store == nil {
		return sdk.ErrorResult("plan store not available"), nil
	}

	title, _ := args["title"].(string)
	rawSteps, _ := args["steps"].([]any)

	steps := make([]string, 0, len(rawSteps))
	for _, s := range rawSteps {
		if text, ok := s.(string); ok {
			steps = append(steps, text)
		}
	}

	p, err := plan.NewPlan(title, steps)
	if err != nil {
		return sdk.ErrorResult(err.Error()), nil
	}

	_ = store.Deactivate()

	if err := store.Save(p); err != nil {
		return sdk.ErrorResult(fmt.Sprintf("save: %v", err)), nil
	}

	return sdk.TextResult(plan.FormatPrompt(p)), nil
}

func handleUpdate(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
	if store == nil {
		return sdk.ErrorResult("plan store not available"), nil
	}

	p, err := store.Active()
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("load plan: %v", err)), nil
	}
	if p == nil {
		return sdk.ErrorResult("no active plan"), nil
	}

	stepID := intArg(args, "step")
	status, _ := args["status"].(string)
	notes, _ := args["notes"].(string)
	addAfter, _ := args["add_after"].(string)
	remove, _ := args["remove"].(bool)

	if remove {
		if err := p.RemoveStep(stepID); err != nil {
			return sdk.ErrorResult(err.Error()), nil
		}
	} else {
		if status != "" || notes != "" {
			if err := p.UpdateStep(stepID, status, notes); err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
		}
		if addAfter != "" {
			if err := p.AddStepAfter(stepID, addAfter); err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
		}
	}

	if err := store.Save(p); err != nil {
		return sdk.ErrorResult(fmt.Sprintf("save: %v", err)), nil
	}

	result := plan.FormatPrompt(p)
	if p.IsComplete() {
		_ = store.Deactivate()
		result += "\n\nAll steps complete — plan archived."
	}
	return sdk.TextResult(result), nil
}

func makeCommandHandler(e *sdk.Extension) func(context.Context, string) error {
	return func(_ context.Context, args string) error {
		if store == nil {
			e.ShowMessage("plan store not available")
			return nil
		}

		args = strings.TrimSpace(args)
		switch {
		case args == "":
			showActive(e)
		case args == "list":
			listPlans(e)
		case strings.HasPrefix(args, "switch "):
			slug := strings.TrimSpace(strings.TrimPrefix(args, "switch "))
			switchPlan(e, slug)
		case args == "archive":
			archivePlan(e)
		case args == "clear":
			clearPlan(e)
		case strings.HasPrefix(args, "delete "):
			slug := strings.TrimSpace(strings.TrimPrefix(args, "delete "))
			deletePlan(e, slug)
		default:
			e.ShowMessage("Usage: /plan [list|switch <slug>|archive|clear|delete <slug>]")
		}
		return nil
	}
}

func showActive(e *sdk.Extension) {
	p, err := store.Active()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if p == nil {
		e.ShowMessage("No active plan.")
		return
	}
	e.ShowMessage(plan.FormatPrompt(p))
}

func listPlans(e *sdk.Extension) {
	plans, err := store.List()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if len(plans) == 0 {
		e.ShowMessage("No plans.")
		return
	}
	var b strings.Builder
	b.WriteString("Plans:\n\n")
	for _, p := range plans {
		done, total := p.Progress()
		marker := "  "
		if p.Active {
			marker = "* "
		}
		fmt.Fprintf(&b, "%s%s (%d/%d done)\n", marker, p.Slug, done, total)
	}
	e.ShowMessage(b.String())
}

func switchPlan(e *sdk.Extension, slug string) {
	if err := store.SetActive(slug); err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	e.ShowMessage(fmt.Sprintf("Switched to plan: %s", slug))
}

func archivePlan(e *sdk.Extension) {
	if err := store.Deactivate(); err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	e.ShowMessage("Active plan archived.")
}

func clearPlan(e *sdk.Extension) {
	p, err := store.Active()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if p == nil {
		e.ShowMessage("No active plan to clear.")
		return
	}
	if err := store.Delete(p.Slug); err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	e.ShowMessage("Active plan deleted.")
}

func deletePlan(e *sdk.Extension, slug string) {
	if err := store.Delete(slug); err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	e.ShowMessage(fmt.Sprintf("Deleted plan: %s", slug))
}

func intArg(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
