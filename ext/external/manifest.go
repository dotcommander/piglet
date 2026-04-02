package external

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/dotcommander/piglet/config"
	"gopkg.in/yaml.v3"
)

// DefaultFile describes a config file to seed on first install.
type DefaultFile struct {
	Src  string `yaml:"src"`
	Dest string `yaml:"dest"`
}

// Manifest describes an external extension.
type Manifest struct {
	Name         string        `yaml:"name"`
	Version      string        `yaml:"version,omitempty"`
	Runtime      string        `yaml:"runtime"`                // "bun", "node", "deno", "python", or absolute path
	Entry        string        `yaml:"entry"`                  // e.g. "index.ts", "main.py"
	Capabilities []string      `yaml:"capabilities,omitempty"` // "tools", "commands", "prompt"
	Defaults     []DefaultFile `yaml:"defaults,omitempty"`
	Dir          string        `yaml:"-"` // populated at load time
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
	// Entry is optional for compiled Go binaries (runtime is the binary itself)

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
		// Skip directories without a manifest (stale leftovers from pack consolidation)
		if _, err := os.Stat(filepath.Join(dir, "manifest.yaml")); err != nil {
			continue
		}
		m, err := LoadManifest(dir)
		if err != nil {
			slog.Warn("skip invalid extension", "dir", dir, "err", err)
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

// ProjectExtensionsDir returns the project-local extensions directory: <cwd>/.piglet/extensions/.
// Returns empty string if cwd is empty.
func ProjectExtensionsDir(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, ".piglet", "extensions")
}

// RuntimeCommand returns the command and args to execute an extension.
func (m *Manifest) RuntimeCommand() (string, []string) {
	switch m.Runtime {
	case "bun":
		return "bun", []string{"run", filepath.Join(m.Dir, m.Entry)}
	case "node":
		return "node", []string{filepath.Join(m.Dir, m.Entry)}
	case "deno":
		return "deno", []string{"run", "--allow-all", filepath.Join(m.Dir, m.Entry)}
	case "python":
		return "python3", []string{filepath.Join(m.Dir, m.Entry)}
	default:
		// For compiled binaries: runtime is the binary path (relative to dir or absolute).
		// If entry is empty, run the binary directly with no args.
		bin := m.Runtime
		if !filepath.IsAbs(bin) {
			bin = filepath.Join(m.Dir, bin)
		}
		if m.Entry == "" {
			return bin, nil
		}
		return bin, []string{filepath.Join(m.Dir, m.Entry)}
	}
}
