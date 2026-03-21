package tool_test

import (
	"context"
	"os"
	"path/filepath"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/tool"
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
	assert.Len(t, tools, 7)
	assert.Contains(t, tools, "read")
	assert.Contains(t, tools, "write")
	assert.Contains(t, tools, "edit")
	assert.Contains(t, tools, "bash")
	assert.Contains(t, tools, "grep")
	assert.Contains(t, tools, "find")
	assert.Contains(t, tools, "ls")
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

func TestGrep_FindPattern(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("nothing here"), 0644))

	out := execTool(t, app, "grep", map[string]any{"pattern": "Println"})
	assert.Contains(t, out, "a.go:2:")
	assert.Contains(t, out, "Println")
}

func TestGrep_WithGlob(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("hello world"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello world"), 0644))

	out := execTool(t, app, "grep", map[string]any{"pattern": "hello", "glob": "*.go"})
	assert.Contains(t, out, "a.go")
	assert.NotContains(t, out, "b.txt")
}

func TestGrep_NoMatches(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("nothing"), 0644))

	out := execTool(t, app, "grep", map[string]any{"pattern": "xyz123"})
	assert.Contains(t, out, "no matches")
}

func TestFind_GlobPattern(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte(""), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "c.go"), []byte(""), 0644))

	out := execTool(t, app, "find", map[string]any{"pattern": "**/*.go"})
	assert.Contains(t, out, "a.go")
	assert.Contains(t, out, "c.go")
	assert.NotContains(t, out, "b.txt")
}

func TestFind_DoubleStarWithPrefix(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "pkg", "util.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "readme.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.go"), []byte(""), 0644))

	out := execTool(t, app, "find", map[string]any{"pattern": "src/**/*.go"})
	assert.Contains(t, out, "main.go")
	assert.Contains(t, out, "util.go")
	assert.NotContains(t, out, "readme.go", "should not match files outside src/")
	assert.NotContains(t, out, "root.go", "should not match files outside src/")
}

func TestFind_SkipsHiddenDirs(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "objects", "abc.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.go"), []byte(""), 0644))

	out := execTool(t, app, "find", map[string]any{"pattern": "**/*.go"})
	assert.Contains(t, out, "real.go")
	assert.NotContains(t, out, "abc.go", "should skip .git directory")
}

func TestFind_NoResults(t *testing.T) {
	t.Parallel()
	app, _ := setupApp(t)

	out := execTool(t, app, "find", map[string]any{"pattern": "**/*.xyz"})
	assert.Contains(t, out, "no files found")
}

func TestLs_Directory(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))

	out := execTool(t, app, "ls", map[string]any{})
	assert.Contains(t, out, "file.txt")
	assert.Contains(t, out, "subdir/")
}

func TestLs_EmptyDir(t *testing.T) {
	t.Parallel()
	app, dir := setupApp(t)

	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.MkdirAll(empty, 0755))

	out := execTool(t, app, "ls", map[string]any{"path": empty})
	assert.Contains(t, out, "empty directory")
}
