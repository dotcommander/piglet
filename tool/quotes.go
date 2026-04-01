package tool

import (
	"strings"
	"unicode"
)

// curly quote pairs that should be treated as equivalent to ASCII quotes.
var curlyToStraight = strings.NewReplacer(
	"\u2018", "'", // left single
	"\u2019", "'", // right single
	"\u201C", `"`, // left double
	"\u201D", `"`, // right double
	"\u2032", "'", // prime
	"\u2033", `"`, // double prime
)

// normalizeStraight replaces all curly/smart quote variants with ASCII equivalents.
func normalizeStraight(s string) string {
	return curlyToStraight.Replace(s)
}

// findWithQuoteNormalization attempts an exact match first, then falls back
// to normalized-quote matching. Returns the actual substring from content
// (preserving the file's original curly quotes) and the match count.
//
// When the normalized match succeeds, the returned string is the original
// content's text at the matched position — so replacement preserves the
// file's quote style for unchanged portions.
func findWithQuoteNormalization(content, search string) (actual string, count int) {
	// Fast path: exact match.
	count = strings.Count(content, search)
	if count > 0 {
		return search, count
	}

	// Normalize both sides and search again.
	normContent := normalizeStraight(content)
	normSearch := normalizeStraight(search)

	// If neither side changed, normalization can't help.
	if normContent == content && normSearch == search {
		return "", 0
	}

	count = strings.Count(normContent, normSearch)
	if count != 1 {
		return "", count
	}

	// Find the position in the normalized content.
	pos := strings.Index(normContent, normSearch)
	if pos < 0 {
		return "", 0 // shouldn't happen after Count==1, but be safe
	}

	// Map the byte offset from normalized back to the original content.
	// Both strings have the same rune count at each position — curly quotes
	// are single runes replaced by single ASCII runes. So rune-index mapping
	// works: count runes up to pos in normalized, then count the same number
	// of runes in original.
	runeStart := len([]rune(normContent[:pos]))
	runeEnd := runeStart + len([]rune(normSearch))

	origRunes := []rune(content)
	if runeEnd > len(origRunes) {
		return "", 0
	}

	actual = string(origRunes[runeStart:runeEnd])
	return actual, 1
}

// applyCurlyQuotes converts straight quotes in newText to match the curly
// style found in oldActual. This preserves the file's typographic style
// when the model sends straight-quoted replacement text.
func applyCurlyQuotes(oldActual, newText string) string {
	// If the file doesn't use curly quotes, return as-is.
	if !containsCurlyQuotes(oldActual) {
		return newText
	}

	// Build a mapping of quote positions in the old text to learn the style.
	// For new text, apply heuristic: opening quote after whitespace/punctuation,
	// closing quote otherwise. Apostrophes between letters stay as right single.
	runes := []rune(newText)
	out := make([]rune, 0, len(runes))

	for i, r := range runes {
		switch r {
		case '"':
			if isOpeningPosition(runes, i) {
				out = append(out, '\u201C') // left double
			} else {
				out = append(out, '\u201D') // right double
			}
		case '\'':
			if isApostrophe(runes, i) {
				out = append(out, '\u2019') // right single (apostrophe)
			} else if isOpeningPosition(runes, i) {
				out = append(out, '\u2018') // left single
			} else {
				out = append(out, '\u2019') // right single
			}
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

func containsCurlyQuotes(s string) bool {
	for _, r := range s {
		switch r {
		case '\u2018', '\u2019', '\u201C', '\u201D', '\u2032', '\u2033':
			return true
		}
	}
	return false
}

// isOpeningPosition returns true if position i looks like the start of a
// quoted span — preceded by whitespace, punctuation, or start of string.
func isOpeningPosition(runes []rune, i int) bool {
	if i == 0 {
		return true
	}
	prev := runes[i-1]
	return unicode.IsSpace(prev) || unicode.IsPunct(prev) || prev == '(' || prev == '['
}

// isApostrophe returns true if the single quote at position i is between
// two letters (e.g., "don't", "it's").
func isApostrophe(runes []rune, i int) bool {
	if i == 0 || i >= len(runes)-1 {
		return false
	}
	return unicode.IsLetter(runes[i-1]) && unicode.IsLetter(runes[i+1])
}
