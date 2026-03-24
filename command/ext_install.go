package command

import (
	"fmt"
	"io"
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

func InstallOfficialExtensions(w io.Writer) error {
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(w, "git not found in PATH — skipping extension install")
		return nil
	}
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(w, "go not found in PATH — skipping extension install")
		return nil
	}

	extDir, err := external.ExtensionsDir()
	if err != nil {
		return fmt.Errorf("extensions dir: %w", err)
	}

	total := len(officialExtensions)
	fmt.Fprintf(w, "Cloning piglet-extensions...\n")

	tmpDir, err := os.MkdirTemp("", "piglet-extensions-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneCmd := exec.Command("git", "clone", "--depth", "1", "--quiet", extensionsRepoURL, tmpDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(w, "Failed to clone extensions repo:\n%s\n", string(out))
		return nil
	}

	// Strip local replace directives that break builds outside the dev machine
	dropReplace := exec.Command("go", "mod", "edit",
		"-dropreplace=github.com/dotcommander/piglet/sdk",
	)
	dropReplace.Dir = tmpDir
	_ = dropReplace.Run()

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = tmpDir
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(w, "Warning: go mod tidy failed: %s\n", strings.TrimSpace(string(out)))
	}

	var built, failed int
	var failures []string

	// Use \r for TTY (os.Stderr), newline for non-TTY (strings.Builder in TUI path)
	isTTY := w == os.Stderr
	for i, name := range officialExtensions {
		if isTTY {
			fmt.Fprintf(w, "\rBuilding extensions (%d/%d) %s...\033[K", i+1, total, name)
		}

		destDir := filepath.Join(extDir, name)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			failures = append(failures, fmt.Sprintf("  %s: mkdir: %v", name, err))
			failed++
			continue
		}

		buildCmd := exec.Command("go", "build", "-o", filepath.Join(destDir, name), "./"+name+"/cmd/")
		buildCmd.Dir = tmpDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			failures = append(failures, fmt.Sprintf("  %s: %s", name, strings.TrimSpace(string(out))))
			failed++
			continue
		}

		src := filepath.Join(tmpDir, name, "cmd", "manifest.yaml")
		dst := filepath.Join(destDir, "manifest.yaml")
		data, err := os.ReadFile(src)
		if err != nil {
			failures = append(failures, fmt.Sprintf("  %s: manifest: %v", name, err))
			failed++
			continue
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			failures = append(failures, fmt.Sprintf("  %s: write manifest: %v", name, err))
			failed++
			continue
		}

		built++
	}

	if isTTY {
		fmt.Fprintf(w, "\r\033[K")
	}
	fmt.Fprintf(w, "Extensions: %d built, %d failed\n", built, failed)
	for _, f := range failures {
		fmt.Fprintln(w, f)
	}
	return nil
}

func installExtensions(a *ext.App) error {
	var b strings.Builder
	if err := InstallOfficialExtensions(&b); err != nil {
		return err
	}
	if msg := b.String(); msg != "" {
		a.ShowMessage(msg)
	}
	return nil
}
