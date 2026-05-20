package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ── Template Expand ───────────────────────────────────────────────────────────

func TestExpand(t *testing.T) {
	t.Parallel()

	tc := &TemplateContext{
		Params: map[string]string{
			"env":  "prod",
			"host": "db.example.com",
		},
		Prev: &StepOutput{
			Stdout: `{"status":"ok","count":42}`,
			Status: "ok",
		},
		Steps: map[string]*StepOutput{
			"build": {Stdout: "binary-v1.2", Status: "ok"},
		},
		Item:     "alpha",
		HasItem:  true,
		LoopVars: map[string]string{"day": "2024-01-15"},
		CWD:      "/repo",
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no placeholders", "echo hello", "echo hello"},
		{"param var", "deploy to {param.env}", "deploy to prod"},
		{"param host", "connect {param.host}", "connect db.example.com"},
		{"prev stdout", "process {prev.stdout}", `process {"status":"ok","count":42}`},
		{"prev status", "was {prev.status}", "was ok"},
		{"prev lines", "lines: {prev.lines}", `lines: {"status":"ok","count":42}`},
		{"prev json string field", "status={prev.json.status}", "status=ok"},
		{"prev json numeric field", "count={prev.json.count}", "count=42"},
		{"step stdout", "artifact={step.build.stdout}", "artifact=binary-v1.2"},
		{"step status", "build_ok={step.build.status}", "build_ok=ok"},
		{"item var", "process {item}", "process alpha"},
		{"loop var", "day={loop.day}", "day=2024-01-15"},
		{"cwd", "in {cwd}", "in /repo"},
		{"unknown var left as-is", "foo {unknown.var} bar", "foo {unknown.var} bar"},
		{"multiple placeholders", "{param.env}/{param.host}", "prod/db.example.com"},
		{"no braces at all", "plain string", "plain string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tc.Expand(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExpandNilPrev(t *testing.T) {
	t.Parallel()

	tc := &TemplateContext{
		Params: map[string]string{},
		Steps:  map[string]*StepOutput{},
	}

	// When prev is nil, {prev.*} should be left as-is
	assert.Equal(t, "{prev.stdout}", tc.Expand("{prev.stdout}"))
	assert.Equal(t, "{prev.status}", tc.Expand("{prev.status}"))
	assert.Equal(t, "{prev.json.foo}", tc.Expand("{prev.json.foo}"))
}

func TestExpandDateAndTimestamp(t *testing.T) {
	t.Parallel()

	tc := &TemplateContext{
		Params: map[string]string{},
		Steps:  map[string]*StepOutput{},
	}

	date := tc.Expand("{date}")
	_, err := time.Parse("2006-01-02", date)
	assert.NoError(t, err, "date should be YYYY-MM-DD format")

	ts := tc.Expand("{timestamp}")
	assert.NotEmpty(t, ts)
	// should be all digits
	for _, r := range ts {
		assert.True(t, r >= '0' && r <= '9', "timestamp should be numeric")
	}
}

// ── Expand edge cases ─────────────────────────────────────────────────────────

func TestExpandNoPlaceholders(t *testing.T) {
	t.Parallel()

	tc := &TemplateContext{
		Params: map[string]string{},
		Steps:  map[string]*StepOutput{},
	}
	// Fast path: no '{' in string
	input := "no placeholders here"
	assert.Equal(t, input, tc.Expand(input))
}

func TestExpandUnclosedBrace(t *testing.T) {
	t.Parallel()

	tc := &TemplateContext{
		Params: map[string]string{"x": "X"},
		Steps:  map[string]*StepOutput{},
	}
	// Unclosed brace: should emit remainder as-is
	result := tc.Expand("hello {param.x} and {unclosed")
	assert.True(t, strings.HasPrefix(result, "hello X and"))
}
