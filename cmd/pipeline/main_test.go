// Tests for cmd/pipeline. Pure package-level helpers (parseParams,
// paramList, printResult) are tested directly. The CLI entry point
// (main + runList + runPipeline) is exercised via the canonical
// TestMain re-exec pattern: when BE_PIPELINE_MAIN=1 is set, the test
// binary acts as the pipeline binary so main() runs under the test
// process and counts toward coverage.
package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/extensions/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	envRunMain = "BE_PIPELINE_MAIN"

	simpleYAML = `name: smoke
steps:
  - name: hello
    run: "echo hi"
`
)

func TestMain(m *testing.M) {
	if os.Getenv(envRunMain) == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

func runPipelineCLI(t *testing.T, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), envRunMain+"=1")
	if dir != "" {
		cmd.Dir = dir
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("runPipelineCLI: unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// captureStdout temporarily redirects os.Stdout, runs fn, and returns
// the captured output. Used for printResult which writes to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	require.NoError(t, w.Close())
	os.Stdout = orig
	return <-done
}

// ---------- direct unit tests for package-level helpers ----------

func TestParamList_StringAndSet(t *testing.T) {
	t.Parallel()
	var p paramList
	assert.Equal(t, "", p.String())

	require.NoError(t, p.Set("foo=bar"))
	require.NoError(t, p.Set("baz=qux"))
	assert.Equal(t, "foo=bar, baz=qux", p.String())
	assert.Len(t, p, 2)
}

func TestParseParams_Cases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []string
		want map[string]string
	}{
		{
			name: "empty input",
			in:   nil,
			want: map[string]string{},
		},
		{
			name: "single kv",
			in:   []string{"name=alice"},
			want: map[string]string{"name": "alice"},
		},
		{
			name: "multiple kv",
			in:   []string{"a=1", "b=2", "c=three"},
			want: map[string]string{"a": "1", "b": "2", "c": "three"},
		},
		{
			name: "ignores entries without equals",
			in:   []string{"bad", "ok=yes"},
			want: map[string]string{"ok": "yes"},
		},
		{
			name: "empty value preserved",
			in:   []string{"k="},
			want: map[string]string{"k": ""},
		},
		{
			name: "value containing equals keeps everything after first",
			in:   []string{"expr=a=b=c"},
			want: map[string]string{"expr": "a=b=c"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseParams(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPrintResult_NonQuiet_ShowsHeaderStepsAndFooter(t *testing.T) {
	// No t.Parallel — captureStdout swaps the global os.Stdout, so the
	// two printResult tests must not run concurrently with each other or
	// with anything else that writes to stdout.
	result := &pipeline.PipelineResult{
		Name:    "my-pipe",
		Status:  pipeline.StatusOK,
		Message: "all good",
		Steps: []pipeline.StepResult{
			{Name: "build", Status: pipeline.StatusOK, Output: "compiled\n", DurationMS: 12},
			{Name: "lint", Status: pipeline.StatusSkipped, DurationMS: 0},
			{Name: "test", Status: pipeline.StatusError, Error: "boom", DurationMS: 7},
		},
		DurationMS: 19,
	}
	out := captureStdout(t, func() { printResult(result, false) })
	assert.Contains(t, out, "Pipeline: my-pipe")
	assert.Contains(t, out, "[+] build")
	assert.Contains(t, out, "    compiled")
	assert.Contains(t, out, "[-] lint")
	assert.Contains(t, out, "[x] test")
	assert.Contains(t, out, "error: boom")
	assert.Contains(t, out, "Result: ok")
	assert.Contains(t, out, "all good")
}

func TestPrintResult_Quiet_OnlyShowsErrors(t *testing.T) {
	// No t.Parallel — see comment on TestPrintResult_NonQuiet_*.
	result := &pipeline.PipelineResult{
		Name:   "p",
		Status: pipeline.StatusError,
		Steps: []pipeline.StepResult{
			{Name: "ok-step", Status: pipeline.StatusOK, Output: "ignored", DurationMS: 1},
			{Name: "bad-step", Status: pipeline.StatusError, Error: "kaboom", DurationMS: 2},
		},
	}
	out := captureStdout(t, func() { printResult(result, true) })
	assert.NotContains(t, out, "Pipeline: p")
	assert.NotContains(t, out, "ok-step")
	assert.NotContains(t, out, "ignored")
	assert.Contains(t, out, "[x] bad-step")
	assert.Contains(t, out, "error: kaboom")
	assert.Contains(t, out, "Result: error")
}

// ---------- CLI behavior tests via re-exec ----------

func TestCLI_NoArgs_PrintsUsage(t *testing.T) {
	t.Parallel()
	_, stderr, code := runPipelineCLI(t, "")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "pipeline [flags] <file.yaml>")
	assert.Contains(t, stderr, "list <directory>")
}

func TestCLI_UnknownFlag_ExitsTwo(t *testing.T) {
	t.Parallel()
	_, stderr, code := runPipelineCLI(t, "", "-no-such-flag")
	assert.Equal(t, 2, code)
	// flag.ContinueOnError writes the parse error to stderr.
	assert.Contains(t, stderr, "flag provided but not defined")
}

func TestCLI_MissingFile_ErrorExitOne(t *testing.T) {
	t.Parallel()
	_, stderr, code := runPipelineCLI(t, "", "/nonexistent/does-not-exist.yaml")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "error:")
}

func TestCLI_List_EmptyDir_PrintsNotice(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, stderr, code := runPipelineCLI(t, "", "list", dir)
	// runList does not os.Exit on empty dir — it just prints the notice.
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "no pipelines found")
}

// writeSimpleYAML writes simpleYAML to a temp file and returns the path.
func writeSimpleYAML(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.yaml")
	require.NoError(t, os.WriteFile(path, []byte(simpleYAML), 0o600))
	return path
}

func TestCLI_DryRun_SimplePipeline_Succeeds(t *testing.T) {
	t.Parallel()
	stdout, _, code := runPipelineCLI(t, "", "-dry-run", writeSimpleYAML(t))
	require.Equal(t, 0, code, "stdout=%q", stdout)
	assert.Contains(t, stdout, "Pipeline: smoke")
	assert.Contains(t, stdout, "hello")
}

func TestCLI_DryRun_JSON_EmitsJSON(t *testing.T) {
	t.Parallel()
	stdout, _, code := runPipelineCLI(t, "", "-dry-run", "-json", writeSimpleYAML(t))
	require.Equal(t, 0, code)
	trimmed := strings.TrimSpace(stdout)
	assert.True(t, strings.HasPrefix(trimmed, "{"), "expected JSON object, got %q", trimmed)
	assert.Contains(t, stdout, `"name"`)
	assert.Contains(t, stdout, `"steps"`)
}

func TestCLI_DryRun_ParamOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "p.yaml")
	yaml := `name: smoke
params:
  greeting:
    default: hello
steps:
  - name: print
    run: "echo {{.greeting}}"
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	// Confirm -param parses; we don't assert on substituted output since
	// templating happens in execute path, just that the run succeeds.
	_, _, code := runPipelineCLI(t, "", "-dry-run", "-param", "greeting=world", path)
	require.Equal(t, 0, code)
}

func TestCLI_List_WithPipelines_PrintsNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yaml := `name: first
description: the first one
steps:
  - name: noop
    run: "true"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(yaml), 0o600))
	yaml2 := `name: second
steps:
  - name: noop
    run: "true"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(yaml2), 0o600))

	stdout, _, code := runPipelineCLI(t, "", "list", dir)
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "first")
	assert.Contains(t, stdout, "the first one")
	assert.Contains(t, stdout, "second")
}
