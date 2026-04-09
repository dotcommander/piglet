package plan

import (
	"fmt"
	"unicode/utf8"

	sdk "github.com/dotcommander/piglet/sdk"
)

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
		if utf8.RuneCountInString(cmd) > 100 {
			cmd = string([]rune(cmd)[:100]) + "..."
		}
		return fmt.Sprintf("Run command: %s", cmd)
	case "multi_edit":
		edits, _ := args["edits"].([]any)
		return fmt.Sprintf("Multi-edit: %d files", len(edits))
	default:
		return fmt.Sprintf("Tool call: %s", toolName)
	}
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
