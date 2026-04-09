package plan

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

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

// withActivePlan calls fn with the active plan, or shows an error message.
func (s *planState) withActivePlan(e *sdk.Extension, fn func(*Plan)) {
	p, err := s.store.Active()
	if err != nil {
		e.ShowMessage("Failed to get active plan: " + err.Error())
		return
	}
	if p == nil {
		e.ShowMessage("No active plan")
		return
	}
	fn(p)
}

func showActivePlan(e *sdk.Extension, s *planState) {
	s.withActivePlan(e, func(p *Plan) {
		e.ShowMessage(FormatPrompt(p))
	})
}

func clearActivePlan(e *sdk.Extension, s *planState) {
	s.withActivePlan(e, func(p *Plan) {
		if err := s.store.Delete(""); err != nil {
			e.ShowMessage(fmt.Sprintf("error: %v", err))
			return
		}
		e.ShowMessage("plan.md deleted.")
	})
}

func approveActivePlan(e *sdk.Extension, s *planState) {
	s.withActivePlan(e, func(p *Plan) {
		p.Mode = ModeExecute
		if err := s.store.Save(p); err != nil {
			e.ShowMessage(fmt.Sprintf("error: %v", err))
			return
		}
		e.ShowMessage("Plan mode: execute — changes are now allowed.")
	})
}

func showActivePlanMode(e *sdk.Extension, s *planState) {
	s.withActivePlan(e, func(p *Plan) {
		mode := p.Mode
		if mode == "" {
			mode = ModeExecute
		}
		e.ShowMessage(fmt.Sprintf("Plan mode: %s", mode))
	})
}

func togglePlanCheckpoints(e *sdk.Extension, s *planState) {
	s.withActivePlan(e, func(p *Plan) {
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
	})
}

func showPlanResume(e *sdk.Extension, s *planState) {
	s.withActivePlan(e, func(p *Plan) {
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
	})
}
