package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"
)

// Run executes a pipeline sequentially. Each step runs after the previous completes.
// Returns the aggregate result.
func Run(ctx context.Context, p *Pipeline, params map[string]string) (*PipelineResult, error) {
	p.defaults()

	merged := p.MergeParams(params)
	if err := p.Validate(merged); err != nil {
		return nil, err
	}

	cwd, _ := os.Getwd()
	now := time.Now()

	tc := &TemplateContext{
		Params:    merged,
		Steps:     make(map[string]*StepOutput),
		CWD:       cwd,
		StartTime: now,
	}

	start := time.Now()
	result := &PipelineResult{
		Name:  p.Name,
		Steps: make([]StepResult, 0, len(p.Steps)),
	}

	var okCount, errCount, skipCount int

	for _, step := range p.Steps {
		if step.When != "" {
			when := tc.Expand(step.When)
			if !shellPredicate(ctx, when, cwd) {
				sr := StepResult{
					Name:   step.Name,
					Status: StatusSkipped,
					Output: fmt.Sprintf("when predicate failed: %s", step.When),
				}
				result.Steps = append(result.Steps, sr)
				skipCount++
				continue
			}
		}

		sr := executeStep(ctx, &step, tc, p.Concurrency)
		result.Steps = append(result.Steps, sr)

		tc.Steps[step.Name] = &StepOutput{
			Stdout: sr.Output,
			Status: sr.Status,
		}
		tc.Prev = &StepOutput{
			Stdout: sr.Output,
			Status: sr.Status,
		}

		if sr.Status == StatusOK {
			okCount++
		} else {
			errCount++
			if !step.AllowFailure {
				result.Status = StatusError
				result.DurationMS = time.Since(start).Milliseconds()
				result.Message = fmt.Sprintf("%d ok, %d failed (halted at %q), %d skipped",
					okCount, errCount, step.Name, len(p.Steps)-len(result.Steps))
				return result, nil
			}
		}
	}

	result.DurationMS = time.Since(start).Milliseconds()
	if errCount > 0 {
		result.Status = StatusPartial
		result.Message = fmt.Sprintf("%d ok, %d failed (allowed), %d skipped", okCount, errCount, skipCount)
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("%d steps completed in %dms", okCount+skipCount, result.DurationMS)
	}
	return result, nil
}

// DryRun returns a preview of what would execute without running anything.
func DryRun(p *Pipeline, params map[string]string) (*PipelineResult, error) {
	p.defaults()

	merged := p.MergeParams(params)
	if err := p.Validate(merged); err != nil {
		return nil, err
	}

	cwd, _ := os.Getwd()
	tc := &TemplateContext{
		Params:    merged,
		Steps:     make(map[string]*StepOutput),
		CWD:       cwd,
		StartTime: time.Now(),
	}

	result := &PipelineResult{
		Name:   p.Name,
		Status: StatusDryRun,
		Steps:  make([]StepResult, 0, len(p.Steps)),
	}

	for _, step := range p.Steps {
		expanded := tc.Expand(step.Run)

		iters, _ := ExpandIterations(&step)
		iterCount := 1
		if len(iters) > 0 {
			iterCount = len(iters)
		}

		sr := StepResult{
			Name:       step.Name,
			Status:     StatusSkipped,
			Output:     fmt.Sprintf("dry run — would run: %s", strings.TrimSpace(expanded)),
			Iterations: iterCount,
		}
		result.Steps = append(result.Steps, sr)

		tc.Prev = &StepOutput{Stdout: "(dry run)", Status: StatusOK}
		tc.Steps[step.Name] = tc.Prev
	}

	result.Message = fmt.Sprintf("dry run: %d steps would execute", len(p.Steps))
	return result, nil
}

// executeStep runs a single step, handling iterations and retries.
func executeStep(ctx context.Context, step *Step, tc *TemplateContext, concurrency int) StepResult {
	start := time.Now()

	iters, err := ExpandIterations(step)
	if err != nil {
		return StepResult{
			Name:       step.Name,
			Status:     StatusError,
			Error:      err.Error(),
			DurationMS: time.Since(start).Milliseconds(),
		}
	}

	if iters == nil {
		return executeSingle(ctx, step, tc, start)
	}

	if concurrency <= 0 {
		concurrency = 4
	}

	type iterResult struct {
		idx    int
		output string
		err    error
	}

	results := make([]iterResult, len(iters))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i, iter := range iters {
		// Deep copy TemplateContext so iterations don't share mutable maps
		iterTC := tc.Clone()
		iterTC.Item = iter.Item
		iterTC.HasItem = true
		iterTC.LoopVars = iter.LoopVars

		g.Go(func() error {
			expanded := iterTC.Expand(step.Run)
			workDir := step.WorkDir
			if workDir != "" {
				workDir = iterTC.Expand(workDir)
			}

			out, execErr := shellRun(gctx, step.Shell, expanded, workDir, step.Env, step.StepTimeout())
			results[i] = iterResult{idx: i, output: out, err: execErr}
			return nil
		})
	}

	_ = g.Wait()

	var outputs []string
	var hasErr bool
	for _, r := range results {
		if r.err != nil {
			hasErr = true
			outputs = append(outputs, fmt.Sprintf("[%d] error: %s", r.idx, r.err))
		} else {
			outputs = append(outputs, r.output)
		}
	}

	status := StatusOK
	errMsg := ""
	if hasErr {
		status = StatusError
		errMsg = "one or more iterations failed"
	}

	return StepResult{
		Name:       step.Name,
		Status:     status,
		Output:     strings.Join(outputs, "\n"),
		Error:      errMsg,
		DurationMS: time.Since(start).Milliseconds(),
		Iterations: len(iters),
	}
}

// executeSingle runs one step command with retry support.
func executeSingle(ctx context.Context, step *Step, tc *TemplateContext, start time.Time) StepResult {
	expanded := tc.Expand(step.Run)
	workDir := step.WorkDir
	if workDir != "" {
		workDir = tc.Expand(workDir)
	}

	var lastErr error
	var output string
	retries := 0

	for attempt := range step.Retries + 1 {
		if attempt > 0 {
			time.Sleep(time.Duration(step.RetryDelay) * time.Second)
			retries++
		}

		out, err := shellRun(ctx, step.Shell, expanded, workDir, step.Env, step.StepTimeout())
		if err == nil {
			return StepResult{
				Name:       step.Name,
				Status:     StatusOK,
				Output:     strings.TrimSpace(out),
				DurationMS: time.Since(start).Milliseconds(),
				RetryCount: retries,
			}
		}
		lastErr = err
		output = out
	}

	return StepResult{
		Name:       step.Name,
		Status:     StatusError,
		Output:     strings.TrimSpace(output),
		Error:      lastErr.Error(),
		DurationMS: time.Since(start).Milliseconds(),
		RetryCount: retries,
	}
}

// shellRun executes a shell command and returns stdout+stderr combined.
func shellRun(ctx context.Context, shell, command, workDir string, env map[string]string, timeout time.Duration) (string, error) {
	if shell == "" {
		shell = "sh"
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, shell, "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}

	if len(env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(buf.String()))
	}
	return buf.String(), nil
}

// shellPredicate runs a shell command and returns true if exit code is 0.
func shellPredicate(ctx context.Context, command, workDir string) bool {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}
	return cmd.Run() == nil
}

// TruncateUTF8 safely truncates a string to at most maxBytes without splitting
// a multi-byte character.
func TruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk back from maxBytes to find a valid UTF-8 boundary
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes] + "... (truncated)"
}
