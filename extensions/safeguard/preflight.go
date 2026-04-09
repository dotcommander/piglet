package safeguard

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

// PreflightConfig controls which checks run before destructive tool calls.
type PreflightConfig struct {
	Enabled          bool     `yaml:"enabled"`
	BranchGuard      []string `yaml:"branchGuard"`      // branches to protect (default: main, master)
	CheckDirtyOnEdit bool     `yaml:"checkDirtyOnEdit"` // warn if uncommitted changes exist
}

func DefaultPreflightConfig() PreflightConfig {
	return PreflightConfig{
		Enabled:          true,
		BranchGuard:      []string{"main", "master"},
		CheckDirtyOnEdit: false, // off by default — too noisy for most workflows
	}
}

func LoadPreflightConfig() PreflightConfig {
	return xdg.LoadYAMLExt("safeguard", "preflight.yaml", DefaultPreflightConfig())
}

// destructiveTools is the set of tools that trigger pre-flight checks.
var destructiveTools = map[string]bool{
	"write":      true,
	"edit":       true,
	"multi_edit": true,
}

// RegisterPreflight adds a pre-flight checklist interceptor.
func RegisterPreflight(e *sdk.Extension) {
	cfg := LoadPreflightConfig()
	if !cfg.Enabled {
		return
	}

	var cwdPtr atomic.Pointer[string]
	e.OnInitAppend(func(x *sdk.Extension) {
		c := x.CWD()
		cwdPtr.Store(&c)
	})

	// Cache the branch check result — it won't change mid-session.
	var branchChecked atomic.Bool
	var onProtectedBranch atomic.Bool
	var protectedBranchName atomic.Pointer[string]

	var lastPreflightReason atomic.Value

	e.RegisterInterceptor(sdk.InterceptorDef{
		Name:     "preflight",
		Priority: 1900,
		Before: func(_ context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			if !destructiveTools[toolName] && !isDestructiveBash(toolName, args) {
				return true, args, nil
			}

			cp := cwdPtr.Load()
			if cp == nil {
				return true, args, nil
			}
			cwd := *cp

			// Branch guard check (cached after first check)
			if !branchChecked.Load() {
				branch := currentBranch(cwd)
				if slices.Contains(cfg.BranchGuard, branch) {
					onProtectedBranch.Store(true)
					protectedBranchName.Store(&branch)
				}
				branchChecked.Store(true)
			}

			if onProtectedBranch.Load() {
				name := protectedBranchName.Load()
				branchName := "protected branch"
				if name != nil {
					branchName = *name
				}
				reason := fmt.Sprintf(
					"pre-flight: blocked %s on %s — create a feature branch first",
					toolName, branchName)
				lastPreflightReason.Store(reason)
				return false, nil, nil
			}

			return true, args, nil
		},
		Preview: func(_ context.Context, _ string, _ map[string]any) string {
			if v, ok := lastPreflightReason.Load().(string); ok {
				return v
			}
			return ""
		},
	})
}

// isDestructiveBash checks if a bash tool call contains destructive commands.
func isDestructiveBash(toolName string, args map[string]any) bool {
	if toolName != "bash" {
		return false
	}
	cmd, ok := args["command"].(string)
	if !ok {
		return false
	}
	// Only check for commands that modify the filesystem
	destructive := []string{"rm ", "rm\t", "rmdir", "mv ", "chmod ", "chown "}
	lower := strings.ToLower(cmd)
	for _, d := range destructive {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}

func currentBranch(cwd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
