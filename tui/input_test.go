package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadHistory_MissingFile(t *testing.T) {
	t.Parallel()
	m := &InputModel{}
	m.LoadHistory("/tmp/piglet-test-nonexistent/history")
	assert.Empty(t, m.history)
	assert.Equal(t, 0, m.histIdx)
}

func TestLoadHistory_ValidFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history")
	require.NoError(t, os.WriteFile(path, []byte("hello\nworld\n"), 0o600))

	m := &InputModel{}
	m.LoadHistory(path)
	assert.Equal(t, []string{"hello", "world"}, m.history)
	assert.Equal(t, 2, m.histIdx)
}

func TestLoadHistory_SkipsEmptyLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history")
	require.NoError(t, os.WriteFile(path, []byte("a\n\nb\n\n"), 0o600))

	m := &InputModel{}
	m.LoadHistory(path)
	assert.Equal(t, []string{"a", "b"}, m.history)
}

func TestLoadHistory_CapsAt500(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history")

	var b strings.Builder
	for i := range 600 {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))

	m := &InputModel{}
	m.LoadHistory(path)
	assert.Len(t, m.history, 500)
	assert.Equal(t, 500, m.histIdx)
	assert.Equal(t, "line100", m.history[0])
	assert.Equal(t, "line599", m.history[499])
}

func TestPushHistory_PersistsToDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history")

	m := &InputModel{historyPath: path}
	m.PushHistory("first")
	m.PushHistory("second")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "first\nsecond\n", string(data))
}

func TestPushHistory_NoPathNoWrite(t *testing.T) {
	t.Parallel()
	m := &InputModel{}
	m.PushHistory("hello")
	assert.Equal(t, []string{"hello"}, m.history)
}

func TestHistory_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history")

	m1 := &InputModel{historyPath: path}
	m1.PushHistory("alpha")
	m1.PushHistory("beta")
	m1.PushHistory("gamma")

	m2 := &InputModel{}
	m2.LoadHistory(path)
	assert.Equal(t, m1.history, m2.history)
	assert.Equal(t, 3, m2.histIdx)
}
