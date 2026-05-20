package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Status is the lifecycle state of a tool call row.
type Status int

const (
	StatusPending Status = iota
	StatusRunning
	StatusOK
	StatusFail
)

// CallNode is a single row in the call tree. Children are populated for
// `task` subagent rows from the tool result Details payload.
type CallNode struct {
	ID       string
	Tool     string
	Arg      string
	Status   Status
	Meta     string
	TailLine string
	Children []*CallNode
	Mutates  bool
	Detail   func() string
}

const (
	glyphCol = 2
	toolCol  = 8
	maxDepth = 3 // visual nesting cap per spec
)

// toolStyle maps a tool name to its palette color.
func toolStyle(tool string, styles Styles) lipgloss.Style {
	switch tool {
	case "read":
		return styles.ToolRead
	case "grep":
		return styles.ToolGrep
	case "write":
		return styles.ToolWrite
	case "edit":
		return styles.ToolEdit
	case "task":
		return styles.ToolTask
	case "bash":
		return styles.ToolBash
	default:
		return styles.Muted
	}
}

func statusGlyph(s Status) string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusFail:
		return "✗"
	case StatusRunning:
		return "…"
	default:
		return " "
	}
}

func padOrTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) == w {
		return s
	}
	if len(r) < w {
		return s + strings.Repeat(" ", w-len(r))
	}
	if w == 1 {
		return "…"
	}
	keep := w - 1
	left := keep / 2
	right := keep - left
	return string(r[:left]) + "…" + string(r[len(r)-right:])
}

// RenderLine renders one tree row. width is the total available cell width.
func RenderLine(node CallNode, styles Styles, focused, expanded bool, width int) string {
	if width <= 0 {
		return ""
	}

	glyph := "▸"
	if expanded {
		glyph = "▾"
	}
	glyphCell := lipgloss.NewStyle().Width(glyphCol).Render(glyph + " ")

	tStyle := toolStyle(node.Tool, styles)
	toolCell := tStyle.Width(toolCol).Render(padOrTruncate(node.Tool, toolCol))

	var metaText string
	var metaStyle lipgloss.Style
	switch {
	case node.Status == StatusRunning && node.TailLine != "":
		metaText = node.TailLine
		metaStyle = styles.Muted
	case node.Status == StatusFail:
		metaText = strings.TrimSpace(statusGlyph(node.Status) + " " + node.Meta)
		metaStyle = styles.ToolError
	case node.Status == StatusOK:
		metaText = strings.TrimSpace(statusGlyph(node.Status) + " " + node.Meta)
		metaStyle = styles.Success
	default:
		metaText = node.Meta
		metaStyle = styles.Muted
	}

	fixed := glyphCol + toolCol
	remaining := width - fixed
	if remaining < 1 {
		remaining = 1
	}

	metaWidth := lipgloss.Width(metaText)
	if metaWidth > remaining-1 {
		metaWidth = remaining - 1
		if metaWidth < 0 {
			metaWidth = 0
		}
		metaText = padOrTruncate(metaText, metaWidth)
	}
	argWidth := remaining - metaWidth
	if argWidth < 1 {
		argWidth = 1
	}

	argCell := lipgloss.NewStyle().Width(argWidth).Render(padOrTruncate(node.Arg, argWidth))
	metaCell := metaStyle.Width(metaWidth).Align(lipgloss.Right).Render(metaText)

	row := lipgloss.JoinHorizontal(lipgloss.Top, glyphCell, toolCell, argCell, metaCell)
	if focused {
		row = lipgloss.NewStyle().Width(width).Background(styles.BorderColor).Render(row)
	}
	return row
}

// RenderTree walks nodes and renders each. Expanded nodes with children
// render those indented; nesting is capped at maxDepth.
func RenderTree(nodes []CallNode, styles Styles, focusedID string, expanded map[string]bool, width int) string {
	var b strings.Builder
	renderNodes(&b, nodes, styles, focusedID, expanded, width, 0, true)
	return b.String()
}

func renderNodes(b *strings.Builder, nodes []CallNode, styles Styles, focusedID string, expanded map[string]bool, width, depth int, first bool) {
	indent := strings.Repeat("  ", depth)
	indentW := lipgloss.Width(indent)
	avail := width - indentW
	if avail < glyphCol+toolCol+2 {
		avail = glyphCol + toolCol + 2
	}
	for i := range nodes {
		n := nodes[i]
		exp := expanded[n.ID]
		line := RenderLine(n, styles, n.ID == focusedID, exp, avail)
		if !(first && i == 0) {
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		b.WriteString(line)
		if exp && len(n.Children) > 0 && depth < maxDepth-1 {
			children := make([]CallNode, len(n.Children))
			for j, c := range n.Children {
				children[j] = *c
			}
			renderNodes(b, children, styles, focusedID, expanded, width, depth+1, false)
		}
		first = false
	}
}
