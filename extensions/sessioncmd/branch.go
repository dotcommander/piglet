package sessioncmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/piglet/sdk"
)

const (
	summaryOptionNone = "No summary"
	summaryOptionAuto = "Auto-summarize"
)

func registerBranch(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "branch",
		Description: "Branch to an earlier entry. Optionally pass summary text: /branch <summary>",
		Handler: func(ctx context.Context, args string) error {
			customSummary := strings.TrimSpace(args)

			infos, err := e.SessionEntryInfos(ctx)
			if err != nil {
				e.ShowMessage("Failed to list entries: " + err.Error())
				return nil
			}
			if len(infos) == 0 {
				e.ShowMessage("No entries in current session")
				return nil
			}

			items := make([]sdk.PickerItem, len(infos))
			for i, info := range infos {
				idTail := info.ID
				if len(idTail) > 8 {
					idTail = idTail[:8]
				}
				label := fmt.Sprintf("[%s] %s", info.Type, idTail)
				desc := formatShortTimestamp(info.Timestamp)
				if info.Children > 1 {
					desc += fmt.Sprintf(" (%d branches)", info.Children)
				}
				items[i] = sdk.PickerItem{ID: info.ID, Label: label, Desc: desc}
			}

			selected, err := e.ShowPicker(ctx, "Branch to entry", items)
			if err != nil || selected == "" {
				return nil
			}

			// Custom summary provided as args: branch immediately with it.
			if customSummary != "" {
				if err := e.BranchSessionWithSummary(ctx, selected, customSummary); err != nil {
					e.ShowMessage("Branch failed: " + err.Error())
					return nil
				}
				e.ShowMessage("Branched to " + selected)
				return nil
			}

			// No args: ask how to handle the summary.
			choice, cancelled, err := e.AskUser(ctx,
				"Summarize the abandoned path?",
				[]string{summaryOptionNone, summaryOptionAuto},
			)
			if err != nil || cancelled {
				return nil
			}

			switch choice {
			case summaryOptionNone:
				if err := e.BranchSession(ctx, selected); err != nil {
					e.ShowMessage("Branch failed: " + err.Error())
					return nil
				}
				e.ShowMessage("Branched to " + selected)

			case summaryOptionAuto:
				summary, err := generateBranchSummary(ctx, e)
				if err != nil {
					// Degrade gracefully: branch without a summary.
					e.ShowMessage(fmt.Sprintf("Summary generation failed (%s) — branching without summary", err.Error()))
					if berr := e.BranchSession(ctx, selected); berr != nil {
						e.ShowMessage("Branch failed: " + berr.Error())
					} else {
						e.ShowMessage("Branched to " + selected)
					}
					return nil
				}
				if err := e.BranchSessionWithSummary(ctx, selected, summary); err != nil {
					e.ShowMessage("Branch failed: " + err.Error())
					return nil
				}
				e.ShowMessage("Branched to " + selected)
			}

			return nil
		},
	})
}

// generateBranchSummary asks the LLM for a one-paragraph summary of the
// current conversation branch using the host's Chat endpoint.
func generateBranchSummary(ctx context.Context, e *sdk.Extension) (string, error) {
	msgs, err := e.ConversationMessages(ctx)
	if err != nil {
		return "", fmt.Errorf("get messages: %w", err)
	}

	resp, err := e.Chat(ctx, sdk.ChatRequest{
		System: "You are a session historian. Write a single concise paragraph " +
			"summarising the conversation: what was attempted, what succeeded, " +
			"and any decisions or findings worth preserving. Be specific — " +
			"name files, commands, and conclusions. Output only the paragraph.",
		Messages: []sdk.ChatMessage{
			{Role: "user", Content: "Summarise this conversation:\n\n" + string(msgs)},
		},
		Model:     "small",
		MaxTokens: 300,
	})
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}
	return strings.TrimSpace(resp.Text), nil
}

// formatShortTimestamp parses an RFC3339 timestamp and returns HH:MM:SS.
// On parse failure returns the original string.
func formatShortTimestamp(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("15:04:05")
}
