package eval

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/dotcommander/piglet/sdk"
)

// Runner executes evaluation suites against an LLM.
type Runner struct {
	ext *sdk.Extension
}

// RunResult holds the full results of a suite run.
type RunResult struct {
	Suite   string       `json:"suite"`
	Model   string       `json:"model"`
	RanAt   time.Time    `json:"ranAt"`
	Cases   []CaseResult `json:"cases"`
	Summary RunSummary   `json:"summary"`
}

// CaseResult holds the result of a single case execution.
type CaseResult struct {
	Name       string         `json:"name"`
	Prompt     string         `json:"prompt"`
	Response   string         `json:"response"`
	Score      float64        `json:"score"`
	Pass       bool           `json:"pass"`
	Reason     string         `json:"reason"`
	DurationMs int64          `json:"durationMs"`
	Usage      sdk.TokenUsage `json:"usage"`
}

// RunSummary aggregates stats across all cases in a run.
type RunSummary struct {
	Total           int     `json:"total"`
	Passed          int     `json:"passed"`
	Failed          int     `json:"failed"`
	AvgScore        float64 `json:"avgScore"`
	TotalDurationMs int64   `json:"totalDurationMs"`
}

// NewRunner creates a Runner backed by the given extension.
func NewRunner(e *sdk.Extension) *Runner {
	return &Runner{ext: e}
}

// Run executes all cases in the suite (filtered by caseFilter if non-empty).
func (r *Runner) Run(ctx context.Context, suite *Suite, caseFilter []string) (*RunResult, error) {
	model := suite.Model
	if model == "" {
		model = "default"
	}

	result := &RunResult{
		Suite: suite.Name,
		Model: model,
		RanAt: time.Now(),
	}

	for _, c := range suite.Cases {
		if len(caseFilter) > 0 && !slices.Contains(caseFilter, c.Name) {
			continue
		}

		cr, err := r.runCase(ctx, c, model)
		if err != nil {
			return nil, fmt.Errorf("run case %q: %w", c.Name, err)
		}
		result.Cases = append(result.Cases, cr)
	}

	result.Summary = computeSummary(result.Cases)
	return result, nil
}

func (r *Runner) runCase(ctx context.Context, c Case, model string) (CaseResult, error) {
	scorer, err := NewScorer(c.Scorer, r.ext)
	if err != nil {
		return CaseResult{}, fmt.Errorf("create scorer: %w", err)
	}

	start := time.Now()
	resp, err := r.ext.Chat(ctx, sdk.ChatRequest{
		System:   c.System,
		Messages: []sdk.ChatMessage{{Role: "user", Content: c.Prompt}},
		Model:    model,
	})
	elapsed := time.Since(start)

	var responseText string
	var usage sdk.TokenUsage
	if err != nil {
		return CaseResult{
			Name:       c.Name,
			Prompt:     c.Prompt,
			Reason:     fmt.Sprintf("chat failed: %v", err),
			DurationMs: elapsed.Milliseconds(),
		}, nil
	}
	responseText = resp.Text
	usage = resp.Usage

	score, scoreErr := scorer.Score(ctx, responseText, c.Expected, c.Criteria)
	if scoreErr != nil {
		return CaseResult{}, fmt.Errorf("score: %w", scoreErr)
	}

	return CaseResult{
		Name:       c.Name,
		Prompt:     c.Prompt,
		Response:   responseText,
		Score:      score.Score,
		Pass:       score.Pass,
		Reason:     score.Reason,
		DurationMs: elapsed.Milliseconds(),
		Usage:      usage,
	}, nil
}

func computeSummary(cases []CaseResult) RunSummary {
	s := RunSummary{Total: len(cases)}
	if s.Total == 0 {
		return s
	}

	var totalScore float64
	for _, c := range cases {
		if c.Pass {
			s.Passed++
		} else {
			s.Failed++
		}
		totalScore += c.Score
		s.TotalDurationMs += c.DurationMs
	}
	s.AvgScore = totalScore / float64(s.Total)
	return s
}
