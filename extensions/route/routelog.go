package route

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RouteLogEntry records a single routing evaluation for diagnostics.
type RouteLogEntry struct {
	Timestamp  string   `json:"ts"`
	PromptHash string   `json:"prompt_hash"`
	Intent     string   `json:"intent"`
	Domains    []string `json:"domains,omitzero"`
	Primary    []string `json:"primary,omitzero"`
	Confidence float64  `json:"confidence"`
	Source     string   `json:"source"` // "tool" or "hook"
}

// logRoute appends a routing result to route-log.jsonl in the feedback directory.
func logRoute(dir string, result RouteResult, promptHash, source string) {
	if dir == "" {
		return
	}

	primary := make([]string, 0, len(result.Primary))
	for _, sc := range result.Primary {
		primary = append(primary, fmt.Sprintf("%s:%.2f", sc.Name, sc.Score))
	}

	entry := RouteLogEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		PromptHash: promptHash,
		Intent:     strings.TrimSpace(result.Intent.Primary),
		Domains:    result.Domains,
		Primary:    primary,
		Confidence: result.Confidence,
		Source:     source,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	path := filepath.Join(dir, "route-log.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(append(data, '\n'))
}
