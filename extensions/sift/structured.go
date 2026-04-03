package sift

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

func CompressStructured(toolName, text string, cfg Config, cwd string) (string, bool) {
	if len(cfg.Structured) == 0 {
		return "", false
	}

	rule := matchRule(toolName, text, cfg.Structured)
	if rule == nil {
		return "", false
	}

	rows := parseLinterOutput(text, rule.Detect, cwd)
	if len(rows) == 0 {
		return "", false
	}

	if rule.SortBy != "" {
		sortRows(rows, rule.Columns, rule.SortBy)
	}

	if rule.MaxRows > 0 && len(rows) > rule.MaxRows {
		rows = rows[:rule.MaxRows]
	}

	table := buildTable(rule.Columns, rows)
	header := fmt.Sprintf("[SIFT: structured — %d findings extracted from %d bytes]\n", len(rows), len(text))
	return header + table, true
}

func matchRule(toolName, text string, rules []StructuredRule) *StructuredRule {
	for i := range rules {
		r := &rules[i]
		if r.Tool == toolName && strings.Contains(text, r.Detect) {
			return r
		}
	}
	return nil
}

type row map[string]string

var (
	reFileLineCol = regexp.MustCompile(`^(.+?):(\d+):(\d+):\s+(.+)$`)
	reFileLine    = regexp.MustCompile(`^(.+?):(\d+):\s+(.+)$`)
	reFileParen   = regexp.MustCompile(`^(.+?)\((\d+)\):\s+(.+)$`)
)

func parseLinterOutput(text, detect, cwd string) []row {
	lines := strings.Split(text, "\n")
	var rows []row

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		r := parseLine(line, detect)
		if r == nil {
			continue
		}

		if cwd != "" {
			if rel, err := deprefixPath(r["file"], cwd); err == nil {
				r["file"] = rel
			}
		}

		if msg, ok := r["message"]; ok {
			runes := []rune(msg)
			if len(runes) > 80 {
				r["message"] = string(runes[:77]) + "..."
			}
		}

		rows = append(rows, r)
	}

	return rows
}

func parseLine(line, detect string) row {
	var r row

	if m := reFileLineCol.FindStringSubmatch(line); m != nil {
		r = row{"file": m[1], "line": m[2], "message": m[4]}
		r["col"] = m[3]
		if detect == "golangci-lint" {
			if linter := extractLinter(m[4]); linter != "" {
				r["linter"] = linter
				r["message"] = trimLinter(m[4], linter)
			}
		}
		return r
	}

	if m := reFileLine.FindStringSubmatch(line); m != nil {
		r = row{"file": m[1], "line": m[2], "message": m[3]}
		return r
	}

	if m := reFileParen.FindStringSubmatch(line); m != nil {
		r = row{"file": m[1], "line": m[2], "message": m[3]}
		return r
	}

	return nil
}

func extractLinter(message string) string {
	idx := strings.LastIndex(message, " (")
	if idx < 0 {
		return ""
	}
	candidate := message[idx+2:]
	close := strings.Index(candidate, ")")
	if close < 0 {
		return ""
	}
	name := candidate[:close]
	if strings.Contains(name, " ") {
		return ""
	}
	return name
}

func trimLinter(message, linter string) string {
	suffix := " (" + linter + ")"
	return strings.TrimSuffix(message, suffix)
}

func deprefixPath(path, prefix string) (string, error) {
	if !strings.HasPrefix(prefix, "/") {
		return path, fmt.Errorf("prefix not absolute")
	}
	clean := strings.TrimRight(prefix, "/") + "/"
	if strings.HasPrefix(path, clean) {
		return strings.TrimPrefix(path, clean), nil
	}
	return path, fmt.Errorf("no prefix match")
}

func sortRows(rows []row, columns []string, sortBy string) {
	if slices.Index(columns, sortBy) < 0 {
		return
	}
	slices.SortStableFunc(rows, func(a, b row) int {
		return severityRank(a[sortBy]) - severityRank(b[sortBy])
	})
}

func severityRank(s string) int {
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "error"):
		return 0
	case strings.HasPrefix(lower, "warn"):
		return 1
	case strings.HasPrefix(lower, "info"):
		return 2
	default:
		return 3
	}
}

func buildTable(columns []string, rows []row) string {
	var b strings.Builder

	b.WriteByte('|')
	for _, col := range columns {
		b.WriteByte(' ')
		b.WriteString(col)
		b.WriteByte(' ')
		b.WriteByte('|')
	}
	b.WriteByte('\n')

	b.WriteByte('|')
	for range columns {
		b.WriteString("------|")
	}
	b.WriteByte('\n')

	for _, r := range rows {
		b.WriteByte('|')
		for _, col := range columns {
			b.WriteByte(' ')
			b.WriteString(r[col])
			b.WriteByte(' ')
			b.WriteByte('|')
		}
		b.WriteByte('\n')
	}

	return b.String()
}
