package memory

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

// handleMemoryCommand implements the /memory slash command.
// It lists, deletes, clears, or shows related facts from the project store.
func handleMemoryCommand(e *sdk.Extension, s *memoryState, _ context.Context, args string) error {
	if s.store == nil {
		e.ShowMessage("memory store not available")
		return nil
	}
	args = strings.TrimSpace(args)
	switch {
	case args == "":
		facts := s.store.List("")
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
		if err := s.store.Clear(); err != nil {
			e.ShowMessage(fmt.Sprintf("error: %s", err))
			return nil
		}
		e.ShowMessage("Project memory cleared.")
	case args == "clear context":
		facts := s.store.List("_context")
		for _, f := range facts {
			_ = s.store.Delete(f.Key)
		}
		e.ShowMessage(fmt.Sprintf("Cleared %d context fact(s).", len(facts)))
	case strings.HasPrefix(args, "delete "):
		key := strings.TrimSpace(strings.TrimPrefix(args, "delete "))
		if err := s.store.Delete(key); err != nil {
			e.ShowMessage(fmt.Sprintf("error: %s", err))
			return nil
		}
		e.ShowMessage(fmt.Sprintf("Deleted: %s", key))
	case strings.HasPrefix(args, "related "):
		key := strings.TrimSpace(strings.TrimPrefix(args, "related "))
		facts := Related(s.store, key, 3)
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
}
