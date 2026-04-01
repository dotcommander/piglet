package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadExtensionConfig reads a markdown config file for the named extension.
// Looks for ~/.config/piglet/extensions/<name>/<name>.md first,
// falling back to ~/.config/piglet/<name>.md for backward compatibility.
// Returns empty string (not error) if the file doesn't exist.
func ReadExtensionConfig(name string) (string, error) {
	if filepath.Base(name) != name || strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("invalid extension name: %q", name)
	}

	// New location: extensions/<name>/<name>.md
	extDir, err := ExtensionConfigDir(name)
	if err != nil {
		return "", err
	}
	newPath := filepath.Join(extDir, name+".md")
	if data, err := os.ReadFile(newPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	// Fallback: old flat location
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	oldPath := filepath.Join(dir, name+".md")
	data, err := os.ReadFile(oldPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	// Migrate: copy old to new atomically (best-effort)
	if err := os.MkdirAll(extDir, 0o755); err == nil {
		_ = AtomicWrite(newPath, data, 0o600)
	}

	return strings.TrimSpace(string(data)), nil
}
