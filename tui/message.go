package tui

import (
	"fmt"
	"log/slog"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/shell"
	"github.com/dotcommander/piglet/tool"
)

// MessageView renders conversation messages.
type MessageView struct {
	styles       Styles
	renderer     *glamour.TermRenderer
	width        int
	glamourStyle string // glamour standard style name (e.g. "dark", "light")
	cachedSep    string // cached separator line, invalidated on width change
}

// NewMessageView creates a message renderer. glamourStyle selects the glamour
// markdown theme (e.g. "dark", "light", "notty"); empty string defaults to "dark".
func NewMessageView(styles Styles, width int, glamourStyle string) MessageView {
	if glamourStyle == "" {
		glamourStyle = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourStyle),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		slog.Warn("glamour init failed, using plain text", "error", err)
	}
	return MessageView{styles: styles, renderer: r, width: width, glamourStyle: glamourStyle}
}

// SetWidth updates the rendering width, re-creating the glamour renderer.
func (v *MessageView) SetWidth(w int) {
	if w == v.width {
		return
	}
	v.width = w
	v.cachedSep = "" // invalidate separator cache
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(v.glamourStyle),
		glamour.WithWordWrap(w-4),
	)
	if err != nil {
		slog.Warn("glamour resize failed, using plain text", "error", err)
	}
	v.renderer = r
}

// RenderMessage renders a single message. diffMeta supplies per-ToolCallID
// edit metadata (added/removed/files/hunks) keyed for tool-result rows;
// nil is safe — callers without diff data pass nil.
func (v *MessageView) RenderMessage(msg core.Message, diffMeta map[string]tool.DiffMeta) string {
	switch m := msg.(type) {
	case *core.UserMessage:
		return v.renderUser(m)
	case *core.AssistantMessage:
		return v.renderAssistant(m)
	case *core.ToolResultMessage:
		return v.renderToolResult(m, diffMeta)
	default:
		return ""
	}
}

func (v *MessageView) separator() string {
	if v.cachedSep == "" {
		w := v.width
		if w > 30 {
			w = 30
		}
		v.cachedSep = v.styles.Muted.Render(strings.Repeat("─", w))
	}
	return v.cachedSep
}

func (v *MessageView) renderUser(m *core.UserMessage) string {
	content := m.Content
	if content == "" && len(m.Blocks) > 0 {
		var parts []string
		for _, b := range m.Blocks {
			switch bl := b.(type) {
			case core.TextContent:
				if bl.Text != "" {
					parts = append(parts, bl.Text)
				}
			case *core.ImageContent:
				parts = append(parts, v.styles.Muted.Render("[image attached]"))
			}
		}
		content = strings.Join(parts, "\n")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return v.styles.UserMsg.Render("> "+content) + "\n\n"
}

func (v *MessageView) renderAssistant(m *core.AssistantMessage) string {
	// Classify content to decide header visibility
	hasText, hasThinking := false, false
	for _, c := range m.Content {
		switch block := c.(type) {
		case core.TextContent:
			if strings.TrimSpace(block.Text) != "" {
				hasText = true
			}
		case core.ThinkingContent:
			if block.Thinking != "" {
				hasThinking = true
			}
		}
	}

	// Tool-call-only messages: skip entirely — results follow immediately
	if !hasText && !hasThinking {
		return ""
	}

	var b strings.Builder
	for _, c := range m.Content {
		switch block := c.(type) {
		case core.TextContent:
			if v.renderer != nil {
				rendered, err := v.renderer.Render(block.Text)
				if err == nil {
					b.WriteString(rendered)
				} else {
					b.WriteString(block.Text)
					b.WriteByte('\n')
				}
			} else {
				b.WriteString(block.Text)
				b.WriteByte('\n')
			}
		case core.ThinkingContent:
			preview := truncateRunes(block.Thinking, 80)
			b.WriteString(v.styles.Thinking.Render("◦ "+preview) + "\n")
		case core.ToolCall:
			node := CallNode{
				Tool:   block.Name,
				Arg:    shell.ToolDetail(block.Name, block.Arguments),
				Status: StatusPending,
			}
			b.WriteString(RenderLine(node, v.styles, false, false, v.width) + "\n")
		}
	}

	return b.String()
}

func (v *MessageView) renderToolResult(m *core.ToolResultMessage, diffMeta map[string]tool.DiffMeta) string {
	row := toolRowFromResult(m, nil, diffMeta)
	return v.renderToolRow(row, false, false)
}

func (v *MessageView) renderToolRow(row toolRow, focused, expanded bool) string {
	node := CallNode{
		ID:     row.ID,
		Tool:   row.Tool,
		Arg:    row.Arg,
		Status: row.Status,
		Meta:   row.Meta,
	}
	out := RenderLine(node, v.styles, focused, expanded, v.width) + "\n"
	if expanded {
		out += v.renderToolDetail(row.Content)
	}
	return out
}

func (v *MessageView) renderToolDetail(content string) string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		content = "(no output)"
	}
	lines := strings.Split(content, "\n")
	const maxDetailLines = 24
	if len(lines) > maxDetailLines {
		hidden := len(lines) - maxDetailLines
		lines = append(lines[:maxDetailLines], fmt.Sprintf("... (%d lines hidden)", hidden))
	}

	var b strings.Builder
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+"):
			b.WriteString(v.styles.Success.Render(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(v.styles.ToolError.Render(line))
		case strings.HasPrefix(line, "@@"):
			b.WriteString(v.styles.ToolEdit.Render(line))
		default:
			b.WriteString(v.styles.Muted.Render(line))
		}
		b.WriteByte('\n')
	}

	w := v.width - 6
	if w < 20 {
		w = 20
	}
	return lipgloss.NewStyle().
		Border(lipgloss.Border{Left: "│"}).
		BorderForeground(v.styles.BorderColor).
		Foreground(lipgloss.Color("#6e738d")).
		Padding(0, 1).
		MarginLeft(3).
		MarginBottom(1).
		Width(w).
		Render(b.String()) + "\n"
}

// formatDiffMeta renders a DiffMeta as the compact "+47 -8 · 1f 3h" form
// shown in the call-tree meta column.
func formatDiffMeta(dm tool.DiffMeta) string {
	return fmt.Sprintf("+%d -%d · %df %dh", dm.Added, dm.Removed, dm.Files, dm.Hunks)
}

// streamCache holds cached glamour output during streaming to avoid re-rendering every tick.
type streamCache struct {
	render   string
	newlines int // newline count at last render
	textLen  int // byte length at last render — detects text reset (compaction/abort)
}

// RenderStreaming renders a partial assistant response being streamed.
// Uses glamour with caching — only re-renders when newline count changes.
// Newline-only triggering avoids mid-line re-renders during code blocks,
// where incomplete syntax causes glamour to produce unstable output (flicker).
func (v *MessageView) RenderStreaming(text string, thinking string, cache *streamCache) string {
	var b strings.Builder

	if text == "" && thinking == "" {
		b.WriteString(v.styles.Muted.Render("waiting...") + "\n")
		return b.String()
	}

	if thinking != "" {
		preview := truncateRunes(thinking, 80)
		b.WriteString(v.styles.Thinking.Render("◦ "+preview) + "\n")
	}

	if text != "" {
		nl := strings.Count(text, "\n")
		needsRender := nl != cache.newlines || len(text) < cache.textLen

		if v.renderer != nil && needsRender {
			if rendered, err := v.renderer.Render(text); err == nil {
				cache.render = rendered
				cache.newlines = nl
				cache.textLen = len(text)
			}
		}

		if cache.render != "" {
			b.WriteString(cache.render)
		} else {
			b.WriteString(text)
		}
	}
	b.WriteString(v.styles.Spinner.Render(" _"))
	b.WriteByte('\n')
	return b.String()
}
