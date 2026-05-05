package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotcommander/piglet/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"OpenAI", "openai"},
		{"ANTHROPIC", "anthropic"},
		{"gemini", "google"},
		{"vertex", "google-vertex"},
		{"bedrock", "amazon-bedrock"},
		{"copilot", "github-copilot"},
		{"azure", "azure-openai"},
		{"  Google  ", "google"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, config.NormalizeProvider(tt.input))
		})
	}
}

func TestAuth_InMemory(t *testing.T) {
	t.Parallel()
	a := config.NewAuth("")

	require.NoError(t, a.SetKey("testprovider", "sk-test-123"))
	assert.Equal(t, "sk-test-123", a.GetAPIKey("testprovider"))
	assert.True(t, a.HasAuth("testprovider"))
	assert.False(t, a.HasAuth("otherprovider"))

	require.NoError(t, a.RemoveKey("testprovider"))
	assert.Equal(t, "", a.GetAPIKey("testprovider"))
}

func TestAuth_RuntimeOverride(t *testing.T) {
	t.Parallel()
	a := config.NewAuth("")

	require.NoError(t, a.SetKey("testprovider", "stored-key"))
	a.SetRuntimeKey("testprovider", "runtime-key")

	assert.Equal(t, "runtime-key", a.GetAPIKey("testprovider"))
}

func TestAuth_EnvFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key-123")

	a := config.NewAuth("")
	assert.Equal(t, "env-key-123", a.GetAPIKey("openai"))
}

func TestAuth_EnvFallback_Hyphenated(t *testing.T) {
	t.Setenv("GOOGLE_VERTEX_API_KEY", "vertex-key")

	a := config.NewAuth("")
	assert.Equal(t, "vertex-key", a.GetAPIKey("google-vertex"))
}

func TestAuth_AliasResolution(t *testing.T) {
	t.Parallel()
	a := config.NewAuth("")
	require.NoError(t, a.SetKey("google", "google-key"))

	// "gemini" is an alias for "google"
	assert.Equal(t, "google-key", a.GetAPIKey("gemini"))
}

func TestAuth_PersistToDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	// Write
	a1 := config.NewAuth(path)
	require.NoError(t, a1.SetKey("anthropic", "sk-ant-test"))

	// Read back
	a2 := config.NewAuth(path)
	assert.Equal(t, "sk-ant-test", a2.GetAPIKey("anthropic"))
}

func TestAuth_PersistRemove(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	a := config.NewAuth(path)
	require.NoError(t, a.SetKey("testprovider", "key1"))
	require.NoError(t, a.RemoveKey("testprovider"))

	a2 := config.NewAuth(path)
	assert.Equal(t, "", a2.GetAPIKey("testprovider"))
}

func TestAuth_Providers(t *testing.T) {
	t.Parallel()
	a := config.NewAuth("")
	require.NoError(t, a.SetKey("providerA", "k1"))
	require.NoError(t, a.SetKey("providerB", "k2"))

	providers := a.Providers()
	assert.Len(t, providers, 2)
	assert.Contains(t, providers, "providera")
	assert.Contains(t, providers, "providerb")
}

func TestAuth_CommandValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	a := config.NewAuth(path)
	require.NoError(t, a.SetKey("testprovider", "!echo test-key-from-cmd"))

	assert.Equal(t, "test-key-from-cmd", a.GetAPIKey("testprovider"))
}

func TestAuth_EnvRefValue(t *testing.T) {
	t.Setenv("MY_SECRET_KEY", "secret-123")

	a := config.NewAuth("")
	require.NoError(t, a.SetKey("testprovider", "${MY_SECRET_KEY}"))

	assert.Equal(t, "secret-123", a.GetAPIKey("testprovider"))
}

func TestAuth_NoTmpFileLeftBehind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	a := config.NewAuth(path)
	require.NoError(t, a.SetKey("testprovider", "key"))

	_, err := os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err))
}

func TestAuth_HasAuth_NoKeyConfigured(t *testing.T) {
	t.Parallel()

	a := config.NewAuth("")

	// Use providers unlikely to have env vars set
	assert.False(t, a.HasAuth("nonexistent-provider"))
	assert.False(t, a.HasAuth("somefakeprovider"))
}

func TestAuth_ResolutionPriority(t *testing.T) {
	t.Parallel()

	a := config.NewAuth("")
	require.NoError(t, a.SetKey("testprovider", "stored-key"))
	a.SetRuntimeKey("testprovider", "runtime-key")

	// Runtime > stored > env; runtime wins
	assert.Equal(t, "runtime-key", a.GetAPIKey("testprovider"))

	// Remove runtime; stored wins
	a.SetRuntimeKey("testprovider", "")
	assert.Equal(t, "stored-key", a.GetAPIKey("testprovider"))
}

func TestAuth_EmptyProvider(t *testing.T) {
	t.Parallel()

	a := config.NewAuth("")
	assert.Equal(t, "", a.GetAPIKey(""), "empty provider returns empty key")
	assert.False(t, a.HasAuth(""))
}

func TestAuth_EnvRef_DollarPrefix(t *testing.T) {
	t.Setenv("MY_DOLLAR_KEY", "dollar-ref-value")

	a := config.NewAuth("")
	require.NoError(t, a.SetKey("testprovider", "$MY_DOLLAR_KEY"))

	assert.Equal(t, "dollar-ref-value", a.GetAPIKey("testprovider"))
}

func TestAuth_CommandValue_Empty(t *testing.T) {
	t.Parallel()

	a := config.NewAuth("")
	// "!" with no command after it returns empty
	require.NoError(t, a.SetKey("testprovider", "!"))

	assert.Equal(t, "", a.GetAPIKey("testprovider"))
}

func TestAuth_CommandValue_Failing(t *testing.T) {
	t.Parallel()

	a := config.NewAuth("")
	// A command that always exits non-zero returns empty
	require.NoError(t, a.SetKey("testprovider", "!false"))

	assert.Equal(t, "", a.GetAPIKey("testprovider"))
}

// writeAuthFile writes a credentials map to path with exactly the given mode.
// Explicit Chmod follows WriteFile because the umask filters WriteFile's perm
// bits — a strict umask would otherwise mask 0644 down to 0600 and silently
// invert the "permissive" test.
func writeAuthFile(t *testing.T, path string, creds map[string]string, mode os.FileMode) {
	t.Helper()
	data, err := json.MarshalIndent(creds, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, mode))
	require.NoError(t, os.Chmod(path, mode))
}

func TestAuth_CommandValue_PermissivePermsBlocked(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	writeAuthFile(t, path, map[string]string{"testprovider": "!echo leaked-key"}, 0644)

	a := config.NewAuth(path)
	assert.Equal(t, "", a.GetAPIKey("testprovider"))
}

func TestAuth_CommandValue_RestrictivePermsAllowed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	writeAuthFile(t, path, map[string]string{"testprovider": "!echo safe-key"}, 0600)

	a := config.NewAuth(path)
	assert.Equal(t, "safe-key", a.GetAPIKey("testprovider"))
}

func TestAuth_CommandValue_InMemoryAlwaysAllowed(t *testing.T) {
	t.Parallel()

	a := config.NewAuth("")
	require.NoError(t, a.SetKey("testprovider", "!echo inmem-key"))
	// In-memory auth has no file to check; !command always allowed
	assert.Equal(t, "inmem-key", a.GetAPIKey("testprovider"))
}

func TestAuth_CommandValue_LazyPermCheck(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	// NewAuth when file doesn't exist yet — permChecked stays false
	a := config.NewAuth(path)

	// SetKey writes the file with 0600 via AtomicWrite
	require.NoError(t, a.SetKey("testprovider", "!echo lazy-key"))

	// resolveValue's lazy re-check should find the file with 0600 and allow the command
	assert.Equal(t, "lazy-key", a.GetAPIKey("testprovider"))
}
