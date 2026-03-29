package command

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext/external"
)

func InstallOfficialExtensions(w io.Writer, settings config.Settings) error {
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

	extensions := settings.ExtInstall.ResolveOfficial()
	repoURL := settings.ExtInstall.ResolveRepoURL()
	fmt.Fprintf(w, "Cloning piglet-extensions...\n")

	tmpDir, err := os.MkdirTemp("", "piglet-extensions-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneCmd := exec.Command("git", "clone", "--depth", "1", "--quiet", repoURL, tmpDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(w, "Failed to clone extensions repo:\n%s\n", string(out))
		return nil
	}

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = tmpDir
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(w, "Warning: go mod tidy failed: %s\n", strings.TrimSpace(string(out)))
	}

	var built, failed int

	for _, name := range extensions {
		fmt.Fprintf(w, "  %-20s ", name)

		destDir := filepath.Join(extDir, name)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			fmt.Fprintf(w, "FAIL (mkdir: %v)\n", err)
			failed++
			continue
		}

		buildCmd := exec.Command("go", "build", "-o", filepath.Join(destDir, name), "./"+name+"/cmd/")
		buildCmd.Dir = tmpDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(w, "FAIL (%s)\n", strings.TrimSpace(string(out)))
			failed++
			continue
		}

		src := filepath.Join(tmpDir, name, "cmd", "manifest.yaml")
		dst := filepath.Join(destDir, "manifest.yaml")
		data, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(w, "FAIL (manifest: %v)\n", err)
			failed++
			continue
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			fmt.Fprintf(w, "FAIL (write manifest: %v)\n", err)
			failed++
			continue
		}

		fmt.Fprintln(w, "ok")
		built++
	}

	fmt.Fprintf(w, "Extensions: %d built, %d failed\n", built, failed)

	// Build standalone CLIs from cmd/*/
	cliBuilt, cliFailed := installCLIs(w, tmpDir)
	if cliBuilt+cliFailed > 0 {
		fmt.Fprintf(w, "CLIs: %d built, %d failed\n", cliBuilt, cliFailed)
	}

	return nil
}

// installCLIs discovers and builds standalone CLI tools from cmd/*/ in the
// cloned extensions repo, installing them to GOBIN.
func installCLIs(w io.Writer, repoDir string) (built, failed int) {
	cmdDir := filepath.Join(repoDir, "cmd")
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return 0, 0 // no cmd/ directory — nothing to build
	}

	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(w, "Warning: cannot determine GOBIN: %v\n", err)
			return 0, 0
		}
		gobin = filepath.Join(home, "go", "bin")
	}

	fmt.Fprintln(w, "Building CLIs...")
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		fmt.Fprintf(w, "  %-20s ", name)

		buildCmd := exec.Command("go", "build", "-o", filepath.Join(gobin, name), "./cmd/"+name+"/")
		buildCmd.Dir = repoDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(w, "FAIL (%s)\n", strings.TrimSpace(string(out)))
			failed++
			continue
		}
		fmt.Fprintln(w, "ok")
		built++
	}
	return built, failed
}
