package xdg

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path via a temp file + rename.
// Uses os.CreateTemp so concurrent writers to the same target do not clobber
// each other's temp. Fsyncs the temp file before close and the parent
// directory after rename for crash durability.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	// CreateTemp creates with 0600 by default on unix — chmod is a no-op but
	// documents intent.
	tmp, err := os.CreateTemp(dir, ".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	cleanup = false
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	// Fsync the parent directory to make the rename durable on power loss.
	// Best-effort: if the dir cannot be opened, the rename already succeeded.
	if dirFile, err := os.Open(dir); err == nil {
		if syncErr := dirFile.Sync(); syncErr != nil {
			dirFile.Close()
			return fmt.Errorf("sync dir: %w", syncErr)
		}
		dirFile.Close()
	}
	return nil
}
