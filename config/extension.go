package config

import (
	"os"
	"path/filepath"
	"strings"
)

// ReadExtensionConfig reads a markdown config file for the named extension.
// Looks for ~/.config/piglet/{name}.md.
// Returns empty string (not error) if the file doesn't exist.
func ReadExtensionConfig(name string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
