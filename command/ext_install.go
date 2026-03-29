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
	"gopkg.in/yaml.v3"
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

		// Resolve source paths based on pack vs individual extension.
		var buildPkg, manifestDir, srcRoot string
		if strings.HasPrefix(name, "pack-") {
			packName := strings.TrimPrefix(name, "pack-")
			buildPkg = "./packs/" + packName + "/"
			manifestDir = filepath.Join(tmpDir, "packs", packName)
			srcRoot = manifestDir
		} else {
			buildPkg = "./" + name + "/cmd/"
			manifestDir = filepath.Join(tmpDir, name, "cmd")
			srcRoot = filepath.Join(tmpDir, name)
		}

		buildCmd := exec.Command("go", "build", "-o", filepath.Join(destDir, name), buildPkg)
		buildCmd.Dir = tmpDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(w, "FAIL (%s)\n", strings.TrimSpace(string(out)))
			failed++
			continue
		}

		src := filepath.Join(manifestDir, "manifest.yaml")
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

		// Seed default config files (skip if dest already exists — preserves user edits).
		var mf external.Manifest
		if err := yaml.Unmarshal(data, &mf); err == nil && len(mf.Defaults) > 0 {
			configDir, cerr := config.ConfigDir()
			if cerr != nil {
				fmt.Fprintf(w, "  warning: config dir: %v\n", cerr)
			} else {
				for _, entry := range mf.Defaults {
					destPath := filepath.Join(configDir, entry.Dest)
					if _, err := os.Stat(destPath); err == nil {
						continue // already exists — preserve user edits
					}
					srcPath := filepath.Join(srcRoot, entry.Src)
					contents, rerr := os.ReadFile(srcPath)
					if rerr != nil {
						fmt.Fprintf(w, "  warning: seed %s: %v\n", entry.Dest, rerr)
						continue
					}
					if merr := os.MkdirAll(filepath.Dir(destPath), 0755); merr != nil {
						fmt.Fprintf(w, "  warning: seed %s: %v\n", entry.Dest, merr)
						continue
					}
					if werr := os.WriteFile(destPath, contents, 0644); werr != nil {
						fmt.Fprintf(w, "  warning: seed %s: %v\n", entry.Dest, werr)
					}
				}
			}
		}

		fmt.Fprintln(w, "ok")
		built++
	}

	fmt.Fprintf(w, "Extensions: %d built, %d failed\n", built, failed)

	// Remove old individual extension directories now consolidated into packs.
	packMembers := []string{
		// pack-core
		"admin", "export", "extensions-list", "undo", "scaffold", "background",
		// pack-agent
		"safeguard", "rtk", "autotitle", "clipboard", "subagent", "provider", "loop",
		// pack-context
		"memory", "skill", "gitcontext", "behavior", "prompts", "session-tools", "inbox",
		// pack-code
		"lsp", "repomap", "sift", "plan", "suggest",
		// pack-workflow
		"pipeline", "bulk", "webfetch", "cache", "usage", "modelsdev",
	}
	for _, old := range packMembers {
		oldDir := filepath.Join(extDir, old)
		if _, err := os.Stat(oldDir); err == nil {
			_ = os.RemoveAll(oldDir)
		}
	}

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
