package route

import (
	"context"
	"fmt"
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
		Deferred:    true,
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
			return executeRoute(s, ctx, args)
		},
	})

	// Tool: route_feedback — record correct/wrong routing for learning
	e.RegisterTool(sdk.ToolDef{
		Name:        "route_feedback",
		Description: "Record whether a routing recommendation was correct or wrong. Use after completing a task to improve future routing accuracy.",
		Deferred:    true,
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
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			return executeFeedback(s, ctx, args)
		},
	})

	// Command: /route — diagnostic routing, learn, stats
	e.RegisterCommand(sdk.CommandDef{
		Name:        "route",
		Description: "Route a prompt, learn from feedback, or show stats",
		Handler: func(ctx context.Context, args string) error {
			return handleRouteCommand(e, s, ctx, args)
		},
	})

	// Message hook: auto-classify and inject routing context
	e.RegisterMessageHook(sdk.MessageHookDef{
		Name:     "route-classify",
		Priority: 900, // high priority, runs early
		OnMessage: func(ctx context.Context, msg string) (string, error) {
			return handleRouteHook(s, ctx, msg)
		},
	})
}
