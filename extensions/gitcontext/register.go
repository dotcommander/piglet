// Package gitcontext injects uncommitted changes, recent commits, and
// small diffs into the system prompt so the model knows the repo state.
package gitcontext

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

const (
	promptOrder = 40 // before memory (50)

	defaultMaxDiffStatFiles  = 30
	defaultMaxLogLines       = 5
	defaultMaxDiffHunkLines  = 50
	defaultGitCommandTimeout = 5 * time.Second
)

// Register schedules OnInit work via OnInitAppend to build and inject
// git context into the system prompt.
func Register(e *sdk.Extension) {
	e.OnInitAppend(func(ext *sdk.Extension) {
		start := time.Now()
		ext.Log("debug", "[gitcontext] OnInit start")

		cwd := ext.CWD()
		content := buildGitContext(cwd, ext)
		ext.Log("debug", fmt.Sprintf("[gitcontext] git commands done (%s)", time.Since(start)))

		if content == "" {
			ext.Log("debug", fmt.Sprintf("[gitcontext] OnInit complete — no changes (%s)", time.Since(start)))
			return
		}
		ext.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Recent Changes",
			Content: content,
			Order:   promptOrder,
		})
		ext.Log("debug", fmt.Sprintf("[gitcontext] OnInit complete (%s)", time.Since(start)))
	})
}

func buildGitContext(cwd string, ext *sdk.Extension) string {
	timeout := defaultGitCommandTimeout

	var diffStat, log, hunks string
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		t := time.Now()
		diffStat = gitRun(cwd, timeout, "diff", "--stat")
		ext.Log("debug", fmt.Sprintf("[gitcontext] git diff --stat done (%s)", time.Since(t)))
	}()
	go func() {
		defer wg.Done()
		t := time.Now()
		log = gitRun(cwd, timeout, "log", "--oneline", fmt.Sprintf("-%d", defaultMaxLogLines))
		ext.Log("debug", fmt.Sprintf("[gitcontext] git log done (%s)", time.Since(t)))
	}()
	go func() {
		defer wg.Done()
		t := time.Now()
		hunks = gitRun(cwd, timeout, "diff", "--no-color")
		ext.Log("debug", fmt.Sprintf("[gitcontext] git diff --no-color done (%s)", time.Since(t)))
	}()
	wg.Wait()

	var b strings.Builder

	if diffStat != "" {
		b.WriteString("Uncommitted changes:\n```\n")
		b.WriteString(capDiffStat(diffStat, defaultMaxDiffStatFiles))
		b.WriteString("```\n\n")
	}

	if log != "" {
		b.WriteString("Recent commits:\n```\n")
		b.WriteString(log)
		b.WriteString("```\n\n")
	}

	if hunks != "" {
		lines := strings.Split(hunks, "\n")
		if len(lines) <= defaultMaxDiffHunkLines {
			b.WriteString("Diff:\n```diff\n")
			b.WriteString(hunks)
			b.WriteString("\n```\n")
		}
	}

	return strings.TrimSpace(b.String())
}

func capDiffStat(stat string, maxFiles int) string {
	lines := strings.Split(stat, "\n")
	if len(lines) <= maxFiles+1 {
		return stat
	}
	kept := lines[:maxFiles]
	remaining := len(lines) - maxFiles - 1
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
