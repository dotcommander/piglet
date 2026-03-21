package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

func grepTool(app *ext.App, cfg ToolConfig) *ext.ToolDef {
	return &ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "grep",
			Description: "Search file contents using regex. Returns matching lines with file paths and line numbers.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern":     map[string]any{"type": "string", "description": "Regex pattern to search for"},
					"path":        map[string]any{"type": "string", "description": "Directory or file to search (default: cwd)"},
					"glob":        map[string]any{"type": "string", "description": "File glob filter (e.g. \"*.go\")"},
					"ignore_case": map[string]any{"type": "boolean", "description": "Case-insensitive search"},
					"limit":       map[string]any{"type": "integer", "description": "Max matches (default 100)"},
				},
				"required": []string{"pattern"},
			},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return textResult("error: pattern is required"), nil
			}

			searchPath := stringArg(args, "path", app.CWD())
			searchPath = resolvePath(app.CWD(), searchPath)
			globPattern := stringArg(args, "glob", "")
			ignoreCase := boolArg(args, "ignore_case", false)
			limit := intArg(args, "limit", cfg.grepLimit())

			if ignoreCase {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return textResult(fmt.Sprintf("error: invalid regex: %v", err)), nil
			}

			info, err := os.Stat(searchPath)
			if err != nil {
				return textResult(fmt.Sprintf("error: %v", err)), nil
			}

			var matches []string
			matchCount := 0

			if !info.IsDir() {
				matches = grepFile(re, searchPath, "", limit, &matchCount)
			} else {
				filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
					if err != nil || d.IsDir() {
						if d != nil && d.IsDir() && shouldSkipDir(d.Name()) {
							return filepath.SkipDir
						}
						return nil
					}
					if matchCount >= limit {
						return filepath.SkipAll
					}
					if globPattern != "" {
						matched, _ := filepath.Match(globPattern, d.Name())
						if !matched {
							return nil
						}
					}
					rel, _ := filepath.Rel(searchPath, path)
					results := grepFile(re, path, rel, limit-matchCount, &matchCount)
					matches = append(matches, results...)
					return nil
				})
			}

			if len(matches) == 0 {
				return textResult("no matches found"), nil
			}

			var b strings.Builder
			for _, m := range matches {
				b.WriteString(m)
				b.WriteString("\n")
			}
			if matchCount >= limit {
				fmt.Fprintf(&b, "\n... (limit of %d matches reached)", limit)
			}
			return textResult(b.String()), nil
		},
		PromptHint:     "Search file contents with regex",
		PromptGuides:   []string{"Use glob to filter file types", "Default limit is 100 matches"},
		BackgroundSafe: true,
	}
}

func grepFile(re *regexp.Regexp, path, displayPath string, limit int, count *int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	if displayPath == "" {
		displayPath = path
	}

	var results []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if *count >= limit {
			break
		}
		line := scanner.Text()
		if re.MatchString(line) {
			if len(line) > 200 {
				line = line[:200] + "..."
			}
			results = append(results, fmt.Sprintf("%s:%d:%s", displayPath, lineNum, line))
			*count++
		}
	}
	return results
}
