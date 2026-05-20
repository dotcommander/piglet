package tui

import (
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/tool"
)

// TestApplyBashTail_CorrelatesByCallID verifies the live bash tail line is
// retained only for the currently in-flight tool call and dropped otherwise.
func TestApplyBashTail_CorrelatesByCallID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		activeToolID string
		msg          bashTailMsg
		wantChanged  bool
		wantTail     string
	}{
		{
			name:         "matching call retained",
			activeToolID: "call-1",
			msg:          bashTailMsg{callID: "call-1", line: "building..."},
			wantChanged:  true,
			wantTail:     "building...",
		},
		{
			name:         "stale call dropped",
			activeToolID: "call-2",
			msg:          bashTailMsg{callID: "call-1", line: "old line"},
			wantChanged:  false,
			wantTail:     "",
		},
		{
			name:         "no active call dropped",
			activeToolID: "",
			msg:          bashTailMsg{callID: "call-1", line: "orphan"},
			wantChanged:  false,
			wantTail:     "",
		},
		{
			name:         "empty callID dropped",
			activeToolID: "call-1",
			msg:          bashTailMsg{callID: "", line: "no id"},
			wantChanged:  false,
			wantTail:     "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &Model{activeToolID: tc.activeToolID}
			changed := m.applyBashTail(tc.msg)
			if changed != tc.wantChanged {
				t.Errorf("applyBashTail changed = %v, want %v", changed, tc.wantChanged)
			}
			if m.bashTail != tc.wantTail {
				t.Errorf("bashTail = %q, want %q", m.bashTail, tc.wantTail)
			}
		})
	}
}

// TestApplyBashTail_HoldsLatestLine verifies a sequence of tail lines leaves
// the model holding the most recent line for the active call.
func TestApplyBashTail_HoldsLatestLine(t *testing.T) {
	t.Parallel()

	m := &Model{activeToolID: "call-7"}
	lines := []string{"step 1", "step 2", "step 3"}
	for _, l := range lines {
		m.applyBashTail(bashTailMsg{callID: "call-7", line: l})
	}
	if m.bashTail != "step 3" {
		t.Errorf("bashTail = %q, want last line %q", m.bashTail, "step 3")
	}
}

// TestSubscribeBashTail_ForwardsBusEvents verifies the bus subscription
// converts core.EventToolUpdate values published on tool.BashTailTopic into
// bashTailMsg values delivered on the returned channel.
func TestSubscribeBashTail_ForwardsBusEvents(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	ch := subscribeBashTail(app)
	if ch == nil {
		t.Fatal("subscribeBashTail returned nil channel for non-nil app")
	}

	app.Publish(tool.BashTailTopic, core.EventToolUpdate{
		ToolCallID: "call-42",
		ToolName:   "bash",
		Partial:    "compiling main.go",
	})

	got := <-ch
	if got.callID != "call-42" {
		t.Errorf("callID = %q, want call-42", got.callID)
	}
	if got.line != "compiling main.go" {
		t.Errorf("line = %q, want %q", got.line, "compiling main.go")
	}
}

// TestSubscribeBashTail_NilApp verifies a nil app yields a nil channel so
// the TUI degrades gracefully when constructed without an ext.App.
func TestSubscribeBashTail_NilApp(t *testing.T) {
	t.Parallel()
	if ch := subscribeBashTail(nil); ch != nil {
		t.Error("subscribeBashTail(nil) should return nil channel")
	}
	if cmd := drainBashTail(nil); cmd != nil {
		t.Error("drainBashTail(nil) should return nil Cmd")
	}
}
