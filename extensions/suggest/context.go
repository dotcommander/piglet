package suggest

import (
	"os/exec"
	"strings"
)

// ProjectContext holds context about the current project state.
type ProjectContext struct {
	GitStatus     string   // "clean", "dirty", "no-repo"
	ModifiedFiles []string
	LastTool      string
	LastError     bool
}

// GatherContext collects project context from git and turn data.
func GatherContext(cwd string, turn TurnData) ProjectContext {
	ctx := ProjectContext{
		GitStatus: "no-repo",
	}

	// Get git status
	gitStatus := exec.Command("git", "-C", cwd, "status", "--porcelain")
	gitStatus.Dir = cwd
	if output, err := gitStatus.Output(); err == nil {
		if len(output) == 0 {
			ctx.GitStatus = "clean"
		} else {
			ctx.GitStatus = "dirty"
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				if len(line) > 3 {
					// Status format: "XY filename" - extract filename
					file := strings.TrimSpace(line[2:])
					if file != "" {
						ctx.ModifiedFiles = append(ctx.ModifiedFiles, file)
					}
				}
			}
		}
	}

	// Extract last tool and error from turn data
	for _, tr := range turn.ToolResults {
		ctx.LastTool = tr.ToolName
		ctx.LastError = tr.IsError
	}

	return ctx
}
