package sift

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompress_Passthrough(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	small := "hello world"
	result := Compress(small, cfg)
	assert.Equal(t, small, result)
}

func TestCompress_NoReduction(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SizeThreshold = 10

	text := strings.Repeat("x", 20)
	result := Compress(text, cfg)
	assert.Equal(t, text, result, "should return original when no reduction achieved")
}

func TestStripTrailingWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trailing spaces", "hello   \nworld  ", "hello\nworld"},
		{"trailing tabs", "hello\t\t\nworld\t", "hello\nworld"},
		{"no trailing", "hello\nworld", "hello\nworld"},
		{"empty lines", "\n\n", "\n\n"},
		{"mixed", "a  \nb\t \nc", "a\nb\nc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(stripTrailingWhitespace(strings.Split(tt.in, "\n")), "\n")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollapseBlankLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		threshold int
		want      string
	}{
		{
			"below threshold",
			"a\n\n\nb",
			4,
			"a\n\n\nb",
		},
		{
			"at threshold collapses",
			"a\n\n\n\nb",
			3,
			"a\n\nb",
		},
		{
			"above threshold collapses",
			"a\n\n\n\n\n\nb",
			3,
			"a\n\nb",
		},
		{
			"multiple runs",
			"a\n\n\n\nb\n\n\n\nc",
			3,
			"a\n\nb\n\nc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(collapseBlankLines(strings.Split(tt.in, "\n"), tt.threshold), "\n")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollapseRepeatedLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		threshold int
		want      string
	}{
		{
			"below threshold",
			"a\na\na\nb",
			5,
			"a\na\na\nb",
		},
		{
			"at threshold collapses",
			"a\na\na\na\na\nb",
			5,
			"a\n[... 4 identical lines collapsed]\nb",
		},
		{
			"above threshold",
			"x\nx\nx\nx\nx\nx\nx\ny",
			3,
			"x\n[... 6 identical lines collapsed]\ny",
		},
		{
			"different groups",
			"a\na\na\na\na\nb\nb\nb\nb\nb",
			5,
			"a\n[... 4 identical lines collapsed]\nb\n[... 4 identical lines collapsed]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(collapseRepeatedLines(strings.Split(tt.in, "\n"), tt.threshold), "\n")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	marker := "\n[TRUNCATED {kept}/{total}]"

	tests := []struct {
		name string
		in   string
		max  int
	}{
		{"below max passes through", "short text", 100},
		{"truncates long text", strings.Repeat("line\n", 20), 20},
		{"truncates with many lines", strings.Repeat("abcdefghij\n", 100), 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncate(tt.in, tt.max, marker)
			if len(tt.in) <= tt.max {
				assert.Equal(t, tt.in, got, "below max should pass through")
			} else {
				assert.Contains(t, got, "[TRUNCATED", "should contain truncation marker")
				assert.Less(t, len(got), len(tt.in), "truncated should be smaller than original")
			}
		})
	}
}

func TestCompress_Combined(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SizeThreshold = 50

	var b strings.Builder
	b.WriteString("header   \n")
	for range 5 {
		b.WriteString("\n")
	}
	b.WriteString("content\n")
	for range 10 {
		b.WriteString("repeated line\n")
	}
	b.WriteString("footer\n")

	text := b.String()
	result := Compress(text, cfg)

	assert.Contains(t, result, "[SIFT:")
	assert.Contains(t, result, "reduction")
	assert.Contains(t, result, "[... 9 identical lines collapsed]")
	assert.Less(t, len(result), len(text), "compressed should be smaller")
}
