package coordinator

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
	"golang.org/x/sync/errgroup"
)

// DispatchResult holds the outcome of a single sub-task.
type DispatchResult struct {
	Index     int
	Task      string
	Text      string
	Turns     int
	InputTok  int
	OutputTok int
	Error     string
}

// Dispatch runs sub-tasks via RunAgent and collects results.
// Tasks run concurrently with a concurrency limit of 3.
func Dispatch(ctx context.Context, ext *sdk.Extension, tasks []SubTask) []DispatchResult {
	results := make([]DispatchResult, len(tasks))

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(3)

	for i, task := range tasks {
		eg.Go(func() error {
			resp, err := ext.RunAgent(egCtx, sdk.AgentRequest{
				Task:     task.Task,
				Tools:    task.Tools,
				Model:    task.Model,
				MaxTurns: task.MaxTurns,
			})

			if err != nil {
				results[i] = DispatchResult{
					Index: i,
					Task:  task.Task,
					Error: err.Error(),
				}
				return nil // don't cancel siblings
			}

			results[i] = DispatchResult{
				Index:     i,
				Task:      task.Task,
				Text:      resp.Text,
				Turns:     resp.Turns,
				InputTok:  resp.Usage.Input,
				OutputTok: resp.Usage.Output,
			}
			return nil
		})
	}

	eg.Wait() //nolint:errcheck // errors are captured in results
	return results
}

// MergeResults combines dispatch results into a single response string.
func MergeResults(results []DispatchResult) string {
	var b strings.Builder

	totalTurns := 0
	totalIn := 0
	totalOut := 0

	for _, r := range results {
		totalTurns += r.Turns
		totalIn += r.InputTok
		totalOut += r.OutputTok
	}

	fmt.Fprintf(&b, "[coordinator: %d task(s), %d turns, %dk in / %dk out]\n\n",
		len(results), totalTurns, totalIn/1000, totalOut/1000)

	for i, r := range results {
		if len(results) > 1 {
			fmt.Fprintf(&b, "─── Task %d ───\n", i+1)
		}
		if r.Error != "" {
			fmt.Fprintf(&b, "Error: %s\n", r.Error)
		} else if r.Text != "" {
			b.WriteString(r.Text)
			b.WriteByte('\n')
		} else {
			b.WriteString("[no output]\n")
		}
		if i < len(results)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}
