package xdg

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadYAML reads a YAML config file from the piglet config directory.
// If the file is missing, it writes the marshaled defaults atomically and returns them.
func LoadYAML[T any](filename string, defaults T) T {
	dir, err := ConfigDir()
	if err != nil {
		return defaults
	}

	cfgPath := filepath.Join(dir, filename)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		defaultData, _ := yaml.Marshal(defaults)
		_ = WriteFileAtomic(cfgPath, defaultData)
		return defaults
	}

	_ = yaml.Unmarshal(data, &defaults)
	return defaults
}

// LoadYAMLExt reads a YAML config from the extension's namespaced directory.
// Resolution order:
//  1. extensions/<extName>/<filename> (new location)
//  2. <filename> in flat config dir (old location, backward compat)
//
// If found at old location, migrates to new location.
// If neither exists, returns defaults.
func LoadYAMLExt[T any](extName, filename string, defaults T) T {
	extDir, err := ExtensionDir(extName)
	if err != nil {
		return defaults
	}
	newPath := filepath.Join(extDir, filename)

	// Try new location first
	if data, err := os.ReadFile(newPath); err == nil {
		var cfg T
		if err := yaml.Unmarshal(data, &cfg); err == nil {
			return cfg
		}
		// Corrupt file at new location — fall through to flat fallback
	}

	// Fallback: read flat location directly (no create side effect)
	dir, err := ConfigDir()
	if err != nil {
		return createDefaultYAML(extDir, newPath, defaults)
	}
	flatPath := filepath.Join(dir, filename)
	if data, err := os.ReadFile(flatPath); err == nil {
		var cfg T
		if err := yaml.Unmarshal(data, &cfg); err == nil {
			// Migrate valid flat file to new location
			if migrated, merr := yaml.Marshal(cfg); merr == nil {
				if err := os.MkdirAll(extDir, 0o755); err == nil {
					_ = WriteFileAtomic(newPath, migrated)
				}
			}
			return cfg
		}
		// Flat file also corrupt — fall through to defaults
	}

	return createDefaultYAML(extDir, newPath, defaults)
}

func createDefaultYAML[T any](extDir, path string, defaults T) T {
	if data, err := yaml.Marshal(defaults); err == nil {
		if err := os.MkdirAll(extDir, 0o755); err == nil {
			_ = WriteFileAtomic(path, data)
		}
	}
	return defaults
}
