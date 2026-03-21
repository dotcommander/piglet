package prompt

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/piglet/ext"
)

const gitContextOrder = 40 // before memory (50)

// Default git context limits.
const (
	defaultMaxDiffStatFiles  = 30
	defaultMaxLogLines       = 5
	defaultMaxDiffHunkLines  = 50
	defaultGitCommandTimeout = 5 * time.Second
)

// GitContextConfig holds configurable limits for git context injection.
type GitContextConfig struct {
	MaxDiffStatFiles int
	MaxLogLines      int
	MaxDiffHunkLines int
	CommandTimeout   time.Duration
}

func (c GitContextConfig) withDefaults() GitContextConfig {
	if c.MaxDiffStatFiles <= 0 {
		c.MaxDiffStatFiles = defaultMaxDiffStatFiles
	}
	if c.MaxLogLines <= 0 {
		c.MaxLogLines = defaultMaxLogLines
	}
	if c.MaxDiffHunkLines <= 0 {
		c.MaxDiffHunkLines = defaultMaxDiffHunkLines
	}
	if c.CommandTimeout <= 0 {
		c.CommandTimeout = defaultGitCommandTimeout
	}
	return c
}

// RegisterGitContext registers a "Recent Changes" prompt section if cwd is a git repo.
// Silently skips if git is not available or cwd is not a repo.
func RegisterGitContext(app *ext.App, cfg GitContextConfig) {
	cfg = cfg.withDefaults()
	cwd := app.CWD()

	content := buildGitContext(cwd, cfg)
	if content == "" {
		return
	}

	app.RegisterPromptSection(ext.PromptSection{
		Title:   "Recent Changes",
		Content: content,
		Order:   gitContextOrder,
	})
}

func buildGitContext(cwd string, cfg GitContextConfig) string {
	// Run git commands in parallel
	var diffStat, log, hunks string
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		diffStat = gitRun(cwd, cfg.CommandTimeout, "diff", "--stat")
	}()
	go func() {
		defer wg.Done()
		log = gitRun(cwd, cfg.CommandTimeout, "log", "--oneline", fmt.Sprintf("-%d", cfg.MaxLogLines))
	}()
	go func() {
		defer wg.Done()
		hunks = gitRun(cwd, cfg.CommandTimeout, "diff", "--no-color")
	}()
	wg.Wait()

	var b strings.Builder

	if diffStat != "" {
		b.WriteString("Uncommitted changes:\n```\n")
		b.WriteString(capDiffStat(diffStat, cfg.MaxDiffStatFiles))
		b.WriteString("```\n\n")
	}

	if log != "" {
		b.WriteString("Recent commits:\n```\n")
		b.WriteString(log)
		b.WriteString("```\n\n")
	}

	// Include actual diff hunks for small changes
	if hunks != "" {
		lines := strings.Split(hunks, "\n")
		if len(lines) <= cfg.MaxDiffHunkLines {
			b.WriteString("Diff:\n```diff\n")
			b.WriteString(hunks)
			b.WriteString("\n```\n")
		}
	}

	return strings.TrimSpace(b.String())
}

// capDiffStat truncates diff stat output to maxFiles lines.
func capDiffStat(stat string, maxFiles int) string {
	lines := strings.Split(stat, "\n")
	if len(lines) <= maxFiles+1 {
		return stat
	}
	kept := lines[:maxFiles]
	remaining := len(lines) - maxFiles - 1 // exclude summary line
	summary := lines[len(lines)-1]
	kept = append(kept, fmt.Sprintf(" ... and %d more files", remaining))
	kept = append(kept, summary)
	return strings.Join(kept, "\n")
}

func gitRun(cwd string, timeout time.Duration, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
