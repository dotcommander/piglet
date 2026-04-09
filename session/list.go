package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// List returns summaries of all sessions in a directory, newest first.
func List(dir string) ([]Summary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var summaries []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		summary, err := scanSummary(path)
		if err != nil {
			slog.Warn("session: scan summary failed", "path", path, "err", err)
			continue
		}
		if summary.ID != "" {
			summaries = append(summaries, summary)
		}
	}

	slices.SortFunc(summaries, func(a, b Summary) int {
		return b.CreatedAt.Compare(a.CreatedAt) // descending: newest first
	})

	return summaries, nil
}

func scanSummary(path string) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	s := Summary{Path: path}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			slog.Warn("session: skipping corrupt entry", "path", path, "err", err)
			continue
		}

		switch entry.Type {
		case entryTypeMeta:
			var meta Meta
			if err := json.Unmarshal(entry.Data, &meta); err == nil {
				s.ID = meta.ID
				s.Title = meta.Title
				s.Model = meta.Model
				s.CWD = meta.CWD
				s.CreatedAt = meta.CreatedAt
				s.ParentID = meta.ParentID
			}
		case entryTypeCompact:
			var entries []Entry
			if err := json.Unmarshal(entry.Data, &entries); err == nil {
				s.Messages = len(entries)
			}
		case entryTypeBranchSummary:
			// Not a conversation message; skip count
		case entryTypeUser, entryTypeAssistant, entryTypeToolResult, entryTypeCustomMessage:
			s.Messages++
			// default: skip unknown/custom types (e.g., "ext:*" entries)
		}
	}

	if err := scanner.Err(); err != nil {
		return Summary{}, fmt.Errorf("scan session %s: %w", path, err)
	}

	return s, nil
}
