package ext_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Guardrail 1: Package dependency boundary
//
// The dependency direction must flow downward:
//
//	core/      → stdlib only (foundation)
//	ext/       → core/ + stdlib (registration surface)
//	tool/, command/, memory/, prompt/ → ext/, core/, config/, session/ (extensions)
//	tui/, cmd/ → anything (wiring layer)
//
// Violations indicate functionality leaking into the wrong layer.
// ---------------------------------------------------------------------------

// forbiddenImports maps packages to import prefixes they must never use.
var forbiddenImports = map[string][]string{
	// core/ must never import extension or UI packages
	"github.com/dotcommander/piglet/core": {
		"github.com/dotcommander/piglet/ext",
		"github.com/dotcommander/piglet/tool",
		"github.com/dotcommander/piglet/command",
		"github.com/dotcommander/piglet/memory",
		"github.com/dotcommander/piglet/prompt",
		"github.com/dotcommander/piglet/tui",
		"github.com/dotcommander/piglet/session",
		"github.com/dotcommander/piglet/provider",
		"github.com/dotcommander/piglet/config",
		"github.com/dotcommander/piglet/skill",
		"github.com/dotcommander/piglet/safeguard",
		"github.com/dotcommander/piglet/rtk",
		"github.com/dotcommander/piglet/subagent",
	},
	// ext/ must never import extension implementations or UI
	"github.com/dotcommander/piglet/ext": {
		"github.com/dotcommander/piglet/tool",
		"github.com/dotcommander/piglet/command",
		"github.com/dotcommander/piglet/memory",
		"github.com/dotcommander/piglet/prompt",
		"github.com/dotcommander/piglet/tui",
		"github.com/dotcommander/piglet/session",
		"github.com/dotcommander/piglet/provider",
		"github.com/dotcommander/piglet/skill",
		"github.com/dotcommander/piglet/safeguard",
		"github.com/dotcommander/piglet/rtk",
		"github.com/dotcommander/piglet/subagent",
	},
}

func TestDependencyBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dependency boundary check in short mode")
	}
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}:{{join .Imports \",\"}}", "./...")
	cmd.Dir = ".."
	out, err := cmd.Output()
	require.NoError(t, err, "go list failed")

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		pkg := parts[0]
		imports := strings.Split(parts[1], ",")

		forbidden, ok := forbiddenImports[pkg]
		if !ok {
			continue
		}

		for _, imp := range imports {
			imp = strings.TrimSpace(imp)
			for _, f := range forbidden {
				if strings.HasPrefix(imp, f) {
					t.Errorf("BOUNDARY VIOLATION: %s imports %s\n"+
						"  → %s must not depend on %s\n"+
						"  → Move this functionality to an extension package instead",
						pkg, imp, pkg, f)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Guardrail 2: Registration completeness
//
// All tools and commands must be registered through ext.App.
// Nothing should bypass the extension API to wire directly into the agent.
// ---------------------------------------------------------------------------

func TestAllToolsRegisteredThroughExtApp(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	tool.RegisterBuiltins(app, tool.BashConfig{}, tool.ToolConfig{})

	// Every tool in CoreTools must have a corresponding ToolDef
	coreTools := app.CoreTools()
	toolDefs := app.ToolDefs()

	defNames := make(map[string]bool, len(toolDefs))
	for _, td := range toolDefs {
		defNames[td.Name] = true
	}

	for _, ct := range coreTools {
		assert.True(t, defNames[ct.Name],
			"tool %q appears in CoreTools but was not registered through ext.App.RegisterTool()", ct.Name)
	}

	// And vice versa: every ToolDef should appear in CoreTools
	coreNames := make(map[string]bool, len(coreTools))
	for _, ct := range coreTools {
		coreNames[ct.Name] = true
	}

	for _, td := range toolDefs {
		assert.True(t, coreNames[td.Name],
			"tool %q registered in ext.App but missing from CoreTools()", td.Name)
	}
}

func TestAllToolsHaveConsistentCounts(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	tool.RegisterBuiltins(app, tool.BashConfig{}, tool.ToolConfig{})

	coreTools := app.CoreTools()
	toolDefs := app.ToolDefs()
	toolNames := app.Tools()

	assert.Equal(t, len(toolDefs), len(coreTools),
		"ToolDefs count must match CoreTools count — something bypassed registration")
	assert.Equal(t, len(toolDefs), len(toolNames),
		"ToolDefs count must match Tools() name count — something bypassed registration")
}

func TestBackgroundSafeToolsAreSubsetOfCoreTools(t *testing.T) {
	t.Parallel()

	app := ext.NewApp(t.TempDir())
	tool.RegisterBuiltins(app, tool.BashConfig{}, tool.ToolConfig{})

	bgTools := app.BackgroundSafeTools()
	coreNames := make(map[string]bool)
	for _, ct := range app.CoreTools() {
		coreNames[ct.Name] = true
	}

	for _, bt := range bgTools {
		assert.True(t, coreNames[bt.Name],
			"background-safe tool %q not found in CoreTools — registration mismatch", bt.Name)
	}

	// At least some tools should be background-safe
	assert.NotEmpty(t, bgTools, "no tools marked BackgroundSafe — is this intentional?")
}
