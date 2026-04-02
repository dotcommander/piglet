package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/shell"
)

func runREPL(ctx context.Context, rt *runtime) error {
	b := buildShell(ctx, rt, true)
	defer b.cleanup()
	if b.sess != nil {
		defer b.sess.Close()
	}
	sh := b.sh

	fmt.Fprintf(os.Stderr, "piglet %s · %s\n", resolveVersion(), rt.model.DisplayName())
	fmt.Fprintf(os.Stderr, "/help for commands · Ctrl+D to quit\n\n")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() || ctx.Err() != nil {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		resp := sh.Submit(input)
		if replDrainNotifications(sh) {
			break
		}

		switch resp.Kind {
		case shell.ResponseAgentStarted:
			fmt.Println()
			for evt := range resp.Events {
				switch e := evt.(type) {
				case core.EventStreamDelta:
					if e.Kind == "text" {
						fmt.Print(e.Delta)
					}
				case core.EventToolStart:
					fmt.Fprintf(os.Stderr, "[%s]\n", e.ToolName)
				case core.EventToolEnd:
					if e.IsError {
						fmt.Fprintf(os.Stderr, "[%s error]\n", e.ToolName)
					}
				case core.EventRetry:
					fmt.Fprintf(os.Stderr, "[retry %d/%d: %s]\n", e.Attempt, e.Max, e.Error)
				case core.EventCompact:
					fmt.Fprintf(os.Stderr, "[compacted %d → %d messages]\n", e.Before, e.After)
				case core.EventMaxTurns:
					fmt.Fprintf(os.Stderr, "[max turns: %d/%d]\n", e.Count, e.Max)
				}
				sh.ProcessEvent(evt)
				if replDrainNotifications(sh) {
					fmt.Println()
					return nil
				}
			}
			fmt.Println()
			fmt.Println()

		case shell.ResponseQueued:
			fmt.Fprintln(os.Stderr, "Queued")
		case shell.ResponseCommand:
			// already handled via notifications
		case shell.ResponseHandled:
			// handled by shell
		case shell.ResponseNotReady:
			fmt.Fprintln(os.Stderr, "Extensions loading, try again")
		case shell.ResponseError:
			fmt.Fprintf(os.Stderr, "error: %v\n", resp.Error)
		}
	}

	fmt.Println()
	return nil
}

// replDrainNotifications prints pending shell notifications to stderr.
// Returns true if a quit was requested.
func replDrainNotifications(sh *shell.Shell) bool {
	for _, n := range sh.Notifications() {
		switch n.Kind {
		case shell.NotifyMessage:
			fmt.Fprintln(os.Stderr, n.Text)
		case shell.NotifyWarn:
			fmt.Fprintf(os.Stderr, "warning: %s\n", n.Text)
		case shell.NotifyError:
			fmt.Fprintf(os.Stderr, "error: %s\n", n.Text)
		case shell.NotifyStatus:
			if n.Key == ext.StatusKeyModel && n.Text != "" {
				fmt.Fprintf(os.Stderr, "model: %s\n", n.Text)
			}
		case shell.NotifyQuit:
			return true
		}
	}
	return false
}
