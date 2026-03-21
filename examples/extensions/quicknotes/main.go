// Package quicknotes is an example piglet extension that adds a /note command.
//
// This file demonstrates how to write a piglet extension that registers
// a slash command. Extensions are plain functions that receive *ext.App.
//
// Usage:
//
//	app.RegisterCommand(&ext.Command{
//	    Name:        "note",
//	    Description: "Save a quick note",
//	    Handler:     handleNote,
//	})
package quicknotes

import (
	"fmt"
	"os"
	"github.com/dotcommander/piglet/ext"
	"time"
)

// Register is the extension entry point.
func Register(app *ext.App) error {
	app.RegisterCommand(&ext.Command{
		Name:        "note",
		Description: "Save a quick note to notes.md",
		Handler: func(args string, a *ext.App) error {
			if args == "" {
				a.Notify("Usage: /note <text>")
				return nil
			}

			f, err := os.OpenFile("notes.md", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("open notes: %w", err)
			}
			defer f.Close()

			entry := fmt.Sprintf("- [%s] %s\n", time.Now().Format("2006-01-02 15:04"), args)
			if _, err := f.WriteString(entry); err != nil {
				return fmt.Errorf("write note: %w", err)
			}

			a.Notify("Note saved")
			return nil
		},
	})
	return nil
}
