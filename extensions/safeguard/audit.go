package safeguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

// AuditLogger writes tool decisions to a JSONL file.
type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
}

type auditEntry struct {
	Timestamp string `json:"ts"`
	Tool      string `json:"tool"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// NewAuditLogger opens or creates the audit log file in the extension directory.
func NewAuditLogger() *AuditLogger {
	dir, err := xdg.ExtensionDir("safeguard")
	if err != nil {
		return nil
	}
	path := filepath.Join(dir, "safeguard-audit.jsonl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil
	}
	return &AuditLogger{file: f}
}

// Log writes an audit entry. Safe for concurrent use.
func (a *AuditLogger) Log(tool, decision, reason, detail string) {
	if a == nil || a.file == nil {
		return
	}
	entry := auditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Tool:      tool,
		Decision:  decision,
		Reason:    reason,
		Detail:    detail,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	a.mu.Lock()
	_, _ = a.file.Write(data)
	a.mu.Unlock()
}

// Close closes the audit log file.
func (a *AuditLogger) Close() error {
	if a == nil || a.file == nil {
		return nil
	}
	return a.file.Close()
}
