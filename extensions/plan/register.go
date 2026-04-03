package plan

import (
	"context"
	"fmt"
	"strings"
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

	e.OnInit(func(x *sdk.Extension) {
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

func handlePlanCreate(s *planState, args map[string]any) (*sdk.ToolResult, error) {
	if s.store == nil {
		return sdk.ErrorResult("plan store not available"), nil
	}

	title, _ := args["title"].(string)
	rawSteps, _ := args["steps"].([]any)
	checkpoints, hasCheckpoints := args["checkpoints"].(bool)

	steps := make([]string, 0, len(rawSteps))
	for _, step := range rawSteps {
		if text, ok := step.(string); ok {
			steps = append(steps, text)
		}
	}

	p, err := NewPlan(title, steps)
	if err != nil {
		return sdk.ErrorResult(err.Error()), nil
	}

	if hasCheckpoints {
		p.GitEnabled = checkpoints
	} else if s.git != nil {
		p.GitEnabled = true
	}

	if err := s.store.Save(p); err != nil {
		return sdk.ErrorResult(fmt.Sprintf("save: %v", err)), nil
	}

	return sdk.TextResult(FormatPrompt(p)), nil
}

func handlePlanUpdate(s *planState, args map[string]any) (*sdk.ToolResult, error) {
	if s.store == nil {
		return sdk.ErrorResult("plan store not available"), nil
	}

	p, err := s.store.Active()
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("load plan: %v", err)), nil
	}
	if p == nil {
		return sdk.ErrorResult("no plan.md found in project directory"), nil
	}

	stepID := intArg(args, "step")
	status, _ := args["status"].(string)
	notes, _ := args["notes"].(string)
	addAfter, _ := args["add_after"].(string)
	remove, _ := args["remove"].(bool)
	forceCheckpoint, _ := args["checkpoint"].(bool)

	if remove {
		if err := p.RemoveStep(stepID); err != nil {
			return sdk.ErrorResult(err.Error()), nil
		}
	} else {
		var commitSHA string

		isTerminal := status == StatusDone || status == StatusSkipped || status == StatusFailed
		if isTerminal || forceCheckpoint {
			if s.git != nil && p.GitEnabled {
				var stepText string
				for _, step := range p.Steps {
					if step.ID == stepID {
						stepText = step.Text
						break
					}
				}
				sha, err := s.git.Checkpoint(p.Slug, stepID, stepText)
				if err != nil {
					notes = notes + fmt.Sprintf(" [checkpoint failed: %v]", err)
				} else {
					commitSHA = sha
				}
			}
		}

		if status != "" || notes != "" {
			if err := p.UpdateStep(stepID, status, notes, commitSHA); err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
		}
		if addAfter != "" {
			if err := p.AddStepAfter(stepID, addAfter); err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
		}
	}

	if err := s.store.Save(p); err != nil {
		return sdk.ErrorResult(fmt.Sprintf("save: %v", err)), nil
	}

	result := FormatPrompt(p)
	if p.IsComplete() {
		result += "\n\nAll steps complete — plan archived."
	}
	return sdk.TextResult(result), nil
}

func handlePlanMode(s *planState, args map[string]any) (*sdk.ToolResult, error) {
	if s.store == nil {
		return sdk.ErrorResult("plan store not available"), nil
	}

	mode, _ := args["mode"].(string)
	if mode != ModeExecute && mode != ModePropose {
		return sdk.ErrorResult("mode must be \"propose\" or \"execute\""), nil
	}

	p, err := s.store.Active()
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("load plan: %v", err)), nil
	}
	if p == nil {
		return sdk.ErrorResult("no plan.md found in project directory"), nil
	}

	p.Mode = mode
	if err := s.store.Save(p); err != nil {
		return sdk.ErrorResult(fmt.Sprintf("save: %v", err)), nil
	}

	return sdk.TextResult(FormatPrompt(p)), nil
}

func interceptPlanPropose(s *planState, toolName string, args map[string]any) (bool, map[string]any, error) {
	switch toolName {
	case "write", "edit", "bash", "multi_edit":
		// check propose mode
	default:
		return true, args, nil
	}

	if s.store == nil {
		return true, args, nil
	}

	p, err := s.store.Active()
	if err != nil || p == nil || !p.InProposeMode() {
		return true, args, nil
	}

	description := formatProposal(toolName, args)
	p.AppendStep(description)
	_ = s.store.Save(p)

	return false, nil, fmt.Errorf("plan propose mode: blocked %s — recorded as plan step. Use plan_mode(execute) to allow changes", toolName)
}

func formatProposal(toolName string, args map[string]any) string {
	switch toolName {
	case "write":
		path, _ := args["file_path"].(string)
		return fmt.Sprintf("Write file: %s", path)
	case "edit":
		path, _ := args["file_path"].(string)
		return fmt.Sprintf("Edit file: %s", path)
	case "bash":
		cmd, _ := args["command"].(string)
		if runes := []rune(cmd); len(runes) > 100 {
			cmd = string(runes[:100]) + "..."
		}
		return fmt.Sprintf("Run command: %s", cmd)
	case "multi_edit":
		edits, _ := args["edits"].([]any)
		return fmt.Sprintf("Multi-edit: %d files", len(edits))
	default:
		return fmt.Sprintf("Tool call: %s", toolName)
	}
}

func makePlanCommandHandler(e *sdk.Extension, s *planState) func(context.Context, string) error {
	return func(_ context.Context, args string) error {
		if s.store == nil {
			e.ShowMessage("plan store not available")
			return nil
		}

		args = strings.TrimSpace(args)
		switch {
		case args == "":
			showActivePlan(e, s)
		case args == "delete" || args == "clear":
			clearActivePlan(e, s)
		case args == "approve":
			approveActivePlan(e, s)
		case args == "mode":
			showActivePlanMode(e, s)
		case args == "checkpoints":
			togglePlanCheckpoints(e, s)
		case args == "resume":
			showPlanResume(e, s)
		default:
			e.ShowMessage("Usage: /plan [delete|approve|mode|checkpoints|resume]")
		}
		return nil
	}
}

func showActivePlan(e *sdk.Extension, s *planState) {
	p, err := s.store.Active()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if p == nil {
		e.ShowMessage("No plan.md found. Use plan_create to create one.")
		return
	}
	e.ShowMessage(FormatPrompt(p))
}

func clearActivePlan(e *sdk.Extension, s *planState) {
	p, err := s.store.Active()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if p == nil {
		e.ShowMessage("No plan.md to delete.")
		return
	}
	if err := s.store.Delete(""); err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	e.ShowMessage("plan.md deleted.")
}

func approveActivePlan(e *sdk.Extension, s *planState) {
	p, err := s.store.Active()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if p == nil {
		e.ShowMessage("No plan.md found.")
		return
	}
	p.Mode = ModeExecute
	if err := s.store.Save(p); err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	e.ShowMessage("Plan mode: execute — changes are now allowed.")
}

func showActivePlanMode(e *sdk.Extension, s *planState) {
	p, err := s.store.Active()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if p == nil {
		e.ShowMessage("No plan.md found.")
		return
	}
	mode := p.Mode
	if mode == "" {
		mode = ModeExecute
	}
	e.ShowMessage(fmt.Sprintf("Plan mode: %s", mode))
}

func togglePlanCheckpoints(e *sdk.Extension, s *planState) {
	p, err := s.store.Active()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if p == nil {
		e.ShowMessage("No plan.md found.")
		return
	}
	if s.git == nil {
		e.ShowMessage("Not in a git repository — checkpoints unavailable.")
		return
	}
	p.GitEnabled = !p.GitEnabled
	if err := s.store.Save(p); err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	status := "disabled"
	if p.GitEnabled {
		status = "enabled"
	}
	e.ShowMessage(fmt.Sprintf("Checkpoint commits: %s", status))
}

func showPlanResume(e *sdk.Extension, s *planState) {
	p, err := s.store.Active()
	if err != nil {
		e.ShowMessage(fmt.Sprintf("error: %v", err))
		return
	}
	if p == nil {
		e.ShowMessage("No plan.md found.")
		return
	}
	resume := p.ResumeStep()
	if resume == nil {
		e.ShowMessage("Plan is complete — no resume point.")
		return
	}
	var msg strings.Builder
	fmt.Fprintf(&msg, "Resume: step %d — %s\n", resume.ID, resume.Text)
	if cp := p.LastCheckpoint(); cp != nil {
		fmt.Fprintf(&msg, "Last checkpoint: step %d (%s)\n", cp.ID, ShortSHA(cp.CommitSHA))
	}
	e.ShowMessage(msg.String())
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
