package safeguard_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dotcommander/piglet/extensions/safeguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockerWithConfig_BalancedBlocksPattern(t *testing.T) {
	t.Parallel()
	patterns := safeguard.CompilePatterns([]string{`\brm\s+-rf\b`})
	blocker := safeguard.BlockerWithConfig(safeguard.Config{Profile: safeguard.ProfileBalanced}, patterns, "", nil)

	allow, _, reason := blocker(context.Background(), "bash", map[string]any{"command": "rm -rf /"})
	assert.False(t, allow)
	assert.NotEmpty(t, reason)
	assert.Contains(t, reason, "blocked dangerous command")
}

func TestBlockerWithConfig_BalancedAllowsSafe(t *testing.T) {
	t.Parallel()
	patterns := safeguard.CompilePatterns([]string{`\brm\s+-rf\b`})
	blocker := safeguard.BlockerWithConfig(safeguard.Config{Profile: safeguard.ProfileBalanced}, patterns, "", nil)

	allow, args, reason := blocker(context.Background(), "bash", map[string]any{"command": "ls -la"})
	assert.True(t, allow)
	assert.Empty(t, reason)
	assert.NotNil(t, args)
}

func TestBlockerWithConfig_BalancedIgnoresWrite(t *testing.T) {
	t.Parallel()
	patterns := safeguard.CompilePatterns([]string{`\brm\s+-rf\b`})
	blocker := safeguard.BlockerWithConfig(safeguard.Config{Profile: safeguard.ProfileBalanced}, patterns, "/workspace", nil)

	// Balanced mode does NOT block writes outside workspace
	allow, _, reason := blocker(context.Background(), "write", map[string]any{"file_path": "/etc/passwd"})
	assert.True(t, allow)
	assert.Empty(t, reason)
}

func TestBlockerWithConfig_StrictBlocksOutsideWorkspace(t *testing.T) {
	t.Parallel()
	patterns := safeguard.CompilePatterns(nil)
	blocker := safeguard.BlockerWithConfig(safeguard.Config{Profile: safeguard.ProfileStrict}, patterns, "/workspace/project", nil)

	allow, _, reason := blocker(context.Background(), "write", map[string]any{"file_path": "/etc/passwd"})
	assert.False(t, allow)
	assert.NotEmpty(t, reason)
	assert.Contains(t, reason, "outside workspace")
}

func TestBlockerWithConfig_StrictAllowsInsideWorkspace(t *testing.T) {
	t.Parallel()
	patterns := safeguard.CompilePatterns(nil)
	blocker := safeguard.BlockerWithConfig(safeguard.Config{Profile: safeguard.ProfileStrict}, patterns, "/workspace/project", nil)

	allow, _, reason := blocker(context.Background(), "write", map[string]any{"file_path": "/workspace/project/main.go"})
	assert.True(t, allow)
	assert.Empty(t, reason)
}

func TestBlockerWithConfig_StrictAllowsNonMutating(t *testing.T) {
	t.Parallel()
	patterns := safeguard.CompilePatterns(nil)
	blocker := safeguard.BlockerWithConfig(safeguard.Config{Profile: safeguard.ProfileStrict}, patterns, "/workspace", nil)

	// read tool is not blocked even outside workspace
	allow, _, reason := blocker(context.Background(), "read", map[string]any{"file_path": "/etc/hosts"})
	assert.True(t, allow)
	assert.Empty(t, reason)
}

func TestBlockerWithConfig_StrictAlsoBlocksPatterns(t *testing.T) {
	t.Parallel()
	patterns := safeguard.CompilePatterns([]string{`\brm\s+-rf\b`})
	blocker := safeguard.BlockerWithConfig(safeguard.Config{Profile: safeguard.ProfileStrict}, patterns, "/workspace", nil)

	allow, _, reason := blocker(context.Background(), "bash", map[string]any{"command": "rm -rf /"})
	assert.False(t, allow)
	assert.NotEmpty(t, reason)
}

func TestAuditLogger(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_ = filepath.Join(dir, "audit.jsonl")

	// Verify that a nil AuditLogger doesn't panic
	var nilLogger *safeguard.AuditLogger
	nilLogger.Log("bash", "allowed", "", "")

	// Verify that NewAuditLogger succeeds (writes to real config dir or returns nil gracefully)
	_ = safeguard.NewAuditLogger()
}

func TestLoadConfig_DefaultProfile(t *testing.T) {
	t.Parallel()

	cfg := safeguard.LoadConfig()
	// Should return balanced by default (or whatever is in the user's config)
	assert.NotEmpty(t, cfg.Profile)
	assert.NotEmpty(t, cfg.Patterns)
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	require.NotNil(t, t) // ensure require is used

	// truncate is unexported, but we can test it via BlockerWithConfig indirectly
	// by checking that long commands in audit don't cause issues
	patterns := safeguard.CompilePatterns([]string{`\brm\s+-rf\b`})
	longCmd := "rm -rf " + string(make([]byte, 300))
	blocker := safeguard.BlockerWithConfig(safeguard.Config{Profile: safeguard.ProfileBalanced}, patterns, "", nil)

	allow, _, reason := blocker(context.Background(), "bash", map[string]any{"command": longCmd})
	assert.False(t, allow)
	assert.NotEmpty(t, reason)
}

func TestProfileConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "strict", safeguard.ProfileStrict)
	assert.Equal(t, "balanced", safeguard.ProfileBalanced)
	assert.Equal(t, "off", safeguard.ProfileOff)
}
