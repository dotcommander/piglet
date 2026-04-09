package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/config"
)

// snapshotFile saves a copy of the file at path to the undo directory.
// Uses ~/.config/piglet/undo/ with a hash of the file path as the filename.
// Only the most recent snapshot per file is kept (overwritten on each write).
// Errors are silently ignored — undo is best-effort.
func snapshotFile(path string) {
	info, err := os.Stat(path)
	const maxSnapshotSize = 10 << 20 // 10 MB
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSnapshotSize {
		return // new file, non-regular, or too large to snapshot
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	dir, err := undoDir()
	if err != nil {
		return
	}

	// Use sha256 of absolute path as filename to avoid path separator issues
	h := sha256.Sum256([]byte(path))
	name := hex.EncodeToString(h[:16]) // 32-char hex
	snapPath := filepath.Join(dir, name+".snap")

	// Skip if snapshot already exists with same content
	existing, err := os.ReadFile(snapPath)
	if err == nil && bytes.Equal(existing, data) {
		return
	}

	// Write snapshot: <hash>.snap (content) + <hash>.path (original path)
	_ = config.AtomicWrite(snapPath, data, 0600)
	_ = config.AtomicWrite(filepath.Join(dir, name+".path"), []byte(path), 0600)
}

// undoDir returns ~/.config/piglet/undo/, creating it if needed.
func undoDir() (string, error) {
	cfgDir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfgDir, "undo")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// UndoSnapshots returns all undo snapshots as path→content pairs.
func UndoSnapshots() (map[string][]byte, error) {
	dir, err := undoDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]byte)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".path") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".path")
		pathData, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		snapData, err := os.ReadFile(filepath.Join(dir, base+".snap"))
		if err != nil {
			continue
		}
		result[string(pathData)] = snapData
	}
	return result, nil
}
