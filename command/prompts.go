package command

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
	"gopkg.in/yaml.v3"
)

// promptFrontmatter holds optional YAML frontmatter fields.
type promptFrontmatter struct {
	Description string `yaml:"description"`
}

// RegisterPrompts scans prompt template directories and registers each .md file
// as a slash command. Project-local prompts (.piglet/prompts/) override global
// prompts (~/.config/piglet/prompts/) when names collide.
func RegisterPrompts(app *ext.App) {
	prompts := make(map[string]promptEntry) // name → entry

	// Global prompts (lower priority)
	if dir, err := config.ConfigDir(); err == nil {
		loadPromptDir(filepath.Join(dir, "prompts"), prompts)
	}

	// Project-local prompts (higher priority — overwrites global on collision)
	loadPromptDir(filepath.Join(app.CWD(), ".piglet", "prompts"), prompts)

	for _, entry := range prompts {
		e := entry // capture
		app.RegisterCommand(&ext.Command{
			Name:        e.name,
			Description: e.description,
			Handler: func(args string, a *ext.App) error {
				parts := strings.Fields(args)
				expanded := expandTemplate(e.body, parts)
				a.SendMessage(expanded)
				return nil
			},
		})
	}
}

type promptEntry struct {
	name        string
	description string
	body        string
}

// loadPromptDir reads all .md files from dir and adds them to the map.
// Silently skips if the directory does not exist.
func loadPromptDir(dir string, out map[string]promptEntry) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		desc, body := parsePromptFile(data)
		if desc == "" {
			desc = "Prompt template: " + name
		}
		out[name] = promptEntry{name: name, description: desc, body: body}
	}
}

// parsePromptFile splits optional YAML frontmatter from the markdown body.
// Frontmatter is delimited by "---" on the first line and a closing "---".
func parsePromptFile(data []byte) (description, body string) {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return "", strings.TrimSpace(content)
	}
	rest := content[4:] // skip opening "---\n"

	// Find closing delimiter: either at start of rest (empty frontmatter)
	// or preceded by a newline.
	var fmRaw, afterClose string
	if strings.HasPrefix(rest, "---") {
		fmRaw = ""
		afterClose = rest[3:]
	} else {
		idx := strings.Index(rest, "\n---")
		if idx < 0 {
			return "", strings.TrimSpace(content)
		}
		fmRaw = rest[:idx]
		afterClose = rest[idx+4:] // skip "\n---"
	}

	// Consume optional newline after closing delimiter.
	afterClose = strings.TrimPrefix(afterClose, "\n")
	body = strings.TrimSpace(afterClose)

	var fm promptFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return "", strings.TrimSpace(content)
	}
	return fm.Description, body
}

// reSliceArgs matches ${@:N} and ${@:N:L} patterns.
var reSliceArgs = regexp.MustCompile(`\$\{@:(\d+)(?::(\d+))?\}`)

// expandTemplate replaces positional arg placeholders in a template body.
//
//   - $1..$9 — individual positional args (empty string if missing)
//   - $@ — all args joined by space
//   - ${@:N} — args from position N onward (1-indexed)
//   - ${@:N:L} — L args starting from position N (1-indexed)
func expandTemplate(body string, args []string) string {
	// Replace ${@:N:L} and ${@:N} first (longer patterns before $@)
	result := reSliceArgs.ReplaceAllStringFunc(body, func(match string) string {
		sub := reSliceArgs.FindStringSubmatch(match)
		n, _ := strconv.Atoi(sub[1])
		idx := n - 1 // convert to 0-based
		if idx < 0 || idx >= len(args) {
			return ""
		}
		if sub[2] != "" {
			l, _ := strconv.Atoi(sub[2])
			end := idx + l
			if end > len(args) {
				end = len(args)
			}
			return strings.Join(args[idx:end], " ")
		}
		return strings.Join(args[idx:], " ")
	})

	// Replace $@ with all args
	result = strings.ReplaceAll(result, "$@", strings.Join(args, " "))

	// Replace $1..$9 (descending to avoid $1 matching inside $10-like text)
	for i := 9; i >= 1; i-- {
		placeholder := "$" + strconv.Itoa(i)
		val := ""
		if i-1 < len(args) {
			val = args[i-1]
		}
		result = strings.ReplaceAll(result, placeholder, val)
	}

	return result
}
