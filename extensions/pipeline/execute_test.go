package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Run (integration) ─────────────────────────────────────────────────────────

func TestRun(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "test-pipeline",
		Steps: []Step{
			{Name: "step1", Run: "echo hello-world"},
			{Name: "step2", Run: "echo got:{prev.stdout}"},
		},
	}

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "ok", result.Status)
	require.Len(t, result.Steps, 2)

	assert.Equal(t, "ok", result.Steps[0].Status)
	assert.Equal(t, "hello-world", result.Steps[0].Output)

	assert.Equal(t, "ok", result.Steps[1].Status)
	assert.Equal(t, "got:hello-world", result.Steps[1].Output)
}

func TestRunWithParam(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "param-pipe",
		Params: map[string]Param{
			"msg": {Default: "default"},
		},
		Steps: []Step{
			{Name: "print", Run: "echo {param.msg}"},
		},
	}

	result, err := Run(context.Background(), p, map[string]string{"msg": "custom"})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Equal(t, "custom", result.Steps[0].Output)
}

func TestRunHaltsOnFailure(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "halt-pipe",
		Steps: []Step{
			{Name: "fail", Run: "exit 1"},
			{Name: "after", Run: "echo should-not-run"},
		},
	}

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)
	require.Len(t, result.Steps, 1) // halted, second step never added
}

// ── DryRun ────────────────────────────────────────────────────────────────────

func TestDryRun(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "dry-pipe",
		Steps: []Step{
			{Name: "step1", Run: "rm -rf /"},
			{Name: "step2", Run: "curl http://example.com"},
		},
	}

	result, err := DryRun(p, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "dry_run", result.Status)
	require.Len(t, result.Steps, 2)

	for _, sr := range result.Steps {
		assert.Equal(t, "skipped", sr.Status)
		assert.Contains(t, sr.Output, "dry run — would run:")
	}

	assert.Contains(t, result.Message, "dry run")
	assert.Contains(t, result.Message, "2 steps")
}

func TestDryRunShowsExpandedCommand(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "expand-pipe",
		Params: map[string]Param{
			"env": {Default: "staging"},
		},
		Steps: []Step{
			{Name: "deploy", Run: "deploy.sh --env {param.env}"},
		},
	}

	result, err := DryRun(p, nil)
	require.NoError(t, err)
	assert.Contains(t, result.Steps[0].Output, "deploy.sh --env staging")
}

// ── StepTimeout ───────────────────────────────────────────────────────────────

func TestStepTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		timeout int
		want    time.Duration
	}{
		{0, 30 * time.Second},
		{-1, 30 * time.Second},
		{10, 10 * time.Second},
		{120, 120 * time.Second},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("timeout=%d", tt.timeout), func(t *testing.T) {
			t.Parallel()
			s := &Step{Timeout: tt.timeout}
			assert.Equal(t, tt.want, s.StepTimeout())
		})
	}
}

// ── JSONExtract ───────────────────────────────────────────────────────────────

func TestJSONExtract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		field string
		want  string
	}{
		{"string field", `{"name":"alice","age":30}`, "name", "alice"},
		{"numeric field", `{"count":42}`, "count", "42"},
		{"nested object", `{"meta":{"k":"v"}}`, "meta", `{"k":"v"}`},
		{"missing field", `{"a":"b"}`, "missing", ""},
		{"invalid json", `not json`, "field", ""},
		{"empty json", `{}`, "field", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := jsonExtract(tt.input, tt.field)
			assert.Equal(t, tt.want, got)
		})
	}
}
