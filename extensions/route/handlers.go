package route

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

// executeRoute handles the route tool call: classify a prompt and return ranked components.
func executeRoute(s *state, ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return sdk.ErrorResult("prompt is required"), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.ready || s.reg == nil {
		return sdk.ErrorResult("route registry not ready"), nil
	}

	result := s.scorer.Score(prompt, s.cwd, s.reg)
	logRoute(s.fbDir, result, hashPrompt(prompt), "tool")

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return sdk.ErrorResult(fmt.Sprintf("marshal: %v", err)), nil
	}
	return sdk.TextResult(string(data)), nil
}

// executeFeedback handles the route_feedback tool call: record correct/wrong routing.
func executeFeedback(s *state, _ context.Context, args map[string]any) (*sdk.ToolResult, error) {
	prompt, _ := args["prompt"].(string)
	component, _ := args["component"].(string)
	correct, _ := args["correct"].(bool)

	if prompt == "" || component == "" {
		return sdk.ErrorResult("prompt and component are required"), nil
	}

	s.mu.RLock()
	fb := s.feedback
	s.mu.RUnlock()

	if fb == nil {
		return sdk.ErrorResult("feedback store not ready"), nil
	}

	if err := fb.Record(prompt, component, correct); err != nil {
		return sdk.ErrorResult(fmt.Sprintf("record feedback: %v", err)), nil
	}

	action := "correct"
	if !correct {
		action = "wrong"
	}
	return sdk.TextResult(fmt.Sprintf("Recorded %s feedback for %q on prompt %q. Run /route learn to apply.", action, component, truncatePrompt(prompt, 50))), nil
}

// handleRouteCommand handles the /route command: diagnostic routing, learn, or stats.
func handleRouteCommand(e *sdk.Extension, s *state, _ context.Context, args string) error {
	args = strings.TrimSpace(args)

	switch {
	case args == "":
		e.ShowMessage("Usage: /route <prompt> | /route learn | /route stats")
		return nil

	case args == "learn":
		s.mu.RLock()
		fb := s.feedback
		reg := s.reg
		s.mu.RUnlock()

		if fb == nil {
			e.ShowMessage("Feedback store not ready.")
			return nil
		}

		learned, err := fb.Learn()
		if err != nil {
			e.ShowMessage(fmt.Sprintf("Learn error: %v", err))
			return nil
		}

		// Merge into live registry
		if reg != nil {
			s.mu.Lock()
			mergeLearnedIntoRegistry(reg, learned)
			s.learned = learned
			s.mu.Unlock()
		}

		trigCount := 0
		antiCount := 0
		for _, v := range learned.Triggers {
			trigCount += len(v)
		}
		for _, v := range learned.AntiTriggers {
			antiCount += len(v)
		}
		e.ShowMessage(fmt.Sprintf("Learned %d triggers, %d anti-triggers across %d components.",
			trigCount, antiCount, len(learned.Triggers)+len(learned.AntiTriggers)))
		return nil

	case args == "stats":
		s.mu.RLock()
		reg := s.reg
		learned := s.learned
		s.mu.RUnlock()

		var b strings.Builder
		if reg != nil {
			extCount := 0
			toolCount := 0
			cmdCount := 0
			for _, c := range reg.Components {
				switch c.Type {
				case TypeExtension:
					extCount++
				case TypeTool:
					toolCount++
				case TypeCommand:
					cmdCount++
				}
			}
			fmt.Fprintf(&b, "Registry: %d extensions, %d tools, %d commands\n", extCount, toolCount, cmdCount)
		}
		if learned != nil {
			fmt.Fprintf(&b, "Learned triggers: %d components\n", len(learned.Triggers))
			fmt.Fprintf(&b, "Learned anti-triggers: %d components\n", len(learned.AntiTriggers))
		}
		e.ShowMessage(b.String())
		return nil

	default:
		s.mu.RLock()
		defer s.mu.RUnlock()

		if !s.ready || s.reg == nil {
			e.ShowMessage("Route registry not ready.")
			return nil
		}

		result := s.scorer.Score(args, s.cwd, s.reg)
		logRoute(s.fbDir, result, hashPrompt(args), "command")
		e.ShowMessage(FormatRouteResult(result))
		return nil
	}
}

// handleRouteHook handles the message hook: auto-classify and inject routing context.
func handleRouteHook(s *state, _ context.Context, msg string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.ready || s.reg == nil || !s.config.MessageHook.Enabled {
		return "", nil
	}

	result := s.scorer.Score(msg, s.cwd, s.reg)

	if result.Confidence < s.config.MessageHook.MinConfidence && len(result.Primary) == 0 {
		return "", nil
	}

	logRoute(s.fbDir, result, hashPrompt(msg), "hook")
	return FormatHookContext(result), nil
}

// mergeLearnedIntoRegistry adds learned triggers and anti-triggers to matching
// registry components. Learned triggers extend existing Keywords; anti-triggers
// would need scorer support (deferred — currently just extends keywords for now).
func mergeLearnedIntoRegistry(reg *Registry, lt *LearnedTriggers) {
	if lt == nil {
		return
	}
	for i := range reg.Components {
		comp := &reg.Components[i]
		key := comp.Name

		// Merge learned triggers into keywords
		if triggers, ok := lt.Triggers[key]; ok {
			comp.Keywords = dedupStrings(append(comp.Keywords, triggers...))
		}
		if comp.Extension != "" && comp.Extension != key {
			if triggers, ok := lt.Triggers[comp.Extension]; ok {
				comp.Keywords = dedupStrings(append(comp.Keywords, triggers...))
			}
		}

		// Merge learned anti-triggers
		if anti, ok := lt.AntiTriggers[key]; ok {
			comp.AntiTriggers = dedupStrings(append(comp.AntiTriggers, anti...))
		}
		if comp.Extension != "" && comp.Extension != key {
			if anti, ok := lt.AntiTriggers[comp.Extension]; ok {
				comp.AntiTriggers = dedupStrings(append(comp.AntiTriggers, anti...))
			}
		}
	}
}

func truncatePrompt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
