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

	fmt.Fprintln(w, "Installing extensions from piglet-extensions...")

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

	var built, failed int

	for _, name := range officialExtensions {
		destDir := filepath.Join(extDir, name)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			fmt.Fprintf(w, "  %s: FAILED (mkdir: %v)\n", name, err)
			failed++
			continue
		}

		buildCmd := exec.Command("go", "build", "-o", filepath.Join(destDir, name), "./"+name+"/cmd/")
		buildCmd.Dir = tmpDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(w, "  %s: FAILED (%s)\n", name, strings.TrimSpace(string(out)))
			failed++
			continue
		}

		src := filepath.Join(tmpDir, name, "cmd", "manifest.yaml")
		dst := filepath.Join(destDir, "manifest.yaml")
		data, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(w, "  %s: FAILED (manifest: %v)\n", name, err)
			failed++
			continue
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			fmt.Fprintf(w, "  %s: FAILED (write manifest: %v)\n", name, err)
			failed++
			continue
		}

		fmt.Fprintf(w, "  %s: OK\n", name)
		built++
	}

	fmt.Fprintf(w, "\n%d built, %d failed\n", built, failed)
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
