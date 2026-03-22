package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Store struct {
	mu  sync.RWMutex
	dir string
}

func NewStore(cwd string) (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("plan: user config dir: %w", err)
	}

	sum := sha256.Sum256([]byte(cwd))
	hash := hex.EncodeToString(sum[:])[:12]
	dir := filepath.Join(base, "piglet", "plans", hash)

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("plan: mkdir: %w", err)
	}

	return &Store{dir: dir}, nil
}

// savePlan marshals and atomically writes a plan. Caller must hold s.mu.
func (s *Store) savePlan(p *Plan) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("plan: marshal: %w", err)
	}
	return atomicWrite(s.planPath(p.Slug), data)
}

func (s *Store) Save(p *Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.savePlan(p)
}

func (s *Store) Load(slug string) (*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadFile(s.planPath(slug))
}

func (s *Store) Active() (*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	plans, err := s.listAll()
	if err != nil {
		return nil, err
	}
	for _, p := range plans {
		if p.Active {
			return p, nil
		}
	}
	return nil, nil
}

func (s *Store) List() ([]*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listAll()
}

func (s *Store) Delete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.planPath(slug)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("plan: delete: %w", err)
	}
	return nil
}

func (s *Store) SetActive(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	plans, err := s.listAll()
	if err != nil {
		return err
	}

	found := false
	for _, p := range plans {
		wasActive := p.Active
		p.Active = (p.Slug == slug)
		if p.Slug == slug {
			found = true
		}
		if p.Active != wasActive {
			if err := s.savePlan(p); err != nil {
				return err
			}
		}
	}

	if !found {
		return fmt.Errorf("plan: %q not found", slug)
	}
	return nil
}

func (s *Store) Deactivate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	plans, err := s.listAll()
	if err != nil {
		return err
	}

	for _, p := range plans {
		if p.Active {
			p.Active = false
			return s.savePlan(p)
		}
	}
	return nil
}

func (s *Store) planPath(slug string) string {
	return filepath.Join(s.dir, slug+".yaml")
}

func (s *Store) listAll() ([]*Plan, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("plan: read dir: %w", err)
	}

	var plans []*Plan
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		p, err := s.loadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		plans = append(plans, p)
	}
	return plans, nil
}

func (s *Store) loadFile(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plan: read: %w", err)
	}

	var p Plan
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("plan: unmarshal: %w", err)
	}
	return &p, nil
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".piglet-tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("plan: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("plan: rename: %w", err)
	}
	return nil
}
