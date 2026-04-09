package inbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// atomicWriteJSON marshals v, writes it to a temp file, then atomically renames to dest.
func atomicWriteJSON(dest string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0640); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func (s *Scanner) writeAck(id, status, reason string) {
	ack := Ack{
		ID:     id,
		Status: status,
		Reason: reason,
		Ts:     time.Now().UTC().Format(time.RFC3339),
	}
	_ = atomicWriteJSON(filepath.Join(s.acksDir(), id+".json"), ack)
}

func (s *Scanner) writeHeartbeat() {
	hb := Heartbeat{
		PID:       s.pid,
		CWD:       s.cwd,
		Started:   s.started,
		Heartbeat: time.Now().UTC().Format(time.RFC3339),
	}
	_ = atomicWriteJSON(filepath.Join(s.registryDir(), fmt.Sprintf("%d.json", s.pid)), hb)
}

func (s *Scanner) pruneAcks() {
	if time.Since(s.lastPrune) < PruneInterval {
		return
	}
	s.lastPrune = time.Now()
	dir := s.acksDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > AckMaxAge {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// Validate checks an envelope for structural validity.
func Validate(env *Envelope) (bool, string) {
	if env.Version != 1 {
		return false, "unsupported_version"
	}
	if env.ID == "" {
		return false, "missing_id"
	}
	if env.Text == "" {
		return false, "missing_text"
	}
	if len([]rune(env.Text)) > MaxTextRunes {
		return false, "text_too_long"
	}
	switch env.Mode {
	case "", ModeQueue, ModeInterrupt:
		// valid
	default:
		return false, "invalid_mode"
	}
	return true, ""
}

func (s *Scanner) isDuplicate(id string) bool {
	s.mu.Lock()
	_, inMem := s.seen[id]
	s.mu.Unlock()
	if inMem {
		return true
	}
	ackPath := filepath.Join(s.acksDir(), id+".json")
	_, err := os.Stat(ackPath)
	return err == nil
}

func (s *Scanner) ackAndRemove(path, id, status, reason string) {
	if id != "" {
		s.writeAck(id, status, reason)
	}
	if status == "failed" {
		s.mu.Lock()
		s.stats.Failed++
		s.mu.Unlock()
	}
	_ = os.Remove(path)
}

// isExpired checks whether the envelope has exceeded its TTL.
// It returns false if TTL is unset (zero).
func isExpired(env *Envelope, fileModTime time.Time) bool {
	if env.TTL <= 0 {
		return false
	}
	created := fileModTime
	if env.Created != "" {
		if t, err := time.Parse(time.RFC3339, env.Created); err == nil {
			created = t
		}
	}
	return time.Now().After(created.Add(time.Duration(env.TTL) * time.Second))
}
