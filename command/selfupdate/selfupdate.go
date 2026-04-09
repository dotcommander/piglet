package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	releasesURL = "https://api.github.com/repos/dotcommander/piglet/releases/latest"
	repoURL     = "https://github.com/dotcommander/piglet.git"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// FetchLatestRelease returns the latest piglet version using git ls-remote
// (immune to GitHub release creation-order bugs). Falls back to the GitHub
// releases API if git is not available.
func FetchLatestRelease(ctx context.Context) (ReleaseInfo, error) {
	// Prefer git ls-remote (uses tags, not releases — immune to creation-order bugs).
	if _, err := exec.LookPath("git"); err == nil {
		out, err := exec.CommandContext(ctx, "git", "ls-remote", "--tags", repoURL).Output()
		if err == nil {
			var best string
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) < 2 {
					continue
				}
				ref := parts[1]
				// Skip dereferenced tags and non-version tags.
				if strings.HasSuffix(ref, "^{}") {
					continue
				}
				tag := strings.TrimPrefix(ref, "refs/tags/")
				if !strings.HasPrefix(tag, "v") {
					continue
				}
				// Skip tags with slashes (e.g., sdk/v1.0.0).
				if strings.Contains(tag, "/") {
					continue
				}
				if best == "" || CompareVersions(tag, best) > 0 {
					best = tag
				}
			}
			if best != "" {
				return ReleaseInfo{
					TagName: best,
					HTMLURL: "https://github.com/dotcommander/piglet/releases/tag/" + best,
				}, nil
			}
		}
	}

	// Fallback: GitHub releases API.
	return fetchLatestFromAPI(ctx)
}

// fetchLatestFromAPI fetches the latest release info from the GitHub releases API.
func fetchLatestFromAPI(ctx context.Context) (ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "piglet")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("fetch release: unexpected status %d", resp.StatusCode)
	}

	var r ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return ReleaseInfo{}, fmt.Errorf("decode release: %w", err)
	}
	return r, nil
}

// RunUpgrade clones the piglet repo at the given tag, builds the binary,
// and installs it to GOBIN.
func RunUpgrade(ctx context.Context, w io.Writer, tag string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go not found in PATH — install Go from https://go.dev/dl/: %w", err)
	}

	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("determine home dir: %w", err)
		}
		gobin = filepath.Join(home, "go", "bin")
	}

	tmpDir, err := os.MkdirTemp("", "piglet-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintf(w, "Cloning piglet %s...\n", tag)
	cloneCmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", tag, "--quiet", repoURL, tmpDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone piglet: %s", strings.TrimSpace(string(out)))
	}

	fmt.Fprintf(w, "  %-20s ", "piglet")
	buildCmd := exec.CommandContext(ctx, goPath, "build", "-o", filepath.Join(gobin, "piglet"), "./cmd/piglet/")
	buildCmd.Dir = tmpDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintln(w, "FAIL")
		return fmt.Errorf("build piglet: %s", strings.TrimSpace(string(out)))
	}
	fmt.Fprintln(w, "ok")

	return nil
}

// CheckAndUpgrade fetches the latest release, compares against currentVersion,
// clones the repo, and builds the binary. Progress is written to w.
// Returns (true, nil) if an upgrade was performed, (false, nil) if already up to date.
func CheckAndUpgrade(ctx context.Context, w io.Writer, currentVersion string) (bool, error) {
	fmt.Fprintln(w, "Checking for updates...")

	release, err := FetchLatestRelease(ctx)
	if err != nil {
		return false, fmt.Errorf("check latest version: %w", err)
	}
	_ = WriteCache(release)

	cleanVersion, _, _ := strings.Cut(strings.TrimPrefix(currentVersion, "v"), "+")

	if CompareVersions(currentVersion, release.TagName) >= 0 {
		fmt.Fprintf(w, "CLI already up to date (v%s)\n", cleanVersion)
		return false, nil
	}

	fmt.Fprintf(w, "CLI v%s → %s\n", cleanVersion, release.TagName)
	if err := RunUpgrade(ctx, w, release.TagName); err != nil {
		return false, fmt.Errorf("upgrade failed: %w", err)
	}
	return true, nil
}

// UpdateNotice returns a human-readable notice if a newer version is cached,
// or an empty string if the current version is up to date.
func UpdateNotice(currentVersion string) string {
	r := CachedRelease()
	if r.TagName == "" {
		return ""
	}
	if CompareVersions(currentVersion, r.TagName) >= 0 {
		return ""
	}
	cur := strings.TrimPrefix(currentVersion, "v")
	latest := strings.TrimPrefix(r.TagName, "v")
	return fmt.Sprintf("Update available: v%s (current: v%s) — run: piglet update", latest, cur)
}
