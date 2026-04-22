package guardrail

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

const usageFilename = "usage.json"

// Register wires the daily token guardrail into the extension.
// Hard block (used >= limit): InputTransformer returning handled=true + NotifyError.
// Soft warn (used >= 80% limit): MessageHook injecting context for the LLM.
// Accumulation: EventTurnEnd handler parsing Assistant.usage tokens.
// Status: returned as sdk.ActionSetStatus from the event handler.
// Config (daily_token_limit) is read once in OnInit; restart required for changes.
func Register(e *sdk.Extension) {
	var tracker *Tracker
	var limit int64 // 0 = disabled

	e.OnInitAppend(func(x *sdk.Extension) {
		dir, err := xdg.ConfigDir()
		if err != nil {
			x.Log("warn", fmt.Sprintf("[guardrail] cannot resolve config dir: %v", err))
			return
		}
		tracker = NewTracker(filepath.Join(dir, usageFilename))
		if err := tracker.LoadFrom(tracker.Path()); err != nil {
			x.Log("warn", fmt.Sprintf("[guardrail] could not load usage.json: %v — starting from zero", err))
		}

		// daily_token_limit is a top-level YAML key; 0 or absent means disabled.
		if vals, err := x.ConfigGet(context.Background(), "daily_token_limit"); err == nil {
			if v, ok := vals["daily_token_limit"]; ok {
				limit = toInt64(v)
			}
		}
	})

	// Hard block — InputTransformer. Only primitive that actually aborts submission
	// (shell/submit.go:59-69). MessageHook errors are swallowed at submit.go:97.
	e.RegisterInputTransformer(sdk.InputTransformerDef{
		Name:     "guardrail-block",
		Priority: 100,
		Transform: func(_ context.Context, input string) (string, bool, error) {
			if tracker == nil || limit <= 0 {
				return input, false, nil
			}
			used := tracker.Used()
			if used >= limit {
				e.NotifyError(fmt.Sprintf(
					"Daily token limit reached: used %d of %d (resets at midnight local time)",
					used, limit,
				))
				return "", true, nil // consume — message shown above; no error propagation
			}
			return input, false, nil
		},
	})

	// Soft warn — MessageHook injects context the LLM sees for this turn only.
	// No abort needed here; transformer already passed if we reach this point.
	e.RegisterMessageHook(sdk.MessageHookDef{
		Name:     "guardrail-warn",
		Priority: 100,
		OnMessage: func(_ context.Context, _ string) (string, error) {
			if tracker == nil || limit <= 0 {
				return "", nil
			}
			used := tracker.Used()
			threshold := limit * 8 / 10
			if used >= threshold && used < limit {
				return fmt.Sprintf(
					"[guardrail] Daily token usage at %d of %d (%d%%)",
					used, limit, (used*100)/limit,
				), nil
			}
			return "", nil
		},
	})

	// Accumulation + status update — EventTurnEnd carries Assistant.usage.
	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "guardrail-accumulate",
		Priority: 100,
		Events:   []string{"EventTurnEnd"},
		Handle: func(_ context.Context, _ string, data json.RawMessage) *sdk.Action {
			if tracker == nil {
				return nil
			}
			var payload struct {
				Assistant *struct {
					Usage struct {
						InputTokens  int64 `json:"inputTokens"`
						OutputTokens int64 `json:"outputTokens"`
					} `json:"usage"`
				} `json:"Assistant"`
			}
			if err := json.Unmarshal(data, &payload); err != nil || payload.Assistant == nil {
				return nil
			}
			tracker.Add(payload.Assistant.Usage.InputTokens, payload.Assistant.Usage.OutputTokens)
			return sdk.ActionSetStatus("guardrail", formatStatus(tracker.Used(), limit))
		},
	})
}

// formatStatus returns the status-bar text for the guardrail section.
// Empty string hides the section when there is nothing to show.
func formatStatus(used, limit int64) string {
	if limit <= 0 && used == 0 {
		return ""
	}
	if limit > 0 {
		return fmt.Sprintf("Tokens: %d/%d (%d%%)", used, limit, (used*100)/limit)
	}
	return fmt.Sprintf("Tokens: %d", used)
}

// toInt64 converts JSON-decoded numeric values (float64, int, int64) to int64.
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	}
	return 0
}
