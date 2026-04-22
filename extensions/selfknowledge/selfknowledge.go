// Package selfknowledge registers a prompt section describing piglet's current
// capabilities: working directory, platform, time, registered tools, commands,
// and keyboard shortcuts.
//
// Timing: content is assembled inside OnInit — after the host sends initialize
// (CWD is available) but before registrations are transmitted. At that point
// the host's registry includes all compiled-in extensions and any external packs
// that initialized before pack-core. The snapshot is intentionally taken here
// rather than at main() time because CWD and configDir are only available after
// the initialize handshake.
package selfknowledge

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/sdk"
)

const sectionOrder = 20 // before project docs (30), git context (40), memory (50)

// Register wires the self-knowledge prompt section. Content assembly is deferred
// to OnInit so that CWD and host-registry data are available when it runs.
func Register(e *sdk.Extension) {
	e.OnInit(func(e *sdk.Extension) {
		ctx := context.Background()
		content := buildContent(ctx, e)
		if content == "" {
			return
		}
		e.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Current Capabilities",
			Content: content,
			Order:   sectionOrder,
		})
	})
}

// buildContent assembles the prompt section body from live host state.
func buildContent(ctx context.Context, e *sdk.Extension) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Working directory: %s\n", e.CWD())
	fmt.Fprintf(&b, "Platform: %s\n", runtime.GOOS)
	fmt.Fprintf(&b, "Current time: %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST"))

	// Tools — query the host for names in registration order.
	if tools, err := e.ToolDefs(ctx); err == nil && len(tools) > 0 {
		names := make([]string, len(tools))
		for i, t := range tools {
			names[i] = t.Name
		}
		b.WriteString("Registered tools: ")
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("\n\n")
	}

	// Commands — sorted for stable output.
	if cmds, err := e.Commands(ctx); err == nil && len(cmds) > 0 {
		names := make([]string, len(cmds))
		for i, c := range cmds {
			names[i] = c.Name
		}
		slices.Sort(names)
		b.WriteString("Slash commands: /")
		b.WriteString(strings.Join(names, ", /"))
		b.WriteString("\n\n")
	}

	// Shortcuts — sorted for stable output.
	if shortcuts, err := e.Shortcuts(ctx); err == nil && len(shortcuts) > 0 {
		keys := make([]string, 0, len(shortcuts))
		for k := range shortcuts {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		b.WriteString("Keyboard shortcuts:\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "- %s — %s\n", k, shortcuts[k].Description)
		}
		b.WriteString("\n")
	}

	// Config directory — use xdg to match the host's resolution logic exactly.
	if dir, err := xdg.ConfigDir(); err == nil {
		fmt.Fprintf(&b, "Config directory: %s\n", dir)
	}

	return strings.TrimSpace(b.String())
}
