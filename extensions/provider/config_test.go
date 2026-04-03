package provider

import (
	"context"
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeHeaders_NilBase(t *testing.T) {
	t.Parallel()
	got := mergeHeaders(nil, map[string]string{"X-Key": "val"})
	assert.Equal(t, map[string]string{"X-Key": "val"}, got)
}

func TestMergeHeaders_NilExtra(t *testing.T) {
	t.Parallel()
	base := map[string]string{"X-Key": "val"}
	got := mergeHeaders(base, nil)
	assert.Equal(t, base, got)
}

func TestMergeHeaders_Merge(t *testing.T) {
	t.Parallel()
	base := map[string]string{"A": "1"}
	extra := map[string]string{"B": "2"}
	got := mergeHeaders(base, extra)
	assert.Equal(t, map[string]string{"A": "1", "B": "2"}, got)
}

func TestMergeHeaders_ExtraOverrides(t *testing.T) {
	t.Parallel()
	base := map[string]string{"A": "old"}
	extra := map[string]string{"A": "new"}
	got := mergeHeaders(base, extra)
	assert.Equal(t, "new", got["A"])
}

func TestMatchCompat_NoMatch(t *testing.T) {
	t.Parallel()
	compat := map[string]compatOverride{
		"gpt-": {StripToolChoice: true},
	}
	_, found := matchCompat(compat, "claude-3")
	assert.False(t, found)
}

func TestMatchCompat_SingleMatch(t *testing.T) {
	t.Parallel()
	compat := map[string]compatOverride{
		"deepseek-": {StripToolChoice: true},
	}
	co, found := matchCompat(compat, "deepseek-coder-v2")
	assert.True(t, found)
	assert.True(t, co.StripToolChoice)
}

func TestMatchCompat_LongestPrefixWins(t *testing.T) {
	t.Parallel()
	compat := map[string]compatOverride{
		"gpt-":   {Headers: map[string]string{"X": "short"}},
		"gpt-4o": {Headers: map[string]string{"X": "long"}},
	}
	co, found := matchCompat(compat, "gpt-4o-mini")
	assert.True(t, found)
	assert.Equal(t, "long", co.Headers["X"])
}

func TestMatchCompat_EmptyMap(t *testing.T) {
	t.Parallel()
	_, found := matchCompat(nil, "anything")
	assert.False(t, found)
}

func TestApplyOverrides_ProviderProxy(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Overrides: map[string]providerOverride{
			"anthropic": {
				BaseURL: "https://proxy.corp.com/anthropic",
				Headers: map[string]string{"X-Corp": "token"},
			},
		},
	}
	model := core.Model{
		ID:       "claude-3-opus",
		Provider: "anthropic",
		API:      core.APIAnthropic,
	}

	applyOverrides(cfg, &model)

	assert.Equal(t, "https://proxy.corp.com/anthropic", model.BaseURL)
	assert.Equal(t, "token", model.Headers["X-Corp"])
}

func TestApplyOverrides_NoMatchNoChange(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Overrides: map[string]providerOverride{
			"anthropic": {BaseURL: "https://proxy.corp.com"},
		},
	}
	model := core.Model{
		ID:       "gpt-4o",
		Provider: "openai",
		API:      core.APIOpenAI,
		BaseURL:  "https://api.openai.com/v1",
	}
	original := model.BaseURL

	applyOverrides(cfg, &model)

	assert.Equal(t, original, model.BaseURL)
}

func TestApplyOverrides_CustomProvider(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Custom: map[string]customProvider{
			"openrouter": {
				API:     "openai",
				BaseURL: "https://openrouter.ai/api/v1",
				Headers: map[string]string{"HTTP-Referer": "https://piglet.dev"},
			},
		},
	}
	model := core.Model{
		ID:       "anthropic/claude-3-opus",
		Provider: "openrouter",
	}

	applyOverrides(cfg, &model)

	assert.Equal(t, core.APIOpenAI, model.API)
	assert.Equal(t, "https://openrouter.ai/api/v1", model.BaseURL)
	assert.Equal(t, "https://piglet.dev", model.Headers["HTTP-Referer"])
}

func TestApplyOverrides_CustomProviderDoesNotOverwriteExistingBaseURL(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Custom: map[string]customProvider{
			"azure": {
				API:     "openai",
				BaseURL: "https://default.azure.com",
			},
		},
	}
	model := core.Model{
		ID:       "gpt-4o",
		Provider: "azure",
		BaseURL:  "https://specific.azure.com", // model-level takes precedence
	}

	applyOverrides(cfg, &model)

	assert.Equal(t, "https://specific.azure.com", model.BaseURL)
}

func TestApplyOverrides_CompatHeaders(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Compat: map[string]compatOverride{
			"deepseek-": {
				Headers: map[string]string{"X-Custom": "ds"},
			},
		},
	}
	model := core.Model{
		ID:       "deepseek-coder-v2",
		Provider: "openai",
		API:      core.APIOpenAI,
	}

	applyOverrides(cfg, &model)

	assert.Equal(t, "ds", model.Headers["X-Custom"])
}

func TestApplyOverrides_AllThreeLayers(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Overrides: map[string]providerOverride{
			"openai": {
				BaseURL: "https://proxy.corp.com/openai",
				Headers: map[string]string{"X-Corp": "token"},
			},
		},
		Compat: map[string]compatOverride{
			"gpt-4o": {
				Headers: map[string]string{"X-Compat": "yes"},
			},
		},
	}
	model := core.Model{
		ID:       "gpt-4o-mini",
		Provider: "openai",
		API:      core.APIOpenAI,
	}

	applyOverrides(cfg, &model)

	assert.Equal(t, "https://proxy.corp.com/openai", model.BaseURL)
	assert.Equal(t, "token", model.Headers["X-Corp"])
	assert.Equal(t, "yes", model.Headers["X-Compat"])
}

func TestMatchCompat_ExactIDBeatsPrefix(t *testing.T) {
	t.Parallel()
	compat := map[string]compatOverride{
		"deepseek-":         {Headers: map[string]string{"X": "prefix"}},
		"deepseek-coder-v2": {Headers: map[string]string{"X": "exact"}},
	}
	co, found := matchCompat(compat, "deepseek-coder-v2")
	assert.True(t, found)
	assert.Equal(t, "exact", co.Headers["X"])
}

func TestApplyOverrides_CompatNameOverride(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Compat: map[string]compatOverride{
			"deepseek-coder-v2": {Name: "DeepSeek Coder v2 (Custom)"},
		},
	}
	model := core.Model{
		ID:       "deepseek-coder-v2",
		Provider: "openai",
		Name:     "deepseek-coder-v2",
	}

	applyOverrides(cfg, &model)

	assert.Equal(t, "DeepSeek Coder v2 (Custom)", model.Name)
}

func TestApplyOverrides_CompatNameNotOverriddenWhenEmpty(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Compat: map[string]compatOverride{
			"deepseek-": {StripToolChoice: true},
		},
	}
	model := core.Model{
		ID:       "deepseek-coder-v2",
		Provider: "openai",
		Name:     "original",
	}

	applyOverrides(cfg, &model)

	assert.Equal(t, "original", model.Name)
}

func TestResolveKeyCommand_FromOverride(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Overrides: map[string]providerOverride{
			"anthropic": {KeyCommand: "echo secret"},
		},
	}
	assert.Equal(t, "echo secret", resolveKeyCommand(cfg, "anthropic"))
}

func TestResolveKeyCommand_FromCustom(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Custom: map[string]customProvider{
			"openrouter": {KeyCommand: "cat /tmp/key"},
		},
	}
	assert.Equal(t, "cat /tmp/key", resolveKeyCommand(cfg, "openrouter"))
}

func TestResolveKeyCommand_OverrideTakesPrecedence(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{
		Overrides: map[string]providerOverride{
			"myapi": {KeyCommand: "from-override"},
		},
		Custom: map[string]customProvider{
			"myapi": {KeyCommand: "from-custom"},
		},
	}
	assert.Equal(t, "from-override", resolveKeyCommand(cfg, "myapi"))
}

func TestResolveKeyCommand_NoMatch(t *testing.T) {
	t.Parallel()
	cfg := providerConfig{}
	assert.Equal(t, "", resolveKeyCommand(cfg, "unknown"))
}

func TestExecKeyCommand_Success(t *testing.T) {
	t.Parallel()
	key, err := execKeyCommand(context.Background(), "echo test-key")
	require.NoError(t, err)
	assert.Equal(t, "test-key", key)
}

func TestExecKeyCommand_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	key, err := execKeyCommand(context.Background(), "printf '  spaced  \\n'")
	require.NoError(t, err)
	assert.Equal(t, "spaced", key)
}

func TestExecKeyCommand_Failure(t *testing.T) {
	t.Parallel()
	_, err := execKeyCommand(context.Background(), "false")
	assert.Error(t, err)
}

func TestResolveKey_ShellCommand(t *testing.T) {
	t.Parallel()
	// Clear cache to avoid cross-test contamination
	key, err := resolveKey(context.Background(), "test-shell", "!echo shell-secret")
	require.NoError(t, err)
	assert.Equal(t, "shell-secret", key)
}

func TestResolveKey_ShellCommandCached(t *testing.T) {
	t.Parallel()
	// First call populates cache
	key1, err := resolveKey(context.Background(), "test-cached", "!echo cached-val")
	require.NoError(t, err)
	assert.Equal(t, "cached-val", key1)

	// Second call should return cached value (even though command could differ)
	key2, err := resolveKey(context.Background(), "test-cached", "!echo cached-val")
	require.NoError(t, err)
	assert.Equal(t, "cached-val", key2)
}

func TestResolveKey_EnvVar(t *testing.T) {
	t.Setenv("TEST_PIGLET_KEY_ABC", "env-secret")
	key, err := resolveKey(context.Background(), "test-env", "TEST_PIGLET_KEY_ABC")
	require.NoError(t, err)
	assert.Equal(t, "env-secret", key)
}

func TestResolveKey_EnvVarMissing(t *testing.T) {
	t.Parallel()
	key, err := resolveKey(context.Background(), "test-env-miss", "PIGLET_NONEXISTENT_VAR_XYZ")
	require.NoError(t, err)
	assert.Equal(t, "", key)
}

func TestResolveKey_Literal(t *testing.T) {
	t.Parallel()
	key, err := resolveKey(context.Background(), "test-literal", "sk-abc123def")
	require.NoError(t, err)
	assert.Equal(t, "sk-abc123def", key)
}

func TestResolveKey_Empty(t *testing.T) {
	t.Parallel()
	key, err := resolveKey(context.Background(), "test-empty", "")
	require.NoError(t, err)
	assert.Equal(t, "", key)
}

func TestResolveKey_ShellCommandFailure(t *testing.T) {
	t.Parallel()
	_, err := resolveKey(context.Background(), "test-fail", "!false")
	assert.Error(t, err)
}

func TestEnvVarPattern(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		match bool
	}{
		{"MY_API_KEY", true},
		{"OPENAI_API_KEY", true},
		{"_PRIVATE", true},
		{"A", false},               // single char — too short for env var
		{"sk-abc123", false},       // lowercase + dash
		{"my_key", false},          // lowercase
		{"123_KEY", false},         // starts with digit
		{"!echo hello", false},     // shell command
		{"KEY WITH SPACES", false}, // spaces
		{"", false},                // empty
	}
	for _, tt := range tests {
		assert.Equal(t, tt.match, envVarPattern.MatchString(tt.input), "input: %q", tt.input)
	}
}

func TestDefaultProviderConfig_Empty(t *testing.T) {
	t.Parallel()
	cfg := defaultProviderConfig()
	assert.Nil(t, cfg.Overrides)
	assert.Nil(t, cfg.Custom)
	assert.Nil(t, cfg.Compat)
}
