// Package plan provides persistent structured task tracking for piglet.
package plan

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusDone       = "done"
	StatusSkipped    = "skipped"
	StatusFailed     = "failed"

	ModeExecute = "execute"
	ModePropose = "propose"
)

type Plan struct {
	Title      string
	Slug       string
	Mode       string
	Created    time.Time
	Updated    time.Time
	Active     bool
	Steps      []Step
	GitEnabled bool
}

type Step struct {
	ID        int
	Text      string
	Status    string
	Notes     string
	CommitSHA string
}

func NewPlan(title string, steps []string) (*Plan, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("plan: title is required")
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("plan: at least one step is required")
	}

	now := time.Now().UTC()
	p := &Plan{
		Title:   title,
		Slug:    slugify(title),
		Created: now,
		Updated: now,
		Active:  true,
		Steps:   make([]Step, len(steps)),
	}
	for i, text := range steps {
		p.Steps[i] = Step{
			ID:     i + 1,
			Text:   text,
			Status: StatusPending,
		}
	}
	return p, nil
}

func (p *Plan) UpdateStep(id int, status, notes, commitSHA string) error {
	idx := p.stepIndex(id)
	if idx < 0 {
		return fmt.Errorf("plan: step %d not found", id)
	}

	if status != "" {
		if !validStatus(status) {
			return fmt.Errorf("plan: invalid status %q", status)
		}
		if status == StatusInProgress {
			for i := range p.Steps {
				if p.Steps[i].Status == StatusInProgress {
					p.Steps[i].Status = StatusPending
				}
			}
		}
		p.Steps[idx].Status = status
	}
	if notes != "" {
		p.Steps[idx].Notes = notes
	}
	if commitSHA != "" {
		p.Steps[idx].CommitSHA = commitSHA
	}
	p.Updated = time.Now().UTC()
	return nil
}

func (p *Plan) AddStepAfter(afterID int, text string) error {
	idx := p.stepIndex(afterID)
	if idx < 0 {
		return fmt.Errorf("plan: step %d not found", afterID)
	}

	newID := p.nextID()
	step := Step{ID: newID, Text: text, Status: StatusPending}

	p.Steps = slices.Insert(p.Steps, idx+1, step)
	p.Updated = time.Now().UTC()
	return nil
}

func (p *Plan) RemoveStep(id int) error {
	idx := p.stepIndex(id)
	if idx < 0 {
		return fmt.Errorf("plan: step %d not found", id)
	}

	p.Steps = append(p.Steps[:idx], p.Steps[idx+1:]...)
	p.Updated = time.Now().UTC()
	return nil
}

func (p *Plan) Progress() (done, total int) {
	total = len(p.Steps)
	for _, s := range p.Steps {
		if s.Status == StatusDone {
			done++
		}
	}
	return done, total
}

// IsComplete returns true when every step has a terminal status (done, skipped,
// or failed). Returns false if there are no steps.
func (p *Plan) IsComplete() bool {
	if len(p.Steps) == 0 {
		return false
	}
	for _, s := range p.Steps {
		switch s.Status {
		case StatusDone, StatusSkipped, StatusFailed:
			// terminal — continue
		default:
			return false
		}
	}
	return true
}

// InProposeMode returns true when the plan is in propose mode.
func (p *Plan) InProposeMode() bool {
	return p.Mode == ModePropose
}

// AppendStep adds a new step at the end of the plan.
func (p *Plan) AppendStep(text string) {
	p.Steps = append(p.Steps, Step{
		ID:     p.nextID(),
		Text:   text,
		Status: StatusPending,
	})
	p.Updated = time.Now().UTC()
}

// ResumeStep returns the next incomplete step, or nil if complete.
func (p *Plan) ResumeStep() *Step {
	for i := range p.Steps {
		switch p.Steps[i].Status {
		case StatusDone, StatusSkipped, StatusFailed:
			continue
		default:
			return &p.Steps[i]
		}
	}
	return nil
}

// LastCheckpoint returns the most recent step with a commit SHA.
func (p *Plan) LastCheckpoint() *Step {
	for i := len(p.Steps) - 1; i >= 0; i-- {
		if p.Steps[i].CommitSHA != "" {
			return &p.Steps[i]
		}
	}
	return nil
}

func (p *Plan) stepIndex(id int) int {
	for i, s := range p.Steps {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func (p *Plan) nextID() int {
	max := 0
	for _, s := range p.Steps {
		if s.ID > max {
			max = s.ID
		}
	}
	return max + 1
}

func validStatus(s string) bool {
	switch s {
	case StatusPending, StatusInProgress, StatusDone, StatusSkipped, StatusFailed:
		return true
	}
	return false
}

func slugify(title string) string {
	var b strings.Builder
	prev := false
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prev = false
		case !prev && b.Len() > 0:
			b.WriteByte('-')
			prev = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
