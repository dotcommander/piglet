package xdg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	Name    string `yaml:"name"`
	Count   int    `yaml:"count"`
	Enabled bool   `yaml:"enabled"`
}

func TestConfigDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.Equal(t, "/custom/config/piglet", dir)
}

func TestConfigDir_DefaultsToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := ConfigDir()
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".config", "piglet"), dir)
}

func TestLoadYAML_CreatesDefaultFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	defaults := testConfig{Name: "default", Count: 42, Enabled: true}
	result := LoadYAML("test.yaml", defaults)

	assert.Equal(t, defaults, result)

	cfgPath := filepath.Join(tmp, "piglet", "test.yaml")
	_, err := os.Stat(cfgPath)
	require.NoError(t, err, "config file should have been created on disk")
}

func TestLoadYAML_ReadsExistingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "piglet")
	require.NoError(t, os.MkdirAll(dir, 0755))

	fileData := "name: fromfile\ncount: 7\nenabled: false\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(fileData), 0644))

	defaults := testConfig{Name: "default", Count: 1, Enabled: true}
	result := LoadYAML("test.yaml", defaults)

	assert.Equal(t, testConfig{Name: "fromfile", Count: 7, Enabled: false}, result)
}

func TestLoadYAML_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "piglet")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(":::not valid yaml:::"), 0644))

	defaults := testConfig{Name: "default", Count: 5, Enabled: true}
	result := LoadYAML("test.yaml", defaults)

	assert.Equal(t, defaults, result)
}

func TestLoadYAML_MergesPartialFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "piglet")
	require.NoError(t, os.MkdirAll(dir, 0755))

	// Only name is set in the file; count and enabled are omitted.
	fileData := "name: partial\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(fileData), 0644))

	defaults := testConfig{Name: "default", Count: 99, Enabled: true}
	result := LoadYAML("test.yaml", defaults)

	// name comes from file; count and enabled keep their defaults.
	assert.Equal(t, "partial", result.Name)
	assert.Equal(t, 99, result.Count)
	assert.True(t, result.Enabled)

}
