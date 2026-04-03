package depgraph

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestGraph returns a hand-constructed *Graph with topology:
//
//	a → b, c
//	b → d
//	c → d
//	d → (nothing)
//
// All packages are under module "example.com/app".
func buildTestGraph() *Graph {
	mod := "example.com/app"
	pkgs := map[string]*Package{
		"example.com/app/a": {ImportPath: "example.com/app/a", Name: "a", Dir: "/src/a"},
		"example.com/app/b": {ImportPath: "example.com/app/b", Name: "b", Dir: "/src/b"},
		"example.com/app/c": {ImportPath: "example.com/app/c", Name: "c", Dir: "/src/c"},
		"example.com/app/d": {ImportPath: "example.com/app/d", Name: "d", Dir: "/src/d"},
	}
	forward := map[string][]string{
		"example.com/app/a": {"example.com/app/b", "example.com/app/c"},
		"example.com/app/b": {"example.com/app/d"},
		"example.com/app/c": {"example.com/app/d"},
	}
	reverse := map[string][]string{
		"example.com/app/b": {"example.com/app/a"},
		"example.com/app/c": {"example.com/app/a"},
		"example.com/app/d": {"example.com/app/b", "example.com/app/c"},
	}
	return &Graph{
		Root:     "/src",
		Module:   mod,
		Packages: pkgs,
		Forward:  forward,
		Reverse:  reverse,
	}
}

// importPaths extracts the ImportPath field from a []DepEntry slice.
func importPaths(entries []DepEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ImportPath
	}
	return out
}

// ── Query tests ──────────────────────────────────────────────────────────────

func TestDeps_Unlimited(t *testing.T) {
	g := buildTestGraph()
	entries := g.Deps("example.com/app/a", 0)
	got := importPaths(entries)
	assert.ElementsMatch(t, []string{
		"example.com/app/b",
		"example.com/app/c",
		"example.com/app/d",
	}, got)
}

func TestDeps_Depth1(t *testing.T) {
	g := buildTestGraph()
	entries := g.Deps("example.com/app/a", 1)
	got := importPaths(entries)
	assert.ElementsMatch(t, []string{
		"example.com/app/b",
		"example.com/app/c",
	}, got)
	for _, e := range entries {
		assert.Equal(t, 1, e.Depth)
	}
}

func TestDeps_Leaf(t *testing.T) {
	g := buildTestGraph()
	entries := g.Deps("example.com/app/d", 0)
	assert.Empty(t, entries)
}

func TestReverseDeps_Unlimited(t *testing.T) {
	g := buildTestGraph()
	entries := g.ReverseDeps("example.com/app/d", 0)
	got := importPaths(entries)
	assert.ElementsMatch(t, []string{
		"example.com/app/b",
		"example.com/app/c",
		"example.com/app/a",
	}, got)
}

func TestReverseDeps_Depth1(t *testing.T) {
	g := buildTestGraph()
	entries := g.ReverseDeps("example.com/app/d", 1)
	got := importPaths(entries)
	// Only direct importers of d (b and c) at depth 1; a is two hops away.
	assert.ElementsMatch(t, []string{
		"example.com/app/b",
		"example.com/app/c",
	}, got)
}

func TestImpact(t *testing.T) {
	g := buildTestGraph()
	affected := g.Impact("example.com/app/d")
	assert.ElementsMatch(t, []string{
		"example.com/app/a",
		"example.com/app/b",
		"example.com/app/c",
		"example.com/app/d",
	}, affected)
}

func TestDetectCycles_NoCycles(t *testing.T) {
	g := buildTestGraph()
	cycles := g.DetectCycles()
	assert.Empty(t, cycles)
}

func TestDetectCycles_WithCycle(t *testing.T) {
	// Build a minimal graph with a→b→a cycle.
	g := &Graph{
		Root:   "/src",
		Module: "example.com/app",
		Packages: map[string]*Package{
			"example.com/app/a": {ImportPath: "example.com/app/a", Name: "a"},
			"example.com/app/b": {ImportPath: "example.com/app/b", Name: "b"},
		},
		Forward: map[string][]string{
			"example.com/app/a": {"example.com/app/b"},
			"example.com/app/b": {"example.com/app/a"},
		},
		Reverse: map[string][]string{
			"example.com/app/a": {"example.com/app/b"},
			"example.com/app/b": {"example.com/app/a"},
		},
	}
	cycles := g.DetectCycles()
	require.Len(t, cycles, 1)
	assert.ElementsMatch(t, []string{"example.com/app/a", "example.com/app/b"}, cycles[0].Path)
}

func TestShortestPath_Direct(t *testing.T) {
	g := buildTestGraph()
	path := g.ShortestPath("example.com/app/a", "example.com/app/b")
	require.NotNil(t, path)
	assert.Equal(t, []string{"example.com/app/a", "example.com/app/b"}, path)
}

func TestShortestPath_Transitive(t *testing.T) {
	g := buildTestGraph()
	path := g.ShortestPath("example.com/app/a", "example.com/app/d")
	require.NotNil(t, path)
	assert.Len(t, path, 3)
	assert.Equal(t, "example.com/app/a", path[0])
	assert.Equal(t, "example.com/app/d", path[2])
}

func TestShortestPath_Unreachable(t *testing.T) {
	g := buildTestGraph()
	// d has no forward edges, so no path from d to a.
	path := g.ShortestPath("example.com/app/d", "example.com/app/a")
	assert.Nil(t, path)
}

func TestShortestPath_Same(t *testing.T) {
	g := buildTestGraph()
	path := g.ShortestPath("example.com/app/a", "example.com/app/a")
	assert.Equal(t, []string{"example.com/app/a"}, path)
}

// ── Format tests ─────────────────────────────────────────────────────────────

func TestFormatDeps(t *testing.T) {
	g := buildTestGraph()
	entries := g.Deps("example.com/app/a", 0)
	out := FormatDeps("example.com/app/a", g.Module, entries, 0)

	// Root line is the full import path.
	assert.True(t, strings.HasPrefix(out, "example.com/app/a"), "output should start with root")
	// Short names (module prefix stripped) appear in the body.
	assert.Contains(t, out, "b")
	assert.Contains(t, out, "c")
	assert.Contains(t, out, "d")
	// Indentation: depth-1 entries have 2 leading spaces.
	lines := strings.Split(out, "\n")
	require.Greater(t, len(lines), 1)
	for _, line := range lines[1:] {
		assert.True(t, strings.HasPrefix(line, " "), "entry lines should be indented: %q", line)
	}
}

func TestFormatImpact(t *testing.T) {
	g := buildTestGraph()
	packages := g.Impact("example.com/app/d")
	out := FormatImpact(packages, g.Module, 0)
	assert.True(t, strings.HasPrefix(out, "Impact: 4 package"), "should report 4 packages")
	assert.Contains(t, out, "a")
	assert.Contains(t, out, "b")
	assert.Contains(t, out, "c")
	assert.Contains(t, out, "d")
}

func TestFormatCycles(t *testing.T) {
	cycles := []Cycle{
		{Path: []string{"example.com/app/a", "example.com/app/b"}},
	}
	out := FormatCycles(cycles, "example.com/app")
	assert.Contains(t, out, "→")
	assert.Contains(t, out, "a")
	assert.Contains(t, out, "b")
	// Cycle is closed: first node repeated at end.
	assert.Contains(t, out, "a → b → a")
}

func TestFormatPath(t *testing.T) {
	g := buildTestGraph()
	path := g.ShortestPath("example.com/app/a", "example.com/app/d")
	require.NotNil(t, path)
	out := FormatPath(path, g.Module)
	assert.Contains(t, out, "→")
	assert.True(t, strings.HasPrefix(out, "a"), "output should start with short name 'a'")
	assert.True(t, strings.HasSuffix(out, "d"), "output should end with short name 'd'")
}

func TestFormatPath_Empty(t *testing.T) {
	out := FormatPath(nil, "example.com/app")
	assert.Equal(t, "No path found", out)
}

// ── ResolvePackage tests ──────────────────────────────────────────────────────

func TestResolvePackage(t *testing.T) {
	g := buildTestGraph()

	// Full import path resolves directly.
	got, ok := g.ResolvePackage("example.com/app/a")
	require.True(t, ok)
	assert.Equal(t, "example.com/app/a", got)

	// Directory path resolves via exact dir match.
	got, ok = g.ResolvePackage("/src/b")
	require.True(t, ok)
	assert.Equal(t, "example.com/app/b", got)

	// Unknown path returns false.
	_, ok = g.ResolvePackage("example.com/app/unknown")
	assert.False(t, ok)
}
