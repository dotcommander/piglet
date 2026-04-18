// Package projectdocs reads markdown files listed in the projectdocs extension
// config and injects each as a system prompt section. It replaces the former
// compiled-in prompt.RegisterProjectDocs function.
package projectdocs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/sdk"
)

const projectDocsOrder = 30 // before git context (40) and memory (50)

const maxProjectDocSize = 512 << 10 // 512 KB

// Doc maps a filename (relative to the repo root) to a prompt section title.
type Doc struct {
	Name  string `yaml:"name"`
	Title string `yaml:"title"`
}

// config holds the extension's YAML configuration.
type config struct {
	Docs []Doc `yaml:"docs"`
}

// defaultConfig returns the default document list.
func defaultConfig() config {
	return config{
		Docs: []Doc{
			{Name: "CLAUDE.md", Title: "Project Instructions"},
			{Name: "agents.md", Title: "Agents"},
		},
	}
}

// Register schedules OnInit work to load and inject project documentation.
func Register(e *sdk.Extension) {
	e.OnInitAppend(func(ext *sdk.Extension) {
		start := time.Now()
		ext.Log("debug", "[projectdocs] OnInit start")

		cfg := xdg.LoadYAMLExt("projectdocs", "config.yaml", defaultConfig())
		if len(cfg.Docs) == 0 {
			ext.Log("debug", fmt.Sprintf("[projectdocs] OnInit complete — no docs configured (%s)", time.Since(start)))
			return
		}

		root := repoRoot(ext.CWD())
		if root == "" {
			root = ext.CWD()
		}

		count := 0
		for _, doc := range cfg.Docs {
			path := filepath.Join(root, doc.Name)
			info, err := os.Stat(path)
			if err != nil || info.Size() > maxProjectDocSize {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content := strings.TrimSpace(string(data))
			if content == "" {
				continue
			}
			ext.RegisterPromptSection(sdk.PromptSectionDef{
				Title:   doc.Title,
				Content: content,
				Order:   projectDocsOrder,
			})
			count++
		}

		ext.Log("debug", fmt.Sprintf("[projectdocs] OnInit complete — %d section(s) registered (%s)", count, time.Since(start)))
	})
}

// repoRoot walks up from dir to find the nearest .git directory.
func repoRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
