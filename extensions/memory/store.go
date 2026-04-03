package memory

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

// Fact is a single key/value memory entry with optional category and relations.
type Fact struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Category  string    `json:"category,omitzero"`
	Relations []string  `json:"relations,omitzero"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store holds facts in memory backed by a JSONL file.
type Store struct {
	mu   sync.RWMutex
	path string
	data map[string]Fact
}

// NewStore creates a Store whose file path is derived from the sha256 of cwd.
// Existing facts are loaded if the file is present.
func NewStore(cwd string) (*Store, error) {
	path, err := storePath(cwd)
	if err != nil {
		return nil, fmt.Errorf("memory: resolve path: %w", err)
	}

	s := &Store{
		path: path,
		data: make(map[string]Fact),
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

// Path returns the backing file path.
func (s *Store) Path() string {
	return s.path
}

// Set upserts a fact. CreatedAt is preserved on update.
func (s *Store) Set(key, value, category string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	f, exists := s.data[key]
	if !exists {
		f = Fact{Key: key, CreatedAt: now}
	}

	f.Value = value
	f.Category = category
	f.UpdatedAt = now
	s.data[key] = f

	return s.flush()
}

// Get retrieves a fact by key.
func (s *Store) Get(key string) (Fact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, ok := s.data[key]
	return f, ok
}

// List returns all facts, optionally filtered by category.
// Results are sorted by key for deterministic output.
func (s *Store) List(category string) []Fact {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Fact, 0, len(s.data))
	for _, f := range s.data {
		if category == "" || f.Category == category {
			out = append(out, f)
		}
	}

	slices.SortFunc(out, func(a, b Fact) int {
		return strings.Compare(a.Key, b.Key)
	})

	return out
}

// Delete removes a fact by key, cleans up its relations, and rewrites the file.
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data[key]; !ok {
		return nil
	}

	delete(s.data, key)

	// Remove deleted key from all remaining facts' Relations slices.
	for k, f := range s.data {
		if slices.Contains(f.Relations, key) {
			f.Relations = slices.DeleteFunc(f.Relations, func(r string) bool { return r == key })
			s.data[k] = f
		}
	}

	return s.flush()
}

// Relate creates a bidirectional link between two facts.
// Both keys must exist. Idempotent — duplicate relations are ignored.
func (s *Store) Relate(keyA, keyB string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	factA, okA := s.data[keyA]
	factB, okB := s.data[keyB]
	if !okA || !okB {
		return fmt.Errorf("memory: both keys must exist")
	}

	if !slices.Contains(factA.Relations, keyB) {
		factA.Relations = append(factA.Relations, keyB)
		slices.Sort(factA.Relations)
		s.data[keyA] = factA
	}
	if !slices.Contains(factB.Relations, keyA) {
		factB.Relations = append(factB.Relations, keyA)
		slices.Sort(factB.Relations)
		s.data[keyB] = factB
	}

	return s.flush()
}

// Unrelate removes a bidirectional link between two facts.
func (s *Store) Unrelate(keyA, keyB string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if factA, ok := s.data[keyA]; ok {
		factA.Relations = slices.DeleteFunc(factA.Relations, func(k string) bool { return k == keyB })
		s.data[keyA] = factA
	}
	if factB, ok := s.data[keyB]; ok {
		factB.Relations = slices.DeleteFunc(factB.Relations, func(k string) bool { return k == keyA })
		s.data[keyB] = factB
	}

	return s.flush()
}

// Clear removes all facts and deletes the backing file.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = make(map[string]Fact)

	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("memory: clear: %w", err)
	}

	return nil
}

// storePath returns the JSONL path for the given cwd.
func storePath(cwd string) (string, error) {
	base, err := xdg.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("memory: user config dir: %w", err)
	}

	sum := sha256.Sum256([]byte(cwd))
	name := hex.EncodeToString(sum[:])[:12] + ".jsonl"

	return filepath.Join(base, "memory", name), nil
}

// load reads the JSONL file into s.data. Missing file is not an error.
func (s *Store) load() error {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("memory: open: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var fact Fact
		if err := json.Unmarshal(line, &fact); err != nil {
			return fmt.Errorf("memory: parse line: %w", err)
		}

		s.data[fact.Key] = fact
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("memory: read: %w", err)
	}

	return nil
}

// flush serialises s.data to JSONL and writes atomically.
// Caller must hold s.mu.
func (s *Store) flush() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("memory: mkdir: %w", err)
	}

	facts := make([]Fact, 0, len(s.data))
	for _, f := range s.data {
		facts = append(facts, f)
	}

	slices.SortFunc(facts, func(a, b Fact) int {
		return strings.Compare(a.Key, b.Key)
	})

	var buf []byte
	for _, f := range facts {
		line, err := json.Marshal(f)
		if err != nil {
			return fmt.Errorf("memory: marshal fact %q: %w", f.Key, err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}

	return xdg.WriteFileAtomic(s.path, buf)
}
