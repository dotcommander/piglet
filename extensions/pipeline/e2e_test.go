package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
