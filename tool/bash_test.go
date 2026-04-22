package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

func TestClassifyExitCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		command string
		code    int
		label   string
		isErr   bool
	}{
		{"grep no match", "grep foo /etc/hosts", 1, "no matches", false},
		{"rg no match", "rg foo src/", 1, "no matches", false},
		{"egrep no match", "egrep foo file", 1, "no matches", false},
		{"fgrep no match", "fgrep foo file", 1, "no matches", false},
		{"diff differ", "diff a.txt b.txt", 1, "files differ", false},
		{"cmp differ", "cmp a.txt b.txt", 1, "files differ", false},
		{"test false", "test 1 -eq 2", 1, "condition false", false},
		{"[ false", "[ 1 -eq 2 ]", 1, "condition false", false},
		{"sudo grep no match", "sudo grep foo file", 1, "no matches", false},
		{"env var grep no match", "LC_ALL=C grep foo file", 1, "no matches", false},
		{"absolute path grep", "/usr/bin/grep foo file", 1, "no matches", false},
		{"unrelated exit 1", "false", 1, "", true},
		{"grep exit 2 (error)", "grep foo file", 2, "", true},
		{"unknown cmd exit 1", "somecmd arg", 1, "", true},
		{"grep exit 0 never classified", "grep foo file", 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			label, isErr := classifyExitCode(tc.command, tc.code)
			if label != tc.label || isErr != tc.isErr {
				t.Errorf("classifyExitCode(%q, %d) = (%q, %v); want (%q, %v)",
					tc.command, tc.code, label, isErr, tc.label, tc.isErr)
			}
		})
	}
}

// resultText concatenates the text of a *core.ToolResult's TextContent blocks.
func resultText(r *core.ToolResult) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range r.Content {
		if tc, ok := block.(core.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// isErrorResult reports whether the tool result represents an error.
// Tool errors are signaled by the "[error:CODE]" prefix produced by errfmt.ToolErr.
func isErrorResult(r *core.ToolResult) bool {
	return strings.HasPrefix(resultText(r), "[error:")
}

func TestBashTool_GrepNoMatches(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("grep"); err != nil {
		t.Skip("grep not on PATH")
	}
	app := ext.NewApp(t.TempDir())
	tool := bashTool(app, BashConfig{}.withDefaults())
	res, err := tool.Execute(context.Background(), "id1", map[string]any{
		"command": "grep __piglet_nonexistent_pattern__ /etc/hosts",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if isErrorResult(res) {
		t.Fatalf("expected non-error result for grep no-match; got: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "no matches") {
		t.Errorf("expected annotation 'no matches' in output, got: %s", resultText(res))
	}
}

func TestBashTool_DiffFilesDiffer(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("diff"); err != nil {
		t.Skip("diff not on PATH")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("alpha\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("beta\n"), 0644); err != nil {
		t.Fatal(err)
	}
	app := ext.NewApp(t.TempDir())
	tool := bashTool(app, BashConfig{}.withDefaults())
	res, err := tool.Execute(context.Background(), "id2", map[string]any{
		"command": "diff " + a + " " + b,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if isErrorResult(res) {
		t.Fatalf("expected non-error result for diff differing files; got: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "files differ") {
		t.Errorf("expected annotation 'files differ' in output, got: %s", resultText(res))
	}
}

func TestBashTool_FalseIsError(t *testing.T) {
	t.Parallel()
	app := ext.NewApp(t.TempDir())
	tool := bashTool(app, BashConfig{}.withDefaults())
	res, err := tool.Execute(context.Background(), "id3", map[string]any{
		"command": "false",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !isErrorResult(res) {
		t.Fatalf("expected error result for unclassified exit 1; got: %s", resultText(res))
	}
}
