package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writePipelineYAML writes content to <dir>/<name> with 0o600 permissions and
// returns the absolute path. Used by tests that exercise LoadFile/LoadDir
// against ephemeral fixtures under t.TempDir().
func writePipelineYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
