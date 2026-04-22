package tui

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// compositeOverlay splices `over` (just the bordered box, no surrounding
// padding) onto `base` at the position determined by hPos/vPos anchors.
// Base characters outside the box bounds remain visible — this is the
// "composited over base" invariant: the overlay floats over the viewport,
// not replaces it.
//
// ANSI-aware: each base line is split at the box x-offset using ansi.Truncate
// (left fragment) and ansi.TruncateLeft (right fragment) so ANSI SGR codes
// are never sliced mid-sequence.
func compositeOverlay(base, over string, w, h int, hPos, vPos lipgloss.Position) string {
	if over == "" {
		return base
	}

	baseLines := strings.Split(base, "\n")
	// Ensure base has at least h lines so the overlay has rows to splice into.
	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}

	overLines := strings.Split(over, "\n")
	boxW := lipgloss.Width(over)
	boxH := len(overLines)

	// Compute top-left corner of the box in terminal cells.
	x := int(math.Round(float64(w-boxW) * float64(hPos)))
	y := int(math.Round(float64(h-boxH) * float64(vPos)))
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	for i, ol := range overLines {
		row := y + i
		if row >= len(baseLines) {
			break
		}
		baseLine := baseLines[row]

		// Left fragment: columns [0, x).
		left := ansi.Truncate(baseLine, x, "")
		// Right fragment: columns [x+boxW, ...).
		right := ansi.TruncateLeft(baseLine, x+boxW, "")

		baseLines[row] = left + ol + right
	}

	return strings.Join(baseLines, "\n")
}
