package coordinator

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// Register registers the coordinator extension.
func Register(e *sdk.Extension) {
	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", fmt.Sprintf("[%s] OnInit start", extName))
		x.Log("debug", fmt.Sprintf("[%s] OnInit complete (%s)", extName, time.Since(start)))
	})

	e.RegisterTool(sdk.ToolDef{
		Name:              "coordinate",
		Description:       "Decompose a complex task into parallel sub-tasks and dispatch them to independent agents. Each sub-agent runs to completion with scoped tool access. Use for tasks that benefit from parallel execution or capability-scoped delegation.",
		Deferred:          true,
		InterruptBehavior: "block",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "The task to decompose and coordinate",
				},
				"max_tasks": map[string]any{
					"type":        "integer",
					"description": "Maximum number of parallel sub-tasks (default: 3, max: 5)",
				},
			},
			"required": []string{"task"},
		},
		PromptHint: "Coordinate complex multi-part tasks across parallel agents",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			task, _ := args["task"].(string)
			if task == "" {
				return sdk.ErrorResult("task is required"), nil
			}

			maxTasks := 3
			if mt, ok := args["max_tasks"].(float64); ok && int(mt) > 0 {
				maxTasks = min(int(mt), MaxPlanTasks)
			}

			// Discover capabilities
			caps, err := DiscoverCapabilities(ctx, e)
			if err != nil {
				e.Log("warn", fmt.Sprintf("[%s] discover failed: %v", extName, err))
				// Continue without capability info
			}

			// Plan sub-tasks
			tasks, err := PlanTasks(ctx, e, task, caps)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("planning failed: %v", err)), nil
			}

			if len(tasks) > maxTasks {
				tasks = tasks[:maxTasks]
			}

			// Dispatch
			results := Dispatch(ctx, e, tasks)

			// Merge
			return sdk.TextResult(MergeResults(results)), nil
		},
	})
}
