package admin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFiles(t *testing.T) {
	t.Parallel()

	files := configFiles("/tmp/test")

	require.Len(t, files, 5, "expected 5 config file entries")

	labels := make([]string, len(files))
	for i, f := range files {
		labels[i] = f.label
	}
	assert.Equal(t, []string{"config.yaml", "behavior.md", "auth.json", "models.yaml", "sessions/"}, labels)

	for _, f := range files {
		assert.Equal(t, filepath.Join("/tmp/test", f.label), f.path)
	}
}

func TestFormatFileStatus_Existing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte("key: val"), 0o644))

	got := formatFileStatus(configFile{"config.yaml", p})
	assert.Equal(t, p, got)
}

func TestFormatFileStatus_NotCreated(t *testing.T) {
	t.Parallel()

	got := formatFileStatus(configFile{"config.yaml", "/nonexistent/path/config.yaml"})
	assert.Equal(t, "(not created)", got)
}

func TestFormatFileStatus_SymlinkLoop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "loop.yaml")
	require.NoError(t, os.Symlink(p, p)) // self-referencing symlink

	got := formatFileStatus(configFile{"loop.yaml", p})
	assert.Contains(t, got, "(error:")
	assert.NotContains(t, got, "(not created)")
}

func TestRunSetup_CreatesFiles(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "piglet-config")
	var messages []string
	mock := &stubExt{messages: &messages}

	runSetup(mock, dir)

	assert.Equal(t, 1, len(messages), "expected one status message")
	assert.Contains(t, messages[0], "Created:")

	for _, name := range []string{"config.yaml", "behavior.md"} {
		assert.FileExists(t, filepath.Join(dir, name))
	}
	assert.DirExists(t, filepath.Join(dir, "sessions"))
}

func TestRunSetup_AlreadyExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"config.yaml", "behavior.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sessions"), 0o755))

	var messages []string
	mock := &stubExt{messages: &messages}

	runSetup(mock, dir)

	assert.Equal(t, 1, len(messages))
	assert.Contains(t, messages[0], "already set up")
}

func TestRunSetup_Idempotent(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "piglet-config2")

	var msgs1, msgs2 []string
	runSetup(&stubExt{messages: &msgs1}, dir)
	runSetup(&stubExt{messages: &msgs2}, dir)

	assert.Contains(t, msgs1[0], "Created:")
	assert.Contains(t, msgs2[0], "already set up")
}

// stubExt captures ShowMessage calls for testing.
type stubExt struct {
	messages *[]string
}

func (s *stubExt) ShowMessage(msg string) {
	*s.messages = append(*s.messages, msg)
}
