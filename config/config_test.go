package config_test

import (
	"os"
	"path/filepath"
	"github.com/dotcommander/piglet/config"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFrom_NonExistent(t *testing.T) {
	t.Parallel()
	s, err := config.LoadFrom("/tmp/piglet-test-nonexistent/config.yaml")
	require.NoError(t, err)
	assert.Equal(t, config.Settings{}, s)
}

func TestSaveToAndLoadFrom(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	s := config.Settings{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-4o",
		SmallModel:      "gpt-4o-mini",
		Theme:           "dark",
		Extensions:      []string{"git-tool"},
		Providers:       map[string]string{"custom": "https://example.com/v1"},
	}

	err := config.SaveTo(s, path)
	require.NoError(t, err)

	loaded, err := config.LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, s, loaded)
}

func TestSaveTo_CreatesDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "config.yaml")

	err := config.SaveTo(config.Settings{Theme: "light"}, path)
	require.NoError(t, err)

	loaded, err := config.LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "light", loaded.Theme)
}

func TestSaveTo_AtomicWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write initial
	require.NoError(t, config.SaveTo(config.Settings{Theme: "dark"}, path))

	// Overwrite
	require.NoError(t, config.SaveTo(config.Settings{Theme: "light"}, path))

	loaded, err := config.LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "light", loaded.Theme)

	// No .tmp file left behind
	_, err = os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err))
}

func TestLoadFrom_InvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	require.NoError(t, os.WriteFile(path, []byte("defaultProvider: [\ninvalid yaml\n"), 0600))

	_, err := config.LoadFrom(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse config")
}

func TestConfigDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	dir, err := config.ConfigDir()
	require.NoError(t, err)
	assert.Equal(t, "/custom/config/piglet", dir)
}

func TestConfigDir_DefaultsToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	dir, err := config.ConfigDir()
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	assert.Equal(t, home+"/.config/piglet", dir)
}

func TestSessionsDir_DeriveFromConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	dir, err := config.SessionsDir()
	require.NoError(t, err)
	assert.Equal(t, "/custom/config/piglet/sessions", dir)
}

func TestAuthPath_DeriveFromConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	path, err := config.AuthPath()
	require.NoError(t, err)
	assert.Equal(t, "/custom/config/piglet/auth.json", path)
}

func TestSettingsPath_DeriveFromConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	path, err := config.SettingsPath()
	require.NoError(t, err)
	assert.Equal(t, "/custom/config/piglet/config.yaml", path)
}

