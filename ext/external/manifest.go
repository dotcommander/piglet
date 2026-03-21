package external

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dotcommander/piglet/config"
	"gopkg.in/yaml.v3"
)

// Manifest describes an external extension.
type Manifest struct {
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version,omitempty"`
	Runtime      string   `yaml:"runtime"`              // "bun", "node", "deno", "python", or absolute path
	Entry        string   `yaml:"entry"`                // e.g. "index.ts", "main.py"
	Capabilities []string `yaml:"capabilities,omitempty"` // "tools", "commands", "prompt"
	Dir          string   `yaml:"-"`                     // populated at load time
}

// LoadManifest reads a manifest.yaml from the given directory.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "manifest.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}

	if m.Name == "" {
		return nil, fmt.Errorf("manifest %s: name is required", path)
	}
	if m.Runtime == "" {
		return nil, fmt.Errorf("manifest %s: runtime is required", path)
	}
	if m.Entry == "" {
		return nil, fmt.Errorf("manifest %s: entry is required", path)
	}

	m.Dir = dir
	return &m, nil
}

// DiscoverExtensions finds all extension directories under the given base path.
// Each subdirectory must contain a manifest.yaml.
func DiscoverExtensions(baseDir string) ([]*Manifest, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read extensions dir: %w", err)
	}

	var manifests []*Manifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(baseDir, entry.Name())
		m, err := LoadManifest(dir)
		if err != nil {
			// Skip invalid extensions, log would be nice but not required
			continue
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

// ExtensionsDir returns ~/.config/piglet/extensions/.
func ExtensionsDir() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "extensions"), nil
}

// RuntimeCommand returns the command and args to execute an extension.
func (m *Manifest) RuntimeCommand() (string, []string) {
	entry := filepath.Join(m.Dir, m.Entry)

	switch m.Runtime {
	case "bun":
		return "bun", []string{"run", entry}
	case "node":
		return "node", []string{entry}
	case "deno":
		return "deno", []string{"run", "--allow-all", entry}
	case "python":
		return "python3", []string{entry}
	default:
		// Treat runtime as an absolute path to an executable
		return m.Runtime, []string{entry}
	}
}
