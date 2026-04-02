package tui

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/dotcommander/piglet/core"
)

// MessageView renders conversation messages.
type MessageView struct {
	styles    Styles
	renderer  *glamour.TermRenderer
	width     int
	cachedSep string // cached separator line, invalidated on width change
}

// NewMessageView creates a message renderer.
func NewMessageView(styles Styles, width int) MessageView {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		slog.Warn("glamour init failed, using plain text", "error", err)
	}
	return MessageView{styles: styles, renderer: r, width: width}
}

// SetWidth updates the rendering width.
func (v *MessageView) SetWidth(w int) {
	if w == v.width {
		return
	}
	v.width = w
	v.cachedSep = "" // invalidate separator cache
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(w-4),
	)
	if err != nil {
		slog.Warn("glamour resize failed, using plain text", "error", err)
	}
	v.renderer = r
}

// RenderMessage renders a single message.
func (v *MessageView) RenderMessage(msg core.Message) string {
	switch m := msg.(type) {
	case *core.UserMessage:
		return v.renderUser(m)
	case *core.AssistantMessage:
		return v.renderAssistant(m)
	case *core.ToolResultMessage:
		return v.renderToolResult(m)
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
	label := v.styles.UserMsg.Render("you")
	content := m.Content
	if content == "" && len(m.Blocks) > 0 {
		for _, b := range m.Blocks {
			if tc, ok := b.(core.TextContent); ok {
				content = tc.Text
				break
			}
		}
	}
	return label + "\n" + v.separator() + "\n" + content + "\n"
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
	b.WriteString(v.styles.AssistantLabel.Render("piglet") + "\n")
	b.WriteString(v.separator() + "\n")

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
			b.WriteString(v.styles.Thinking.Render("thinking: "+preview) + "\n")
		case core.ToolCall:
			b.WriteString(v.styles.Muted.Render("▸ "+block.Name) + "\n")
		}
	}

	return b.String()
}

func (v *MessageView) renderToolResult(m *core.ToolResultMessage) string {
	var prefix string
	if m.IsError {
		prefix = v.styles.ToolError.Render("✗")
	} else {
		prefix = v.styles.Success.Render("✓")
	}

	name := v.styles.Muted.Render(m.ToolName)

	var content string
	for _, c := range m.Content {
		if tc, ok := c.(core.TextContent); ok {
			content = tc.Text
			break
		}
	}

	trimmed := strings.TrimSpace(content)

	// Single-line: compact inline format
	if !strings.Contains(trimmed, "\n") {
		if trimmed == "" {
			return prefix + " " + name + "\n"
		}
		return prefix + " " + name + "  " + v.styles.Muted.Render(trimmed) + "\n"
	}

	// Multi-line: truncate long output
	lines := strings.Split(content, "\n")
	if len(lines) > 10 {
		content = strings.Join(lines[:5], "\n") +
			v.styles.Muted.Render(
				fmt.Sprintf("\n... (%d lines truncated)\n", len(lines)-10)) +
			strings.Join(lines[len(lines)-5:], "\n")
	}

	return prefix + " " + name + "\n" + v.styles.Muted.Render(content) + "\n"
}

// streamCache holds cached glamour output during streaming to avoid re-rendering every tick.
type streamCache struct {
	render   string
	newlines int // newline count at last render
}

// RenderStreaming renders a partial assistant response being streamed.
// Uses glamour with caching — only re-renders when newline count changes.
// Newline-only triggering avoids mid-line re-renders during code blocks,
// where incomplete syntax causes glamour to produce unstable output (flicker).
func (v *MessageView) RenderStreaming(text string, thinking string, cache *streamCache) string {
	var b strings.Builder
	b.WriteString(v.styles.AssistantLabel.Render("piglet") + "\n")
	b.WriteString(v.separator() + "\n")

	if text == "" && thinking == "" {
		b.WriteString(v.styles.Muted.Render("waiting...") + "\n")
		return b.String()
	}

	if thinking != "" {
		preview := truncateRunes(thinking, 80)
		b.WriteString(v.styles.Thinking.Render("thinking: "+preview) + "\n")
	}

	if text != "" {
		nl := strings.Count(text, "\n")
		needsRender := nl != cache.newlines

		if v.renderer != nil && needsRender {
			if rendered, err := v.renderer.Render(text); err == nil {
				cache.render = rendered
				cache.newlines = nl
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
