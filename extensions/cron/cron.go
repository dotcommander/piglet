package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// RunOptions configures a cron run.
type RunOptions struct {
	Verbose  bool
	TaskName string // empty = run all due tasks
	Force    bool   // bypass schedule check
}

// taskResult carries the outcome of a single task execution.
type taskResult struct {
	name   string
	result ExecuteResult
}

// Run executes the cron cycle: load config, check schedules, execute due tasks.
func Run(ctx context.Context, opts RunOptions) error {
	cfg := LoadConfig()

	if len(cfg.Tasks) == 0 {
		if opts.Verbose {
			slog.Info("no tasks configured")
		}
		return nil
	}

	entries, err := ReadHistory()
	if err != nil {
		slog.Warn("reading history", "error", err)
		// Continue with empty history — all tasks will appear as first run.
	}

	type taskRun struct {
		name   string
		config TaskConfig
		sched  Schedule
	}

	var toRun []taskRun

	for name, task := range cfg.Tasks {
		if !task.IsEnabled() {
			if opts.Verbose {
				slog.Info("skipping disabled task", "task", name)
			}
			continue
		}

		// If targeting a specific task, skip others.
		if opts.TaskName != "" && opts.TaskName != name {
			continue
		}

		sched, err := ParseSchedule(task.Schedule)
		if err != nil {
			slog.Error("invalid schedule", "task", name, "error", err)
			continue
		}

		if opts.Force {
			toRun = append(toRun, taskRun{name: name, config: task, sched: sched})
			continue
		}

		lastRunTime := LastRun(entries, name)
		if sched.ShouldRun(lastRunTime) {
			toRun = append(toRun, taskRun{name: name, config: task, sched: sched})
		} else if opts.Verbose {
			next := sched.Next(lastRunTime)
			slog.Info("not due", "task", name, "next", next.Format(time.RFC3339))
		}
	}

	if len(toRun) == 0 {
		if opts.Verbose {
			slog.Info("no tasks due")
		}
		return nil
	}

	// Execute due tasks in parallel — all run to completion regardless of failures.
	var mu sync.Mutex
	collected := make([]taskResult, 0, len(toRun))

	eg, egCtx := errgroup.WithContext(ctx)
	for _, tr := range toRun {
		eg.Go(func() error {
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					collected = append(collected, taskResult{
						name:   tr.name,
						result: ExecuteResult{Error: fmt.Sprintf("panic: %v", r)},
					})
					mu.Unlock()
				}
			}()

			if opts.Verbose {
				slog.Info("running", "task", tr.name, "action", tr.config.Action)
			}

			res := Execute(egCtx, tr.name, tr.config)
			mu.Lock()
			collected = append(collected, taskResult{name: tr.name, result: res})
			mu.Unlock()
			return nil // never cancel siblings on task failure
		})
	}
	eg.Wait() //nolint:errcheck // always nil — goroutines never return non-nil

	var appendErr error
	for _, r := range collected {
		entry := RunEntry{
			Task:       r.name,
			RanAt:      nowFunc().Format(time.RFC3339),
			Success:    r.result.Success,
			DurationMs: r.result.DurationMs,
			Error:      r.result.Error,
		}

		if opts.Verbose {
			if r.result.Success {
				slog.Info("completed", "task", r.name, "duration_ms", r.result.DurationMs)
			} else {
				slog.Error("failed", "task", r.name, "error", r.result.Error)
			}
		}

		if err := AppendHistory(entry); err != nil {
			appendErr = err
			slog.Error("recording result", "task", r.name, "error", err)
		}
	}

	// Rotate history if needed.
	if err := RotateHistory(); err != nil {
		slog.Warn("rotating history", "error", err)
	}

	return appendErr
}

// ListTasks returns a summary of all configured tasks with their last run info.
func ListTasks() ([]TaskSummary, error) {
	cfg := LoadConfig()
	entries, _ := ReadHistory()

	var summaries []TaskSummary
	for name, task := range cfg.Tasks {
		sched, err := ParseSchedule(task.Schedule)
		if err != nil {
			continue
		}

		lastRunTime := LastRun(entries, name)
		var nextRun time.Time
		if lastRunTime.IsZero() {
			nextRun = nowFunc()
		} else {
			nextRun = sched.Next(lastRunTime)
		}

		overdue := !lastRunTime.IsZero() && sched.ShouldRun(lastRunTime)

		summaries = append(summaries, TaskSummary{
			Name:     name,
			Action:   task.Action,
			Schedule: sched.String(),
			Enabled:  task.IsEnabled(),
			LastRun:  lastRunTime,
			NextRun:  nextRun,
			Overdue:  overdue,
		})
	}
	return summaries, nil
}

// TaskSummary provides a view of a task's state.
type TaskSummary struct {
	Name     string
	Action   string
	Schedule string
	Enabled  bool
	LastRun  time.Time
	NextRun  time.Time
	Overdue  bool
}
