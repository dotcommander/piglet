package tui

import (
	"strings"
	"testing"
)

func TestModalViewRendersPaletteRows(t *testing.T) {
	t.Parallel()

	m := NewModalModel("tool output", []ModalItem{
		{ID: "toggle", Label: "Expand focused tool call", Desc: "alt+i"},
		{ID: "collapse-all", Label: "Collapse everything", Desc: "alt+shift+c"},
	}, NewStyles(DefaultTheme()))
	m.SetSize(90, 30)
	m.Show()

	out := m.View()
	for _, want := range []string{"tool output", "Expand focused tool call", "alt+i", "▾", "↑↓ navigate"} {
		if !strings.Contains(out, want) {
			t.Fatalf("modal output missing %q:\n%s", want, out)
		}
	}
}

func TestModalIconFallback(t *testing.T) {
	t.Parallel()

	if got := modalIcon("toggle"); got != "▾" {
		t.Fatalf("modalIcon(toggle) = %q", got)
	}
	if got := modalIcon("unknown"); got != "·" {
		t.Fatalf("modalIcon(unknown) = %q", got)
	}
}
