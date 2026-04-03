package xdg

import (
	"os"
	"path/filepath"
)

// LoadOrCreateFile reads a text file from the piglet config directory.
// If the file is missing, it writes defaultContent atomically and returns it.
func LoadOrCreateFile(filename, defaultContent string) string {
	dir, err := ConfigDir()
	if err != nil {
		return defaultContent
	}

	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		_ = WriteFileAtomic(path, []byte(defaultContent+"\n"))
		return defaultContent
	}

	return string(data)
}

// ReadFile reads a file from the flat config directory. Returns empty string on any error.
// Does NOT create the file if missing.
func ReadFile(filename string) string {
	dir, err := ConfigDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return ""
	}
	return string(data)
}

// LoadOrCreateExt reads a text file from the extension's namespaced directory.
// Resolution order:
//  1. extensions/<extName>/<filename> (new location)
//  2. <filename> in flat config dir (old location, backward compat)
//
// If found at old location, migrates to new location.
// If neither exists, writes defaultContent to new location.
func LoadOrCreateExt(extName, filename, defaultContent string) string {
	extDir, err := ExtensionDir(extName)
	if err != nil {
		return defaultContent
	}
	newPath := filepath.Join(extDir, filename)

	// Try new location first
	if data, err := os.ReadFile(newPath); err == nil {
		return string(data)
	}

	// Fallback: check old flat location
	if flat := ReadFile(filename); flat != "" {
		// Migrate to new location
		if err := os.MkdirAll(extDir, 0o755); err == nil {
			_ = WriteFileAtomic(newPath, []byte(flat))
		}
		return flat
	}

	// Neither exists: create in new location with defaults
	if err := os.MkdirAll(extDir, 0o755); err == nil {
		_ = WriteFileAtomic(newPath, []byte(defaultContent+"\n"))
	}
	return defaultContent
}
