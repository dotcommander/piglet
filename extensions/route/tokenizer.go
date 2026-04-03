package route

import (
	"strings"
	"unicode"
)

// stopWords are common words that carry no routing signal.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true, "must": true,
	"to": true, "of": true, "in": true, "for": true, "on": true,
	"with": true, "at": true, "by": true, "from": true, "as": true,
	"into": true, "about": true, "like": true, "through": true, "after": true,
	"over": true, "between": true, "out": true, "up": true, "down": true,
	"and": true, "or": true, "but": true, "not": true, "no": true,
	"so": true, "if": true, "then": true, "than": true, "too": true,
	"very": true, "just": true, "also": true, "only": true, "its": true,
	"it": true, "this": true, "that": true, "these": true, "those": true,
	"my": true, "your": true, "our": true, "their": true, "i": true,
	"me": true, "we": true, "you": true, "he": true, "she": true,
	"they": true, "him": true, "her": true, "us": true, "them": true,
	"what": true, "which": true, "who": true, "when": true, "where": true,
	"how": true, "all": true, "each": true, "every": true, "some": true,
	"any": true, "few": true, "more": true, "most": true, "other": true,
	"need": true, "want": true, "please": true, "help": true, "get": true,
	"make": true, "use": true, "using": true, "there": true, "here": true,
}

// Tokenize splits text into normalized tokens: lowercase, split on
// non-alphanumeric (preserving internal hyphens), remove stop words
// and single-char tokens, deduplicate.
func Tokenize(text string) []string {
	lower := strings.ToLower(text)

	// Split on non-alphanumeric boundaries, preserving internal hyphens
	var tokens []string
	var cur strings.Builder
	for _, r := range lower {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cur.WriteRune(r)
		case r == '-' && cur.Len() > 0:
			cur.WriteRune(r) // preserve internal hyphens
		default:
			if cur.Len() > 0 {
				tokens = append(tokens, strings.Trim(cur.String(), "-"))
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, strings.Trim(cur.String(), "-"))
	}

	// Filter and deduplicate
	seen := make(map[string]bool, len(tokens))
	result := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if len(t) <= 1 || stopWords[t] || seen[t] {
			continue
		}
		seen[t] = true
		result = append(result, t)
	}
	return result
}

// NormalizePlural strips a trailing 's' for basic plural normalization.
// Returns the original if already singular or too short.
func NormalizePlural(word string) string {
	if len(word) > 3 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") {
		return word[:len(word)-1]
	}
	return word
}

// TokensContain checks if needle (or its plural/singular form) is present in tokens.
func TokensContain(tokens []string, needle string) bool {
	norm := NormalizePlural(needle)
	for _, t := range tokens {
		if t == needle || NormalizePlural(t) == norm {
			return true
		}
	}
	return false
}

// TokensContainAll checks if all words in a multi-word phrase are present as tokens.
func TokensContainAll(tokens []string, phrase string) bool {
	words := Tokenize(phrase)
	if len(words) == 0 {
		return false
	}
	for _, w := range words {
		if !TokensContain(tokens, w) {
			return false
		}
	}
	return true
}
