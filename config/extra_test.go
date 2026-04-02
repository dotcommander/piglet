package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dotcommander/piglet/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── IntOr ──────────────────────────────────────────────────────────────────

func TestIntOr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		v, fallback, want int
	}{
		{5, 10, 5},
		{1, 99, 1},
		{0, 10, 10},
		{-1, 10, 10},
		{-100, 42, 42},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, config.IntOr(tt.v, tt.fallback))
		})
	}
}

// ── AgentSettings.AutoTitleEnabled ─────────────────────────────────────────

func TestAutoTitleEnabled(t *testing.T) {
	t.Parallel()

	trueVal := true
	falseVal := false

	tests := []struct {
		name      string
		autoTitle *bool
		want      bool
	}{
		{"nil defaults to true", nil, true},
		{"explicit true", &trueVal, true},
		{"explicit false", &falseVal, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := config.AgentSettings{AutoTitle: tt.autoTitle}
			assert.Equal(t, tt.want, s.AutoTitleEnabled())
		})
	}
}

// ── Settings.ResolveDefaultModel ───────────────────────────────────────────

func TestResolveDefaultModel_EnvTakesPriority(t *testing.T) {
	t.Setenv("PIGLET_DEFAULT_MODEL", "env-model")

	s := config.Settings{DefaultModel: "config-model"}
	assert.Equal(t, "env-model", s.ResolveDefaultModel())
}

func TestResolveDefaultModel_FallsBackToConfig(t *testing.T) {
	t.Setenv("PIGLET_DEFAULT_MODEL", "")

	s := config.Settings{DefaultModel: "config-model"}
	assert.Equal(t, "config-model", s.ResolveDefaultModel())
}

func TestResolveDefaultModel_BothEmpty(t *testing.T) {
	t.Setenv("PIGLET_DEFAULT_MODEL", "")

	s := config.Settings{}
	assert.Equal(t, "", s.ResolveDefaultModel())
}

// ── Settings.ResolveSmallModel ─────────────────────────────────────────────

func TestResolveSmallModel_SmallEnvTakesPriority(t *testing.T) {
	t.Setenv("PIGLET_SMALL_MODEL", "env-small")
	t.Setenv("PIGLET_DEFAULT_MODEL", "env-default")

	s := config.Settings{SmallModel: "config-small", DefaultModel: "config-default"}
	assert.Equal(t, "env-small", s.ResolveSmallModel())
}

func TestResolveSmallModel_FallsBackToSmallConfig(t *testing.T) {
	t.Setenv("PIGLET_SMALL_MODEL", "")
	t.Setenv("PIGLET_DEFAULT_MODEL", "")

	s := config.Settings{SmallModel: "config-small", DefaultModel: "config-default"}
	assert.Equal(t, "config-small", s.ResolveSmallModel())
}

func TestResolveSmallModel_FallsBackToDefaultModel(t *testing.T) {
	t.Setenv("PIGLET_SMALL_MODEL", "")
	t.Setenv("PIGLET_DEFAULT_MODEL", "")

	s := config.Settings{DefaultModel: "config-default"}
	assert.Equal(t, "config-default", s.ResolveSmallModel())
}

func TestResolveSmallModel_DefaultEnvFallback(t *testing.T) {
	t.Setenv("PIGLET_SMALL_MODEL", "")
	t.Setenv("PIGLET_DEFAULT_MODEL", "env-default")

	s := config.Settings{} // no small or default model in config
	assert.Equal(t, "env-default", s.ResolveSmallModel())
}

// ── LoadFrom: empty file ────────────────────────────────────────────────────

func TestLoadFrom_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0600))

	s, err := config.LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, config.DefaultSettings(), s)
}

// ── Settings roundtrip: nested structs and pointer booleans ────────────────

func TestSaveToAndLoadFrom_NestedStructs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	trueVal := true
	falseVal := false

	s := config.Settings{
		DefaultProvider: "anthropic",
		DefaultModel:    "claude-opus-4-6",
		SmallModel:      "claude-haiku",
		Theme:           "dark",
		Debug:           true,
		RTK:             &trueVal,
		Safeguard:       &falseVal,
		Agent: config.AgentSettings{
			MaxTurns:          20,
			BgMaxTurns:        8,
			CompactKeepRecent: 10,
			CompactAt:         50000,
			MaxMessages:       100,
			MaxTokens:         4096,
			MaxRetries:        5,
			ToolConcurrency:   4,
			AutoTitle:         &falseVal,
		},
		Git: config.GitSettings{
			MaxDiffStatFiles: 15,
			MaxLogLines:      10,
			MaxDiffHunkLines: 30,
			CommandTimeout:   3,
		},
		Tools: config.ToolSettings{
			ReadLimit: 1000,
			GrepLimit: 50,
		},
		Bash: config.BashSettings{
			DefaultTimeout: 60,
			MaxTimeout:     600,
			MaxStdout:      200000,
			MaxStderr:      100000,
		},
		SubAgent: config.SubAgentSettings{
			MaxTurns: 15,
		},
		Shortcuts:   map[string]string{"model": "ctrl+p"},
		PromptOrder: map[string]int{"repomap": 10, "memory": 50},
		ProjectDocs: &[]config.ProjectDoc{
			{Name: "README.md", Title: "Project readme"},
		},
	}

	require.NoError(t, config.SaveTo(s, path))

	loaded, err := config.LoadFrom(path)
	require.NoError(t, err)

	assert.Equal(t, s.DefaultProvider, loaded.DefaultProvider)
	assert.Equal(t, s.DefaultModel, loaded.DefaultModel)
	assert.Equal(t, s.SmallModel, loaded.SmallModel)
	assert.Equal(t, s.Theme, loaded.Theme)
	assert.Equal(t, s.Debug, loaded.Debug)
	require.NotNil(t, loaded.RTK)
	assert.Equal(t, *s.RTK, *loaded.RTK)
	require.NotNil(t, loaded.Safeguard)
	assert.Equal(t, *s.Safeguard, *loaded.Safeguard)

	assert.Equal(t, s.Agent.MaxTurns, loaded.Agent.MaxTurns)
	assert.Equal(t, s.Agent.BgMaxTurns, loaded.Agent.BgMaxTurns)
	assert.Equal(t, s.Agent.CompactKeepRecent, loaded.Agent.CompactKeepRecent)
	assert.Equal(t, s.Agent.CompactAt, loaded.Agent.CompactAt)
	assert.Equal(t, s.Agent.MaxMessages, loaded.Agent.MaxMessages)
	assert.Equal(t, s.Agent.MaxTokens, loaded.Agent.MaxTokens)
	assert.Equal(t, s.Agent.MaxRetries, loaded.Agent.MaxRetries)
	assert.Equal(t, s.Agent.ToolConcurrency, loaded.Agent.ToolConcurrency)
	require.NotNil(t, loaded.Agent.AutoTitle)
	assert.Equal(t, *s.Agent.AutoTitle, *loaded.Agent.AutoTitle)

	assert.Equal(t, s.Git, loaded.Git)
	assert.Equal(t, s.Tools, loaded.Tools)
	assert.Equal(t, s.Bash, loaded.Bash)
	assert.Equal(t, s.SubAgent, loaded.SubAgent)
	assert.Equal(t, s.Shortcuts, loaded.Shortcuts)
	assert.Equal(t, s.PromptOrder, loaded.PromptOrder)
	assert.Equal(t, s.ProjectDocs, loaded.ProjectDocs)
}

// ── Load / Save (top-level wrappers, redirect via XDG) ─────────────────────

func TestLoadAndSave_ViaXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Load from missing file returns defaults.
	s, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, config.DefaultSettings(), s)

	// Save then Load round-trips correctly (loaded value has defaults merged in).
	want := config.Settings{DefaultProvider: "openai", DefaultModel: "gpt-4o"}
	require.NoError(t, config.Save(want))

	got, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, want.DefaultProvider, got.DefaultProvider)
	assert.Equal(t, want.DefaultModel, got.DefaultModel)
}

// ── ReadExtensionConfig ─────────────────────────────────────────────────────

func TestReadExtensionConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	content, err := config.ReadExtensionConfig("myext")
	require.NoError(t, err)
	assert.Equal(t, "", content)
}

func TestReadExtensionConfig_Present(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	pigletDir := filepath.Join(dir, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0700))
	require.NoError(t, os.WriteFile(
		filepath.Join(pigletDir, "myext.md"),
		[]byte("  # My Extension Config\n\nSome content.  \n"),
		0600,
	))

	content, err := config.ReadExtensionConfig("myext")
	require.NoError(t, err)
	assert.Equal(t, "# My Extension Config\n\nSome content.", content)
}

func TestReadExtensionConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	pigletDir := filepath.Join(dir, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, "empty.md"), []byte("   \n  "), 0600))

	content, err := config.ReadExtensionConfig("empty")
	require.NoError(t, err)
	assert.Equal(t, "", content)
}

// ── ModelsPath ─────────────────────────────────────────────────────────────

func TestModelsPath_DeriveFromConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	path, err := config.ModelsPath()
	require.NoError(t, err)
	assert.Equal(t, "/custom/config/piglet/models.yaml", path)
}

// ── NeedsSetup ─────────────────────────────────────────────────────────────

func TestNeedsSetup_BothPresent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	pigletDir := filepath.Join(dir, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, "config.yaml"), []byte("{}"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, "models.yaml"), []byte("{}"), 0600))

	assert.False(t, config.NeedsSetup())
}

func TestNeedsSetup_ConfigMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	pigletDir := filepath.Join(dir, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0700))
	// Only models.yaml present, config.yaml missing.
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, "models.yaml"), []byte("{}"), 0600))

	assert.True(t, config.NeedsSetup())
}

func TestNeedsSetup_ModelsMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	pigletDir := filepath.Join(dir, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0700))
	// Only config.yaml present, models.yaml missing.
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, "config.yaml"), []byte("{}"), 0600))

	assert.True(t, config.NeedsSetup())
}

func TestNeedsSetup_NeitherPresent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	assert.True(t, config.NeedsSetup())
}

// ── RunSetup ───────────────────────────────────────────────────────────────

func TestRunSetup_NoKeysNoExistingFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Clear provider keys so detectAPIKeys returns empty.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("ZAI_API_KEY", "")

	writeModelsCalled := false
	err := config.RunSetup(func(path string) error {
		writeModelsCalled = true
		return os.WriteFile(path, []byte("models: []"), 0600)
	}, nil)
	require.NoError(t, err)
	assert.True(t, writeModelsCalled)

	// config.yaml must exist after setup.
	pigletDir := filepath.Join(dir, "piglet")
	_, err = os.Stat(filepath.Join(pigletDir, "config.yaml"))
	assert.NoError(t, err)

	// models.yaml must exist after setup.
	_, err = os.Stat(filepath.Join(pigletDir, "models.yaml"))
	assert.NoError(t, err)
}

func TestRunSetup_ExistingFilesNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	pigletDir := filepath.Join(dir, "piglet")
	require.NoError(t, os.MkdirAll(pigletDir, 0700))

	// Pre-write both files with sentinel content.
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, "config.yaml"), []byte("theme: sentinel\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pigletDir, "models.yaml"), []byte("sentinel-models\n"), 0600))

	writeModelsCalled := false
	err := config.RunSetup(func(path string) error {
		writeModelsCalled = true
		return nil
	}, nil)
	require.NoError(t, err)
	assert.False(t, writeModelsCalled, "writeModels should not be called when models.yaml already exists")

	// Existing config.yaml must not be overwritten.
	data, err := os.ReadFile(filepath.Join(pigletDir, "config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "sentinel")
}

func TestRunSetup_WithDetectedKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("OPENAI_API_KEY", "")

	err := config.RunSetup(func(path string) error {
		return os.WriteFile(path, []byte("models: []"), 0600)
	}, map[string]string{"anthropic": "claude-opus-4-6"})
	require.NoError(t, err)

	// config.yaml should have a defaultModel set to anthropic's default model.
	s, err := config.Load()
	require.NoError(t, err)
	assert.NotEmpty(t, s.DefaultModel)
}

func TestRunSetup_WriteModelsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	err := config.RunSetup(func(path string) error {
		return os.ErrPermission
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write models.yaml")
}

// ── NewAuthDefault ─────────────────────────────────────────────────────────

func TestNewAuthDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	a, err := config.NewAuthDefault()
	require.NoError(t, err)
	assert.NotNil(t, a)

	// Should work as a normal Auth instance.
	require.NoError(t, a.SetKey("openai", "sk-test"))
	assert.Equal(t, "sk-test", a.GetAPIKey("openai"))
}

// ── Auth: GEMINI_API_KEY alias for google ───────────────────────────────────

func TestAuth_GeminiEnvAlias(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-env-key")
	t.Setenv("GOOGLE_API_KEY", "")

	a := config.NewAuth("")
	assert.Equal(t, "gemini-env-key", a.GetAPIKey("google"))
}

// ── Auth: raw hyphen env var fallback ──────────────────────────────────────

func TestAuth_RawHyphenEnvVar(t *testing.T) {
	// "GOOGLE-VERTEX_API_KEY" is the raw (non-normalized) form generated for "google-vertex"
	// when the normalized form GOOGLE_VERTEX_API_KEY is absent.
	t.Setenv("GOOGLE_VERTEX_API_KEY", "")
	t.Setenv("GOOGLE-VERTEX_API_KEY", "hyphen-key")

	a := config.NewAuth("")
	assert.Equal(t, "hyphen-key", a.GetAPIKey("google-vertex"))
}

// ── Settings: ProjectDoc roundtrip ────────────────────────────────────────

func TestProjectDoc_Roundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	s := config.Settings{
		ProjectDocs: &[]config.ProjectDoc{
			{Name: "ARCHITECTURE.md", Title: "Architecture"},
			{Name: "CONTRIBUTING.md", Title: "Contributing guide"},
		},
	}
	require.NoError(t, config.SaveTo(s, path))

	loaded, err := config.LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, s.ProjectDocs, loaded.ProjectDocs)
}
