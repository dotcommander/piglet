package bulk_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dotcommander/piglet/bulk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRepo creates a git repository in dir with an initial empty commit.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	run("checkout", "-b", "main")
	run("commit", "--allow-empty", "-m", "initial")
}

// writeFile creates a file in dir with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0600))
}

// ---- GitRepoScanner --------------------------------------------------------

func TestGitRepoScanner_FindsGitRepos(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	repo1 := filepath.Join(root, "alpha")
	repo2 := filepath.Join(root, "beta")
	notARepo := filepath.Join(root, "notarepo")

	require.NoError(t, os.MkdirAll(repo1, 0700))
	require.NoError(t, os.MkdirAll(repo2, 0700))
	require.NoError(t, os.MkdirAll(notARepo, 0700))

	// Create real .git directories (as dirs, matching GitRepoScanner's Stat check).
	require.NoError(t, os.MkdirAll(filepath.Join(repo1, ".git"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(repo2, ".git"), 0700))

	s := bulk.GitRepoScanner(root, 1)
	items, err := s.Scan(context.Background())
	require.NoError(t, err)

	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	assert.Contains(t, names, "alpha")
	assert.Contains(t, names, "beta")
	assert.NotContains(t, names, "notarepo")
}

func TestGitRepoScanner_IgnoresNonGitDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plain"), 0700))

	s := bulk.GitRepoScanner(root, 1)
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestGitRepoScanner_SortedOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, name := range []string{"zebra", "apple", "mango"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, name, ".git"), 0700))
	}

	s := bulk.GitRepoScanner(root, 1)
	items, err := s.Scan(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "apple", items[0].Name)
	assert.Equal(t, "mango", items[1].Name)
	assert.Equal(t, "zebra", items[2].Name)
}

// ---- GitFilter -------------------------------------------------------------

func TestGitFilter_All(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	repo1 := filepath.Join(root, "alpha")
	repo2 := filepath.Join(root, "beta")
	require.NoError(t, os.MkdirAll(repo1, 0700))
	require.NoError(t, os.MkdirAll(repo2, 0700))
	initRepo(t, repo1)
	initRepo(t, repo2)

	f, err := bulk.GitFilter("all")
	require.NoError(t, err)
	assert.Nil(t, f, "all filter should return nil (no filtering)")

	items := []bulk.Item{
		{Name: "alpha", Path: repo1},
		{Name: "beta", Path: repo2},
	}
	result, err := bulk.Apply(context.Background(), items, f, 8)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestGitFilter_Dirty(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	clean := filepath.Join(root, "clean")
	dirty := filepath.Join(root, "dirty")
	require.NoError(t, os.MkdirAll(clean, 0700))
	require.NoError(t, os.MkdirAll(dirty, 0700))
	initRepo(t, clean)
	initRepo(t, dirty)

	// Add an untracked file to dirty repo
	writeFile(t, dirty, "untracked.txt", "hello")

	f, err := bulk.GitFilter("dirty")
	require.NoError(t, err)
	require.NotNil(t, f)

	items := []bulk.Item{
		{Name: "clean", Path: clean},
		{Name: "dirty", Path: dirty},
	}
	result, err := bulk.Apply(context.Background(), items, f, 8)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "dirty", result[0].Name)
}

func TestGitFilter_Clean(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	cleanRepo := filepath.Join(root, "cleanrepo")
	dirtyRepo := filepath.Join(root, "dirtyrepo")
	require.NoError(t, os.MkdirAll(cleanRepo, 0700))
	require.NoError(t, os.MkdirAll(dirtyRepo, 0700))
	initRepo(t, cleanRepo)
	initRepo(t, dirtyRepo)

	writeFile(t, dirtyRepo, "new.txt", "change")

	f, err := bulk.GitFilter("clean")
	require.NoError(t, err)
	require.NotNil(t, f)

	items := []bulk.Item{
		{Name: "cleanrepo", Path: cleanRepo},
		{Name: "dirtyrepo", Path: dirtyRepo},
	}
	result, err := bulk.Apply(context.Background(), items, f, 8)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "cleanrepo", result[0].Name)
}

func TestGitFilter_Ahead(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)

	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = gitEnv
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	// Create a bare "remote" repository
	remote := filepath.Join(root, "remote.git")
	require.NoError(t, os.MkdirAll(remote, 0700))
	run(remote, "init", "--bare")

	// Clone it locally
	local := filepath.Join(root, "local")
	run(root, "clone", remote, "local")

	// Push an initial commit so tracking branch exists
	writeFile(t, local, "init.txt", "init")
	run(local, "add", ".")
	run(local, "commit", "-m", "initial")
	run(local, "push", "-u", "origin", "HEAD:main")

	// Make one more local commit without pushing — local is now ahead
	writeFile(t, local, "extra.txt", "content")
	run(local, "add", ".")
	run(local, "commit", "-m", "unpushed commit")

	// Create a clean repo with no upstream (should be excluded gracefully)
	cleanLocal := filepath.Join(root, "clean")
	require.NoError(t, os.MkdirAll(cleanLocal, 0700))
	initRepo(t, cleanLocal)

	f, err := bulk.GitFilter("ahead")
	require.NoError(t, err)
	require.NotNil(t, f)

	items := []bulk.Item{
		{Name: "local", Path: local},
		{Name: "clean", Path: cleanLocal},
	}
	result, err := bulk.Apply(context.Background(), items, f, 8)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "local", result[0].Name)
}

func TestGitFilter_UnknownReturnsError(t *testing.T) {
	t.Parallel()

	_, err := bulk.GitFilter("nonexistent")
	assert.Error(t, err)
}
