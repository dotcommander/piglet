package command

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext/external"
	"gopkg.in/yaml.v3"
)

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
	if err := config.AtomicWrite(p, []byte(hash+"\n"), 0644); err != nil {
		slog.Warn("write build hash", "err", err)
	}
}

func InstallOfficialExtensions(w io.Writer, settings config.Settings) error {
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(w, "go not found in PATH — skipping extension install")
		return nil
	}

	extDir, err := external.ExtensionsDir()
	if err != nil {
		return fmt.Errorf("extensions dir: %w", err)
	}

	extensions := settings.ExtInstall.Official

	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(w, "git not found in PATH — skipping extension install")
		return nil
	}

	repoURL := settings.ExtInstall.RepoURL

	var remoteHash string
	// Check remote HEAD and compare with cached hash.
	if out, err := exec.Command("git", "ls-remote", repoURL, "HEAD").Output(); err == nil {
		fields := strings.Fields(strings.TrimSpace(string(out)))
		if len(fields) > 0 {
			remoteHash = fields[0]
			if cached := readLastBuildHash(); cached != "" && cached == remoteHash {
				slog.Info("extensions already up to date")
				return nil
			}
		}
	}

	fmt.Fprintf(w, "Cloning piglet...\n")

	// Clone into a subdirectory of the extensions dir rather than the system
	// temp root. Go 1.26+ ignores go.mod in system temp directories
	// (/tmp, /var/folders) which breaks the build.
	srcDir := filepath.Join(extDir, ".build-src")
	_ = os.RemoveAll(srcDir)
	if err := os.MkdirAll(filepath.Dir(srcDir), 0755); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}
	defer os.RemoveAll(srcDir)

	cloneCmd := exec.Command("git", "clone", "--depth", "1", "--quiet", repoURL, srcDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(w, "Failed to clone extensions repo:\n%s\n", string(out))
		return nil
	}

	// GOWORK=off ensures the cloned repo builds as a standalone module,
	// not as part of any local workspace.
	goEnv := append(os.Environ(), "GOWORK=off")

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = srcDir
	tidyCmd.Env = goEnv
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(w, "Warning: go mod tidy failed: %s\n", strings.TrimSpace(string(out)))
	}

	var built, failed int
	for _, name := range extensions {
		fmt.Fprintf(w, "  %-20s ", name)
		if buildExtension(w, name, extDir, srcDir, goEnv) {
			built++
		} else {
			failed++
		}
	}

	fmt.Fprintf(w, "Extensions: %d built, %d failed\n", built, failed)

	// Cache commit hash on successful remote build.
	if remoteHash != "" && failed == 0 {
		writeLastBuildHash(remoteHash)
	}

	cleanStaleExtensions(extDir, extensions)

	return nil
}

// buildExtension compiles one extension, copies its manifest, and seeds default
// config files. Returns true on success.
func buildExtension(w io.Writer, name, extDir, srcDir string, goEnv []string) bool {
	destDir := filepath.Join(extDir, name)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Fprintf(w, "FAIL (mkdir: %v)\n", err)
		return false
	}

	var buildPkg, manifestDir, srcRoot string
	if packName, ok := strings.CutPrefix(name, "pack-"); ok {
		buildPkg = "./extensions/packs/" + packName + "/"
		manifestDir = filepath.Join(srcDir, "extensions", "packs", packName)
		srcRoot = manifestDir
	} else {
		buildPkg = "./extensions/" + name + "/cmd/"
		manifestDir = filepath.Join(srcDir, "extensions", name, "cmd")
		srcRoot = filepath.Join(srcDir, "extensions", name)
	}

	buildCmd := exec.Command("go", "build", "-o", filepath.Join(destDir, name), buildPkg)
	buildCmd.Dir = srcDir
	buildCmd.Env = goEnv
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(w, "FAIL (%s)\n", strings.TrimSpace(string(out)))
		return false
	}

	// Copy manifest and seed default config files.
	src := filepath.Join(manifestDir, "manifest.yaml")
	dst := filepath.Join(destDir, "manifest.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintf(w, "FAIL (manifest: %v)\n", err)
		return false
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		fmt.Fprintf(w, "FAIL (write manifest: %v)\n", err)
		return false
	}

	seedDefaults(w, srcRoot, data)

	fmt.Fprintln(w, "ok")
	return true
}

// seedDefaults copies default config files declared in the manifest to the
// piglet config directory. Existing files are never overwritten.
func seedDefaults(w io.Writer, srcRoot string, manifestData []byte) {
	var mf external.Manifest
	if err := yaml.Unmarshal(manifestData, &mf); err != nil || len(mf.Defaults) == 0 {
		return
	}
	configDir, err := config.ConfigDir()
	if err != nil {
		fmt.Fprintf(w, "  warning: config dir: %v\n", err)
		return
	}
	for _, entry := range mf.Defaults {
		destPath := filepath.Join(configDir, entry.Dest)
		if _, err := os.Stat(destPath); err == nil {
			continue
		}
		srcPath := filepath.Join(srcRoot, entry.Src)
		contents, err := os.ReadFile(srcPath)
		if err != nil {
			fmt.Fprintf(w, "  warning: seed %s: %v\n", entry.Dest, err)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			fmt.Fprintf(w, "  warning: seed %s: %v\n", entry.Dest, err)
			continue
		}
		if err := os.WriteFile(destPath, contents, 0644); err != nil {
			fmt.Fprintf(w, "  warning: seed %s: %v\n", entry.Dest, err)
		}
	}
}

// cleanStaleExtensions removes extension directories that are no longer in
// the official list. Hidden directories (starting with ".") are preserved.
func cleanStaleExtensions(extDir string, official []string) {
	current := make(map[string]bool, len(official))
	for _, name := range official {
		current[name] = true
	}
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if !current[name] {
			_ = os.RemoveAll(filepath.Join(extDir, name))
		}
	}
}
