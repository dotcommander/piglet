package sift

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchRule_NoRules(t *testing.T) {
	t.Parallel()
	got := matchRule("Bash", "some output", nil)
	assert.Nil(t, got)
}

func TestMatchRule_WrongTool(t *testing.T) {
	t.Parallel()
	rules := []StructuredRule{
		{Tool: "Read", Detect: "golangci-lint", Columns: []string{"file", "line", "message"}},
	}
	got := matchRule("Bash", "golangci-lint output", rules)
	assert.Nil(t, got)
}

func TestMatchRule_DetectStringMissing(t *testing.T) {
	t.Parallel()
	rules := []StructuredRule{
		{Tool: "Bash", Detect: "golangci-lint", Columns: []string{"file", "line", "message"}},
	}
	got := matchRule("Bash", "go vet output", rules)
	assert.Nil(t, got)
}

func TestMatchRule_Matches(t *testing.T) {
	t.Parallel()
	rules := []StructuredRule{
		{Tool: "Bash", Detect: "golangci-lint", Columns: []string{"file", "line", "linter", "message"}},
	}
	got := matchRule("Bash", "golangci-lint run ./...", rules)
	require.NotNil(t, got)
	assert.Equal(t, "Bash", got.Tool)
	assert.Equal(t, "golangci-lint", got.Detect)
}

func TestMatchRule_FirstMatchReturned(t *testing.T) {
	t.Parallel()
	rules := []StructuredRule{
		{Tool: "Bash", Detect: "golangci-lint", Columns: []string{"file", "line", "linter", "message"}},
		{Tool: "Bash", Detect: "go vet", Columns: []string{"file", "line", "message"}},
	}
	// Text contains both — first matching rule wins
	got := matchRule("Bash", "golangci-lint and go vet output", rules)
	require.NotNil(t, got)
	assert.Equal(t, "golangci-lint", got.Detect)
}

func TestParseLinterOutput_Empty(t *testing.T) {
	t.Parallel()
	rows := parseLinterOutput("", "golangci-lint", "")
	assert.Empty(t, rows)
}

func TestParseLinterOutput_BlankLines(t *testing.T) {
	t.Parallel()
	rows := parseLinterOutput("\n\n\n", "golangci-lint", "")
	assert.Empty(t, rows)
}

func TestParseLinterOutput_GolangciLintFormat(t *testing.T) {
	t.Parallel()
	text := "internal/foo/bar.go:42:10: some error message (errcheck)"
	rows := parseLinterOutput(text, "golangci-lint", "")
	require.Len(t, rows, 1)
	assert.Equal(t, "internal/foo/bar.go", rows[0]["file"])
	assert.Equal(t, "42", rows[0]["line"])
	assert.Equal(t, "10", rows[0]["col"])
	assert.Equal(t, "errcheck", rows[0]["linter"])
	assert.Equal(t, "some error message", rows[0]["message"])
}

func TestParseLinterOutput_FileLineFormat(t *testing.T) {
	t.Parallel()
	text := "main.go:10: something is wrong here"
	rows := parseLinterOutput(text, "go vet", "")
	require.Len(t, rows, 1)
	assert.Equal(t, "main.go", rows[0]["file"])
	assert.Equal(t, "10", rows[0]["line"])
	assert.Equal(t, "something is wrong here", rows[0]["message"])
}

func TestParseLinterOutput_FileParenFormat(t *testing.T) {
	t.Parallel()
	text := "main.go(25): warning: deprecated function"
	rows := parseLinterOutput(text, "generic", "")
	require.Len(t, rows, 1)
	assert.Equal(t, "main.go", rows[0]["file"])
	assert.Equal(t, "25", rows[0]["line"])
	assert.Equal(t, "warning: deprecated function", rows[0]["message"])
}

func TestParseLinterOutput_MessageTruncatedAt80Runes(t *testing.T) {
	t.Parallel()
	longMsg := strings.Repeat("x", 90)
	text := "main.go:1: " + longMsg
	rows := parseLinterOutput(text, "go vet", "")
	require.Len(t, rows, 1)
	// message should be truncated to 77 runes + "..."
	assert.Len(t, []rune(rows[0]["message"]), 80)
	assert.True(t, strings.HasSuffix(rows[0]["message"], "..."))
}

func TestParseLinterOutput_MessageExactly80Runes(t *testing.T) {
	t.Parallel()
	msg := strings.Repeat("y", 80)
	text := "main.go:1: " + msg
	rows := parseLinterOutput(text, "go vet", "")
	require.Len(t, rows, 1)
	assert.Equal(t, msg, rows[0]["message"])
}

func TestParseLinterOutput_CWDStripped(t *testing.T) {
	t.Parallel()
	text := "/home/user/project/pkg/foo.go:5:1: some error (errcheck)"
	rows := parseLinterOutput(text, "golangci-lint", "/home/user/project")
	require.Len(t, rows, 1)
	assert.Equal(t, "pkg/foo.go", rows[0]["file"])
}

func TestParseLinterOutput_MultipleLines(t *testing.T) {
	t.Parallel()
	text := strings.Join([]string{
		"foo.go:1:1: first error (errcheck)",
		"bar.go:2:3: second error (govet)",
		"baz.go:10: third error",
	}, "\n")
	rows := parseLinterOutput(text, "golangci-lint", "")
	assert.Len(t, rows, 3)
}

func TestParseLinterOutput_NoMatchLines(t *testing.T) {
	t.Parallel()
	text := "Run golangci-lint:\nAll good!"
	rows := parseLinterOutput(text, "golangci-lint", "")
	assert.Empty(t, rows)
}

func TestBuildTable_EmptyRows(t *testing.T) {
	t.Parallel()
	columns := []string{"file", "line", "message"}
	got := buildTable(columns, nil)
	// Should have header and separator rows
	assert.Contains(t, got, "| file |")
	assert.Contains(t, got, "| line |")
	assert.Contains(t, got, "| message |")
	assert.Contains(t, got, "------|")
}

func TestBuildTable_WithRows(t *testing.T) {
	t.Parallel()
	columns := []string{"file", "line", "message"}
	rows := []row{
		{"file": "main.go", "line": "42", "message": "error here"},
	}
	got := buildTable(columns, rows)
	assert.Contains(t, got, "main.go")
	assert.Contains(t, got, "42")
	assert.Contains(t, got, "error here")
}

func TestBuildTable_MissingColumnValueEmptyString(t *testing.T) {
	t.Parallel()
	columns := []string{"file", "line", "linter", "message"}
	rows := []row{
		{"file": "a.go", "line": "1", "message": "err"},
		// linter key absent — should render as empty cell
	}
	got := buildTable(columns, rows)
	assert.Contains(t, got, "a.go")
	// linter cell should be empty (two pipes with spaces around empty string)
	assert.Contains(t, got, "|  |")
}

func TestBuildTable_ColumnOrder(t *testing.T) {
	t.Parallel()
	columns := []string{"message", "file", "line"}
	rows := []row{
		{"file": "f.go", "line": "5", "message": "msg"},
	}
	got := buildTable(columns, rows)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// Header row: | message | file | line |
	assert.Contains(t, lines[0], "message")
	// Ensure message comes before file in the header
	assert.Less(t, strings.Index(lines[0], "message"), strings.Index(lines[0], "file"))
}

func TestCompressStructured_NoRules(t *testing.T) {
	t.Parallel()
	cfg := Config{} // no structured rules
	_, ok := CompressStructured("Bash", "golangci-lint output", cfg, "")
	assert.False(t, ok)
}

func TestCompressStructured_NoMatchingRule(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	_, ok := CompressStructured("Bash", "no linter output here", cfg, "")
	assert.False(t, ok)
}

func TestCompressStructured_GolangciLint(t *testing.T) {
	t.Parallel()
	text := strings.Join([]string{
		"internal/foo/bar.go:10:5: some lint error (errcheck)",
		"internal/baz/qux.go:20:1: another lint error (govet)",
	}, "\n") + "\ngolangci-lint finished"

	cfg := DefaultConfig()
	result, ok := CompressStructured("Bash", text, cfg, "")
	require.True(t, ok)
	assert.Contains(t, result, "[SIFT: structured")
	assert.Contains(t, result, "internal/foo/bar.go")
	assert.Contains(t, result, "errcheck")
}

func TestSeverityRank(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int
	}{
		{"error: something", 0},
		{"Error", 0},
		{"warning: something", 1},
		{"WARN", 1},
		{"info: something", 2},
		{"INFO", 2},
		{"note", 3},
		{"", 3},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, severityRank(tc.input))
		})
	}
}

func TestDeprefixPath_Match(t *testing.T) {
	t.Parallel()
	rel, err := deprefixPath("/home/user/project/pkg/foo.go", "/home/user/project")
	require.NoError(t, err)
	assert.Equal(t, "pkg/foo.go", rel)
}

func TestDeprefixPath_NoMatch(t *testing.T) {
	t.Parallel()
	_, err := deprefixPath("/other/path/file.go", "/home/user/project")
	assert.Error(t, err)
}

func TestDeprefixPath_NotAbsolutePrefix(t *testing.T) {
	t.Parallel()
	_, err := deprefixPath("relative/path.go", "relative/prefix")
	assert.Error(t, err)
}

func TestExtractLinter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"some error (errcheck)", "errcheck"},
		{"another error (govet)", "govet"},
		{"no linter here", ""},
		{"has space (not a linter name)", ""},
		{"nested (outer (inner))", "inner"},
		{"no closing paren (open", ""},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := extractLinter(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}
