package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

// SubTask represents a decomposed sub-task.
type SubTask struct {
	Task     string `json:"task"`
	Tools    string `json:"tools"`
	Model    string `json:"model"`
	MaxTurns int    `json:"max_turns"`
}

// PlanTasks decomposes a user request into sub-tasks using LLM classification.
func PlanTasks(ctx context.Context, ext *sdk.Extension, request string, caps []Capability) ([]SubTask, error) {
	capSummary := FormatCapabilities(caps)

	var b strings.Builder
	fmt.Fprintf(&b, "Available capabilities:\n%s\n", capSummary)

	// Ask route extension for intent/domain classification if available
	if hint := routeHint(ctx, ext, request); hint != "" {
		fmt.Fprintf(&b, "Routing analysis: %s\n\n", hint)
	}

	fmt.Fprintf(&b, "User request: %s", request)
	prompt := b.String()

	resp, err := ext.Chat(ctx, sdk.ChatRequest{
		System:    LoadPlanPrompt(),
		Messages:  []sdk.ChatMessage{{Role: "user", Content: prompt}},
		Model:     "small",
		MaxTokens: 1024,
	})
	if err != nil {
		// Fallback: single task with the whole request
		return []SubTask{{
			Task:     request,
			Tools:    "all",
			Model:    "default",
			MaxTurns: 10,
		}}, nil
	}

	var tasks []SubTask
	if err := json.Unmarshal([]byte(resp.Text), &tasks); err != nil {
		// JSON parse failed — single task fallback
		return []SubTask{{
			Task:     request,
			Tools:    "all",
			Model:    "default",
			MaxTurns: 10,
		}}, nil
	}

	// Validate and cap
	if len(tasks) == 0 {
		return []SubTask{{
			Task:     request,
			Tools:    "all",
			Model:    "default",
			MaxTurns: 10,
		}}, nil
	}
	if len(tasks) > 5 {
		tasks = tasks[:5]
	}

	for i := range tasks {
		if tasks[i].Tools == "" {
			tasks[i].Tools = "all"
		}
		if tasks[i].Model == "" {
			tasks[i].Model = "default"
		}
		if tasks[i].MaxTurns <= 0 || tasks[i].MaxTurns > 20 {
			tasks[i].MaxTurns = 10
		}
	}

	return tasks, nil
}

// routeHint calls the route tool via the host to get intent/domain classification.
// Returns empty string if route is unavailable — coordinator works without it.
func routeHint(ctx context.Context, ext *sdk.Extension, request string) string {
	result, err := ext.CallHostTool(ctx, "route", map[string]any{
		"prompt": request,
	})
	if err != nil || result.IsError {
		return ""
	}

	for _, block := range result.Content {
		if block.Text != "" {
			return block.Text
		}
	}
	return ""
}
