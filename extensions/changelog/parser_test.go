package changelog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCommitLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		line      string
		wantType  string
		wantScope string
		wantMsg   string
		wantBreak bool
	}{
		{
			name:      "feat with scope",
			line:      "abc1234567890|2026-04-01 10:00:00 -0500|Gary|feat(auth): add OAuth2 support",
			wantType:  "feat",
			wantScope: "auth",
			wantMsg:   "add OAuth2 support",
		},
		{
			name:      "breaking fix no scope",
			line:      "def4567890123|2026-04-01 10:00:00 -0500|Gary|fix!: remove deprecated API",
			wantType:  "fix",
			wantMsg:   "remove deprecated API",
			wantBreak: true,
		},
		{
			name:     "docs no scope",
			line:     "ghi7890123456|2026-04-01 10:00:00 -0500|Gary|docs: update README",
			wantType: "docs",
			wantMsg:  "update README",
		},
		{
			name:     "non-conventional",
			line:     "jkl0123456789|2026-04-01 10:00:00 -0500|Gary|random commit message",
			wantType: "other",
			wantMsg:  "random commit message",
		},
		{
			name:     "subject with pipe",
			line:     "mno3456789012|2026-04-01 10:00:00 -0500|Gary|feat: support a|b syntax",
			wantType: "feat",
			wantMsg:  "support a|b syntax",
		},
		{
			name:      "breaking feat with scope",
			line:      "pqr6789012345|2026-04-01 10:00:00 -0500|Gary|feat(api)!: redesign endpoints",
			wantType:  "feat",
			wantScope: "api",
			wantMsg:   "redesign endpoints",
			wantBreak: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, err := parseCommitLine(tt.line)
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, c.Type)
			assert.Equal(t, tt.wantScope, c.Scope)
			assert.Equal(t, tt.wantMsg, c.Message)
			assert.Equal(t, tt.wantBreak, c.Breaking)
			assert.Len(t, c.Hash, 7)
			assert.Len(t, c.Date, 10)
		})
	}
}

func TestParseCommitLine_Malformed(t *testing.T) {
	t.Parallel()
	_, err := parseCommitLine("no-pipes-here")
	assert.Error(t, err)
}

func TestGroupCommits(t *testing.T) {
	t.Parallel()

	commits := []Commit{
		{Type: "feat", Message: "a"},
		{Type: "fix", Message: "b"},
		{Type: "feat", Message: "c"},
		{Type: "other", Message: "d"},
	}

	groups := GroupCommits(commits)
	assert.Len(t, groups["feat"], 2)
	assert.Len(t, groups["fix"], 1)
	assert.Len(t, groups["other"], 1)
}

func TestBreakingCommits(t *testing.T) {
	t.Parallel()

	commits := []Commit{
		{Type: "feat", Breaking: false},
		{Type: "fix", Breaking: true, Message: "break"},
		{Type: "feat", Breaking: true, Message: "also break"},
	}

	breaking := BreakingCommits(commits)
	assert.Len(t, breaking, 2)
}

func TestTypeOrder(t *testing.T) {
	t.Parallel()

	types := map[string]TypeConfig{
		"fix":  {Order: 2},
		"feat": {Order: 1},
		"docs": {Order: 5},
	}

	order := typeOrder(types)
	assert.Equal(t, []string{"feat", "fix", "docs"}, order)
}

func TestDetectRange_ExplicitRef(t *testing.T) {
	t.Parallel()
	got := DetectRange("/tmp", "v1.0.0..HEAD", 20)
	assert.Equal(t, "v1.0.0..HEAD", got)
}

func TestDetectRange_NoTags(t *testing.T) {
	t.Parallel()
	got := DetectRange("/tmp/nonexistent", "", 15)
	assert.Equal(t, "HEAD~15..HEAD", got)
}

func TestRepoURL_NotGitRepo(t *testing.T) {
	t.Parallel()
	got := RepoURL("/tmp/nonexistent-repo-dir")
	assert.Equal(t, "", got)
}
