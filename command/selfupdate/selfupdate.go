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
	"strconv"
	"strings"
	"time"

	"github.com/dotcommander/piglet/config"
)

const (
	releasesURL = "https://api.github.com/repos/dotcommander/piglet/releases/latest"
	repoURL     = "https://github.com/dotcommander/piglet.git"
	cacheFile   = ".update-check.json"
	cacheMaxAge = 24 * time.Hour
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// ReleaseInfo holds the fields we care about from the GitHub releases API.
type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
}

type updateCache struct {
	CheckedAt time.Time   `json:"checked_at"`
	Release   ReleaseInfo `json:"release"`
}

// cacheDir is the directory used for the update cache. Empty string means
// use config.ConfigDir() at runtime. Tests override directly.
var cacheDir string

// resolveCachePath returns the full path to the cache file.
func resolveCachePath() (string, error) {
	dir := cacheDir
	if dir == "" {
		var err error
		dir, err = config.ConfigDir()
		if err != nil {
			return "", fmt.Errorf("selfupdate cache path: %w", err)
		}
	}
	return filepath.Join(dir, cacheFile), nil
}

// FetchLatestRelease fetches the latest release info from the GitHub API.
func FetchLatestRelease(ctx context.Context) (ReleaseInfo, error) {
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

// CompareVersions compares two semver strings (with optional "v" prefix).
// Returns -1 if current < latest, 0 if equal, 1 if current > latest.
// "dev" prefix versions are always treated as older (-1).
func CompareVersions(current, latest string) int {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Strip build metadata per semver — +dirty, +build, etc. ignored for precedence.
	current, _, _ = strings.Cut(current, "+")
	latest, _, _ = strings.Cut(latest, "+")

	if strings.HasPrefix(current, "dev") {
		return -1
	}

	partsA := strings.Split(current, ".")
	partsB := strings.Split(latest, ".")

	length := len(partsA)
	if len(partsB) > length {
		length = len(partsB)
	}

	for i := range length {
		a := segmentInt(partsA, i)
		b := segmentInt(partsB, i)
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
	}
	return 0
}

// segmentInt returns the integer at index i in parts, or 0 if out of bounds
// or non-numeric.
func segmentInt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return n
}

// readCache reads the update cache from disk. Returns nil if missing or corrupt.
func readCache() *updateCache {
	path, err := resolveCachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c updateCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return &c
}

// CheckStale returns true if the update cache is missing, corrupt, or older
// than 24 hours.
func CheckStale() bool {
	c := readCache()
	if c == nil {
		return true
	}
	return time.Since(c.CheckedAt) > cacheMaxAge
}

// CachedRelease returns the cached release if the cache is fresh, otherwise
// returns a zero ReleaseInfo.
func CachedRelease() ReleaseInfo {
	c := readCache()
	if c == nil || time.Since(c.CheckedAt) > cacheMaxAge {
		return ReleaseInfo{}
	}
	return c.Release
}

// WriteCache writes a release to the update cache atomically.
func WriteCache(r ReleaseInfo) error {
	path, err := resolveCachePath()
	if err != nil {
		return err
	}

	c := updateCache{CheckedAt: time.Now(), Release: r}
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal update cache: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write update cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install update cache: %w", err)
	}
	return nil
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
