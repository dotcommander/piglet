package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestParseAnchor(t *testing.T) {
	t.Parallel()

	C := lipgloss.Center
	L := lipgloss.Left
	R := lipgloss.Right
	T := lipgloss.Top
	B := lipgloss.Bottom

	cases := []struct {
		input string
		wantH lipgloss.Position
		wantV lipgloss.Position
	}{
		// Empty / center aliases → Center/Center
		{"", C, C},
		{"center", C, C},
		{"middle", C, C},
		{"center-center", C, C},

		// Single-axis horizontal
		{"left", L, C},
		{"right", R, C},
		{"center-left", L, C},
		{"left-center", L, C},
		{"center-right", R, C},
		{"right-center", R, C},

		// Single-axis vertical
		{"top", C, T},
		{"bottom", C, B},
		{"center-top", C, T},
		{"top-center", C, T},
		{"center-bottom", C, B},
		{"bottom-center", C, B},

		// Compound — dash separator
		{"top-left", L, T},
		{"left-top", L, T},
		{"top-right", R, T},
		{"right-top", R, T},
		{"bottom-left", L, B},
		{"left-bottom", L, B},
		{"bottom-right", R, B},
		{"right-bottom", R, B},

		// Underscore separator
		{"top_right", R, T},
		{"bottom_left", L, B},

		// Case insensitivity
		{"TOP-LEFT", L, T},
		{"Bottom-Right", R, B},
		{"RIGHT", R, C},

		// Whitespace trimming
		{" right ", R, C},
		{"  top-left  ", L, T},

		// Unknown → Center/Center (no crash)
		{"garbage", C, C},
		{"northwest", C, C},
		{"42", C, C},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			gotH, gotV := parseAnchor(tc.input)
			assert.Equal(t, tc.wantH, gotH, "HPos mismatch for %q", tc.input)
			assert.Equal(t, tc.wantV, gotV, "VPos mismatch for %q", tc.input)
		})
	}
}
