// Package prompts scans prompt template directories for .md files and
// registers each as a slash command with positional argument expansion.
package prompts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
	"gopkg.in/yaml.v3"
)

// Register schedules OnInit work via OnInitAppend to scan prompt directories
// and register one command per .md file found.
func Register(e *sdk.Extension) {
	e.OnInitAppend(func(ext *sdk.Extension) {
		start := time.Now()
		ext.Log("debug", "[prompts] OnInit start")

		prompts := make(map[string]promptEntry)

		// Global prompts (lower priority)
		cfgDir, _ := xdg.ConfigDir()
		if cfgDir != "" {
			loadPromptDir(filepath.Join(cfgDir, "prompts"), prompts)
		}

		// Project-local prompts (higher priority — overwrites global on collision)
		loadPromptDir(filepath.Join(ext.CWD(), ".piglet", "prompts"), prompts)

		for _, entry := range prompts {
			en := entry // capture
			ext.RegisterCommand(sdk.CommandDef{
				Name:        en.name,
				Description: en.description,
				Handler: func(_ context.Context, args string) error {
					parts := strings.Fields(args)
					expanded := expandTemplate(en.body, parts)
					e.SendMessage(expanded)
					return nil
				},
			})
		}

		ext.Log("debug", fmt.Sprintf("[prompts] OnInit complete — %d prompt(s) registered (%s)", len(prompts), time.Since(start)))
	})
}

// promptFrontmatter holds optional YAML frontmatter fields.
type promptFrontmatter struct {
	Description string `yaml:"description"`
}

type promptEntry struct {
	name        string
	description string
	body        string
}

// loadPromptDir reads all .md files from dir and adds them to the map.
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
func parsePromptFile(data []byte) (description, body string) {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return "", strings.TrimSpace(content)
	}
	rest := content[4:]

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
		afterClose = rest[idx+4:]
	}

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
func expandTemplate(body string, args []string) string {
	result := reSliceArgs.ReplaceAllStringFunc(body, func(match string) string {
		sub := reSliceArgs.FindStringSubmatch(match)
		n, _ := strconv.Atoi(sub[1])
		idx := n - 1
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

	result = strings.ReplaceAll(result, "$@", strings.Join(args, " "))

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
