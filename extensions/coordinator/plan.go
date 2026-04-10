package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

const (
	// maxSubTaskTurns is the cap applied to each sub-task's MaxTurns during planning.
	maxSubTaskTurns = 30

	// MaxPlanTasks is the maximum number of sub-tasks the planner can produce.
	MaxPlanTasks = 5
)

// SubTask represents a decomposed sub-task.
type SubTask struct {
	Task     string `json:"task"`
	Tools    string `json:"tools"`
	Model    string `json:"model"`
	MaxTurns int    `json:"max_turns"`
}

// defaultSubTask returns a single fallback task covering the entire request.
func defaultSubTask(request string) SubTask {
	return SubTask{
		Task:     request,
		Tools:    "all",
		Model:    "default",
		MaxTurns: maxSubTaskTurns,
	}
}

// applyDefaults fills in missing or invalid fields in sub-tasks.
func applyDefaults(tasks []SubTask) []SubTask {
	for i := range tasks {
		if tasks[i].Tools == "" {
			tasks[i].Tools = "all"
		}
		if tasks[i].Model == "" {
			tasks[i].Model = "default"
		}
		if tasks[i].MaxTurns <= 0 || tasks[i].MaxTurns > maxSubTaskTurns {
			tasks[i].MaxTurns = maxSubTaskTurns
		}
	}
	return tasks
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
		return []SubTask{defaultSubTask(request)}, nil
	}

	var tasks []SubTask
	if err := json.Unmarshal([]byte(resp.Text), &tasks); err != nil {
		return []SubTask{defaultSubTask(request)}, nil
	}

	if len(tasks) == 0 {
		return []SubTask{defaultSubTask(request)}, nil
	}

	if len(tasks) > MaxPlanTasks {
		tasks = tasks[:MaxPlanTasks]
	}

	return applyDefaults(tasks), nil
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
