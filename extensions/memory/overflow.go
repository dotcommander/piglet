package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/dotcommander/piglet/extensions/internal/toolresult"
	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

const overflowThreshold = 50000
const overflowKeepChars = 2048

var overflowCounter atomic.Int64

// registerOverflow registers an After interceptor that persists large tool results
// to disk and replaces them with a truncated version + path reference.
func registerOverflow(x *sdk.Extension) {
	x.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "memory-overflow",
		Priority: 30, // before sift (50)
		After: func(ctx context.Context, toolName string, details any) (any, error) {
			text, ok := toolresult.ExtractText(details)
			if !ok || len(text) <= overflowThreshold {
				return details, nil
			}

			persistPath, err := persistToolResult(toolName, text)
			if err != nil {
				return details, nil
			}

			head := text
			if len(head) > overflowKeepChars {
				runes := []rune(head)
				head = string(runes[:overflowKeepChars])
			}

			replacement := fmt.Sprintf(
				"%s\n[... full output (%d chars) persisted to %s]",
				head, len(text), persistPath,
			)

			return toolresult.ReplaceText(details, replacement), nil
		},
	})
}

// persistToolResult writes the full tool result to disk and returns the file path.
func persistToolResult(toolName, content string) (string, error) {
	base, err := xdg.ConfigDir()
	if err != nil {
		return "", err
	}

	sessionDir := os.Getenv("PIGLET_SESSION_ID")
	if sessionDir == "" {
		sessionDir = "unknown-session"
	}

	dir := filepath.Join(base, "sessions", sessionDir, "tool-results")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create tool-results dir: %w", err)
	}

	counter := overflowCounter.Add(1)
	filename := fmt.Sprintf("%s-%d.json", sanitizeFileName(toolName), counter)
	path := filepath.Join(dir, filename)

	// Write as JSON for structure preservation
	data, err := json.Marshal(map[string]string{
		"tool":   toolName,
		"output": content,
	})
	if err != nil {
		return "", err
	}

	if err := xdg.WriteFileAtomic(path, data); err != nil {
		return "", err
	}

	return path, nil
}

// sanitizeFileName replaces characters that are unsafe in filenames.
func sanitizeFileName(name string) string {
	safe := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			safe = append(safe, r)
		default:
			safe = append(safe, '_')
		}
	}
	return string(safe)
}
