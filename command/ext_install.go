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

// installConfig holds options for InstallOfficialExtensions.
type installConfig struct {
	localDir string
}

// InstallOption configures extension installation behavior.
type InstallOption func(*installConfig)

// WithLocalDir builds extensions from a local directory instead of cloning.
func WithLocalDir(dir string) InstallOption {
	return func(c *installConfig) { c.localDir = dir }
}

// ResolveGoWorkExtPath finds a piglet-extensions directory in the active Go workspace.
func ResolveGoWorkExtPath() (string, error) {
	out, err := exec.Command("go", "env", "GOWORK").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOWORK: %w", err)
	}
	goworkPath := strings.TrimSpace(string(out))
	if goworkPath == "" {
		return "", fmt.Errorf("no active go.work workspace")
	}
	data, err := os.ReadFile(goworkPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", goworkPath, err)
	}
	workDir := filepath.Dir(goworkPath)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		cleaned := strings.TrimPrefix(line, "use ")
		cleaned = strings.TrimSpace(cleaned)
		cleaned = strings.TrimRight(cleaned, "/")
		if strings.HasSuffix(cleaned, "piglet-extensions") {
			abs := filepath.Join(workDir, cleaned)
			if info, serr := os.Stat(abs); serr == nil && info.IsDir() {
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("piglet-extensions not found in %s", goworkPath)
}

// lastBuildHashPath returns the path to the cached commit hash file.
func lastBuildHashPath() (string, error) {
	dir, err := external.ExtensionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".last-build-hash"), nil
}

// readLastBuildHash returns the cached commit hash from the last successful build.
func readLastBuildHash() string {
	p, err := lastBuildHashPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// writeLastBuildHash writes the commit hash after a successful build.
func writeLastBuildHash(hash string) {
	p, err := lastBuildHashPath()
	if err != nil {
		return
	}
	_ = os.WriteFile(p, []byte(hash+"\n"), 0644)
}

func InstallOfficialExtensions(w io.Writer, settings config.Settings, opts ...InstallOption) error {
	var cfg installConfig
	for _, o := range opts {
		o(&cfg)
	}

	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(w, "go not found in PATH — skipping extension install")
		return nil
	}

	extDir, err := external.ExtensionsDir()
	if err != nil {
		return fmt.Errorf("extensions dir: %w", err)
	}

	extensions := settings.ExtInstall.Official

	var srcDir string
	var remoteHash string
	if cfg.localDir != "" {
		srcDir = cfg.localDir
		fmt.Fprintf(w, "Building from local source: %s\n", srcDir)
	} else {
		if _, err := exec.LookPath("git"); err != nil {
			fmt.Fprintln(w, "git not found in PATH — skipping extension install")
			return nil
		}

		repoURL := settings.ExtInstall.RepoURL

		// Check remote HEAD and compare with cached hash.
		if out, err := exec.Command("git", "ls-remote", repoURL, "HEAD").Output(); err == nil {
			fields := strings.Fields(strings.TrimSpace(string(out)))
			if len(fields) > 0 {
				remoteHash = fields[0]
				if cached := readLastBuildHash(); cached != "" && cached == remoteHash {
					fmt.Fprintln(w, "Extensions already up to date.")
					return nil
				}
			}
		}

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

		srcDir = tmpDir
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

		var buildPkg, manifestDir, srcRoot string
		if strings.HasPrefix(name, "pack-") {
			packName := strings.TrimPrefix(name, "pack-")
			buildPkg = "./packs/" + packName + "/"
			manifestDir = filepath.Join(srcDir, "packs", packName)
			srcRoot = manifestDir
		} else {
			buildPkg = "./" + name + "/cmd/"
			manifestDir = filepath.Join(srcDir, name, "cmd")
			srcRoot = filepath.Join(srcDir, name)
		}

		buildCmd := exec.Command("go", "build", "-o", filepath.Join(destDir, name), buildPkg)
		buildCmd.Dir = srcDir
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

		var mf external.Manifest
		if err := yaml.Unmarshal(data, &mf); err == nil && len(mf.Defaults) > 0 {
			configDir, cerr := config.ConfigDir()
			if cerr != nil {
				fmt.Fprintf(w, "  warning: config dir: %v\n", cerr)
			} else {
				for _, entry := range mf.Defaults {
					destPath := filepath.Join(configDir, entry.Dest)
					if _, err := os.Stat(destPath); err == nil {
						continue
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

	// Cache commit hash on successful remote build.
	if remoteHash != "" && failed == 0 {
		writeLastBuildHash(remoteHash)
	}

	// Update this list when pack contents change.
	packMembers := []string{
		"admin", "export", "extensions-list", "undo", "scaffold", "background",
		"safeguard", "rtk", "autotitle", "clipboard", "subagent", "provider", "loop",
		"memory", "skill", "gitcontext", "behavior", "prompts", "session-tools", "inbox",
		"lsp", "repomap", "sift", "plan", "suggest",
		"pipeline", "bulk", "webfetch", "cache", "usage", "modelsdev",
	}
	for _, old := range packMembers {
		oldDir := filepath.Join(extDir, old)
		if _, err := os.Stat(oldDir); err == nil {
			_ = os.RemoveAll(oldDir)
		}
	}

	cliBuilt, cliFailed := installCLIs(w, srcDir)
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
