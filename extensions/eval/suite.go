package eval

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"gopkg.in/yaml.v3"
)

// Suite defines a named collection of evaluation cases.
type Suite struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Model       string `yaml:"model"` // "default", "small", or explicit model ID
	Cases       []Case `yaml:"cases"`
}

// Case is a single prompt/scorer pair within a suite.
type Case struct {
	Name     string `yaml:"name"`
	Prompt   string `yaml:"prompt"`
	System   string `yaml:"system,omitempty"`   // optional system prompt per case
	Scorer   string `yaml:"scorer"`             // "exact", "contains", "regex", "judge"
	Expected string `yaml:"expected,omitempty"` // for exact/contains/regex
	Criteria string `yaml:"criteria,omitempty"` // for judge
}

// SuiteSummary holds metadata about a suite file without loading all cases.
type SuiteSummary struct {
	Name        string
	Description string
	CaseCount   int
	Path        string
}

var validScorers = map[string]bool{
	"exact":    true,
	"contains": true,
	"regex":    true,
	"judge":    true,
}

// LoadSuite reads and validates a suite from a YAML file.
func LoadSuite(path string) (*Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read suite %q: %w", path, err)
	}
	var s Suite
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse suite %q: %w", path, err)
	}
	if err := validateSuite(&s); err != nil {
		return nil, fmt.Errorf("invalid suite %q: %w", path, err)
	}
	return &s, nil
}

func validateSuite(s *Suite) error {
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	for i, c := range s.Cases {
		if c.Name == "" {
			return fmt.Errorf("case[%d]: name is required", i)
		}
		if c.Prompt == "" {
			return fmt.Errorf("case %q: prompt is required", c.Name)
		}
		if c.Scorer == "" {
			return fmt.Errorf("case %q: scorer is required", c.Name)
		}
		if !validScorers[c.Scorer] {
			return fmt.Errorf("case %q: unknown scorer %q (valid: exact, contains, regex, judge)", c.Name, c.Scorer)
		}
		switch c.Scorer {
		case "exact", "contains", "regex":
			if c.Expected == "" {
				return fmt.Errorf("case %q: scorer %q requires expected field", c.Name, c.Scorer)
			}
		case "judge":
			if c.Criteria == "" {
				return fmt.Errorf("case %q: scorer %q requires criteria field", c.Name, c.Scorer)
			}
		}
	}
	return nil
}

// ListSuites scans dir for *.yaml files and returns summaries.
func ListSuites(dir string) ([]SuiteSummary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read suites dir: %w", err)
	}

	var summaries []SuiteSummary
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		s, err := LoadSuite(path)
		if err != nil {
			continue // skip malformed files
		}
		summaries = append(summaries, SuiteSummary{
			Name:        s.Name,
			Description: s.Description,
			CaseCount:   len(s.Cases),
			Path:        path,
		})
	}
	return summaries, nil
}

// suitesDir returns the directory where suite YAML files are stored.
func suitesDir() (string, error) {
	dir, err := xdg.ExtensionDir("eval")
	if err != nil {
		return "", fmt.Errorf("resolve eval dir: %w", err)
	}
	return filepath.Join(dir, "suites"), nil
}

// SeedDefaults writes example-suite.yaml if the suites dir is empty.
func SeedDefaults() error {
	dir, err := suitesDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read suites dir: %w", err)
	}

	hasYAML := false
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			hasYAML = true
			break
		}
	}
	if hasYAML {
		return nil
	}

	dest := filepath.Join(dir, "example-suite.yaml")
	return xdg.WriteFileAtomic(dest, []byte(defaultExampleSuite))
}
