package plan

import (
	"fmt"
	"strings"
)

func FormatPrompt(p *Plan) string {
	if p == nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Active Plan: %s\n\n", p.Title)

	if p.Mode == ModePropose {
		b.WriteString("**MODE: PROPOSE** — Describe changes as plan steps. Mutating tools (write, edit, bash) are blocked.\n\n")
	}

	for _, s := range p.Steps {
		switch s.Status {
		case StatusDone:
			fmt.Fprintf(&b, "- [x] %d. %s\n", s.ID, s.Text)
		case StatusInProgress:
			fmt.Fprintf(&b, "- [ ] **%d. %s** ← in progress\n", s.ID, s.Text)
		case StatusSkipped:
			fmt.Fprintf(&b, "- [-] %d. %s\n", s.ID, s.Text)
		case StatusFailed:
			fmt.Fprintf(&b, "- [!] %d. %s\n", s.ID, s.Text)
		default:
			fmt.Fprintf(&b, "- [ ] %d. %s\n", s.ID, s.Text)
		}
		if s.Notes != "" {
			fmt.Fprintf(&b, "  - %s\n", s.Notes)
		}
	}

	done, total := p.Progress()
	fmt.Fprintf(&b, "\nProgress: %d/%d done", done, total)
	return b.String()
}
