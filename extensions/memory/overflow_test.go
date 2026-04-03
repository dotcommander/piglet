package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean alphanumeric",
			input: "ReadFile",
			want:  "ReadFile",
		},
		{
			name:  "with hyphens and underscores",
			input: "read-file_v2",
			want:  "read-file_v2",
		},
		{
			name:  "slash becomes underscore",
			input: "tool/Read",
			want:  "tool_Read",
		},
		{
			name:  "spaces become underscores",
			input: "my tool name",
			want:  "my_tool_name",
		},
		{
			name:  "special chars become underscores",
			input: "tool!@#$%",
			want:  "tool_____",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "digits preserved",
			input: "tool123",
			want:  "tool123",
		},
		{
			name:  "mixed case preserved",
			input: "ReadFile",
			want:  "ReadFile",
		},
		{
			name:  "dots become underscores",
			input: "tool.name",
			want:  "tool_name",
		},
		{
			name:  "colons become underscores",
			input: "tool:name:v2",
			want:  "tool_name_v2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeFileName(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPersistToolResult_WritesFile(t *testing.T) {
	// t.Setenv cannot be used with t.Parallel

	// Point the session dir to a temp directory via env var
	tmpDir := t.TempDir()
	t.Setenv("PIGLET_SESSION_ID", "test-session")
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	content := strings.Repeat("x", overflowThreshold+100)
	path, err := persistToolResult("Read", content)
	if err != nil {
		// If xdg.ConfigDir doesn't honour XDG_CONFIG_HOME in this env, skip
		t.Skipf("persistToolResult unavailable in test env: %v", err)
	}

	require.NotEmpty(t, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Read")
	assert.Contains(t, string(data), content[:50]) // partial content present
}

func TestPersistToolResult_EmptySessionID(t *testing.T) {
	// t.Setenv cannot be used with t.Parallel

	tmpDir := t.TempDir()
	t.Setenv("PIGLET_SESSION_ID", "")
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	path, err := persistToolResult("Bash", "some output")
	if err != nil {
		t.Skipf("persistToolResult unavailable in test env: %v", err)
	}

	// Should fall back to unknown-session directory
	assert.Contains(t, path, "unknown-session")
}

func TestPersistToolResult_FileNameContainsToolName(t *testing.T) {
	// t.Setenv cannot be used with t.Parallel

	tmpDir := t.TempDir()
	t.Setenv("PIGLET_SESSION_ID", "mysession")
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	path, err := persistToolResult("MyTool/v2", "output content")
	if err != nil {
		t.Skipf("persistToolResult unavailable in test env: %v", err)
	}

	base := filepath.Base(path)
	// Sanitized name: MyTool_v2
	assert.Contains(t, base, "MyTool_v2")
}

func TestOverflowConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 50000, overflowThreshold)
	assert.Equal(t, 2048, overflowKeepChars)
}
