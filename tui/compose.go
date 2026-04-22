package tui

import "strings"

// compositeOverlay places `over` onto `base`, centered vertically within the (w,h) box.
// Both strings are assumed pre-rendered with ANSI. Lines of `over` replace
// lines of `base` at the computed position; base characters underneath are
// dropped at those rows. w is unused for now but kept for future
// column-aware splicing.
func compositeOverlay(base, over string, w, h int) string {
	_ = w
	baseLines := strings.Split(base, "\n")
	overLines := strings.Split(over, "\n")
	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}
	startY := (h - len(overLines)) / 2
	if startY < 0 {
		startY = 0
	}
	for i, ol := range overLines {
		y := startY + i
		if y >= len(baseLines) {
			break
		}
		baseLines[y] = ol
	}
	return strings.Join(baseLines, "\n")
}
