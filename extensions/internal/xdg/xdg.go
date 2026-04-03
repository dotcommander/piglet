// Package xdg resolves the piglet config directory using the same logic as the
// piglet host: $XDG_CONFIG_HOME/piglet or $HOME/.config/piglet.
//
// On macOS os.UserConfigDir() returns ~/Library/Application Support, which
// differs from the host's path. This package ensures extensions and host agree.
package xdg

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the piglet config directory.
// Matches piglet/config.ConfigDir() exactly.
func ConfigDir() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "piglet"), nil
}

// ExtensionDir returns the namespaced config directory for the given extension:
// ~/.config/piglet/extensions/<name>/. Does not create the directory.
func ExtensionDir(name string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "extensions", name), nil
}
