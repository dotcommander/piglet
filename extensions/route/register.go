package route

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// state holds mutable state shared across handlers.
type state struct {
	mu       sync.RWMutex
	scorer   *Scorer
	reg      *Registry
	config   Config
	feedback *FeedbackStore
	learned  *LearnedTriggers
	cwd      string
	fbDir    string
	ready    bool
}

// Register wires the route extension into a shared SDK extension.
func Register(e *sdk.Extension) {
	s := &state{}

	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[route] OnInit start")

		s.cwd = x.CWD()
		s.config = LoadConfig()

		intents := LoadIntents()
		domains := LoadDomains()

		ic := NewIntentClassifier(intents)
		de := NewDomainExtractor(domains)
		s.scorer = NewScorer(s.config, ic, de)

		// Build registry from host data
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		reg, err := BuildRegistry(ctx, x)
		if err != nil {
			x.Log("warn", fmt.Sprintf("[route] registry build failed: %v", err))
			reg = nil
		}

		// Load feedback store and learned triggers
		fbDir, _ := feedbackDir()
		fb := NewFeedbackStore(fbDir)
		learned := fb.LoadLearned()

		// Merge learned triggers into registry
		if reg != nil {
			mergeLearnedIntoRegistry(reg, learned)
		}

		s.mu.Lock()
		s.reg = reg
		s.feedback = fb
		s.learned = learned
		s.fbDir = fbDir
		s.ready = true
		s.mu.Unlock()

		count := 0
		if reg != nil {
			count = len(reg.Components)
		}
		x.Log("debug", fmt.Sprintf("[route] OnInit complete — %d components indexed (%s)", count, time.Since(start)))
	})

	// Tool: route — explicit routing query from LLM
	e.RegisterTool(sdk.ToolDef{
		Name:        "route",
		Description: "Classify a prompt and return ranked piglet extensions/tools most relevant to it. Use when you need to discover which tools or extensions are best suited for a task.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "The prompt or task description to route",
				},
			},
			"required": []string{"prompt"},
		},
		PromptHint: "Find the most relevant tools and extensions for a task",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
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
		},
	})

	// Tool: route_feedback — record correct/wrong routing for learning
	e.RegisterTool(sdk.ToolDef{
		Name:        "route_feedback",
		Description: "Record whether a routing recommendation was correct or wrong. Use after completing a task to improve future routing accuracy.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "The original prompt that was routed",
				},
				"component": map[string]any{
					"type":        "string",
					"description": "The component name (extension or tool) to give feedback on",
				},
				"correct": map[string]any{
					"type":        "boolean",
					"description": "True if this component was the right choice, false if wrong",
				},
			},
			"required": []string{"prompt", "component", "correct"},
		},
		PromptHint: "Record routing accuracy feedback to improve future recommendations",
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
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
		},
	})

	// Command: /route — diagnostic routing, learn, stats
	e.RegisterCommand(sdk.CommandDef{
		Name:        "route",
		Description: "Route a prompt, learn from feedback, or show stats",
		Handler: func(_ context.Context, args string) error {
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
		},
	})

	// Message hook: auto-classify and inject routing context
	e.RegisterMessageHook(sdk.MessageHookDef{
		Name:     "route-classify",
		Priority: 900, // high priority, runs early
		OnMessage: func(_ context.Context, msg string) (string, error) {
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
		},
	})
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

// FormatRouteResult formats a route result for human display.
func FormatRouteResult(r RouteResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Intent: %s", r.Intent.Primary)
	if r.Intent.Secondary != "" {
		fmt.Fprintf(&b, " + %s", r.Intent.Secondary)
	}
	if r.Intent.Confidence > 0 {
		fmt.Fprintf(&b, " (%.0f%%)", r.Intent.Confidence*100)
	}
	b.WriteByte('\n')

	if len(r.Domains) > 0 {
		fmt.Fprintf(&b, "Domains: %s\n", strings.Join(r.Domains, ", "))
	}

	fmt.Fprintf(&b, "Confidence: %.2f\n", r.Confidence)

	if len(r.Primary) > 0 {
		b.WriteString("\nPrimary:\n")
		for _, sc := range r.Primary {
			fmt.Fprintf(&b, "  %s (%s) — %.2f", sc.Name, sc.Type, sc.Score)
			if len(sc.Matched) > 0 {
				fmt.Fprintf(&b, " [%s]", strings.Join(sc.Matched, ", "))
			}
			b.WriteByte('\n')
		}
	}

	if len(r.Secondary) > 0 {
		b.WriteString("\nSecondary:\n")
		for _, sc := range r.Secondary {
			fmt.Fprintf(&b, "  %s (%s) — %.2f\n", sc.Name, sc.Type, sc.Score)
		}
	}

	return b.String()
}

// FormatHookContext formats routing results for injection into conversation context.
// Kept concise to minimize token overhead.
func FormatHookContext(r RouteResult) string {
	if len(r.Primary) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("[routing: ")

	if r.Intent.Primary != "" {
		b.WriteString("intent=")
		b.WriteString(r.Intent.Primary)
	}

	if len(r.Domains) > 0 {
		if r.Intent.Primary != "" {
			b.WriteString(" | ")
		}
		b.WriteString("domains=")
		b.WriteString(strings.Join(r.Domains, ","))
	}

	b.WriteString(" | relevant: ")
	names := make([]string, 0, len(r.Primary))
	for _, sc := range r.Primary {
		names = append(names, sc.Name)
	}
	b.WriteString(strings.Join(names, ", "))

	b.WriteString("]")
	return b.String()
}
