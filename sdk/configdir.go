package sdk

import (
	"os"
	"path/filepath"
)

// ExtensionConfigDir returns the namespaced config directory for an extension:
// <UserConfigDir>/piglet/extensions/<name>/
// Creates the directory if it does not exist.
func ExtensionConfigDir(name string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "piglet", "extensions", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ExtensionConfigFile returns the full path for a file under the extension's
// namespaced config directory. Does not create the file or directory.
func ExtensionConfigFile(name, filename string) (string, error) {
	dir, err := ExtensionConfigDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

// ExtensionDefaultFile reads a file from the extension's config directory.
func ExtensionDefaultFile(name, filename string) ([]byte, error) {
	path, err := ExtensionConfigFile(name, filename)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
