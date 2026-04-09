package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

// delivererShim wraps *sdk.Extension to satisfy inbox.Deliverer.
// Steer is not yet in the SDK; fall back to SendMessage.
type delivererShim struct{ e *sdk.Extension }

func (d *delivererShim) SendMessage(content string) { d.e.SendMessage(content) }
func (d *delivererShim) Steer(content string)       { d.e.SendMessage(content) }
func (d *delivererShim) Notify(msg string)          { d.e.Notify(msg) }

// Register registers the inbox extension's event handler and inbox_status tool.
// No OnInit needed — inbox starts on EventAgentStart.
func Register(e *sdk.Extension) {
	var scanner atomic.Pointer[Scanner]
	var started atomic.Bool

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "inbox.lifecycle",
		Priority: 500,
		Events:   []string{"EventAgentStart"},
		Handle: func(_ context.Context, _ string, _ json.RawMessage) *sdk.Action {
			if !started.CompareAndSwap(false, true) {
				return nil
			}
			dir, err := xdg.ConfigDir()
			if err != nil {
				e.Log("error", fmt.Sprintf("inbox: config dir: %v", err))
				return nil
			}
			inboxDir := filepath.Join(dir, "inbox")
			s := New(inboxDir, e.CWD(), os.Getpid(), &delivererShim{e})
			s.Start(context.Background())
			scanner.Store(s)
			return nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "inbox_status",
		Description: "Check external inbox health and delivery statistics",
Deferred:    true,
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Execute: func(_ context.Context, _ map[string]any) (*sdk.ToolResult, error) {
			s := scanner.Load()
			if s == nil {
				return sdk.TextResult("Inbox scanner not started"), nil
			}
			stats := s.Stats()
			uptime := time.Since(stats.StartedAt).Truncate(time.Second)
			text := fmt.Sprintf(
				"Inbox Status\n  Uptime:     %s\n  Delivered:  %d\n  Failed:     %d\n  Duplicates: %d\n  Expired:    %d",
				uptime, stats.Delivered, stats.Failed, stats.Duplicates, stats.Expired,
			)
			return sdk.TextResult(text), nil
		},
	})
}
