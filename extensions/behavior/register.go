// Package behavior loads behavioral guidelines from ~/.config/piglet/behavior.md
// and injects them as the earliest system prompt section.
package behavior

import (
	"fmt"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

const behaviorOrder = 10

// Register schedules OnInit work via OnInitAppend to load and inject
// the behavior guidelines prompt section.
func Register(e *sdk.Extension) {
	e.OnInitAppend(func(ext *sdk.Extension) {
		start := time.Now()
		ext.Log("debug", "[behavior] OnInit start")

		content := xdg.LoadOrCreateExt("behavior", "behavior.md", "")
		if content == "" {
			ext.Log("debug", fmt.Sprintf("[behavior] OnInit complete — no content (%s)", time.Since(start)))
			return
		}
		ext.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Guidelines",
			Content: content,
			Order:   behaviorOrder,
		})
		ext.Log("debug", fmt.Sprintf("[behavior] OnInit complete (%s)", time.Since(start)))
	})
}
