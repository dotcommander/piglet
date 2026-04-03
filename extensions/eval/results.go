package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

// ResultSummary holds lightweight metadata about a saved result file.
type ResultSummary struct {
	Path   string
	Suite  string
	RanAt  time.Time
	Passed int
	Total  int
}

// Comparison holds the diff between two run results matched by case name.
type Comparison struct {
	RunA   string      `json:"runA"`
	RunB   string      `json:"runB"`
	Deltas []CaseDelta `json:"deltas"`
}

// CaseDelta describes how a single case changed between two runs.
type CaseDelta struct {
	Name   string  `json:"name"`
	ScoreA float64 `json:"scoreA"`
	ScoreB float64 `json:"scoreB"`
	Delta  float64 `json:"delta"` // B - A
	PassA  bool    `json:"passA"`
	PassB  bool    `json:"passB"`
}

// resultsDir returns the directory where run result files are stored.
func resultsDir() (string, error) {
	dir, err := xdg.ExtensionDir("eval")
	if err != nil {
		return "", fmt.Errorf("resolve eval dir: %w", err)
	}
	return filepath.Join(dir, "results"), nil
}

// SaveResult persists a RunResult as JSON and returns the file path.
func SaveResult(result *RunResult) (string, error) {
	dir, err := resultsDir()
	if err != nil {
		return "", err
	}

	ts := result.RanAt.UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s.json", sanitizeName(result.Suite), ts)
	path := filepath.Join(dir, name)

	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	if err := xdg.WriteFileAtomic(path, data); err != nil {
		return "", fmt.Errorf("write result: %w", err)
	}
	return path, nil
}

// LoadResult reads a RunResult from a JSON file.
func LoadResult(path string) (*RunResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read result %q: %w", path, err)
	}
	var r RunResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse result %q: %w", path, err)
	}
	return &r, nil
}

// ListResults scans the results dir and returns summaries.
// If suiteFilter is non-empty, only results for that suite are returned.
func ListResults(suiteFilter string) ([]ResultSummary, error) {
	dir, err := resultsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read results dir: %w", err)
	}

	var summaries []ResultSummary
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		r, err := LoadResult(path)
		if err != nil {
			continue // skip corrupt files
		}
		if suiteFilter != "" && r.Suite != suiteFilter {
			continue
		}
		summaries = append(summaries, ResultSummary{
			Path:   path,
			Suite:  r.Suite,
			RanAt:  r.RanAt,
			Passed: r.Summary.Passed,
			Total:  r.Summary.Total,
		})
	}
	return summaries, nil
}

// Compare matches cases by name between two runs and computes score deltas.
func Compare(a, b *RunResult) *Comparison {
	indexA := make(map[string]CaseResult, len(a.Cases))
	for _, c := range a.Cases {
		indexA[c.Name] = c
	}

	comp := &Comparison{
		RunA: fmt.Sprintf("%s @ %s", a.Suite, a.RanAt.Format(time.RFC3339)),
		RunB: fmt.Sprintf("%s @ %s", b.Suite, b.RanAt.Format(time.RFC3339)),
	}
	for _, cb := range b.Cases {
		ca, ok := indexA[cb.Name]
		if !ok {
			continue
		}
		comp.Deltas = append(comp.Deltas, CaseDelta{
			Name:   cb.Name,
			ScoreA: ca.Score,
			ScoreB: cb.Score,
			Delta:  cb.Score - ca.Score,
			PassA:  ca.Pass,
			PassB:  cb.Pass,
		})
	}
	return comp
}

// Format renders the comparison as a plain-text table.
func (c *Comparison) Format() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Comparison: %s vs %s\n\n", c.RunA, c.RunB)
	fmt.Fprintf(&b, "%-30s  %6s  %6s  %7s  %5s  %5s\n", "Case", "ScoreA", "ScoreB", "Delta", "PassA", "PassB")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 70))
	for _, d := range c.Deltas {
		passA := passStr(d.PassA)
		passB := passStr(d.PassB)
		sign := ""
		if d.Delta > 0 {
			sign = "+"
		}
		fmt.Fprintf(&b, "%-30s  %6.2f  %6.2f  %s%6.2f  %5s  %5s\n",
			d.Name, d.ScoreA, d.ScoreB, sign, d.Delta, passA, passB)
	}
	return b.String()
}

func passStr(p bool) string {
	if p {
		return "pass"
	}
	return "FAIL"
}

// sanitizeName replaces characters unsafe for filenames with hyphens.
func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r == ' ' {
			b.WriteRune('-')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
