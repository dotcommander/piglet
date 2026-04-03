package cron

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

const (
	historyFile     = "cron-history.jsonl"
	maxHistoryLines = 1000
)

// RunEntry is a single line in cron-history.jsonl.
type RunEntry struct {
	Task       string `json:"task"`
	RanAt      string `json:"ran_at"` // RFC3339
	Success    bool   `json:"success"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// historyPath returns the full path to cron-history.jsonl.
func historyPath() (string, error) {
	dir, err := xdg.ExtensionDir("cron")
	if err != nil {
		return "", fmt.Errorf("resolve cron dir: %w", err)
	}
	return filepath.Join(dir, historyFile), nil
}

// AppendHistory atomically appends a run entry to the JSONL file.
func AppendHistory(entry RunEntry) error {
	p, err := historyPath()
	if err != nil {
		return err
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}

	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	data = append(data, '\n')

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return nil
}

// ReadHistory reads all entries from cron-history.jsonl.
func ReadHistory() ([]RunEntry, error) {
	p, err := historyPath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history: %w", err)
	}
	defer f.Close()

	var entries []RunEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e RunEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

// LastRun returns the most recent successful run time for a task.
// Returns zero time if no successful run found.
func LastRun(entries []RunEntry, taskName string) time.Time {
	// Scan backwards for most recent successful entry.
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Task == taskName && entries[i].Success {
			t, err := time.Parse(time.RFC3339, entries[i].RanAt)
			if err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// RotateHistory trims the history file to maxHistoryLines.
// Called after appending, only when line count exceeds threshold.
func RotateHistory() error {
	entries, err := ReadHistory()
	if err != nil {
		return err
	}

	if len(entries) <= maxHistoryLines {
		return nil
	}

	// Keep the most recent entries.
	entries = entries[len(entries)-maxHistoryLines:]

	p, err := historyPath()
	if err != nil {
		return err
	}
	tmp := p + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp history: %w", err)
	}

	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("marshal entry: %w", err)
		}
		data = append(data, '\n')
		if _, err := f.Write(data); err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("write temp history: %w", err)
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close temp history: %w", err)
	}

	return os.Rename(tmp, p)
}
