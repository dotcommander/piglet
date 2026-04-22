package unicodeaudit_test

import (
	"testing"

	"github.com/dotcommander/piglet/extensions/internal/unicodeaudit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudit_CleanText(t *testing.T) {
	t.Parallel()

	clean := []string{
		"hello world",
		"",
		"SELECT * FROM users WHERE id = 1",
		"日本語テキスト", // legitimate Japanese — not homoglyphs
	}
	for _, s := range clean {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			for _, f := range unicodeaudit.Audit(s) {
				if f.Kind == "homoglyph" {
					continue
				}
				t.Errorf("unexpected finding in %q: %s", s, f)
			}
		})
	}
}

func TestAudit_BidiControls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		char rune
	}{
		{"RLO", 0x202E}, // RIGHT-TO-LEFT OVERRIDE
		{"LRE", 0x202A}, // LEFT-TO-RIGHT EMBEDDING
		{"PDF", 0x202C}, // POP DIRECTIONAL FORMATTING
		{"FSI", 0x2068}, // FIRST STRONG ISOLATE
		{"PDI", 0x2069}, // POP DIRECTIONAL ISOLATE
		{"LRM", 0x200E}, // LEFT-TO-RIGHT MARK
		{"RLM", 0x200F}, // RIGHT-TO-LEFT MARK
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := "safe" + string(tc.char) + "text"
			findings := unicodeaudit.Audit(input)
			require.NotEmpty(t, findings)
			found := false
			for _, f := range findings {
				if f.Rune == tc.char {
					assert.Equal(t, "bidi-control", f.Kind)
					found = true
				}
			}
			assert.True(t, found, "expected rune U+%04X in findings", tc.char)
		})
	}
}

func TestAudit_Invisible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		char rune
	}{
		{"ZWSP", 0x200B},        // ZERO WIDTH SPACE
		{"ZWJ", 0x200D},         // ZERO WIDTH JOINER
		{"BOM", 0xFEFF},         // ZERO WIDTH NO-BREAK SPACE
		{"WORD_JOINER", 0x2060}, // WORD JOINER
		{"SOFT_HYPHEN", 0x00AD}, // SOFT HYPHEN
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := "hel" + string(tc.char) + "lo"
			findings := unicodeaudit.Audit(input)
			require.NotEmpty(t, findings)
			found := false
			for _, f := range findings {
				if f.Rune == tc.char {
					assert.Equal(t, "invisible", f.Kind)
					found = true
				}
			}
			assert.True(t, found, "expected rune U+%04X in findings", tc.char)
		})
	}
}

func TestAudit_Homoglyphs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		char rune
	}{
		{"cyrillic-A", 0x0410},    // CYRILLIC CAPITAL LETTER A — looks like A
		{"cyrillic-o", 0x043E},    // CYRILLIC SMALL LETTER O — looks like o
		{"fullwidth-A", 0xFF21},   // FULLWIDTH LATIN CAPITAL LETTER A
		{"greek-omicron", 0x03BF}, // GREEK SMALL LETTER OMICRON — looks like o
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := "hell" + string(tc.char) + " world"
			findings := unicodeaudit.Audit(input)
			require.NotEmpty(t, findings)
			found := false
			for _, f := range findings {
				if f.Rune == tc.char {
					assert.Equal(t, "homoglyph", f.Kind)
					assert.Contains(t, f.Desc, "ASCII")
					found = true
				}
			}
			assert.True(t, found, "expected rune U+%04X in findings", tc.char)
		})
	}
}

func TestAudit_TagChars(t *testing.T) {
	t.Parallel()

	// U+E0041 TAG LATIN CAPITAL LETTER A
	input := "hello" + string(rune(0xE0041)) + "world"
	findings := unicodeaudit.Audit(input)
	require.NotEmpty(t, findings)
	assert.Equal(t, "tag-char", findings[0].Kind)
}

func TestAudit_LineColTracking(t *testing.T) {
	t.Parallel()

	// BOM (U+FEFF) on line 2 col 3: "li" + BOM + "ne2"
	input := "line1\nli" + string(rune(0xFEFF)) + "ne2"
	findings := unicodeaudit.Audit(input)
	require.Len(t, findings, 1)
	assert.Equal(t, 2, findings[0].Line)
	assert.Equal(t, 3, findings[0].Col)
}

func TestSanitize(t *testing.T) {
	t.Parallel()

	t.Run("replaces suspicious chars", func(t *testing.T) {
		t.Parallel()
		input := "hel" + string(rune(0x200B)) + "lo" // ZWSP
		got := unicodeaudit.Sanitize(input, "?")
		assert.Equal(t, "hel?lo", got)
	})

	t.Run("empty replacement removes chars", func(t *testing.T) {
		t.Parallel()
		input := "hel" + string(rune(0x200B)) + "lo"
		got := unicodeaudit.Sanitize(input, "")
		assert.Equal(t, "hello", got)
	})

	t.Run("clean text unchanged", func(t *testing.T) {
		t.Parallel()
		input := "clean text here"
		got := unicodeaudit.Sanitize(input, "?")
		assert.Equal(t, input, got)
	})
}

func TestClean(t *testing.T) {
	t.Parallel()

	assert.True(t, unicodeaudit.Clean("hello world"))
	assert.False(t, unicodeaudit.Clean("hello"+string(rune(0x200B))+"world"))
	assert.False(t, unicodeaudit.Clean("hel"+string(rune(0x202E))+"lo"))
}

func TestFindingString(t *testing.T) {
	t.Parallel()

	input := "x" + string(rune(0x202E)) + "y"
	findings := unicodeaudit.Audit(input)
	require.NotEmpty(t, findings)
	s := findings[0].String()
	assert.Contains(t, s, "U+202E")
	assert.Contains(t, s, "bidi-control")
}

func TestAudit_ReplacementChar(t *testing.T) {
	t.Parallel()

	input := "bad" + string(rune(0xFFFD)) + "encoding"
	findings := unicodeaudit.Audit(input)
	require.NotEmpty(t, findings)
	assert.Equal(t, "replacement-char", findings[0].Kind)
}

func TestAudit_VariationSelectors(t *testing.T) {
	t.Parallel()

	// U+FE00 VARIATION SELECTOR-1
	input := "text" + string(rune(0xFE00)) + "more"
	findings := unicodeaudit.Audit(input)
	require.NotEmpty(t, findings)
	assert.Equal(t, "invisible", findings[0].Kind)
}
