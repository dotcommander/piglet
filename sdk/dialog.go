package sdk

import (
	"context"
	"fmt"
)

// AskUser opens a blocking modal in the TUI asking the user to pick from
// choices. Blocks until the user selects or cancels, or ctx is cancelled.
//
// Returns (selected, false, nil) on selection, ("", true, nil) on cancellation,
// or ("", false, err) on transport error or host timeout.
func (e *Extension) AskUser(ctx context.Context, prompt string, choices []string) (string, bool, error) {
	if len(choices) == 0 {
		return "", false, fmt.Errorf("askUser: choices must not be empty")
	}
	type result struct {
		Selected  string `json:"selected"`
		Cancelled bool   `json:"cancelled"`
	}
	r, err := hostCall[result](e, ctx, "host/askUser", map[string]any{
		"prompt":  prompt,
		"choices": choices,
	})
	if err != nil {
		return "", false, err
	}
	return r.Selected, r.Cancelled, nil
}
