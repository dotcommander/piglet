package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/tool"
)

func TestToolRowsUseToolCallDetailsAndDiffMeta(t *testing.T) {
	t.Parallel()

	m := Model{
		messages: []core.Message{
			&core.AssistantMessage{Content: []core.AssistantContent{
				core.ToolCall{ID: "call-edit", Name: "edit", Arguments: map[string]any{"path": "auth/handler.go"}},
			}},
			&core.ToolResultMessage{
				ToolCallID: "call-edit",
				ToolName:   "edit",
				Content:    []core.ContentBlock{core.TextContent{Text: "edited /tmp/auth/handler.go"}},
			},
		},
		diffMeta: map[string]tool.DiffMeta{
			"call-edit": {Added: 47, Removed: 8, Files: 1, Hunks: 3},
		},
	}

	rows := m.toolRows()
	if len(rows) != 1 {
		t.Fatalf("toolRows len = %d, want 1", len(rows))
	}
	if rows[0].Arg != "auth/handler.go" {
		t.Fatalf("row arg = %q, want auth/handler.go", rows[0].Arg)
	}
	if rows[0].Meta != "+47 -8 · 1f 3h" {
		t.Fatalf("row meta = %q", rows[0].Meta)
	}
}

func TestToolRowsFocusExpandAndFilter(t *testing.T) {
	t.Parallel()

	m := Model{
		messages: []core.Message{
			&core.ToolResultMessage{ToolCallID: "read-1", ToolName: "read", Content: []core.ContentBlock{core.TextContent{Text: "alpha\nbeta"}}},
			&core.ToolResultMessage{ToolCallID: "bash-1", ToolName: "bash", Content: []core.ContentBlock{core.TextContent{Text: "ok"}}},
		},
	}

	if !m.focusFirstToolRow() || m.focusedToolID != "bash-1" {
		t.Fatalf("focusedToolID = %q, want bash-1", m.focusedToolID)
	}
	if !m.moveToolFocus(-1) || m.focusedToolID != "read-1" {
		t.Fatalf("move up focusedToolID = %q, want read-1", m.focusedToolID)
	}
	if !m.toggleFocusedTool() || !m.expandedTools["read-1"] {
		t.Fatalf("toggle did not expand focused row: %#v", m.expandedTools)
	}

	m.toolFilter = "bash"
	rows := m.toolRows()
	if len(rows) != 1 || rows[0].ID != "bash-1" {
		t.Fatalf("filtered rows = %#v, want only bash-1", rows)
	}
	if !m.focusFirstToolRow() || m.focusedToolID != "bash-1" {
		t.Fatalf("filtered focus = %q, want bash-1", m.focusedToolID)
	}
}

func TestRenderExpandedToolRowIncludesDetail(t *testing.T) {
	t.Parallel()

	v := NewMessageView(NewStyles(DefaultTheme()), 80, "notty")
	row := toolRow{
		ID:      "call-1",
		Tool:    "bash",
		Arg:     "go test ./...",
		Status:  StatusOK,
		Meta:    "2 lines",
		Content: "first\n+added\n-removed",
	}

	out := v.renderToolRow(row, true, true)
	for _, want := range []string{"go test ./...", "first", "+added", "-removed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expanded row missing %q:\n%s", want, out)
		}
	}
}

func TestHandleKeyPressToolShortcuts(t *testing.T) {
	t.Parallel()

	m := Model{
		styles:        NewStyles(DefaultTheme()),
		expandedTools: make(map[string]bool),
		messages: []core.Message{
			&core.ToolResultMessage{ToolCallID: "read-1", ToolName: "read", Content: []core.ContentBlock{core.TextContent{Text: "alpha"}}},
		},
	}

	model, _, handled := m.handleKeyPress(tea.KeyPressMsg{Code: 'i', Mod: tea.ModAlt})
	if !handled {
		t.Fatal("alt+i was not handled")
	}
	got := model.(Model)
	if !got.expandedTools["read-1"] {
		t.Fatalf("alt+i did not expand focused row: %#v", got.expandedTools)
	}

	model, _, handled = got.handleKeyPress(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt | tea.ModShift})
	if !handled {
		t.Fatal("alt+shift+c was not handled")
	}
	got = model.(Model)
	if got.expandedTools["read-1"] {
		t.Fatalf("alt+shift+c did not collapse row: %#v", got.expandedTools)
	}
}
