package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/ext/external"
)

const extensionsRepoURL = "https://github.com/dotcommander/piglet-extensions.git"

var officialExtensions = []string{
	"safeguard", "rtk", "autotitle", "clipboard", "skill",
	"memory", "subagent", "lsp", "repomap", "plan", "bulk",
}

func installExtensions(a *ext.App) error {
	if _, err := exec.LookPath("git"); err != nil {
		a.ShowMessage("git not found in PATH. Install git to use /extensions install.")
		return nil
	}
	if _, err := exec.LookPath("go"); err != nil {
		a.ShowMessage("go not found in PATH. Install Go to use /extensions install.")
		return nil
	}

	extDir, err := external.ExtensionsDir()
	if err != nil {
		return fmt.Errorf("extensions dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "piglet-extensions-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneCmd := exec.Command("git", "clone", "--depth", "1", "--quiet", extensionsRepoURL, tmpDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		a.ShowMessage("Failed to clone extensions repo:\n" + string(out))
		return nil
	}

	var results []string
	var built, failed int

	for _, name := range officialExtensions {
		destDir := filepath.Join(extDir, name)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			results = append(results, fmt.Sprintf("  %s: FAILED (mkdir: %v)", name, err))
			failed++
			continue
		}

		buildCmd := exec.Command("go", "build", "-o", filepath.Join(destDir, name), "./"+name+"/cmd/")
		buildCmd.Dir = tmpDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			results = append(results, fmt.Sprintf("  %s: FAILED (%s)", name, strings.TrimSpace(string(out))))
			failed++
			continue
		}

		src := filepath.Join(tmpDir, name, "cmd", "manifest.yaml")
		dst := filepath.Join(destDir, "manifest.yaml")
		data, err := os.ReadFile(src)
		if err != nil {
			results = append(results, fmt.Sprintf("  %s: FAILED (manifest: %v)", name, err))
			failed++
			continue
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			results = append(results, fmt.Sprintf("  %s: FAILED (write manifest: %v)", name, err))
			failed++
			continue
		}

		results = append(results, fmt.Sprintf("  %s: OK", name))
		built++
	}

	var b strings.Builder
	b.WriteString("Extension install complete:\n\n")
	b.WriteString(strings.Join(results, "\n"))
	fmt.Fprintf(&b, "\n\n%d built, %d failed", built, failed)
	if built > 0 {
		b.WriteString("\nRestart piglet to load new extensions.")
	}
	a.ShowMessage(b.String())
	return nil
}
