// Package unicodeaudit detects Unicode characters that are visually similar to
// ASCII but semantically different — homoglyphs, bidirectional control
// characters, zero-width characters, and other invisible/confusable runes.
//
// These characters can appear in skill files, user messages, or LLM output and
// may cause subtle prompt-injection or display-spoofing issues.
package unicodeaudit

import (
	"fmt"
	"strings"
	"unicode"
)

// Finding describes a single suspicious character occurrence.
type Finding struct {
	// Rune is the offending character.
	Rune rune
	// Offset is the byte offset in the input string.
	Offset int
	// Line is the 1-based line number (counting '\n' delimiters).
	Line int
	// Col is the 1-based rune column within the line.
	Col int
	// Kind describes the category of the finding.
	Kind string
	// Desc is a human-readable description of the issue.
	Desc string
}

func (f Finding) String() string {
	return fmt.Sprintf("line %d col %d: U+%04X %s (%s)", f.Line, f.Col, f.Rune, f.Kind, f.Desc)
}

// Audit scans text and returns all suspicious Unicode findings.
// Returns nil if the text is clean.
func Audit(text string) []Finding {
	var findings []Finding

	line, col := 1, 1
	byteOffset := 0

	for _, r := range text {
		if k, desc, bad := classify(r); bad {
			findings = append(findings, Finding{
				Rune:   r,
				Offset: byteOffset,
				Line:   line,
				Col:    col,
				Kind:   k,
				Desc:   desc,
			})
		}

		// Advance position tracking.
		byteOffset += len(string(r))
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}

	return findings
}

// Sanitize returns a copy of text with all suspicious characters replaced by
// the given replacement string (typically "?" or "").
func Sanitize(text, replacement string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if _, _, bad := classify(r); bad {
			b.WriteString(replacement)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Clean returns true if text contains no suspicious Unicode characters.
func Clean(text string) bool {
	return len(Audit(text)) == 0
}

// classify returns the kind, description, and whether the rune is suspicious.
func classify(r rune) (kind, desc string, bad bool) {
	// Bidirectional override/isolate controls (prompt injection risk).
	if isBidiControl(r) {
		return "bidi-control", bidiDesc(r), true
	}

	// Zero-width / invisible characters (except legitimate U+200B ZWSP in some contexts).
	if isInvisible(r) {
		return "invisible", invisibleDesc(r), true
	}

	// Homoglyph: non-ASCII character that looks like an ASCII letter or digit.
	if isHomoglyph(r) {
		return "homoglyph", fmt.Sprintf("looks like ASCII %q", homoglyphAscii(r)), true
	}

	// Tag characters (U+E0000 range) used in some injection techniques.
	if r >= 0xE0000 && r <= 0xE007F {
		return "tag-char", "Unicode tag block character", true
	}

	// Replacement character from bad encoding.
	if r == unicode.ReplacementChar {
		return "replacement-char", "U+FFFD replacement character (encoding error)", true
	}

	return "", "", false
}

// isBidiControl returns true for Unicode bidirectional control characters.
// These can be used to reorder displayed text, hiding malicious content.
func isBidiControl(r rune) bool {
	switch r {
	case 0x202A, // LEFT-TO-RIGHT EMBEDDING
		0x202B, // RIGHT-TO-LEFT EMBEDDING
		0x202C, // POP DIRECTIONAL FORMATTING
		0x202D, // LEFT-TO-RIGHT OVERRIDE
		0x202E, // RIGHT-TO-LEFT OVERRIDE
		0x2066, // LEFT-TO-RIGHT ISOLATE
		0x2067, // RIGHT-TO-LEFT ISOLATE
		0x2068, // FIRST STRONG ISOLATE
		0x2069, // POP DIRECTIONAL ISOLATE
		0x200F, // RIGHT-TO-LEFT MARK
		0x200E: // LEFT-TO-RIGHT MARK
		return true
	}
	return false
}

func bidiDesc(r rune) string {
	names := map[rune]string{
		0x202A: "LEFT-TO-RIGHT EMBEDDING",
		0x202B: "RIGHT-TO-LEFT EMBEDDING",
		0x202C: "POP DIRECTIONAL FORMATTING",
		0x202D: "LEFT-TO-RIGHT OVERRIDE",
		0x202E: "RIGHT-TO-LEFT OVERRIDE",
		0x2066: "LEFT-TO-RIGHT ISOLATE",
		0x2067: "RIGHT-TO-LEFT ISOLATE",
		0x2068: "FIRST STRONG ISOLATE",
		0x2069: "POP DIRECTIONAL ISOLATE",
		0x200F: "RIGHT-TO-LEFT MARK",
		0x200E: "LEFT-TO-RIGHT MARK",
	}
	if name, ok := names[r]; ok {
		return name
	}
	return "bidirectional control"
}

// isInvisible returns true for zero-width and other invisible characters that
// are not legitimate whitespace.
func isInvisible(r rune) bool {
	switch r {
	case 0x00AD, // SOFT HYPHEN
		0x034F, // COMBINING GRAPHEME JOINER
		0x061C, // ARABIC LETTER MARK
		0x115F, // HANGUL CHOSEONG FILLER
		0x1160, // HANGUL JUNGSEONG FILLER
		0x17B4, // KHMER VOWEL INHERENT AQ
		0x17B5, // KHMER VOWEL INHERENT AA
		0x180B, // MONGOLIAN FREE VARIATION SELECTOR ONE
		0x180C, // MONGOLIAN FREE VARIATION SELECTOR TWO
		0x180D, // MONGOLIAN FREE VARIATION SELECTOR THREE
		0x180E, // MONGOLIAN VOWEL SEPARATOR
		0x200B, // ZERO WIDTH SPACE
		0x200C, // ZERO WIDTH NON-JOINER
		0x200D, // ZERO WIDTH JOINER
		0x2028, // LINE SEPARATOR
		0x2029, // PARAGRAPH SEPARATOR
		0x2060, // WORD JOINER
		0x2061, // FUNCTION APPLICATION
		0x2062, // INVISIBLE TIMES
		0x2063, // INVISIBLE SEPARATOR
		0x2064, // INVISIBLE PLUS
		0xFEFF, // ZERO WIDTH NO-BREAK SPACE (BOM)
		0xFFA0: // HALFWIDTH HANGUL FILLER
		return true
	}
	// Variation selectors (U+FE00–FE0F, U+E0100–E01EF).
	if r >= 0xFE00 && r <= 0xFE0F {
		return true
	}
	if r >= 0xE0100 && r <= 0xE01EF {
		return true
	}
	return false
}

func invisibleDesc(r rune) string {
	names := map[rune]string{
		0x200B: "ZERO WIDTH SPACE",
		0x200C: "ZERO WIDTH NON-JOINER",
		0x200D: "ZERO WIDTH JOINER",
		0xFEFF: "BOM / ZERO WIDTH NO-BREAK SPACE",
		0x00AD: "SOFT HYPHEN",
		0x2060: "WORD JOINER",
		0x2028: "LINE SEPARATOR",
		0x2029: "PARAGRAPH SEPARATOR",
	}
	if name, ok := names[r]; ok {
		return name
	}
	return "invisible/zero-width character"
}

// homoglyphTable maps confusable non-ASCII runes to their ASCII lookalike.
// Sourced from Unicode confusables.txt (common cases only — full list is >4k entries).
var homoglyphTable = map[rune]rune{
	// Cyrillic lookalikes
	0x0410: 'A', // CYRILLIC CAPITAL LETTER A
	0x0430: 'a', // CYRILLIC SMALL LETTER A
	0x0412: 'B', // CYRILLIC CAPITAL LETTER VE
	0x0421: 'C', // CYRILLIC CAPITAL LETTER ES
	0x0441: 'c', // CYRILLIC SMALL LETTER ES
	0x0415: 'E', // CYRILLIC CAPITAL LETTER IE
	0x0435: 'e', // CYRILLIC SMALL LETTER IE
	0x0405: 'S', // CYRILLIC CAPITAL LETTER DZE
	0x0406: 'I', // CYRILLIC CAPITAL LETTER BYELORUSSIAN-UKRAINIAN I
	0x0456: 'i', // CYRILLIC SMALL LETTER BYELORUSSIAN-UKRAINIAN I
	0x0408: 'J', // CYRILLIC CAPITAL LETTER JE
	0x041A: 'K', // CYRILLIC CAPITAL LETTER KA
	0x041C: 'M', // CYRILLIC CAPITAL LETTER EM
	0x041D: 'H', // CYRILLIC CAPITAL LETTER EN
	0x041E: 'O', // CYRILLIC CAPITAL LETTER O
	0x043E: 'o', // CYRILLIC SMALL LETTER O
	0x0420: 'P', // CYRILLIC CAPITAL LETTER ER
	0x0422: 'T', // CYRILLIC CAPITAL LETTER TE
	0x0425: 'X', // CYRILLIC CAPITAL LETTER HA
	0x0445: 'x', // CYRILLIC SMALL LETTER HA
	0x0443: 'y', // CYRILLIC SMALL LETTER U

	// Greek lookalikes
	0x0391: 'A', // GREEK CAPITAL LETTER ALPHA
	0x0392: 'B', // GREEK CAPITAL LETTER BETA
	0x0395: 'E', // GREEK CAPITAL LETTER EPSILON
	0x0396: 'Z', // GREEK CAPITAL LETTER ZETA
	0x0397: 'H', // GREEK CAPITAL LETTER ETA
	0x0399: 'I', // GREEK CAPITAL LETTER IOTA
	0x039A: 'K', // GREEK CAPITAL LETTER KAPPA
	0x039C: 'M', // GREEK CAPITAL LETTER MU
	0x039D: 'N', // GREEK CAPITAL LETTER NU
	0x039F: 'O', // GREEK CAPITAL LETTER OMICRON
	0x03BF: 'o', // GREEK SMALL LETTER OMICRON
	0x03A1: 'P', // GREEK CAPITAL LETTER RHO
	0x03A4: 'T', // GREEK CAPITAL LETTER TAU
	0x03A5: 'Y', // GREEK CAPITAL LETTER UPSILON
	0x03A7: 'X', // GREEK CAPITAL LETTER CHI
	0x03B9: 'i', // GREEK SMALL LETTER IOTA (looks like i or l)

	// Fullwidth ASCII
	0xFF01: '!',
	0xFF02: '"',
	0xFF03: '#',
	0xFF04: '$',
	0xFF05: '%',
	0xFF06: '&',
	0xFF07: '\'',
	0xFF08: '(',
	0xFF09: ')',
	0xFF0A: '*',
	0xFF0B: '+',
	0xFF0C: ',',
	0xFF0D: '-',
	0xFF0E: '.',
	0xFF0F: '/',
	0xFF10: '0', 0xFF11: '1', 0xFF12: '2', 0xFF13: '3', 0xFF14: '4',
	0xFF15: '5', 0xFF16: '6', 0xFF17: '7', 0xFF18: '8', 0xFF19: '9',
	0xFF1A: ':', 0xFF1B: ';', 0xFF1C: '<', 0xFF1D: '=', 0xFF1E: '>',
	0xFF1F: '?', 0xFF20: '@',
	0xFF21: 'A', 0xFF22: 'B', 0xFF23: 'C', 0xFF24: 'D', 0xFF25: 'E',
	0xFF26: 'F', 0xFF27: 'G', 0xFF28: 'H', 0xFF29: 'I', 0xFF2A: 'J',
	0xFF2B: 'K', 0xFF2C: 'L', 0xFF2D: 'M', 0xFF2E: 'N', 0xFF2F: 'O',
	0xFF30: 'P', 0xFF31: 'Q', 0xFF32: 'R', 0xFF33: 'S', 0xFF34: 'T',
	0xFF35: 'U', 0xFF36: 'V', 0xFF37: 'W', 0xFF38: 'X', 0xFF39: 'Y',
	0xFF3A: 'Z',
	0xFF41: 'a', 0xFF42: 'b', 0xFF43: 'c', 0xFF44: 'd', 0xFF45: 'e',
	0xFF46: 'f', 0xFF47: 'g', 0xFF48: 'h', 0xFF49: 'i', 0xFF4A: 'j',
	0xFF4B: 'k', 0xFF4C: 'l', 0xFF4D: 'm', 0xFF4E: 'n', 0xFF4F: 'o',
	0xFF50: 'p', 0xFF51: 'q', 0xFF52: 'r', 0xFF53: 's', 0xFF54: 't',
	0xFF55: 'u', 0xFF56: 'v', 0xFF57: 'w', 0xFF58: 'x', 0xFF59: 'y',
	0xFF5A: 'z',
}

func isHomoglyph(r rune) bool {
	_, ok := homoglyphTable[r]
	return ok
}

func homoglyphAscii(r rune) rune {
	return homoglyphTable[r]
}
