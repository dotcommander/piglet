package plan

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed defaults/mode-propose.md
var modeProposeMD string

// FormatPrompt renders the plan for injection into the system prompt.
// Uses the same markdown format as plan.md but with extra status info.
func FormatPrompt(p *Plan) string {
	if p == nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Active Plan: %s\n\n", p.Title)

	if p.Mode == ModePropose {
		b.WriteString(strings.TrimSpace(modeProposeMD) + "\n\n")
	}

	// Show resume point if there's an incomplete step
	if resume := p.ResumeStep(); resume != nil {
		fmt.Fprintf(&b, "▶ Resume: step %d — %s\n\n", resume.ID, resume.Text)
	} else if p.IsComplete() {
		b.WriteString("✓ Plan complete\n\n")
	}

	for _, s := range p.Steps {
		marker := " "
		switch s.Status {
		case StatusDone:
			marker = "x"
		case StatusInProgress:
			marker = ">"
		case StatusSkipped:
			marker = "-"
		case StatusFailed:
			marker = "!"
		}

		fmt.Fprintf(&b, "- [%s] %d. %s", marker, s.ID, s.Text)
		if s.CommitSHA != "" {
			fmt.Fprintf(&b, " (%s)", ShortSHA(s.CommitSHA))
		}
		b.WriteByte('\n')

		if s.Notes != "" {
			fmt.Fprintf(&b, "  - %s\n", s.Notes)
		}
	}

	done, total := p.Progress()
	fmt.Fprintf(&b, "\nProgress: %d/%d done", done, total)
	if p.GitEnabled {
		b.WriteString(" | checkpoints enabled")
	}
	b.WriteString("\nPlan file: plan.md (human-editable)")
	return b.String()
}
