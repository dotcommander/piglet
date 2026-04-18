package filetools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/extensions/filetools"
	sdk "github.com/dotcommander/piglet/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupExt creates a new SDK extension rooted at a temp directory with filetools registered.
func setupExt(t *testing.T) (*sdk.Extension, string) {
	t.Helper()
	dir := t.TempDir()
	e := sdk.New("filetools-test", "0.0.0")
	e.SetCWD(dir)
	filetools.Register(e)
	return e, dir
}

// execTool finds a registered tool by name and calls its Execute function directly.
func execTool(t *testing.T, e *sdk.Extension, name string, args map[string]any) string {
	t.Helper()
	tool := e.Tool(name)
	require.NotNilf(t, tool, "tool %q not found", name)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	return result.Content[0].Text
}

func TestGrep_FindPattern(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("nothing here"), 0644))

	out := execTool(t, e, "grep", map[string]any{"pattern": "Println"})
	assert.Contains(t, out, "a.go:2:")
	assert.Contains(t, out, "Println")
}

func TestGrep_WithGlob(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("hello world"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello world"), 0644))

	out := execTool(t, e, "grep", map[string]any{"pattern": "hello", "glob": "*.go"})
	assert.Contains(t, out, "a.go")
	assert.NotContains(t, out, "b.txt")
}

func TestGrep_NoMatches(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("nothing"), 0644))

	out := execTool(t, e, "grep", map[string]any{"pattern": "xyz123"})
	assert.Contains(t, out, "no matches")
}

func TestGrep_UTF8Truncation(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	// 201 Japanese Hiragana characters (3 bytes each in UTF-8 = 603 bytes).
	// Grep truncates at 200 runes — output should not corrupt characters.
	longLine := strings.Repeat("あ", 201)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "utf8.txt"), []byte(longLine), 0644))

	out := execTool(t, e, "grep", map[string]any{"pattern": "あ"})
	assert.Contains(t, out, "utf8.txt:1:")
	// Verify no partial byte sequences — each あ should be intact.
	assert.NotContains(t, out, "\ufffd", "output should not contain replacement characters")
}

func TestFind_GlobPattern(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte(""), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "c.go"), []byte(""), 0644))

	out := execTool(t, e, "find", map[string]any{"pattern": "**/*.go"})
	assert.Contains(t, out, "a.go")
	assert.Contains(t, out, "c.go")
	assert.NotContains(t, out, "b.txt")
}

func TestFind_DoubleStarWithPrefix(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "pkg", "util.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "readme.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.go"), []byte(""), 0644))

	out := execTool(t, e, "find", map[string]any{"pattern": "src/**/*.go"})
	assert.Contains(t, out, "main.go")
	assert.Contains(t, out, "util.go")
	assert.NotContains(t, out, "readme.go", "should not match files outside src/")
	assert.NotContains(t, out, "root.go", "should not match files outside src/")
}

func TestFind_SkipsHiddenDirs(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "objects", "abc.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.go"), []byte(""), 0644))

	out := execTool(t, e, "find", map[string]any{"pattern": "**/*.go"})
	assert.Contains(t, out, "real.go")
	assert.NotContains(t, out, "abc.go", "should skip .git directory")
}

func TestFind_NoResults(t *testing.T) {
	t.Parallel()
	e, _ := setupExt(t)

	out := execTool(t, e, "find", map[string]any{"pattern": "**/*.xyz"})
	assert.Contains(t, out, "no files found")
}

func TestLs_Directory(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))

	out := execTool(t, e, "ls", map[string]any{})
	assert.Contains(t, out, "file.txt")
	assert.Contains(t, out, "subdir/")
}

func TestLs_EmptyDir(t *testing.T) {
	t.Parallel()
	e, dir := setupExt(t)

	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.MkdirAll(empty, 0755))

	out := execTool(t, e, "ls", map[string]any{"path": empty})
	assert.Contains(t, out, "empty directory")
}
