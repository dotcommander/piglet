package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// overlay is a single named overlay in the stack.
type overlay struct {
	Key     string
	Title   string
	Content string
	Anchor  string // "center" (default), "right", "left"
	Width   string // "50%", "80" (chars), "" (auto)
}

// OverlayModel manages a stack of named overlays. Escape dismisses the topmost.
type OverlayModel struct {
	stack  []overlay
	width  int
	height int
	styles Styles
	// Per-overlay scroll offsets (keyed by overlay.Key)
	scrolls map[string]int
}

// NewOverlayModel creates an overlay manager.
func NewOverlayModel(styles Styles) OverlayModel {
	return OverlayModel{
		styles:  styles,
		scrolls: make(map[string]int),
	}
}

// SetSize updates the available dimensions.
func (o *OverlayModel) SetSize(w, h int) {
	o.width = w
	o.height = h
}

// Visible returns true if at least one overlay is showing.
func (o OverlayModel) Visible() bool { return len(o.stack) > 0 }

// Show pushes an overlay onto the stack. If an overlay with the same key exists,
// it is replaced (moved to the top).
func (o *OverlayModel) Show(key, title, content, anchor, width string) {
	// Remove existing with same key
	for i, ov := range o.stack {
		if ov.Key == key {
			o.stack = append(o.stack[:i], o.stack[i+1:]...)
			break
		}
	}
	o.stack = append(o.stack, overlay{
		Key:     key,
		Title:   title,
		Content: content,
		Anchor:  anchor,
		Width:   width,
	})
	o.scrolls[key] = 0
}

// Close removes a specific overlay by key.
func (o *OverlayModel) Close(key string) {
	for i, ov := range o.stack {
		if ov.Key == key {
			o.stack = append(o.stack[:i], o.stack[i+1:]...)
			delete(o.scrolls, key)
			return
		}
	}
}

// DismissTop removes the topmost overlay (Escape behavior).
func (o *OverlayModel) DismissTop() {
	if len(o.stack) > 0 {
		key := o.stack[len(o.stack)-1].Key
		o.stack = o.stack[:len(o.stack)-1]
		delete(o.scrolls, key)
	}
}

// ScrollUp scrolls the topmost overlay up.
func (o *OverlayModel) ScrollUp() {
	if len(o.stack) == 0 {
		return
	}
	key := o.stack[len(o.stack)-1].Key
	if o.scrolls[key] > 0 {
		o.scrolls[key]--
	}
}

// ScrollDown scrolls the topmost overlay down.
func (o *OverlayModel) ScrollDown() {
	if len(o.stack) == 0 {
		return
	}
	ov := o.stack[len(o.stack)-1]
	maxScroll := strings.Count(ov.Content, "\n")
	if o.scrolls[ov.Key] < maxScroll {
		o.scrolls[ov.Key]++
	}
}

// View renders the topmost overlay. The caller composites this on top of the main view.
func (o OverlayModel) View() string {
	if len(o.stack) == 0 {
		return ""
	}
	ov := o.stack[len(o.stack)-1]

	// Resolve width
	w := o.resolveWidth(ov.Width)
	contentWidth := w - 6 // padding + border
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Resolve max content height
	maxH := o.height - 8
	if maxH < 5 {
		maxH = 5
	}

	var b strings.Builder

	// Title
	if ov.Title != "" {
		b.WriteString(o.styles.Header.Render(ov.Title))
		b.WriteByte('\n')
		b.WriteByte('\n')
	}

	// Content with scrolling
	lines := strings.Split(ov.Content, "\n")
	scroll := o.scrolls[ov.Key]
	if scroll > len(lines)-maxH {
		scroll = max(0, len(lines)-maxH)
	}
	visible := lines
	if len(lines) > maxH {
		end := scroll + maxH
		if end > len(lines) {
			end = len(lines)
		}
		visible = lines[scroll:end]
	}
	b.WriteString(strings.Join(visible, "\n"))

	// Scroll indicator
	if len(lines) > maxH {
		b.WriteByte('\n')
		b.WriteString(o.styles.Muted.Render("scroll: up/down | close: esc"))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(o.styles.BorderColor).
		Padding(1, 2).
		Width(w)

	// Return just the bordered box — compositeOverlay handles placement.
	return box.Render(b.String())
}

// HPos returns the horizontal anchor of the topmost overlay as a lipgloss
// Position (Left=0, Center=0.5, Right=1). Used by compositeOverlay to
// position the box without clobbering the base viewport.
func (o OverlayModel) HPos() lipgloss.Position {
	if len(o.stack) == 0 {
		return lipgloss.Center
	}
	switch o.stack[len(o.stack)-1].Anchor {
	case "left":
		return lipgloss.Left
	case "right":
		return lipgloss.Right
	default:
		return lipgloss.Center
	}
}

// VPos returns the vertical anchor of the topmost overlay. Always Center —
// exposed as a method so callers use the same compositeOverlay API as modals.
func (o OverlayModel) VPos() lipgloss.Position {
	return lipgloss.Center
}

// resolveWidth parses the width spec. "50%" = percentage, "80" = chars, "" = 60% default.
func (o OverlayModel) resolveWidth(spec string) int {
	if spec == "" {
		return o.width * 60 / 100
	}
	if strings.HasSuffix(spec, "%") {
		pct := 60
		if n, err := strconv.Atoi(spec[:len(spec)-1]); err == nil && n > 0 && n <= 100 {
			pct = n
		}
		return o.width * pct / 100
	}
	if n, err := strconv.Atoi(spec); err == nil && n > 0 {
		if n > o.width-4 {
			return o.width - 4
		}
		return n
	}
	return o.width * 60 / 100
}
