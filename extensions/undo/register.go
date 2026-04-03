package undo

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

// Register registers the undo extension's commands.
func Register(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "undo",
		Description: "Restore files to pre-edit state",
		Handler: func(ctx context.Context, args string) error {
			snapshots, err := e.UndoSnapshots(ctx)
			if err != nil || len(snapshots) == 0 {
				e.ShowMessage("No undo snapshots available")
				return nil
			}

			target := strings.TrimSpace(args)

			// If a specific file is given, restore it directly
			if target != "" {
				for path := range snapshots {
					if path == target || strings.HasSuffix(path, "/"+target) {
						if err := e.UndoRestore(ctx, path); err != nil {
							e.ShowMessage("Undo failed: " + err.Error())
							return nil
						}
						e.ShowMessage("Restored: " + path)
						return nil
					}
				}
				e.ShowMessage("No snapshot for: " + target)
				return nil
			}

			// No args — list available snapshots
			paths := make([]string, 0, len(snapshots))
			for p := range snapshots {
				paths = append(paths, p)
			}
			slices.Sort(paths)

			var b strings.Builder
			b.WriteString("Undo snapshots available:\n\n")
			for _, p := range paths {
				fmt.Fprintf(&b, "  %s (%s)\n", filepath.Base(p), formatSize(int64(snapshots[p])))
			}
			b.WriteString("\nUsage: /undo <filename>")
			e.ShowMessage(b.String())
			return nil
		},
	})
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
