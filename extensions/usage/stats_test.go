package usage

import (
	"encoding/json"
	"testing"
)

func TestSessionStats(t *testing.T) {
	t.Parallel()

	stats := NewSessionStats()

	// Initially empty
	if got := stats.TurnCount(); got != 0 {
		t.Errorf("TurnCount() = %d, want 0", got)
	}

	// Record a turn
	event := TurnUsageEvent{
		Turn: 1,
		Usage: TokenUsage{
			Input:     1000,
			Output:    500,
			CacheRead: 5000,
		},
		Breakdown: ComponentBreakdown{
			SystemPrompt: 800,
			RepoMap:      3000,
			Tools:        500,
			History:      1200,
		},
	}
	stats.Record(event)

	if got := stats.TurnCount(); got != 1 {
		t.Errorf("TurnCount() = %d, want 1", got)
	}

	totals := stats.Totals()
	if totals.Input != 1000 {
		t.Errorf("Input = %d, want 1000", totals.Input)
	}
	if totals.CacheRead != 5000 {
		t.Errorf("CacheRead = %d, want 5000", totals.CacheRead)
	}

	// Record another turn
	event2 := TurnUsageEvent{
		Turn: 2,
		Usage: TokenUsage{
			Input:     1500,
			Output:    600,
			CacheRead: 8000,
		},
		Breakdown: ComponentBreakdown{
			SystemPrompt: 800,
			RepoMap:      3500,
			Tools:        500,
			History:      2700,
		},
	}
	stats.Record(event2)

	totals = stats.Totals()
	if totals.Input != 2500 {
		t.Errorf("Input = %d, want 2500", totals.Input)
	}
	if totals.CacheRead != 13000 {
		t.Errorf("CacheRead = %d, want 13000", totals.CacheRead)
	}

	// Current breakdown should reflect last turn
	breakdown := stats.CurrentBreakdown()
	if breakdown.History != 2700 {
		t.Errorf("History = %d, want 2700", breakdown.History)
	}
}

func TestFormatSummary(t *testing.T) {
	t.Parallel()

	stats := NewSessionStats()
	stats.Record(TurnUsageEvent{
		Turn: 1,
		Usage: TokenUsage{
			Input:         1000,
			Output:        500,
			CacheRead:     50000,
			CacheCreation: 2000,
		},
		Breakdown: ComponentBreakdown{
			SystemPrompt: 800,
			RepoMap:      30000,
			Tools:        500,
			History:      1200,
			Extensions: []ExtensionTokens{
				{Name: "memory", Tokens: 200},
				{Name: "rtk", Tokens: 150},
			},
		},
	})

	summary := stats.FormatSummary()

	// Check key sections appear
	if !contains(summary, "CUMULATIVE TOTALS") {
		t.Error("summary missing CUMULATIVE TOTALS")
	}
	if !contains(summary, "CURRENT PROMPT BREAKDOWN") {
		t.Error("summary missing CURRENT PROMPT BREAKDOWN")
	}
	if !contains(summary, "EXTENSION PROMPT SECTIONS") {
		t.Error("summary missing EXTENSION PROMPT SECTIONS")
	}

	// Check values
	if !contains(summary, "Cache Read") {
		t.Error("summary missing Cache Read")
	}
	if !contains(summary, "memory") {
		t.Error("summary missing memory extension")
	}
}

func TestParseEvent(t *testing.T) {
	t.Parallel()

	jsonData := `{
		"turn": 3,
		"usage": {
			"input": 1500,
			"output": 600,
			"cache_read": 10000,
			"cache_write": 0,
			"cache_creation": 500
		},
		"breakdown": {
			"system_prompt": 800,
			"repo_map": 5000,
			"tools": 400,
			"history": 2000,
			"extensions": [
				{"name": "safeguard", "tokens": 300}
			]
		}
	}`

	event, err := ParseEvent(json.RawMessage(jsonData))
	if err != nil {
		t.Fatalf("ParseEvent() error = %v", err)
	}

	if event.Turn != 3 {
		t.Errorf("Turn = %d, want 3", event.Turn)
	}
	if event.Usage.Input != 1500 {
		t.Errorf("Input = %d, want 1500", event.Usage.Input)
	}
	if event.Usage.CacheRead != 10000 {
		t.Errorf("CacheRead = %d, want 10000", event.Usage.CacheRead)
	}
	if event.Breakdown.RepoMap != 5000 {
		t.Errorf("RepoMap = %d, want 5000", event.Breakdown.RepoMap)
	}
	if len(event.Breakdown.Extensions) != 1 {
		t.Errorf("Extensions count = %d, want 1", len(event.Breakdown.Extensions))
	}
	if event.Breakdown.Extensions[0].Name != "safeguard" {
		t.Errorf("Extension name = %s, want safeguard", event.Breakdown.Extensions[0].Name)
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{100000, "100,000"},
		{15000, "15,000"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := formatNumber(tt.input)
			if got != tt.want {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
