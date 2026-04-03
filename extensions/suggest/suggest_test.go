package suggest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{
			name:     "short string unchanged",
			input:    "hello world",
			maxRunes: 80,
			want:     "hello world",
		},
		{
			name:     "exact length",
			input:    "hello",
			maxRunes: 5,
			want:     "hello",
		},
		{
			name:     "truncate at word boundary",
			input:    "write tests for the new feature",
			maxRunes: 20,
			want:     "write tests for the",
		},
		{
			name:     "truncate long string",
			input:    "this is a very long suggestion that should be truncated to fit within the limit",
			maxRunes: 30,
			want:     "this is a very long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncate(tt.input, tt.maxRunes)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	s := NewSuggester(cfg, "", nil)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid suggestion",
			input: "run the tests",
			want:  "run the tests",
		},
		{
			name:  "block continue",
			input: "continue",
			want:  "",
		},
		{
			name:  "block proceed",
			input: "proceed",
			want:  "",
		},
		{
			name:  "block done question",
			input: "done?",
			want:  "",
		},
		{
			name:  "longer valid suggestion passes",
			input: "continue implementing the feature",
			want:  "continue implementing the feature",
		},
		{
			name:  "whitespace trimmed",
			input: "  run tests  ",
			want:  "run tests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.filter(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterDuplicates(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	s := NewSuggester(cfg, "", nil)

	// First suggestion passes
	got1 := s.filter("run the tests")
	assert.Equal(t, "run the tests", got1)

	// Duplicate is blocked
	got2 := s.filter("run the tests")
	assert.Equal(t, "", got2)

	// Different suggestion passes
	got3 := s.filter("check the logs")
	assert.Equal(t, "check the logs", got3)
}

func TestShouldSuggest(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Cooldown = 2
	cfg.Enabled = true
	s := NewSuggester(cfg, "", nil)

	// First call should allow suggestion
	assert.True(t, s.ShouldSuggest())

	// After reset, cooldown kicks in
	s.ResetCooldown()

	// ShouldSuggest decrements cooldown and returns false
	assert.False(t, s.ShouldSuggest()) // cooldown now 1

	// Call again - still on cooldown
	assert.False(t, s.ShouldSuggest()) // cooldown now 0

	// Now cooldown has expired
	assert.True(t, s.ShouldSuggest())
}

func TestShouldSuggestDisabled(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Enabled = false
	s := NewSuggester(cfg, "", nil)

	assert.False(t, s.ShouldSuggest())
}

func TestExtractAssistantText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "content array",
			input: `[{"type":"text","text":"hello world"}]`,
			want:  "hello world",
		},
		{
			name:  "multiple content items",
			input: `[{"type":"text","text":"hello"},{"type":"text","text":"world"}]`,
			want:  "hello\nworld",
		},
		{
			name:  "object with content field",
			input: `{"content":[{"type":"text","text":"response text"}]}`,
			want:  "response text",
		},
		{
			name:  "raw string",
			input: `"just a string"`,
			want:  "just a string",
		},
		{
			name:  "invalid json",
			input: `{invalid}`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractAssistantText(json.RawMessage(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTurnDataToolNames(t *testing.T) {
	t.Parallel()
	turn := TurnData{
		ToolResults: []ToolResult{
			{ToolName: "Read"},
			{ToolName: "Edit"},
			{ToolName: "Bash"},
		},
	}

	names := turn.ToolNames()
	assert.Equal(t, []string{"Read", "Edit", "Bash"}, names)
}

func TestTurnDataHasError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		turn TurnData
		want bool
	}{
		{
			name: "no error",
			turn: TurnData{
				ToolResults: []ToolResult{
					{ToolName: "Read", IsError: false},
				},
			},
			want: false,
		},
		{
			name: "has error",
			turn: TurnData{
				ToolResults: []ToolResult{
					{ToolName: "Read", IsError: false},
					{ToolName: "Bash", IsError: true},
				},
			},
			want: true,
		},
		{
			name: "no tool results",
			turn: TurnData{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.turn.HasError())
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	assert.Equal(t, "small", cfg.Model)
	assert.Equal(t, 50, cfg.MaxTokens)
	assert.Equal(t, 3, cfg.Cooldown)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "auto", cfg.TriggerMode)
}

func TestDefaultPrompt(t *testing.T) {
	t.Parallel()
	prompt := DefaultPrompt()

	assert.Contains(t, prompt, "suggest")
	assert.Contains(t, prompt, "max 80 chars")
	assert.Contains(t, prompt, "actionable")
}
