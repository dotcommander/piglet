package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dotcommander/piglet/sdk"
)

// stats holds the session-wide usage statistics.
var stats *SessionStats

// Register registers the usage extension's event handler, command, and tool.
func Register(e *sdk.Extension) {
	stats = NewSessionStats()

	e.OnInitAppend(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[usage] OnInit start")

		x.RegisterEventHandler(sdk.EventHandlerDef{
			Name:     "usage-tracker",
			Priority: 100,
			Events:   []string{"EventTurnUsage"},
			Handle: func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
				event, err := ParseEvent(data)
				if err != nil {
					return nil
				}
				stats.Record(event)
				return nil
			},
		})

		x.Log("debug", fmt.Sprintf("[usage] OnInit complete (%s)", time.Since(start)))
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "usage",
		Description: "Show session token usage statistics",
		Handler: func(_ context.Context, _ string) error {
			e.ShowMessage(stats.FormatSummary())
			return nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "session_stats",
		Description: "Get current session token usage statistics. Returns cumulative totals and prompt breakdown.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		PromptHint: "Check token usage for the current session",
		Execute: func(_ context.Context, _ map[string]any) (*sdk.ToolResult, error) {
			return sdk.TextResult(stats.FormatSummary()), nil
		},
	})
}
