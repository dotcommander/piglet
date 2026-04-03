package changelog

import (
	"fmt"
	"strings"
	"time"
)

const (
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"
	ansiRed   = "\033[31m"
	ansiReset = "\033[0m"
)

// FormatANSI renders commits as colored terminal output.
func FormatANSI(commits []Commit, ref string, types map[string]TypeConfig) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%sChangelog%s %s(%s)%s\n\n",
		ansiBold, ansiReset, ansiDim, ref, ansiReset)

	breaking := BreakingCommits(commits)
	if len(breaking) > 0 {
		fmt.Fprintf(&b, "%s%s⚠️  BREAKING CHANGES%s\n",
			ansiRed, ansiBold, ansiReset)
		for _, c := range breaking {
			scope := ""
			if c.Scope != "" {
				scope = fmt.Sprintf("(%s) ", c.Scope)
			}
			fmt.Fprintf(&b, "  • %s%s %s(%s)%s\n",
				scope, c.Message, ansiDim, c.Hash, ansiReset)
		}
		b.WriteByte('\n')
	}

	groups := GroupCommits(commits)
	for _, key := range typeOrder(types) {
		group := groups[key]
		if len(group) == 0 {
			continue
		}
		cfg := types[key]
		fmt.Fprintf(&b, "%s%s %s%s\n", ansiBold, cfg.Emoji, cfg.Label, ansiReset)
		for _, c := range group {
			scope := ""
			if c.Scope != "" {
				scope = fmt.Sprintf("(%s) ", c.Scope)
			}
			fmt.Fprintf(&b, "  • %s%s %s(%s)%s\n",
				scope, c.Message, ansiDim, c.Hash, ansiReset)
		}
		b.WriteByte('\n')
	}

	fmt.Fprintf(&b, "%s%d commits%s\n", ansiDim, len(commits), ansiReset)
	return b.String()
}

// FormatMarkdown renders commits as Markdown for CHANGELOG.md.
func FormatMarkdown(commits []Commit, ref, repoURL string, types map[string]TypeConfig) string {
	var b strings.Builder
	date := time.Now().Format("2006-01-02")

	fmt.Fprintf(&b, "## %s (%s)\n\n", ref, date)

	breaking := BreakingCommits(commits)
	if len(breaking) > 0 {
		b.WriteString("### ⚠️ BREAKING CHANGES\n\n")
		for _, c := range breaking {
			scope := ""
			if c.Scope != "" {
				scope = fmt.Sprintf("**%s:** ", c.Scope)
			}
			fmt.Fprintf(&b, "- %s%s (%s)\n", scope, c.Message, commitLink(c.Hash, repoURL))
		}
		b.WriteByte('\n')
	}

	groups := GroupCommits(commits)
	for _, key := range typeOrder(types) {
		group := groups[key]
		if len(group) == 0 {
			continue
		}
		cfg := types[key]
		fmt.Fprintf(&b, "### %s %s\n\n", cfg.Emoji, cfg.Label)
		for _, c := range group {
			scope := ""
			if c.Scope != "" {
				scope = fmt.Sprintf("**%s:** ", c.Scope)
			}
			fmt.Fprintf(&b, "- %s%s (%s)\n", scope, c.Message, commitLink(c.Hash, repoURL))
		}
		b.WriteByte('\n')
	}

	return b.String()
}

func commitLink(hash, repoURL string) string {
	if repoURL == "" {
		return hash
	}
	return fmt.Sprintf("[%s](%s/commit/%s)", hash, repoURL, hash)
}
