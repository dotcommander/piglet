package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/shell"
	"github.com/dotcommander/piglet/tool"
)

type toolCallInfo struct {
	tool string
	arg  string
}

type toolRow struct {
	ID      string
	Tool    string
	Arg     string
	Status  Status
	Meta    string
	Content string
}

func (m Model) toolRows() []toolRow {
	info := collectToolCallInfo(m.messages)
	rows := make([]toolRow, 0)
	for _, msg := range m.messages {
		tr, ok := msg.(*core.ToolResultMessage)
		if !ok {
			continue
		}
		row := toolRowFromResult(tr, info, m.diffMeta)
		if m.toolFilter != "" && row.Tool != m.toolFilter {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func collectToolCallInfo(messages []core.Message) map[string]toolCallInfo {
	out := make(map[string]toolCallInfo)
	for _, msg := range messages {
		am, ok := msg.(*core.AssistantMessage)
		if !ok {
			continue
		}
		for _, c := range am.Content {
			tc, ok := c.(core.ToolCall)
			if !ok || tc.ID == "" {
				continue
			}
			out[tc.ID] = toolCallInfo{
				tool: tc.Name,
				arg:  shell.ToolDetail(tc.Name, tc.Arguments),
			}
		}
	}
	return out
}

func toolRowFromResult(m *core.ToolResultMessage, calls map[string]toolCallInfo, diffMeta map[string]tool.DiffMeta) toolRow {
	content := toolResultText(m)
	status := StatusOK
	if m.IsError {
		status = StatusFail
	}

	toolName := m.ToolName
	arg := firstLine(content)
	if c, ok := calls[m.ToolCallID]; ok {
		if c.tool != "" {
			toolName = c.tool
		}
		if c.arg != "" {
			arg = c.arg
		}
	}
	if toolName == "bash" && arg != "" && !strings.HasPrefix(arg, "$ ") {
		arg = "$ " + arg
	}

	meta := ""
	if dm, ok := diffMeta[m.ToolCallID]; ok {
		meta = formatDiffMeta(dm)
	} else if n := strings.Count(content, "\n"); n > 0 {
		meta = fmt.Sprintf("%d lines", n+1)
	} else if trimmed := strings.TrimSpace(content); trimmed != "" {
		meta = fmt.Sprintf("%d chars", len([]rune(trimmed)))
	}

	return toolRow{
		ID:      m.ToolCallID,
		Tool:    toolName,
		Arg:     arg,
		Status:  status,
		Meta:    meta,
		Content: content,
	}
}

func toolResultText(m *core.ToolResultMessage) string {
	for _, c := range m.Content {
		if tc, ok := c.(core.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func firstLine(s string) string {
	trimmed := strings.TrimSpace(s)
	if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
		return trimmed[:i]
	}
	return trimmed
}

func (m *Model) focusFirstToolRow() bool {
	rows := m.toolRows()
	if len(rows) == 0 {
		m.focusedToolID = ""
		return false
	}
	if m.focusedToolID == "" || !rowIDExists(rows, m.focusedToolID) {
		m.focusedToolID = rows[len(rows)-1].ID
		return true
	}
	return false
}

func rowIDExists(rows []toolRow, id string) bool {
	for _, row := range rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

func (m *Model) moveToolFocus(delta int) bool {
	rows := m.toolRows()
	if len(rows) == 0 {
		m.focusedToolID = ""
		return false
	}
	idx := len(rows) - 1
	for i, row := range rows {
		if row.ID == m.focusedToolID {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
	}
	if m.focusedToolID == rows[idx].ID {
		return false
	}
	m.focusedToolID = rows[idx].ID
	return true
}

func (m *Model) toggleFocusedTool() bool {
	m.focusFirstToolRow()
	if m.focusedToolID == "" {
		return false
	}
	if m.expandedTools == nil {
		m.expandedTools = make(map[string]bool)
	}
	m.expandedTools[m.focusedToolID] = !m.expandedTools[m.focusedToolID]
	return true
}

func (m *Model) setAllToolsExpanded(expanded bool) bool {
	if m.expandedTools == nil {
		m.expandedTools = make(map[string]bool)
	}
	changed := false
	for _, row := range m.toolRows() {
		if m.expandedTools[row.ID] != expanded {
			m.expandedTools[row.ID] = expanded
			changed = true
		}
	}
	return changed
}

func (m *Model) toolNames() []string {
	seen := make(map[string]bool)
	var out []string
	info := collectToolCallInfo(m.messages)
	for _, msg := range m.messages {
		tr, ok := msg.(*core.ToolResultMessage)
		if !ok {
			continue
		}
		row := toolRowFromResult(tr, info, m.diffMeta)
		if row.Tool == "" || seen[row.Tool] {
			continue
		}
		seen[row.Tool] = true
		out = append(out, row.Tool)
	}
	return out
}

func (m Model) showToolPalette() Model {
	items := []ModalItem{
		{ID: "toggle", Label: "Expand focused tool call", Desc: "alt+i"},
		{ID: "expand-all", Label: "Expand all in this turn", Desc: "alt+shift+i"},
		{ID: "collapse-all", Label: "Collapse everything", Desc: "alt+shift+c"},
		{ID: "filter", Label: "Filter tool calls by name", Desc: "alt+f"},
	}
	if m.toolFilter != "" {
		items = append(items, ModalItem{ID: "clear-filter", Label: "Clear tool filter", Desc: m.toolFilter})
	}
	m.modal = NewModalModel("tool output", items, m.styles)
	m.modal.SetSize(m.width, m.height)
	m.modalAction = func(model *Model, id string) tea.Cmd {
		return model.runToolAction(id)
	}
	m.modal.Show()
	return m
}

func (m Model) showToolFilter() Model {
	items := []ModalItem{{ID: "", Label: "All tool calls"}}
	for _, name := range m.toolNames() {
		items = append(items, ModalItem{ID: name, Label: name})
	}
	m.modal = NewModalModel("filter tool calls", items, m.styles)
	m.modal.SetSize(m.width, m.height)
	m.modalAction = func(model *Model, id string) tea.Cmd {
		model.toolFilter = id
		model.focusedToolID = ""
		model.msgCache = nil
		model.refreshAndFollow()
		if id == "" {
			return model.notifyAndTick("Tool filter cleared")
		}
		return model.notifyAndTick("Tool filter: " + id)
	}
	m.modal.Show()
	return m
}

func (m *Model) runToolAction(id string) tea.Cmd {
	switch id {
	case "toggle":
		if m.toggleFocusedTool() {
			m.refreshAndFollow()
		}
	case "expand-all":
		if m.setAllToolsExpanded(true) {
			m.refreshAndFollow()
		}
	case "collapse-all":
		if m.setAllToolsExpanded(false) {
			m.refreshAndFollow()
		}
	case "filter":
		*m = m.showToolFilter()
	case "clear-filter":
		m.toolFilter = ""
		m.focusedToolID = ""
		m.msgCache = nil
		m.refreshAndFollow()
		return m.notifyAndTick("Tool filter cleared")
	}
	return nil
}
