package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/dotcommander/piglet/core"
)

// MessageView renders conversation messages.
type MessageView struct {
	styles   Styles
	renderer *glamour.TermRenderer
	width    int
}

// NewMessageView creates a message renderer.
func NewMessageView(styles Styles, width int) MessageView {
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4),
	)
	return MessageView{styles: styles, renderer: r, width: width}
}

// SetWidth updates the rendering width.
func (v *MessageView) SetWidth(w int) {
	v.width = w
	v.renderer, _ = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(w-4),
	)
}

// RenderMessage renders a single message.
func (v MessageView) RenderMessage(msg core.Message) string {
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

func (v MessageView) renderUser(m *core.UserMessage) string {
	label := v.styles.UserMsg.Render("> You")
	content := m.Content
	if content == "" && len(m.Blocks) > 0 {
		for _, b := range m.Blocks {
			if tc, ok := b.(core.TextContent); ok {
				content = tc.Text
				break
			}
		}
	}
	return label + "\n" + content + "\n"
}

func (v MessageView) renderAssistant(m *core.AssistantMessage) string {
	var b strings.Builder
	b.WriteString(v.styles.AssistantMsg.Render("Assistant") + "\n")

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
			preview := block.Thinking
			if len([]rune(preview)) > 80 {
				preview = string([]rune(preview)[:80]) + "..."
			}
			b.WriteString(v.styles.Thinking.Render("thinking: "+preview) + "\n")
		case core.ToolCall:
			b.WriteString(v.styles.ToolName.Render("tool: "+block.Name) + "\n")
		}
	}

	return b.String()
}

func (v MessageView) renderToolResult(m *core.ToolResultMessage) string {
	style := v.styles.ToolName
	if m.IsError {
		style = v.styles.ToolError
	}

	label := style.Render(fmt.Sprintf("[%s]", m.ToolName))

	var content string
	for _, c := range m.Content {
		if tc, ok := c.(core.TextContent); ok {
			content = tc.Text
			break
		}
	}

	// Truncate long tool output
	lines := strings.Split(content, "\n")
	if len(lines) > 10 {
		content = strings.Join(lines[:5], "\n") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(
				fmt.Sprintf("\n... (%d lines truncated)\n", len(lines)-10)) +
			strings.Join(lines[len(lines)-5:], "\n")
	}

	return label + "\n" + content + "\n"
}

// RenderStreaming renders a partial assistant response being streamed.
func (v MessageView) RenderStreaming(text string, thinking string) string {
	var b strings.Builder
	b.WriteString(v.styles.AssistantMsg.Render("Assistant") + "\n")

	if thinking != "" {
		preview := thinking
		if len([]rune(preview)) > 80 {
			preview = string([]rune(preview)[:80]) + "..."
		}
		b.WriteString(v.styles.Thinking.Render("thinking: "+preview) + "\n")
	}

	if text != "" {
		b.WriteString(text)
	}
	b.WriteString(v.styles.Spinner.Render(" _"))
	b.WriteByte('\n')
	return b.String()
}
