// Package tokengate is a post-tool output compaction pipeline.
//
// Tool results pass through each registered Stage before entering the LLM
// context. Stages run in declared order; RunPipeline reports which stages
// actually modified the text so callers can attribute compression.
//
// Default stages: StripAnsi (universal cleanup), SmartTruncate (hard cap
// with head+tail preservation when output exceeds MaxChars).
package tokengate

import (
	"context"
	"fmt"
	"regexp"

	"github.com/dotcommander/piglet/extensions/internal/toolresult"
	sdk "github.com/dotcommander/piglet/sdk"
)

// defaultMaxChars is the SmartTruncate cap used by Register when a tool
// result exceeds this length. Sized to stay well under typical 8k-16k
// per-message budgets while preserving enough context for the model to
// continue reasoning.
const defaultMaxChars = 20000

// Stage is one post-processing pass over tool output text.
type Stage interface {
	// Name identifies the stage for attribution; returned by RunPipeline.
	Name() string
	// Apply returns the transformed text. Implementations MUST return the
	// input unchanged when they have nothing to do so RunPipeline can
	// detect which stages actually fired.
	Apply(text string) string
}

// RunPipeline applies each stage in order and returns the final text plus
// the names of stages that modified it.
func RunPipeline(stages []Stage, text string) (string, []string) {
	applied := make([]string, 0, len(stages))
	for _, s := range stages {
		next := s.Apply(text)
		if next != text {
			applied = append(applied, s.Name())
			text = next
		}
	}
	return text, applied
}

// ansiPattern matches CSI / OSC escape sequences commonly emitted by
// shell tools (colors, cursor moves, terminal title sets). Compiled once
// at package load; safe for concurrent Apply calls.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[PX^_][^\x1b]*\x1b\\`)

// StripAnsi removes ANSI escape sequences.
type StripAnsi struct{}

func (StripAnsi) Name() string             { return "strip_ansi" }
func (StripAnsi) Apply(text string) string { return ansiPattern.ReplaceAllString(text, "") }

// SmartTruncate hard-caps output length while preserving both ends so
// the model still sees the command header and its final result.
type SmartTruncate struct {
	MaxChars int
}

func (SmartTruncate) Name() string { return "smart_truncate" }

func (t SmartTruncate) Apply(text string) string {
	if t.MaxChars <= 0 || len(text) <= t.MaxChars {
		return text
	}
	// Split the preserved budget 60/40 head/tail — first bytes carry the
	// command prompt and early errors, final bytes carry the exit status
	// and last-line summary models latch onto.
	head := (t.MaxChars * 6) / 10
	tail := t.MaxChars - head
	omitted := len(text) - head - tail
	return fmt.Sprintf("%s\n... [truncated %d chars] ...\n%s", text[:head], omitted, text[len(text)-tail:])
}

// DefaultStages returns the stage set registered by Register.
func DefaultStages() []Stage {
	return []Stage{StripAnsi{}, SmartTruncate{MaxChars: defaultMaxChars}}
}

// Register installs the tokengate After interceptor. Priority 10 places
// it at the tail of the After chain — truncation and ANSI cleanup happen
// AFTER higher-priority interceptors (e.g. fossil blame, priority 100)
// have appended their own content.
func Register(e *sdk.Extension) {
	stages := DefaultStages()
	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "tokengate",
		Priority: 10,
		After: func(_ context.Context, _ string, details any) (any, error) {
			text, ok := toolresult.ExtractText(details)
			if !ok {
				return details, nil
			}
			out, applied := RunPipeline(stages, text)
			if len(applied) == 0 {
				return details, nil
			}
			return toolresult.ReplaceText(details, out), nil
		},
	})
}
