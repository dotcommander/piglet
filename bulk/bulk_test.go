package bulk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotcommander/piglet/bulk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- expandTemplate --------------------------------------------------------

func TestExpandTemplate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		tmpl     string
		item     bulk.Item
		expected string
	}{
		{
			name:     "path substitution",
			tmpl:     "make -C {path} clean",
			item:     bulk.Item{Name: "myrepo", Path: "/home/user/projects/myrepo"},
			expected: "make -C /home/user/projects/myrepo clean",
		},
		{
			name:     "name substitution",
			tmpl:     "echo {name}",
			item:     bulk.Item{Name: "myrepo", Path: "/home/user/projects/myrepo"},
			expected: "echo myrepo",
		},
		{
			name:     "dir substitution",
			tmpl:     "ls {dir}",
			item:     bulk.Item{Name: "myrepo", Path: "/home/user/projects/myrepo"},
			expected: "ls /home/user/projects",
		},
		{
			name:     "basename substitution strips extension",
			tmpl:     "process {basename}",
			item:     bulk.Item{Name: "report.txt", Path: "/data/report.txt"},
			expected: "process report",
		},
		{
			name:     "multiple substitutions in one template",
			tmpl:     "cp {path}/{name} {dir}/backup",
			item:     bulk.Item{Name: "file", Path: "/src/file"},
			expected: "cp /src/file/file /src/backup",
		},
		{
			name:     "no substitution plain command",
			tmpl:     "git status",
			item:     bulk.Item{Name: "repo", Path: "/repos/repo"},
			expected: "git status",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// expandTemplate is unexported; test via Run with DryRun=true
			// which includes expanded template in the output field.
			ctx := context.Background()
			items := []bulk.Item{tc.item}
			cmd := bulk.Command{Template: tc.tmpl}
			cfg := bulk.Config{DryRun: true}
			summary, err := bulk.Execute(ctx, &bulk.ListScanner{Paths: []string{tc.item.Path}}, nil, cmd, cfg)
			require.NoError(t, err)
			require.Len(t, summary.Results, 1)
			assert.Contains(t, summary.Results[0].Output, tc.expected, "expanded template mismatch")
			_ = items // suppress unused
		})
	}
}

// ---- DirScanner ------------------------------------------------------------

func TestDirScanner_FindsDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "alpha"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "beta"), 0700))
	// Create a file (not a dir) — should be ignored
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0600))

	s := &bulk.DirScanner{Root: root}
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "alpha", items[0].Name)
	assert.Equal(t, "beta", items[1].Name)
}

func TestDirScanner_RespectsDepthLimit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// depth-1 repo (has .git)
	shallow := filepath.Join(root, "shallow")
	require.NoError(t, os.MkdirAll(filepath.Join(shallow, ".git"), 0700))
	// depth-2 repo (nested inside a non-repo parent)
	deep := filepath.Join(root, "parent", "deep")
	require.NoError(t, os.MkdirAll(filepath.Join(deep, ".git"), 0700))

	hasGit := func(path string) bool {
		info, err := os.Stat(filepath.Join(path, ".git"))
		return err == nil && info.IsDir()
	}

	// At depth=1, only "shallow" should appear; "deep" is inside "parent" which has no .git.
	s1 := &bulk.DirScanner{Root: root, Depth: 1, Match: hasGit}
	items1, err := s1.Scan(context.Background())
	require.NoError(t, err)
	names1 := make([]string, len(items1))
	for i, it := range items1 {
		names1[i] = it.Name
	}
	assert.Contains(t, names1, "shallow")
	assert.NotContains(t, names1, "deep")

	// At depth=2, "deep" should also appear.
	s2 := &bulk.DirScanner{Root: root, Depth: 2, Match: hasGit}
	items2, err := s2.Scan(context.Background())
	require.NoError(t, err)
	names2 := make([]string, len(items2))
	for i, it := range items2 {
		names2[i] = it.Name
	}
	assert.Contains(t, names2, "shallow")
	assert.Contains(t, names2, "deep")
}

func TestDirScanner_EmptyDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	s := &bulk.DirScanner{Root: root}
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestDirScanner_SortedOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, name := range []string{"zebra", "apple", "mango"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, name), 0700))
	}

	s := &bulk.DirScanner{Root: root}
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "apple", items[0].Name)
	assert.Equal(t, "mango", items[1].Name)
	assert.Equal(t, "zebra", items[2].Name)
}

func TestDirScanner_MatchFunctionFilters(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// "withmarker" contains a marker file; "plain" does not.
	withMarker := filepath.Join(root, "withmarker")
	plain := filepath.Join(root, "plain")
	require.NoError(t, os.MkdirAll(withMarker, 0700))
	require.NoError(t, os.MkdirAll(plain, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(withMarker, "MARKER"), []byte(""), 0600))

	s := &bulk.DirScanner{
		Root: root,
		Match: func(path string) bool {
			_, err := os.Stat(filepath.Join(path, "MARKER"))
			return err == nil
		},
	}
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "withmarker", items[0].Name)
}

// ---- ListScanner -----------------------------------------------------------

func TestListScanner_WrapsPathsIntoItems(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	p1 := filepath.Join(root, "one")
	p2 := filepath.Join(root, "two")
	require.NoError(t, os.MkdirAll(p1, 0700))
	require.NoError(t, os.MkdirAll(p2, 0700))

	s := &bulk.ListScanner{Paths: []string{p1, p2}}
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	// Check that Name is derived from path base
	for _, it := range items {
		assert.Equal(t, filepath.Base(it.Path), it.Name)
	}
}

func TestListScanner_SortedOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "zebra"),
		filepath.Join(root, "apple"),
		filepath.Join(root, "mango"),
	}
	for _, p := range paths {
		require.NoError(t, os.MkdirAll(p, 0700))
	}

	s := &bulk.ListScanner{Paths: paths}
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "apple", items[0].Name)
	assert.Equal(t, "mango", items[1].Name)
	assert.Equal(t, "zebra", items[2].Name)
}

// ---- GlobScanner -----------------------------------------------------------

func TestGlobScanner_FindsMatchingFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte(""), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.go"), []byte(""), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "c.txt"), []byte(""), 0600))

	s := &bulk.GlobScanner{Pattern: "*.go", Root: root}
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "a.go", items[0].Name)
	assert.Equal(t, "b.go", items[1].Name)
}

func TestGlobScanner_RelativePatternWithRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "match.yaml"), []byte(""), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.json"), []byte(""), 0600))

	s := &bulk.GlobScanner{Pattern: "*.yaml", Root: root}
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "match.yaml", items[0].Name)
}

// ---- ShellFilter -----------------------------------------------------------

func TestShellFilter_Exit0KeepsItem(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	item := bulk.Item{Name: "x", Path: root}

	f := bulk.ShellFilter("true")
	keep, err := f(context.Background(), item)
	require.NoError(t, err)
	assert.True(t, keep)
}

func TestShellFilter_Exit1ExcludesItem(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	item := bulk.Item{Name: "x", Path: root}

	f := bulk.ShellFilter("false")
	keep, err := f(context.Background(), item)
	require.NoError(t, err)
	assert.False(t, keep)
}

func TestShellFilter_FileCheckCommand(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	markerPath := filepath.Join(root, "present.txt")
	require.NoError(t, os.WriteFile(markerPath, []byte(""), 0600))

	withFile := bulk.Item{Name: "withfile", Path: root}
	withoutFile := bulk.Item{Name: "withoutfile", Path: t.TempDir()}

	f := bulk.ShellFilter("test -f present.txt")

	keepWith, err := f(context.Background(), withFile)
	require.NoError(t, err)
	assert.True(t, keepWith)

	keepWithout, err := f(context.Background(), withoutFile)
	require.NoError(t, err)
	assert.False(t, keepWithout)
}

// ---- Apply -----------------------------------------------------------------

func TestApply_NilFilterReturnsAll(t *testing.T) {
	t.Parallel()

	items := []bulk.Item{
		{Name: "a", Path: "/a"},
		{Name: "b", Path: "/b"},
	}
	result, err := bulk.Apply(context.Background(), items, nil, 8)
	require.NoError(t, err)
	assert.Equal(t, items, result)
}

func TestApply_FilterExcludesSomeItems(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir1 := filepath.Join(root, "alpha")
	dir2 := filepath.Join(root, "beta")
	require.NoError(t, os.MkdirAll(dir1, 0700))
	require.NoError(t, os.MkdirAll(dir2, 0700))
	// Only alpha gets a marker
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "KEEP"), []byte(""), 0600))

	items := []bulk.Item{
		{Name: "alpha", Path: dir1},
		{Name: "beta", Path: dir2},
	}
	f := bulk.ShellFilter("test -f KEEP")
	result, err := bulk.Apply(context.Background(), items, f, 8)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "alpha", result[0].Name)
}

func TestApply_ResultsSortedByName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirs := []string{"zebra", "apple", "mango"}
	items := make([]bulk.Item, len(dirs))
	for i, name := range dirs {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(p, 0700))
		items[i] = bulk.Item{Name: name, Path: p}
	}

	// Keep everything
	result, err := bulk.Apply(context.Background(), items, bulk.ShellFilter("true"), 8)
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "apple", result[0].Name)
	assert.Equal(t, "mango", result[1].Name)
	assert.Equal(t, "zebra", result[2].Name)
}

// ---- Run -------------------------------------------------------------------

func TestRun_ExecutesCommandOnEachItem(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirs := []string{"alpha", "beta"}
	items := make([]bulk.Item, len(dirs))
	for i, name := range dirs {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(p, 0700))
		items[i] = bulk.Item{Name: name, Path: p}
	}

	cmd := bulk.Command{Template: "echo hello"}
	results := bulk.Run(context.Background(), items, cmd, bulk.Config{})
	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, "ok", r.Status)
		assert.Contains(t, r.Output, "hello")
	}
}

func TestRun_SortedResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirs := []string{"zebra", "apple", "mango"}
	items := make([]bulk.Item, len(dirs))
	for i, name := range dirs {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(p, 0700))
		items[i] = bulk.Item{Name: name, Path: p}
	}

	results := bulk.Run(context.Background(), items, bulk.Command{Template: "true"}, bulk.Config{})
	require.Len(t, results, 3)
	assert.Equal(t, "apple", results[0].Item)
	assert.Equal(t, "mango", results[1].Item)
	assert.Equal(t, "zebra", results[2].Item)
}

func TestRun_HandlesCommandErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	items := []bulk.Item{{Name: "x", Path: root}}

	results := bulk.Run(context.Background(), items, bulk.Command{Template: "exit 1"}, bulk.Config{})
	require.Len(t, results, 1)
	assert.Equal(t, "error", results[0].Status)
}

func TestRun_DryRunReturnsSkipped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "mydir")
	require.NoError(t, os.MkdirAll(dir, 0700))

	scanner := &bulk.ListScanner{Paths: []string{dir}}
	cmd := bulk.Command{Template: "rm -rf {path}"}
	cfg := bulk.Config{DryRun: true}

	summary, err := bulk.Execute(context.Background(), scanner, nil, cmd, cfg)
	require.NoError(t, err)
	require.Len(t, summary.Results, 1)
	assert.Equal(t, "skipped", summary.Results[0].Status)
	assert.Contains(t, summary.Results[0].Output, "dry run")
}

// ---- Execute pipeline ------------------------------------------------------

func TestExecute_FullPipeline(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Three subdirs; only two contain a "GO" marker file.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(p, 0700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "alpha", "GO"), []byte(""), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "gamma", "GO"), []byte(""), 0600))

	scanner := &bulk.DirScanner{Root: root}
	filter := bulk.ShellFilter("test -f GO")
	cmd := bulk.Command{Template: "echo done"}
	cfg := bulk.Config{}

	summary, err := bulk.Execute(context.Background(), scanner, filter, cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, 3, summary.TotalCollected)
	assert.Equal(t, 2, summary.MatchedFilter)
	require.Len(t, summary.Results, 2)
	// Results are sorted; alpha < gamma
	assert.Equal(t, "alpha", summary.Results[0].Item)
	assert.Equal(t, "gamma", summary.Results[1].Item)
	for _, r := range summary.Results {
		assert.Equal(t, "ok", r.Status)
	}
}
