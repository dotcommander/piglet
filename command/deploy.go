package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const sdkModule = "github.com/dotcommander/piglet/sdk"

// RunDeploy automates the cross-repo deployment sequence for piglet + piglet-extensions.
// It detects SDK changes, tags, pushes, waits for the module proxy, bumps extension
// deps, verifies the build, tags piglet, and creates a GitHub release.
func RunDeploy(w io.Writer, dryRun, skipSDK bool) error {
	pigletDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	extDir, err := ResolveGoWorkExtPath()
	if err != nil {
		return fmt.Errorf("resolve extensions path: %w", err)
	}

	// === Preflight checks ===
	fmt.Fprintf(w, "=== Preflight checks ===\n")

	dirty, err := hasUncommittedChanges(pigletDir)
	if err != nil {
		return fmt.Errorf("check piglet status: %w", err)
	}
	if dirty {
		return fmt.Errorf("piglet has uncommitted changes — commit or stash first")
	}
	fmt.Fprintf(w, "  piglet: clean\n")

	dirty, err = hasUncommittedChanges(extDir, "go.mod", "go.sum")
	if err != nil {
		return fmt.Errorf("check extensions status: %w", err)
	}
	if dirty {
		return fmt.Errorf("piglet-extensions has uncommitted changes (beyond go.mod/go.sum) — commit or stash first")
	}
	fmt.Fprintf(w, "  piglet-extensions: clean\n")

	// === Detect SDK changes ===
	lastSDKTag, err := latestTag(pigletDir, "sdk/v")
	if err != nil {
		return fmt.Errorf("find latest SDK tag: %w", err)
	}

	sdkNeedsTag := false
	if !skipSDK {
		changed, err := sdkChanged(pigletDir, lastSDKTag)
		if err != nil {
			return fmt.Errorf("detect SDK changes: %w", err)
		}
		sdkNeedsTag = changed
	}

	// === Compute versions ===
	lastPigletTag, err := latestTag(pigletDir, "v")
	if err != nil {
		return fmt.Errorf("find latest piglet tag: %w", err)
	}

	newPigletTag := bumpPatch(lastPigletTag)
	var newSDKTag string
	if sdkNeedsTag {
		newSDKTag = bumpPatch(lastSDKTag)
	}

	// === Print plan ===
	fmt.Fprintf(w, "\n=== Deployment plan ===\n")
	fmt.Fprintf(w, "  piglet:     %s → %s\n", lastPigletTag, newPigletTag)
	if sdkNeedsTag {
		fmt.Fprintf(w, "  SDK:        %s → %s\n", lastSDKTag, newSDKTag)
		fmt.Fprintf(w, "  extensions: bump SDK dep, commit, push\n")
	} else {
		reason := "no changes"
		if skipSDK {
			reason = "skipped (--skip-sdk)"
		}
		fmt.Fprintf(w, "  SDK:        %s (no new tag — %s)\n", lastSDKTag, reason)
	}
	fmt.Fprintf(w, "  extensions: verify GOWORK=off build, push\n")

	if _, err := exec.LookPath("gh"); err != nil {
		fmt.Fprintf(w, "  release:    skip (gh CLI not found)\n")
	} else {
		fmt.Fprintf(w, "  release:    create via gh\n")
	}

	if dryRun {
		fmt.Fprintf(w, "\n--dry-run: stopping here.\n")
		return nil
	}

	// === SDK tag + push ===
	if sdkNeedsTag {
		fmt.Fprintf(w, "\n=== Tagging SDK ===\n")
		if err := runGit(pigletDir, "tag", newSDKTag); err != nil {
			return fmt.Errorf("tag SDK: %w", err)
		}
		fmt.Fprintf(w, "  Tagged %s\n", newSDKTag)

		if err := runGit(pigletDir, "push", "origin", "main", newSDKTag); err != nil {
			return fmt.Errorf("push SDK tag: %w", err)
		}
		fmt.Fprintf(w, "  Pushed main + %s\n", newSDKTag)

		// Wait for module proxy
		sdkVersion := strings.TrimPrefix(newSDKTag, "sdk/")
		proxyCtx, proxyCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer proxyCancel()
		if err := waitForModuleProxy(proxyCtx, w, sdkModule, sdkVersion); err != nil {
			return err
		}

		// Update extensions go.mod
		fmt.Fprintf(w, "\n=== Updating extensions SDK dep ===\n")
		goworkOff := []string{"GOWORK=off"}

		out, err := runCmd(extDir, goworkOff, "go", "get", sdkModule+"@"+sdkVersion)
		if err != nil {
			return fmt.Errorf("go get SDK in extensions: %s", out)
		}
		fmt.Fprintf(w, "  go get %s@%s\n", sdkModule, sdkVersion)

		out, err = runCmd(extDir, goworkOff, "go", "mod", "tidy")
		if err != nil {
			return fmt.Errorf("go mod tidy in extensions: %s", out)
		}
		fmt.Fprintf(w, "  go mod tidy\n")
	}

	// === Verify extensions build ===
	fmt.Fprintf(w, "\n=== Verifying extensions build (GOWORK=off) ===\n")
	out, err := runCmd(extDir, []string{"GOWORK=off"}, "go", "build", "./...")
	if err != nil {
		return fmt.Errorf("extensions build failed (GOWORK=off) — aborting:\n%s", out)
	}
	fmt.Fprintf(w, "  Build OK\n")

	// === Commit extensions if go.mod changed ===
	if sdkNeedsTag {
		fmt.Fprintf(w, "\n=== Committing extensions ===\n")
		sdkVersion := strings.TrimPrefix(newSDKTag, "sdk/")
		if err := runGit(extDir, "add", "go.mod", "go.sum"); err != nil {
			return fmt.Errorf("stage extensions go.mod: %w", err)
		}
		msg := fmt.Sprintf("deps: bump piglet SDK to %s", sdkVersion)
		if err := runGit(extDir, "commit", "-m", msg); err != nil {
			return fmt.Errorf("commit extensions: %w", err)
		}
		fmt.Fprintf(w, "  Committed: %s\n", msg)
	}

	// === Push extensions ===
	fmt.Fprintf(w, "\n=== Pushing extensions ===\n")
	if err := runGit(extDir, "push", "origin", "main"); err != nil {
		return fmt.Errorf("push extensions: %w", err)
	}
	fmt.Fprintf(w, "  Pushed\n")

	// === Push piglet + tag ===
	fmt.Fprintf(w, "\n=== Tagging and pushing piglet ===\n")
	if !sdkNeedsTag {
		// SDK path already pushed main; only push if we haven't yet
		if err := runGit(pigletDir, "push", "origin", "main"); err != nil {
			return fmt.Errorf("push piglet: %w", err)
		}
		fmt.Fprintf(w, "  Pushed main\n")
	}

	if err := runGit(pigletDir, "tag", newPigletTag); err != nil {
		return fmt.Errorf("tag piglet: %w", err)
	}
	fmt.Fprintf(w, "  Tagged %s\n", newPigletTag)

	if err := runGit(pigletDir, "push", "origin", newPigletTag); err != nil {
		return fmt.Errorf("push piglet tag: %w", err)
	}
	fmt.Fprintf(w, "  Pushed %s\n", newPigletTag)

	// === GitHub release ===
	if ghPath, err := exec.LookPath("gh"); err == nil {
		fmt.Fprintf(w, "\n=== Creating GitHub release ===\n")
		cmd := exec.Command(ghPath, "release", "create", newPigletTag,
			"--generate-notes",
			"--repo", "dotcommander/piglet",
			"--title", newPigletTag)
		cmd.Dir = pigletDir
		releaseOut, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(w, "  Warning: release creation failed: %s\n", strings.TrimSpace(string(releaseOut)))
		} else {
			fmt.Fprintf(w, "  Release created: %s\n", strings.TrimSpace(string(releaseOut)))
		}
	}

	// === Summary ===
	fmt.Fprintf(w, "\n=== Deploy complete ===\n")
	fmt.Fprintf(w, "  piglet:     %s\n", newPigletTag)
	if sdkNeedsTag {
		fmt.Fprintf(w, "  SDK:        %s\n", newSDKTag)
	}
	fmt.Fprintf(w, "  extensions: pushed\n")
	fmt.Fprintf(w, "  release:    https://github.com/dotcommander/piglet/releases/tag/%s\n", newPigletTag)

	return nil
}

// latestTag returns the most recent tag matching the given prefix (e.g., "v" or "sdk/v").
func latestTag(dir, prefix string) (string, error) {
	cmd := exec.Command("git", "tag", "-l", prefix+"*", "--sort=-v:refname")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("list tags with prefix %s: %w", prefix, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", fmt.Errorf("no tags found with prefix %s", prefix)
	}
	return lines[0], nil
}

// bumpPatch increments the patch version of a tag (e.g., v1.2.3 -> v1.2.4, sdk/v1.4.0 -> sdk/v1.4.1).
func bumpPatch(tag string) string {
	prefix := ""
	ver := tag
	if i := strings.LastIndex(tag, "v"); i > 0 {
		prefix = tag[:i]
		ver = tag[i:]
	}
	ver = strings.TrimPrefix(ver, "v")
	parts := strings.Split(ver, ".")
	if len(parts) != 3 {
		return tag + ".1"
	}
	patch, _ := strconv.Atoi(parts[2])
	parts[2] = strconv.Itoa(patch + 1)
	return prefix + "v" + strings.Join(parts, ".")
}

// sdkChanged returns true if the sdk/ directory has changes since the given tag.
func sdkChanged(pigletDir, lastSDKTag string) (bool, error) {
	cmd := exec.Command("git", "diff", "--quiet", lastSDKTag+"..HEAD", "--", "sdk/")
	cmd.Dir = pigletDir
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("check SDK changes: %w", err)
	}
	return false, nil
}

// hasUncommittedChanges returns true if the repo at dir has uncommitted changes,
// ignoring files whose names match any of the excludeFiles suffixes.
func hasUncommittedChanges(dir string, excludeFiles ...string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status in %s: %w", dir, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return false, nil
	}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		excluded := false
		trimmed := strings.TrimSpace(line)
		for _, pat := range excludeFiles {
			if strings.HasSuffix(trimmed, pat) {
				excluded = true
				break
			}
		}
		if !excluded {
			return true, nil
		}
	}
	return false, nil
}

// waitForModuleProxy polls the Go module proxy until the given module version is indexed.
func waitForModuleProxy(ctx context.Context, w io.Writer, module, version string) error {
	fmt.Fprintf(w, "\n=== Waiting for module proxy ===\n")
	fmt.Fprintf(w, "  %s@%s\n", module, version)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		cmd := exec.CommandContext(ctx, "go", "list", "-m", module+"@"+version)
		cmd.Env = append(os.Environ(), "GOWORK=off", "GONOSUMCHECK=*")
		if err := cmd.Run(); err == nil {
			fmt.Fprintf(w, "  Module proxy ready.\n")
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for module proxy to index %s@%s", module, version)
		case <-ticker.C:
			fmt.Fprintf(w, "  Still waiting...\n")
		}
	}
}

// runGit runs a git command in the given directory.
func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// runCmd runs a command in the given directory with optional extra environment variables.
func runCmd(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
