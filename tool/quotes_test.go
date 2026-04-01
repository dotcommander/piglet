package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindWithQuoteNormalization_ExactMatch(t *testing.T) {
	t.Parallel()
	actual, count := findWithQuoteNormalization(`say "hello"`, `"hello"`)
	assert.Equal(t, 1, count)
	assert.Equal(t, `"hello"`, actual)
}

func TestFindWithQuoteNormalization_CurlyToStraight(t *testing.T) {
	t.Parallel()
	content := "say \u201Chello\u201D world"
	search := `say "hello" world`
	actual, count := findWithQuoteNormalization(content, search)
	assert.Equal(t, 1, count)
	assert.Equal(t, "say \u201Chello\u201D world", actual)
}

func TestFindWithQuoteNormalization_SingleQuotes(t *testing.T) {
	t.Parallel()
	content := "it\u2019s a \u2018test\u2019"
	search := "it's a 'test'"
	actual, count := findWithQuoteNormalization(content, search)
	assert.Equal(t, 1, count)
	assert.Equal(t, content, actual)
}

func TestFindWithQuoteNormalization_NotFound(t *testing.T) {
	t.Parallel()
	_, count := findWithQuoteNormalization("hello world", "goodbye")
	assert.Equal(t, 0, count)
}

func TestFindWithQuoteNormalization_MultipleMatches(t *testing.T) {
	t.Parallel()
	content := "\u201Chello\u201D and \u201Chello\u201D"
	search := `"hello"`
	_, count := findWithQuoteNormalization(content, search)
	assert.Equal(t, 2, count)
}

func TestFindWithQuoteNormalization_NoNormalizationNeeded(t *testing.T) {
	t.Parallel()
	// Search has no quotes at all — normalization can't help.
	_, count := findWithQuoteNormalization("hello world", "goodbye world")
	assert.Equal(t, 0, count)
}

func TestApplyCurlyQuotes_NoCurlies(t *testing.T) {
	t.Parallel()
	result := applyCurlyQuotes(`plain "text"`, `new "text"`)
	assert.Equal(t, `new "text"`, result) // no curlies in old → no transform
}

func TestApplyCurlyQuotes_DoubleQuotes(t *testing.T) {
	t.Parallel()
	result := applyCurlyQuotes("\u201Csample\u201D", `say "hello" now`)
	assert.Equal(t, "say \u201Chello\u201D now", result)
}

func TestApplyCurlyQuotes_Apostrophe(t *testing.T) {
	t.Parallel()
	result := applyCurlyQuotes("it\u2019s", "don't stop")
	assert.Equal(t, "don\u2019t stop", result)
}

func TestApplyCurlyQuotes_SingleQuotePair(t *testing.T) {
	t.Parallel()
	result := applyCurlyQuotes("\u2018test\u2019", "'hello'")
	assert.Equal(t, "\u2018hello\u2019", result)
}

func TestApplyCurlyQuotes_MixedQuotes(t *testing.T) {
	t.Parallel()
	result := applyCurlyQuotes("\u201Cshe said \u2018hi\u2019\u201D", `she said "hi" and 'bye'`)
	assert.Contains(t, result, "\u201C") // has left double
	assert.Contains(t, result, "\u201D") // has right double
	assert.Contains(t, result, "\u2018") // has left single
	assert.Contains(t, result, "\u2019") // has right single
}

func TestNormalizeStraight(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"left double", "\u201Chello\u201D", `"hello"`},
		{"left single", "\u2018hello\u2019", "'hello'"},
		{"prime", "\u2032", "'"},
		{"double prime", "\u2033", `"`},
		{"no change", "hello", "hello"},
		{"mixed", "\u201Chello\u2019s\u201D", `"hello's"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expect, normalizeStraight(tt.input))
		})
	}
}

func TestContainsCurlyQuotes(t *testing.T) {
	t.Parallel()
	assert.True(t, containsCurlyQuotes("\u201Chello\u201D"))
	assert.True(t, containsCurlyQuotes("it\u2019s"))
	assert.False(t, containsCurlyQuotes(`plain "text"`))
	assert.False(t, containsCurlyQuotes("hello"))
}
