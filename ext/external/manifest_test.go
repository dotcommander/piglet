package external

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `name: test-ext
version: 1.0.0
runtime: bun
entry: index.ts
capabilities:
  - tools
  - commands
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadManifest(dir)
	require.NoError(t, err)

	assert.Equal(t, "test-ext", m.Name)
	assert.Equal(t, "1.0.0", m.Version)
	assert.Equal(t, "bun", m.Runtime)
	assert.Equal(t, "index.ts", m.Entry)
	assert.Equal(t, []string{"tools", "commands"}, m.Capabilities)
	assert.Equal(t, dir, m.Dir)
}

func TestLoadManifestMissingName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `runtime: bun
entry: index.ts
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0644))

	_, err := LoadManifest(dir)
	assert.ErrorContains(t, err, "name is required")
}

func TestLoadManifestMissingRuntime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `name: test
entry: index.ts
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0644))

	_, err := LoadManifest(dir)
	assert.ErrorContains(t, err, "runtime is required")
}

func TestLoadManifestEmptyEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `name: test
runtime: ./test-binary
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0644))

	m, err := LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, "", m.Entry)
	assert.Equal(t, "./test-binary", m.Runtime)
}

func TestLoadManifestNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadManifest("/nonexistent/path")
	assert.Error(t, err)
}

func TestDiscoverExtensions(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	// Valid extension
	ext1 := filepath.Join(base, "ext1")
	require.NoError(t, os.Mkdir(ext1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ext1, "manifest.yaml"), []byte(`
name: ext1
runtime: bun
entry: index.ts
`), 0644))

	// Invalid extension (missing name)
	ext2 := filepath.Join(base, "ext2")
	require.NoError(t, os.Mkdir(ext2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(ext2, "manifest.yaml"), []byte(`
runtime: bun
entry: index.ts
`), 0644))

	// Not a directory (file, should be skipped)
	require.NoError(t, os.WriteFile(filepath.Join(base, "not-a-dir"), []byte("hi"), 0644))

	manifests, err := DiscoverExtensions(base)
	require.NoError(t, err)
	require.Len(t, manifests, 1)
	assert.Equal(t, "ext1", manifests[0].Name)
}

func TestDiscoverExtensionsNonexistent(t *testing.T) {
	t.Parallel()

	manifests, err := DiscoverExtensions("/nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, manifests)
}

func TestRuntimeCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runtime string
		entry   string
		wantBin string
		wantArg string
	}{
		{"bun", "bun", "index.ts", "bun", "run"},
		{"node", "node", "main.js", "node", "/dir/main.js"},
		{"deno", "deno", "mod.ts", "deno", "run"},
		{"python", "python", "main.py", "python3", "/dir/main.py"},
		{"custom", "/usr/local/bin/ruby", "ext.rb", "/usr/local/bin/ruby", "/dir/ext.rb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &Manifest{Runtime: tt.runtime, Entry: tt.entry, Dir: "/dir"}
			bin, args := m.RuntimeCommand()
			assert.Equal(t, tt.wantBin, bin)
			assert.NotEmpty(t, args)
		})
	}
}
