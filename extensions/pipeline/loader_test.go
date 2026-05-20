package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
