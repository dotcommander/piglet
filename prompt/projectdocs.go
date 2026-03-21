package prompt

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

const projectDocsOrder = 30 // before git context (40) and memory (50)

var defaultProjectDocs = []config.ProjectDoc{
	{Name: "CLAUDE.md", Title: "Project Instructions"},
	{Name: "agents.md", Title: "Agents"},
}

// RegisterProjectDocs reads markdown files from the repository root and
// injects each as a prompt section. Uses configured docs if provided,
// otherwise falls back to defaults. Silently skips files that don't exist.
func RegisterProjectDocs(app *ext.App, docs []config.ProjectDoc) {
	if len(docs) == 0 {
		docs = defaultProjectDocs
	}

	root := repoRoot(app.CWD())
	if root == "" {
		root = app.CWD()
	}

	for _, doc := range docs {
		data, err := os.ReadFile(filepath.Join(root, doc.Name))
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		app.RegisterPromptSection(ext.PromptSection{
			Title:   doc.Title,
			Content: content,
			Order:   projectDocsOrder,
		})
	}
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
