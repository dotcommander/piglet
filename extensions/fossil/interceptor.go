package fossil

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/dotcommander/piglet/extensions/internal/toolresult"
	sdk "github.com/dotcommander/piglet/sdk"
)

// RegisterInterceptor adds blame context to read tool results.
// When the model reads a file, a compact blame summary is appended,
// showing which commits last touched the code — giving the model
// "why was this written?" context before it edits.
func RegisterInterceptor(e *sdk.Extension) {
	var cwdPtr atomic.Pointer[string]
	var lastFile atomic.Pointer[string]

	e.OnInitAppend(func(x *sdk.Extension) {
		c := x.CWD()
		cwdPtr.Store(&c)
	})

	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "fossil-blame",
		Priority: 100,
		Before: func(_ context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			if toolName != "read" {
				return true, args, nil
			}
			if fp, ok := args["file_path"].(string); ok {
				lastFile.Store(&fp)
			}
			return true, args, nil
		},
		After: func(_ context.Context, toolName string, details any) (any, error) {
			if toolName != "read" {
				return details, nil
			}
			cp := cwdPtr.Load()
			fp := lastFile.Swap(nil)
			if cp == nil || fp == nil {
				return details, nil
			}

			entries, err := Why(*cp, *fp, 0, 0)
			if err != nil || len(entries) == 0 {
				return details, nil
			}

			const maxEntries = 10
			limit := min(len(entries), maxEntries)

			var b strings.Builder
			b.WriteString("\n\n--- Git Blame Context ---\n")
			for _, entry := range entries[:limit] {
				fmt.Fprintf(&b, "%s %s (%s) L%s: %s\n",
					entry.SHA, entry.Author, entry.Date, entry.Lines, entry.Summary)
			}
			if len(entries) > maxEntries {
				fmt.Fprintf(&b, "... and %d more commits\n", len(entries)-maxEntries)
			}

			text, ok := toolresult.ExtractText(details)
			if !ok {
				return details, nil
			}
			return toolresult.ReplaceText(details, text+b.String()), nil
		},
	})
}
