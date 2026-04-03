package sessiontools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// memoryFact mirrors the memory extension's Fact struct for reading JSONL.
type memoryFact struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Category  string    `json:"category,omitzero"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BuildSummary reads memory facts for the given cwd and produces a structured
// markdown handoff summary grouped by category and prefix.
// It returns the summary text, the raw facts (for caller-side enhance decisions), and any error.
func BuildSummary(cwd string) (summary string, facts []memoryFact, err error) {
	path, err := MemoryStorePath(cwd)
	if err != nil {
		return "", nil, fmt.Errorf("build summary: %w", err)
	}

	facts, err = readFacts(path)
	if err != nil {
		return "", nil, fmt.Errorf("build summary: %w", err)
	}

	if len(facts) == 0 {
		return "No memory facts available for this project.", nil, nil
	}

	return formatSummary(facts), facts, nil
}

func readFacts(path string) ([]memoryFact, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open memory file: %w", err)
	}
	defer f.Close()

	var facts []memoryFact
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var fact memoryFact
		if err := json.Unmarshal(line, &fact); err != nil {
			continue // skip malformed lines
		}
		facts = append(facts, fact)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read memory file: %w", err)
	}

	// Sort by key for deterministic output
	slices.SortFunc(facts, func(a, b memoryFact) int {
		return strings.Compare(a.Key, b.Key)
	})

	return facts, nil
}

func formatSummary(facts []memoryFact) string {
	// Single-pass categorization by key prefix.
	buckets := map[string][]memoryFact{
		"ctx:goal":     nil,
		"ctx:edit":     nil,
		"ctx:file":     nil,
		"ctx:error":    nil,
		"ctx:cmd":      nil,
		"ctx:decision": nil,
	}
	var other []memoryFact

	for _, f := range facts {
		matched := false
		for prefix := range buckets {
			if strings.HasPrefix(f.Key, prefix) {
				buckets[prefix] = append(buckets[prefix], f)
				matched = true
				break
			}
		}
		if !matched {
			other = append(other, f)
		}
	}

	var b strings.Builder
	b.WriteString("# Session Handoff Summary\n\n")

	writeSection := func(heading string, items []memoryFact, fmtFn func(f memoryFact) string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(heading)
		b.WriteByte('\n')
		for _, f := range items {
			b.WriteString(fmtFn(f))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	bullet := func(f memoryFact) string { return "- " + f.Value }
	code := func(f memoryFact) string { return "- `" + f.Value + "`" }
	keyed := func(f memoryFact) string { return fmt.Sprintf("- **%s**: %s", f.Key, f.Value) }

	writeSection("## Goal\n", buckets["ctx:goal"], bullet)
	writeSection("## Progress\n", buckets["ctx:edit"], bullet)
	writeSection("## Key Decisions\n", buckets["ctx:decision"], bullet)

	files, cmds := buckets["ctx:file"], buckets["ctx:cmd"]
	if len(files) > 0 || len(cmds) > 0 {
		b.WriteString("## Context\n\n")
		writeSection("### Files\n", files, bullet)
		writeSection("### Commands\n", cmds, code)
	}

	writeSection("## Errors Encountered\n", buckets["ctx:error"], bullet)
	writeSection("## Other Facts\n", other, keyed)

	return strings.TrimRight(b.String(), "\n")
}
