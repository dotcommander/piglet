package tokengate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetState_Record(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ContextWindow: 100000,
		WarnPercent:   80,
	}
	b := NewBudgetState(cfg)

	t.Run("first record does not warn", func(t *testing.T) {
		t.Parallel()
		bs := NewBudgetState(cfg)
		crossed := bs.Record(TurnUsageEvent{
			Turn:  1,
			Usage: TokenUsage{Input: 1000, Output: 500},
			Breakdown: Breakdown{
				SystemPrompt: 5000,
				History:      10000,
				Tools:        3000,
			},
		})
		assert.False(t, crossed)
	})

	t.Run("warns when threshold crossed", func(t *testing.T) {
		t.Parallel()
		bs := NewBudgetState(cfg)
		crossed := bs.Record(TurnUsageEvent{
			Turn:  1,
			Usage: TokenUsage{Input: 5000, Output: 1000},
			Breakdown: Breakdown{
				SystemPrompt: 40000,
				History:      45000,
				Tools:        5000,
			},
		})
		assert.True(t, crossed, "should warn at 90% > 80% threshold")
	})

	t.Run("warns only once", func(t *testing.T) {
		t.Parallel()
		bs := NewBudgetState(cfg)
		first := bs.Record(TurnUsageEvent{
			Turn:      1,
			Usage:     TokenUsage{Input: 5000},
			Breakdown: Breakdown{SystemPrompt: 40000, History: 50000},
		})
		second := bs.Record(TurnUsageEvent{
			Turn:      2,
			Usage:     TokenUsage{Input: 5000},
			Breakdown: Breakdown{SystemPrompt: 40000, History: 55000},
		})
		assert.True(t, first)
		assert.False(t, second, "should not warn again")
	})

	_ = b // use the variable
}

func TestBudgetState_Summary(t *testing.T) {
	t.Parallel()

	b := NewBudgetState(Config{ContextWindow: 200000, WarnPercent: 80})
	b.Record(TurnUsageEvent{
		Turn:  1,
		Usage: TokenUsage{Input: 10000, Output: 3000},
		Breakdown: Breakdown{
			SystemPrompt: 5000,
			RepoMap:      8000,
			Tools:        2000,
			History:      15000,
			Extensions:   []ExtTokens{{Name: "memory", Tokens: 500}},
		},
	})

	summary := b.Summary()
	assert.Contains(t, summary, "Context Budget")
	assert.Contains(t, summary, "200,000")
	assert.Contains(t, summary, "memory")
}

func TestParseTurnUsage(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"turn": 1,
		"usage": {"input": 1000, "output": 500},
		"breakdown": {"system_prompt": 800, "history": 1200}
	}`)

	event, err := ParseTurnUsage(raw)
	require.NoError(t, err)
	assert.Equal(t, 1, event.Turn)
	assert.Equal(t, 1000, event.Usage.Input)
	assert.Equal(t, 800, event.Breakdown.SystemPrompt)
}

func TestFmtNum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{200000, "200,000"},
		{1000000, "1,000,000"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, fmtNum(tt.input), "fmtNum(%d)", tt.input)
	}
}

func TestSkipSummarize(t *testing.T) {
	t.Parallel()

	assert.True(t, skipSummarize("Read"))
	assert.True(t, skipSummarize("Edit"))
	assert.True(t, skipSummarize("dispatch"))
	assert.False(t, skipSummarize("Bash"))
	assert.False(t, skipSummarize("Grep"))
}
