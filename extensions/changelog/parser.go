package changelog

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var convCommitRegex = regexp.MustCompile(`^(\w+)(?:\(([^)]+)\))?(!)?:\s*(.+)`)

type Commit struct {
	Hash     string
	Date     string
	Author   string
	Subject  string
	Type     string
	Scope    string
	Message  string
	Breaking bool
}

// ParseCommits runs git log for the given ref and parses conventional commits.
func ParseCommits(cwd, ref string) ([]Commit, error) {
	cmd := exec.Command("git", "log", ref,
		"--pretty=format:%H|%ai|%an|%s",
		"--no-merges",
	)
	cmd.Dir = cwd

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s: %w", ref, err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	var commits []Commit
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		c, err := parseCommitLine(line)
		if err != nil {
			continue
		}
		commits = append(commits, c)
	}
	return commits, scanner.Err()
}

func parseCommitLine(line string) (Commit, error) {
	parts := strings.SplitN(line, "|", 4)
	if len(parts) < 4 {
		return Commit{}, fmt.Errorf("malformed line: %s", line)
	}

	hash := parts[0]
	if len(hash) > 7 {
		hash = hash[:7]
	}

	date := parts[1]
	if len(date) > 10 {
		date = date[:10]
	}

	subject := parts[3]
	c := Commit{
		Hash:    hash,
		Date:    date,
		Author:  parts[2],
		Subject: subject,
	}

	match := convCommitRegex.FindStringSubmatch(subject)
	if match != nil {
		c.Type = strings.ToLower(match[1])
		c.Scope = match[2]
		c.Breaking = match[3] == "!"
		c.Message = match[4]
	} else {
		c.Type = "other"
		c.Message = subject
	}

	return c, nil
}

// DetectRange determines the git revision range.
// Priority: explicit ref > last tag..HEAD > HEAD~fallbackCount..HEAD.
func DetectRange(cwd, ref string, fallbackCount int) string {
	if ref != "" {
		return ref
	}
	tag := lastTag(cwd)
	if tag != "" {
		return tag + "..HEAD"
	}
	return fmt.Sprintf("HEAD~%d..HEAD", fallbackCount)
}

func lastTag(cwd string) string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RepoURL extracts the origin remote URL and converts SSH to HTTPS.
func RepoURL(cwd string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))

	if strings.HasPrefix(url, "git@") {
		url = strings.TrimPrefix(url, "git@")
		url = strings.Replace(url, ":", "/", 1)
		url = "https://" + url
	}
	return strings.TrimSuffix(url, ".git")
}

// GroupCommits groups commits by Type.
func GroupCommits(commits []Commit) map[string][]Commit {
	groups := make(map[string][]Commit, len(commits))
	for _, c := range commits {
		groups[c.Type] = append(groups[c.Type], c)
	}
	return groups
}

// BreakingCommits filters for commits with Breaking=true.
func BreakingCommits(commits []Commit) []Commit {
	var out []Commit
	for _, c := range commits {
		if c.Breaking {
			out = append(out, c)
		}
	}
	return out
}
