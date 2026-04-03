package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

func writePipelineYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// ── LoadFile ─────────────────────────────────────────────────────────────────

func TestLoadFile(t *testing.T) {
	t.Parallel()

	yaml := `
name: greet
description: A greeting pipeline
concurrency: 8
params:
  target:
    default: world
    description: Who to greet
    required: false
steps:
  - name: say-hello
    run: echo hello {param.target}
    timeout: 10
`
	dir := t.TempDir()
	path := writePipelineYAML(t, dir, "greet.yaml", yaml)

	p, err := LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "greet", p.Name)
	assert.Equal(t, "A greeting pipeline", p.Description)
	assert.Equal(t, 8, p.Concurrency)
	require.Contains(t, p.Params, "target")
	assert.Equal(t, "world", p.Params["target"].Default)
	assert.False(t, p.Params["target"].Required)
	require.Len(t, p.Steps, 1)
	assert.Equal(t, "say-hello", p.Steps[0].Name)
	assert.Equal(t, "echo hello {param.target}", p.Steps[0].Run)
	assert.Equal(t, 10, p.Steps[0].Timeout)
}

func TestLoadFileDefaults(t *testing.T) {
	t.Parallel()

	yaml := `
name: minimal
steps:
  - name: step1
    run: echo hi
`
	dir := t.TempDir()
	path := writePipelineYAML(t, dir, "minimal.yaml", yaml)

	p, err := LoadFile(path)
	require.NoError(t, err)

	// defaults() should be applied
	assert.Equal(t, 4, p.Concurrency)
	assert.Equal(t, "sh", p.Steps[0].Shell)
	assert.Equal(t, 30, p.Steps[0].Timeout)
}

func TestLoadFileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadFile("/nonexistent/pipeline.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read pipeline")
}

func TestLoadFileInvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: [invalid yaml: {"), 0o600))

	_, err := LoadFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse pipeline")
}

// ── Validate ─────────────────────────────────────────────────────────────────

func TestValidate(t *testing.T) {
	t.Parallel()

	validStep := Step{Name: "s1", Run: "echo ok"}

	tests := []struct {
		name    string
		pipe    Pipeline
		params  map[string]string
		wantErr string
	}{
		{
			name:    "missing name",
			pipe:    Pipeline{Steps: []Step{validStep}},
			params:  nil,
			wantErr: "pipeline name is required",
		},
		{
			name:    "no steps",
			pipe:    Pipeline{Name: "p"},
			params:  nil,
			wantErr: `pipeline "p" has no steps`,
		},
		{
			name: "step missing name",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{{Run: "echo hi"}},
			},
			params:  nil,
			wantErr: "step 0 has no name",
		},
		{
			name: "step missing run",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{{Name: "s1"}},
			},
			params:  nil,
			wantErr: `step "s1" has no run command`,
		},
		{
			name: "required param missing",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{validStep},
				Params: map[string]Param{
					"env": {Required: true},
				},
			},
			params:  map[string]string{},
			wantErr: `required parameter "env" not provided`,
		},
		{
			name: "required param has default — ok",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{validStep},
				Params: map[string]Param{
					"env": {Required: true, Default: "dev"},
				},
			},
			params:  map[string]string{},
			wantErr: "",
		},
		{
			name: "required param supplied — ok",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{validStep},
				Params: map[string]Param{
					"env": {Required: true},
				},
			},
			params:  map[string]string{"env": "prod"},
			wantErr: "",
		},
		{
			name: "happy path",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{validStep},
			},
			params:  nil,
			wantErr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.pipe.Validate(tc.params)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ── MergeParams ───────────────────────────────────────────────────────────────

func TestMergeParams(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Params: map[string]Param{
			"host": {Default: "localhost"},
			"port": {Default: "5432"},
			"db":   {},
		},
	}

	t.Run("defaults only", func(t *testing.T) {
		t.Parallel()
		got := p.MergeParams(nil)
		assert.Equal(t, "localhost", got["host"])
		assert.Equal(t, "5432", got["port"])
		assert.NotContains(t, got, "db") // empty default not included
	})

	t.Run("overrides only", func(t *testing.T) {
		t.Parallel()
		got := p.MergeParams(map[string]string{"host": "prod-db", "extra": "yes"})
		assert.Equal(t, "prod-db", got["host"])
		assert.Equal(t, "5432", got["port"])
		assert.Equal(t, "yes", got["extra"]) // extra key from override
	})

	t.Run("override wins over default", func(t *testing.T) {
		t.Parallel()
		got := p.MergeParams(map[string]string{"port": "9999"})
		assert.Equal(t, "localhost", got["host"])
		assert.Equal(t, "9999", got["port"])
	})
}

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

// ── ExpandIterations ──────────────────────────────────────────────────────────

func TestExpandIterations(t *testing.T) {
	t.Parallel()

	t.Run("no loops returns nil", func(t *testing.T) {
		t.Parallel()
		step := &Step{Name: "s", Run: "echo"}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		assert.Nil(t, iters)
	})

	t.Run("each only", func(t *testing.T) {
		t.Parallel()
		step := &Step{
			Name: "s",
			Run:  "echo {item}",
			Each: []string{"a", "b", "c"},
		}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		require.Len(t, iters, 3)
		assert.Equal(t, "a", iters[0].Item)
		assert.Equal(t, "b", iters[1].Item)
		assert.Equal(t, "c", iters[2].Item)
		for _, it := range iters {
			assert.Nil(t, it.LoopVars)
		}
	})

	t.Run("loop numeric range only", func(t *testing.T) {
		t.Parallel()
		step := &Step{
			Name: "s",
			Run:  "echo {loop.n}",
			Loop: map[string]any{"n": "1..4"},
		}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		require.Len(t, iters, 4)
		for _, it := range iters {
			assert.Empty(t, it.Item)
			assert.Contains(t, it.LoopVars, "n")
		}
		vals := make([]string, len(iters))
		for i, it := range iters {
			vals[i] = it.LoopVars["n"]
		}
		assert.Equal(t, []string{"1", "2", "3", "4"}, vals)
	})

	t.Run("both each and loop — cartesian product", func(t *testing.T) {
		t.Parallel()
		step := &Step{
			Name: "s",
			Run:  "echo {item}/{loop.n}",
			Each: []string{"x", "y"},
			Loop: map[string]any{"n": "1..2"},
		}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		// 2 items × 2 loop values = 4
		assert.Len(t, iters, 4)
	})

	t.Run("explicit list loop", func(t *testing.T) {
		t.Parallel()
		step := &Step{
			Name: "s",
			Run:  "echo {loop.env}",
			Loop: map[string]any{"env": []any{"dev", "staging", "prod"}},
		}
		iters, err := ExpandIterations(step)
		require.NoError(t, err)
		require.Len(t, iters, 3)
		vals := []string{
			iters[0].LoopVars["env"],
			iters[1].LoopVars["env"],
			iters[2].LoopVars["env"],
		}
		assert.Equal(t, []string{"dev", "staging", "prod"}, vals)
	})
}

// ── ParseRange ────────────────────────────────────────────────────────────────

func TestParseRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "ascending 1..5",
			input: "1..5",
			want:  []string{"1", "2", "3", "4", "5"},
		},
		{
			name:  "negative range -3..3",
			input: "-3..3",
			want:  []string{"-3", "-2", "-1", "0", "1", "2", "3"},
		},
		{
			name:  "reverse 5..1",
			input: "5..1",
			want:  []string{"5", "4", "3", "2", "1"},
		},
		{
			name:  "single value no dots",
			input: "hello",
			want:  []string{"hello"},
		},
		{
			name:  "zero range 0..0",
			input: "0..0",
			want:  []string{"0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseRange(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ── ExpandDayRange ────────────────────────────────────────────────────────────

func TestExpandDayRange(t *testing.T) {
	t.Parallel()

	t.Run("7 days back to yesterday", func(t *testing.T) {
		t.Parallel()
		got := expandDayRange(-7, -1, time.Time{})
		assert.Len(t, got, 7)
		// all values should be valid dates
		for _, d := range got {
			_, err := time.Parse("2006-01-02", d)
			assert.NoError(t, err, "expected YYYY-MM-DD, got %q", d)
		}
		// first date should be 7 days ago, last should be yesterday
		now := time.Now()
		assert.Equal(t, now.AddDate(0, 0, -7).Format("2006-01-02"), got[0])
		assert.Equal(t, now.AddDate(0, 0, -1).Format("2006-01-02"), got[6])
	})

	t.Run("single day", func(t *testing.T) {
		t.Parallel()
		got := expandDayRange(0, 0, time.Time{})
		assert.Len(t, got, 1)
		assert.Equal(t, time.Now().Format("2006-01-02"), got[0])
	})

	t.Run("swapped start end normalised", func(t *testing.T) {
		t.Parallel()
		// expandDayRange swaps if startDays > endDays
		got := expandDayRange(-1, -7, time.Time{})
		assert.Len(t, got, 7)
	})
}

func TestParseRangeDayRange(t *testing.T) {
	t.Parallel()

	got, err := parseRange("-7d..-1d")
	require.NoError(t, err)
	assert.Len(t, got, 7)
	for _, d := range got {
		_, parseErr := time.Parse("2006-01-02", d)
		assert.NoError(t, parseErr)
	}
}

// ── CartesianLoop ─────────────────────────────────────────────────────────────

func TestCartesianLoop(t *testing.T) {
	t.Parallel()

	t.Run("empty dims returns single empty map", func(t *testing.T) {
		t.Parallel()
		got := cartesianLoop(nil)
		require.Len(t, got, 1)
		assert.Empty(t, got[0])
	})

	t.Run("2 dimensions", func(t *testing.T) {
		t.Parallel()
		dims := []loopDimension{
			{key: "color", values: []string{"red", "blue"}},
			{key: "size", values: []string{"S", "M", "L"}},
		}
		got := cartesianLoop(dims)
		// 2 × 3 = 6 combinations
		assert.Len(t, got, 6)

		// Every combination must have both keys
		for _, combo := range got {
			assert.Contains(t, combo, "color")
			assert.Contains(t, combo, "size")
		}

		// Verify all color×size pairs are represented
		seen := make(map[string]bool)
		for _, combo := range got {
			key := fmt.Sprintf("%s/%s", combo["color"], combo["size"])
			seen[key] = true
		}
		assert.True(t, seen["red/S"])
		assert.True(t, seen["red/M"])
		assert.True(t, seen["red/L"])
		assert.True(t, seen["blue/S"])
		assert.True(t, seen["blue/M"])
		assert.True(t, seen["blue/L"])
	})

	t.Run("single dimension", func(t *testing.T) {
		t.Parallel()
		dims := []loopDimension{
			{key: "n", values: []string{"1", "2"}},
		}
		got := cartesianLoop(dims)
		assert.Len(t, got, 2)
	})
}

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

// ── Retries ───────────────────────────────────────────────────────────────────

func TestRetryOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	counter := filepath.Join(dir, "count.txt")

	// Script: increment a counter file; succeed on attempt >= 2
	script := fmt.Sprintf(`
count=$(cat %s 2>/dev/null || echo 0)
count=$((count + 1))
echo $count > %s
if [ $count -lt 2 ]; then
  exit 1
fi
echo "succeeded on attempt $count"
`, counter, counter)

	p := &Pipeline{
		Name: "retry-pipe",
		Steps: []Step{
			{
				Name:    "flaky",
				Run:     script,
				Retries: 2,
				// RetryDelay intentionally omitted: defaults() sets it to 5s
				// but step succeeds on attempt 2, so only one 5s wait occurs.
				Timeout: 10,
			},
		},
	}

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)

	sr := result.Steps[0]
	assert.Equal(t, "ok", sr.Status)
	assert.GreaterOrEqual(t, sr.RetryCount, 1)
	assert.Contains(t, sr.Output, "succeeded")
}

func TestRetryExhausted(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "exhaust-pipe",
		Steps: []Step{
			{
				Name:    "always-fails",
				Run:     "exit 1",
				Retries: 2,
				// RetryDelay omitted: defaults() sets 5s; 2 retries = ~10s total.
				Timeout: 30,
			},
		},
	}

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err)
	assert.Equal(t, "error", result.Status)

	sr := result.Steps[0]
	assert.Equal(t, "error", sr.Status)
	assert.Equal(t, 2, sr.RetryCount)
}

// ── AllowFailure ──────────────────────────────────────────────────────────────

func TestAllowFailure(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "allow-fail-pipe",
		Steps: []Step{
			{Name: "may-fail", Run: "exit 1", AllowFailure: true},
			{Name: "continues", Run: "echo after-failure"},
		},
	}

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err)

	// Pipeline continues but status is "partial"
	assert.Equal(t, "partial", result.Status)
	require.Len(t, result.Steps, 2)
	assert.Equal(t, "error", result.Steps[0].Status)
	assert.Equal(t, "ok", result.Steps[1].Status)
	assert.Equal(t, "after-failure", result.Steps[1].Output)
}

// ── When predicate ────────────────────────────────────────────────────────────

func TestWhenPredicate(t *testing.T) {
	t.Parallel()

	t.Run("predicate passes — step runs", func(t *testing.T) {
		t.Parallel()
		p := &Pipeline{
			Name: "when-pass",
			Steps: []Step{
				{Name: "conditional", Run: "echo ran", When: "true"},
			},
		}
		result, err := Run(context.Background(), p, nil)
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Status)
		assert.Equal(t, "ok", result.Steps[0].Status)
		assert.Equal(t, "ran", result.Steps[0].Output)
	})

	t.Run("predicate fails — step skipped", func(t *testing.T) {
		t.Parallel()
		p := &Pipeline{
			Name: "when-fail",
			Steps: []Step{
				{Name: "conditional", Run: "echo ran", When: "false"},
				{Name: "after", Run: "echo after"},
			},
		}
		result, err := Run(context.Background(), p, nil)
		require.NoError(t, err)
		// overall pipeline still ok (skipped != error)
		assert.Equal(t, "ok", result.Status)
		require.Len(t, result.Steps, 2)
		assert.Equal(t, "skipped", result.Steps[0].Status)
		assert.Contains(t, result.Steps[0].Output, "when predicate failed")
		assert.Equal(t, "ok", result.Steps[1].Status)
	})

	t.Run("predicate uses param", func(t *testing.T) {
		t.Parallel()
		p := &Pipeline{
			Name: "when-param",
			Params: map[string]Param{
				"skip": {Default: "false"},
			},
			Steps: []Step{
				{
					Name: "guarded",
					Run:  "echo guarded-ran",
					When: `[ "{param.skip}" = "false" ]`,
				},
			},
		}
		// param.skip = "false" → predicate [ "false" = "false" ] → true → step runs
		result, err := Run(context.Background(), p, map[string]string{"skip": "false"})
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Steps[0].Status)

		// param.skip = "true" → predicate [ "true" = "false" ] → false → step skipped
		result2, err := Run(context.Background(), p, map[string]string{"skip": "true"})
		require.NoError(t, err)
		assert.Equal(t, "skipped", result2.Steps[0].Status)
	})
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

// ── LoadDir ───────────────────────────────────────────────────────────────────

func TestLoadDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	validYAML := `
name: pipe-%d
steps:
  - name: s
    run: echo ok
`
	// Write two valid yamls and one non-yaml file
	for i := range 2 {
		writePipelineYAML(t, dir, fmt.Sprintf("p%d.yaml", i), fmt.Sprintf(validYAML, i))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o600))

	pipes, err := LoadDir(dir)
	require.NoError(t, err)
	assert.Len(t, pipes, 2)
}

func TestLoadDirNonExistent(t *testing.T) {
	t.Parallel()

	pipes, err := LoadDir("/nonexistent/path")
	require.NoError(t, err) // non-existent dir returns nil, nil
	assert.Nil(t, pipes)
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

// ── Edge cases ────────────────────────────────────────────────────────────────

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

// ── E2E Integration ──────────────────────────────────────────────────────────

func TestE2EPipeline(t *testing.T) {
	t.Parallel()

	yamlContent := `
name: e2e-test
description: End-to-end test pipeline exercising all features
params:
  greeting:
    default: "world"
    description: Who to greet

steps:
  - name: hello
    run: echo "Hello, {param.greeting}!"
    description: Basic param substitution

  - name: use-prev
    run: echo "Previous said {prev.stdout}"
    description: Output passing via prev.stdout

  - name: json-output
    run: echo '{"status":"ok","count":3}'
    description: Produce JSON for next step

  - name: json-extract
    run: echo "count is {prev.json.count}"
    description: Extract JSON field from prev

  - name: each-loop
    each:
      - alpha
      - beta
      - gamma
    run: echo "item={item}"
    description: Each iteration

  - name: numeric-loop
    loop:
      n: "1..3"
    run: echo "n={loop.n}"
    description: Numeric range loop

  - name: when-true
    when: "true"
    run: echo "when predicate passed"
    description: Step with passing when predicate

  - name: when-false
    when: "false"
    run: echo "this should not run"
    description: Step with failing when predicate (should skip)

  - name: allow-fail
    run: exit 1
    allow_failure: true
    description: Failing step that should not halt pipeline

  - name: after-fail
    run: echo "pipeline continued after failure"
    description: Proves pipeline continues after allow_failure

  - name: use-named-step
    run: echo "hello step said {step.hello.stdout}"
    description: Reference a named earlier step
`

	dir := t.TempDir()
	path := writePipelineYAML(t, dir, "e2e-test.yaml", yamlContent)

	p, err := LoadFile(path)
	require.NoError(t, err, "load e2e pipeline")
	require.Equal(t, "e2e-test", p.Name)
	require.Len(t, p.Steps, 11)

	result, err := Run(context.Background(), p, nil)
	require.NoError(t, err, "run e2e pipeline")
	require.NotNil(t, result)
	require.Len(t, result.Steps, 11)

	// Step 0: hello — param substitution
	assert.Equal(t, "ok", result.Steps[0].Status, "hello status")
	assert.Contains(t, result.Steps[0].Output, "Hello, world!", "hello output")

	// Step 1: use-prev — output passing
	assert.Equal(t, "ok", result.Steps[1].Status, "use-prev status")
	assert.Contains(t, result.Steps[1].Output, "Hello, world!", "use-prev sees prev.stdout")

	// Step 2: json-output — produces JSON
	assert.Equal(t, "ok", result.Steps[2].Status, "json-output status")
	assert.Contains(t, result.Steps[2].Output, `"status":"ok"`, "json-output content")

	// Step 3: json-extract — {prev.json.count}
	assert.Equal(t, "ok", result.Steps[3].Status, "json-extract status")
	assert.Contains(t, result.Steps[3].Output, "count is 3", "json-extract value")

	// Step 4: each-loop — 3 iterations (alpha, beta, gamma)
	assert.Equal(t, "ok", result.Steps[4].Status, "each-loop status")
	assert.Equal(t, 3, result.Steps[4].Iterations, "each-loop iterations")
	assert.Contains(t, result.Steps[4].Output, "item=alpha")
	assert.Contains(t, result.Steps[4].Output, "item=beta")
	assert.Contains(t, result.Steps[4].Output, "item=gamma")

	// Step 5: numeric-loop — 3 iterations (1, 2, 3)
	assert.Equal(t, "ok", result.Steps[5].Status, "numeric-loop status")
	assert.Equal(t, 3, result.Steps[5].Iterations, "numeric-loop iterations")
	assert.Contains(t, result.Steps[5].Output, "n=1")
	assert.Contains(t, result.Steps[5].Output, "n=2")
	assert.Contains(t, result.Steps[5].Output, "n=3")

	// Step 6: when-true — predicate passes, step runs
	assert.Equal(t, "ok", result.Steps[6].Status, "when-true status")
	assert.Contains(t, result.Steps[6].Output, "when predicate passed")

	// Step 7: when-false — predicate fails, step skipped
	assert.Equal(t, "skipped", result.Steps[7].Status, "when-false status")
	assert.Contains(t, result.Steps[7].Output, "when predicate failed")

	// Step 8: allow-fail — exits 1 but pipeline continues
	assert.Equal(t, "error", result.Steps[8].Status, "allow-fail status")

	// Step 9: after-fail — proves pipeline continued
	assert.Equal(t, "ok", result.Steps[9].Status, "after-fail status")
	assert.Contains(t, result.Steps[9].Output, "pipeline continued after failure")

	// Step 10: use-named-step — references {step.hello.stdout}
	assert.Equal(t, "ok", result.Steps[10].Status, "use-named-step status")
	assert.Contains(t, result.Steps[10].Output, "Hello, world!", "use-named-step sees hello output")

	// Overall: partial because allow-fail has an error
	assert.Equal(t, "partial", result.Status, "overall pipeline status")
	t.Logf("E2E result: %s — %s (%dms)", result.Status, result.Message, result.DurationMS)
}

// ── E2E with custom params ───────────────────────────────────────────────────

func TestE2EPipelineCustomParam(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Name: "e2e-param",
		Params: map[string]Param{
			"greeting": {Default: "world"},
		},
		Steps: []Step{
			{Name: "hello", Run: `echo "Hello, {param.greeting}!"`},
			{Name: "check", Run: `echo "prev={prev.stdout}"`},
		},
	}

	result, err := Run(context.Background(), p, map[string]string{"greeting": "Gary"})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Contains(t, result.Steps[0].Output, "Hello, Gary!")
	assert.Contains(t, result.Steps[1].Output, "Hello, Gary!")
}

func TestRunValidationError(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		// missing name — Validate will fail
		Steps: []Step{{Name: "s", Run: "echo ok"}},
	}

	_, err := Run(context.Background(), p, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline name is required")
}
