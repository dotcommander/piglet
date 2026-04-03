package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/sdk"
)

// Scorer evaluates a model response against expected output or criteria.
type Scorer interface {
	Score(ctx context.Context, response, expected, criteria string) (ScoreResult, error)
}

// ScoreResult holds the outcome of a single scorer evaluation.
type ScoreResult struct {
	Score  float64 `json:"score"` // 0.0-1.0
	Pass   bool    `json:"pass"`
	Reason string  `json:"reason"`
}

// ExactScorer passes when the trimmed response equals the expected string.
type ExactScorer struct{}

func (s *ExactScorer) Score(_ context.Context, response, expected, _ string) (ScoreResult, error) {
	if strings.TrimSpace(response) == strings.TrimSpace(expected) {
		return ScoreResult{Score: 1.0, Pass: true, Reason: "exact match"}, nil
	}
	return ScoreResult{Score: 0.0, Pass: false, Reason: "response does not exactly match expected"}, nil
}

// ContainsScorer passes when the response contains the expected substring (case-insensitive).
type ContainsScorer struct{}

func (s *ContainsScorer) Score(_ context.Context, response, expected, _ string) (ScoreResult, error) {
	if strings.Contains(strings.ToLower(response), strings.ToLower(expected)) {
		return ScoreResult{Score: 1.0, Pass: true, Reason: fmt.Sprintf("response contains %q", expected)}, nil
	}
	return ScoreResult{Score: 0.0, Pass: false, Reason: fmt.Sprintf("response does not contain %q", expected)}, nil
}

// RegexScorer passes when the response matches the expected regular expression.
type RegexScorer struct{}

func (s *RegexScorer) Score(_ context.Context, response, expected, _ string) (ScoreResult, error) {
	re, err := regexp.Compile(expected)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("compile regex %q: %w", expected, err)
	}
	if re.MatchString(response) {
		return ScoreResult{Score: 1.0, Pass: true, Reason: fmt.Sprintf("response matches regex %q", expected)}, nil
	}
	return ScoreResult{Score: 0.0, Pass: false, Reason: fmt.Sprintf("response does not match regex %q", expected)}, nil
}

// JudgeScorer asks an LLM to evaluate the response against criteria.
type JudgeScorer struct {
	ext    *sdk.Extension
	prompt string
}

func (s *JudgeScorer) Score(ctx context.Context, response, _ string, criteria string) (ScoreResult, error) {
	input := formatJudgeInput(response, criteria)
	resp, err := s.ext.Chat(ctx, sdk.ChatRequest{
		System:    s.prompt,
		Messages:  []sdk.ChatMessage{{Role: "user", Content: input}},
		Model:     "small",
		MaxTokens: 200,
	})
	if err != nil {
		return ScoreResult{Score: 0.5, Pass: false, Reason: "judge call failed: " + err.Error()}, nil
	}

	var result ScoreResult
	text := strings.TrimSpace(resp.Text)
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return ScoreResult{Score: 0.5, Pass: false, Reason: "judge response unparseable"}, nil
	}
	return result, nil
}

func formatJudgeInput(response, criteria string) string {
	return fmt.Sprintf("Response to evaluate:\n%s\n\nCriteria:\n%s", response, criteria)
}

// NewScorer returns the named Scorer implementation.
func NewScorer(name string, ext *sdk.Extension) (Scorer, error) {
	switch name {
	case "exact":
		return &ExactScorer{}, nil
	case "contains":
		return &ContainsScorer{}, nil
	case "regex":
		return &RegexScorer{}, nil
	case "judge":
		prompt := xdg.LoadOrCreateExt("eval", "judge-prompt.md", strings.TrimSpace(defaultJudgePrompt))
		return &JudgeScorer{ext: ext, prompt: prompt}, nil
	default:
		return nil, fmt.Errorf("unknown scorer %q", name)
	}
}
