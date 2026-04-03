package plan

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const planFilename = "plan.md"

// Store reads and writes plan.md in the project directory.
// plan.md IS the source of truth — human-editable, git-visible, session-surviving.
type Store struct {
	mu  sync.RWMutex
	cwd string
}

func NewStore(cwd string) (*Store, error) {
	return &Store{cwd: cwd}, nil
}

func (s *Store) Save(p *Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	content := FormatMarkdown(p)
	tmp := s.planPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("plan: write: %w", err)
	}
	return os.Rename(tmp, s.planPath())
}

func (s *Store) Load(_ string) (*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadFile()
}

func (s *Store) Active() (*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadFile()
}

func (s *Store) List() ([]*Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, err := s.loadFile()
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	return []*Plan{p}, nil
}

func (s *Store) Delete(_ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.planPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("plan: delete: %w", err)
	}
	return nil
}

func (s *Store) SetActive(_ string) error {
	// With file-per-directory model, there's only one plan — always active
	return nil
}

func (s *Store) Deactivate() error {
	// No-op: plan.md stays on disk, just not injected into prompt
	// The plan is "deactivated" by deleting the file
	return s.Delete("")
}

func (s *Store) planPath() string {
	return filepath.Join(s.cwd, planFilename)
}

func (s *Store) loadFile() (*Plan, error) {
	data, err := os.ReadFile(s.planPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plan: read: %w", err)
	}

	return ParseMarkdown(string(data))
}

// stepPattern matches plan steps: - [x], - [ ], - [-], - [!], - [>]
var stepPattern = regexp.MustCompile(`^-\s+\[([x >!\-])\]\s+(.+)$`)

// metaPattern matches the HTML comment metadata line
var metaPattern = regexp.MustCompile(`^<!--\s*piglet:(.+)\s*-->$`)

// ParseMarkdown parses a plan.md file back into a Plan struct.
func ParseMarkdown(content string) (*Plan, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	p := &Plan{Active: true}
	stepID := 0

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Title: first H1
		if strings.HasPrefix(trimmed, "# ") && p.Title == "" {
			p.Title = strings.TrimPrefix(trimmed, "# ")
			p.Slug = slugify(p.Title)
			continue
		}

		// Step line
		if m := stepPattern.FindStringSubmatch(trimmed); m != nil {
			stepID++
			marker := m[1]
			text := m[2]

			step := Step{ID: stepID}

			switch marker {
			case "x":
				step.Status = StatusDone
			case ">":
				step.Status = StatusInProgress
			case "-":
				step.Status = StatusSkipped
			case "!":
				step.Status = StatusFailed
			default:
				step.Status = StatusPending
			}

			// Extract commit SHA if present: text (abc1234)
			if idx := strings.LastIndex(text, " ("); idx > 0 && strings.HasSuffix(text, ")") {
				sha := text[idx+2 : len(text)-1]
				if len(sha) >= 7 && len(sha) <= 40 {
					step.CommitSHA = sha
					text = text[:idx]
				}
			}

			step.Text = text
			p.Steps = append(p.Steps, step)
			continue
		}

		// Note line (indented under a step): "  - note text"
		if strings.HasPrefix(line, "  - ") && len(p.Steps) > 0 {
			note := strings.TrimPrefix(line, "  - ")
			p.Steps[len(p.Steps)-1].Notes = note
			continue
		}

		// Metadata comment
		if m := metaPattern.FindStringSubmatch(trimmed); m != nil {
			parseMetaLine(p, m[1])
			continue
		}
	}

	if p.Title == "" {
		return nil, fmt.Errorf("plan.md: no title found (expected # heading)")
	}

	if p.Mode == "" {
		p.Mode = ModeExecute
	}

	return p, nil
}

func parseMetaLine(p *Plan, meta string) {
	for _, pair := range strings.Fields(meta) {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "mode":
			p.Mode = parts[1]
		case "checkpoints":
			p.GitEnabled = parts[1] == "true"
		}
	}
}

// FormatMarkdown renders a Plan as a plan.md file.
func FormatMarkdown(p *Plan) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", p.Title)

	for _, s := range p.Steps {
		marker := " "
		switch s.Status {
		case StatusDone:
			marker = "x"
		case StatusInProgress:
			marker = ">"
		case StatusSkipped:
			marker = "-"
		case StatusFailed:
			marker = "!"
		}

		fmt.Fprintf(&b, "- [%s] %s", marker, s.Text)
		if s.CommitSHA != "" {
			fmt.Fprintf(&b, " (%s)", ShortSHA(s.CommitSHA))
		}
		b.WriteByte('\n')

		if s.Notes != "" {
			fmt.Fprintf(&b, "  - %s\n", s.Notes)
		}
	}

	// Metadata as HTML comment (invisible in rendered markdown)
	mode := p.Mode
	if mode == "" {
		mode = ModeExecute
	}
	checkpoints := "false"
	if p.GitEnabled {
		checkpoints = "true"
	}
	fmt.Fprintf(&b, "\n<!-- piglet:mode=%s checkpoints=%s -->\n", mode, checkpoints)

	return b.String()
}
