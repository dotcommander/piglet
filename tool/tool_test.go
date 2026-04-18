package tool_test

import (
	"context"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/tool"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupApp(t *testing.T) (*ext.App, string) {
	t.Helper()
	dir := t.TempDir()
	app := ext.NewApp(dir)
	tool.RegisterBuiltins(app, tool.BashConfig{}, tool.ToolConfig{})
	return app, dir
}

func execTool(t *testing.T, app *ext.App, name string, args map[string]any) string {
	t.Helper()
	tools := app.CoreTools()
	for _, tool := range tools {
		if tool.Name == name {
			result, err := tool.Execute(context.Background(), "test", args)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotEmpty(t, result.Content)
			return result.Content[0].(core.TextContent).Text
		}
	}
	t.Fatalf("tool %q not found", name)
	return ""
}

func TestRegisterBuiltins(t *testing.T) {
	t.Parallel()
	app, _ := setupApp(t)

	tools := app.Tools()
	assert.Len(t, tools, 4)
	assert.Contains(t, tools, "read")
	assert.Contains(t, tools, "write")
	assert.Contains(t, tools, "edit")
	assert.Contains(t, tools, "bash")
}

func TestRead_BasicFile(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("line1\nline2\nline3"), 0644))

	out := execTool(t, app, "read", map[string]any{"path": path})
	assert.Contains(t, out, "1\tline1")
	assert.Contains(t, out, "2\tline2")
	assert.Contains(t, out, "3\tline3")
}

func TestRead_WithOffset(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	var lines []string
	for i := range 100 {
		lines = append(lines, strings.Repeat("x", i+1))
	}
	path := filepath.Join(dir, "big.txt")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644))

	out := execTool(t, app, "read", map[string]any{"path": path, "offset": float64(50), "limit": float64(5)})
	assert.Contains(t, out, "50\t")
	assert.Contains(t, out, "54\t")
	assert.NotContains(t, out, "55\t")
}

func TestRead_NonExistent(t *testing.T) {
	t.Parallel()
	app, _ := setupApp(t)

	out := execTool(t, app, "read", map[string]any{"path": "/nonexistent/file.txt"})
	assert.Contains(t, out, "error")
}

func TestWrite_CreateFile(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	path := filepath.Join(dir, "new.txt")
	out := execTool(t, app, "write", map[string]any{"path": path, "content": "hello world"})
	assert.Contains(t, out, "wrote 11 bytes")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestWrite_CreatesDir(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	path := filepath.Join(dir, "sub", "deep", "file.txt")
	out := execTool(t, app, "write", map[string]any{"path": path, "content": "nested"})
	assert.Contains(t, out, "wrote")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))
}

func TestEdit_ReplaceText(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	path := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(path, []byte("foo bar baz"), 0644))

	out := execTool(t, app, "edit", map[string]any{
		"path":     path,
		"old_text": "bar",
		"new_text": "qux",
	})
	assert.Contains(t, out, "edited")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "foo qux baz", string(data))
}

func TestEdit_NotFound(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	path := filepath.Join(dir, "edit.txt")
	require.NoError(t, os.WriteFile(path, []byte("foo bar"), 0644))

	out := execTool(t, app, "edit", map[string]any{
		"path":     path,
		"old_text": "xyz",
		"new_text": "abc",
	})
	assert.Contains(t, out, "not found")
}

func TestEdit_MultipleOccurrences(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	path := filepath.Join(dir, "dup.txt")
	require.NoError(t, os.WriteFile(path, []byte("foo foo foo"), 0644))

	out := execTool(t, app, "edit", map[string]any{
		"path":     path,
		"old_text": "foo",
		"new_text": "bar",
	})
	assert.Contains(t, out, "3 times")
}

func TestBash_SimpleCommand(t *testing.T) {
	t.Parallel()
	app, _ := setupApp(t)

	out := execTool(t, app, "bash", map[string]any{"command": "echo hello"})
	assert.Contains(t, out, "hello")
}

func TestBash_ExitCode(t *testing.T) {
	t.Parallel()
	app, _ := setupApp(t)

	out := execTool(t, app, "bash", map[string]any{"command": "exit 42"})
	assert.Contains(t, out, "exit code: 42")
}

func TestBash_Stderr(t *testing.T) {
	t.Parallel()
	app, _ := setupApp(t)

	out := execTool(t, app, "bash", map[string]any{"command": "echo err >&2"})
	assert.Contains(t, out, "STDERR")
	assert.Contains(t, out, "err")
}
