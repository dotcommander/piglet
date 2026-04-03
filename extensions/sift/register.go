package sift

import (
	"context"
	"slices"

	"github.com/dotcommander/piglet/extensions/internal/toolresult"
	sdk "github.com/dotcommander/piglet/sdk"
)

// Register wires the sift extension into a shared SDK extension.
func Register(e *sdk.Extension) {
	var cfg Config
	var projectCWD string

	e.OnInit(func(x *sdk.Extension) {
		cfg = LoadConfig()
		projectCWD = x.CWD()

		if !cfg.Enabled {
			return
		}

		prompt := LoadPrompt()
		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Sift Result Compression",
			Content: prompt,
			Order:   91,
		})
	})

	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "sift",
		Priority: 50,
		After: func(_ context.Context, toolName string, details any) (any, error) {
			if !cfg.Enabled {
				return details, nil
			}

			if len(cfg.Tools) > 0 && !slices.Contains(cfg.Tools, toolName) {
				return details, nil
			}

			text, ok := toolresult.ExtractText(details)
			if !ok || len(text) < cfg.SizeThreshold {
				return details, nil
			}

			compressed := CompressWithTool(toolName, text, cfg, projectCWD)
			if compressed == text {
				return details, nil
			}

			return toolresult.ReplaceText(details, compressed), nil
		},
	})
}
