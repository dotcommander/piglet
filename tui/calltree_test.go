package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/dotcommander/piglet/tool"
)

func TestFormatDiffMeta(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dm   tool.DiffMeta
		want string
	}{
		{"typical edit", tool.DiffMeta{Added: 47, Removed: 8, Files: 1, Hunks: 3}, "+47 -8 · 1f 3h"},
		{"new file", tool.DiffMeta{Added: 12, Removed: 0, Files: 1, Hunks: 1}, "+12 -0 · 1f 1h"},
		{"zero", tool.DiffMeta{}, "+0 -0 · 0f 0h"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := formatDiffMeta(tc.dm); got != tc.want {
				t.Errorf("formatDiffMeta(%+v) = %q, want %q", tc.dm, got, tc.want)
			}
		})
	}
}

func TestPadOrTruncate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		s         string
		w         int
		want      string
		checkRune bool // verify rune length == w
	}{
		{"exact", "hello", 5, "hello", true},
		{"pad", "hi", 5, "hi   ", true},
		{"empty", "", 5, "     ", true},
		{"truncate long", "verylongstring", 8, "", true}, // just check rune len + contains "…"
		{"single exact", "x", 1, "x", true},
		{"two to one", "xy", 1, "…", true},
		{"zero width", "anything", 0, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := padOrTruncate(tc.s, tc.w)
			if tc.w == 0 {
				if got != tc.want {
					t.Errorf("padOrTruncate(%q, 0) = %q, want %q", tc.s, got, tc.want)
				}
				return
			}
			if tc.checkRune && len([]rune(got)) != tc.w {
				t.Errorf("padOrTruncate(%q, %d): rune len = %d, want %d (got %q)", tc.s, tc.w, len([]rune(got)), tc.w, got)
			}
			if tc.want != "" && got != tc.want {
				t.Errorf("padOrTruncate(%q, %d) = %q, want %q", tc.s, tc.w, got, tc.want)
			}
			// Truncation cases must contain "…"
			if tc.name == "truncate long" && !strings.Contains(got, "…") {
				t.Errorf("padOrTruncate(%q, %d) = %q: expected truncation ellipsis", tc.s, tc.w, got)
			}
		})
	}
}

func TestStatusGlyph(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status Status
		want   string
	}{
		{StatusOK, "✓"},
		{StatusFail, "✗"},
		{StatusRunning, "…"},
		{StatusPending, " "},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := statusGlyph(tc.status)
			if got != tc.want {
				t.Errorf("statusGlyph(%d) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestRenderLineWidth(t *testing.T) {
	t.Parallel()

	styles := NewStyles(DefaultTheme())
	node := CallNode{
		ID:     "n1",
		Tool:   "read",
		Arg:    "config.go",
		Status: StatusOK,
		Meta:   "142 lines",
	}

	for _, width := range []int{40, 60, 80} {
		width := width
		t.Run("width"+string(rune('0'+width/10))+string(rune('0'+width%10)), func(t *testing.T) {
			t.Parallel()
			result := RenderLine(node, styles, false, false, width)
			got := lipgloss.Width(result)
			if got != width {
				t.Errorf("RenderLine width=%d: lipgloss.Width=%d", width, got)
			}
		})
	}
}

func TestRenderLineColors(t *testing.T) {
	t.Parallel()

	styles := NewStyles(DefaultTheme())
	tools := []string{"read", "write", "edit", "bash", "grep", "task"}

	for _, tool := range tools {
		tool := tool
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			node := CallNode{
				ID:     "n1",
				Tool:   tool,
				Arg:    "some_file.go",
				Status: StatusOK,
				Meta:   "ok",
			}
			result := RenderLine(node, styles, false, false, 60)
			if !strings.Contains(result, "\x1b[") {
				t.Errorf("RenderLine(tool=%q): expected ANSI escape, got %q", tool, result)
			}
			if got := lipgloss.Width(result); got != 60 {
				t.Errorf("RenderLine(tool=%q) width=%d, want 60", tool, got)
			}
		})
	}
}

func TestRenderLineStatusVariants(t *testing.T) {
	t.Parallel()

	styles := NewStyles(DefaultTheme())

	cases := []struct {
		name     string
		status   Status
		tailLine string
		assertFn func(t *testing.T, result string)
	}{
		{
			name:   "ok contains checkmark",
			status: StatusOK,
			assertFn: func(t *testing.T, result string) {
				t.Helper()
				if !strings.Contains(result, "✓") {
					t.Errorf("StatusOK result should contain ✓, got %q", result)
				}
			},
		},
		{
			name:   "fail contains cross",
			status: StatusFail,
			assertFn: func(t *testing.T, result string) {
				t.Helper()
				if !strings.Contains(result, "✗") {
					t.Errorf("StatusFail result should contain ✗, got %q", result)
				}
			},
		},
		{
			name:     "running tailline",
			status:   StatusRunning,
			tailLine: "running command...",
			assertFn: func(t *testing.T, result string) {
				t.Helper()
				if !strings.Contains(result, "running") {
					t.Errorf("StatusRunning with TailLine: result should contain 'running', got %q", result)
				}
			},
		},
		{
			name:   "pending no checkmark or cross",
			status: StatusPending,
			assertFn: func(t *testing.T, result string) {
				t.Helper()
				if strings.Contains(result, "✓") {
					t.Errorf("StatusPending result should not contain ✓, got %q", result)
				}
				if strings.Contains(result, "✗") {
					t.Errorf("StatusPending result should not contain ✗, got %q", result)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node := CallNode{
				ID:       "n1",
				Tool:     "bash",
				Arg:      "ls -la",
				Status:   tc.status,
				TailLine: tc.tailLine,
				Meta:     "meta info",
			}
			result := RenderLine(node, styles, false, false, 60)
			tc.assertFn(t, result)
		})
	}
}

func TestRenderLineTruncation(t *testing.T) {
	t.Parallel()

	styles := NewStyles(DefaultTheme())
	node := CallNode{
		ID:     "n1",
		Tool:   "read",
		Arg:    strings.Repeat("x", 200),
		Status: StatusOK,
		Meta:   "42 lines",
	}

	result := RenderLine(node, styles, false, false, 40)
	if got := lipgloss.Width(result); got != 40 {
		t.Errorf("RenderLine width=40: lipgloss.Width=%d, want 40", got)
	}
	if !strings.Contains(result, "…") {
		t.Errorf("RenderLine with 200-char arg at width=40: expected truncation ellipsis, got %q", result)
	}
}

func TestRenderTreeNesting(t *testing.T) {
	t.Parallel()

	styles := NewStyles(DefaultTheme())
	parent := CallNode{
		ID:     "p",
		Tool:   "task",
		Arg:    "subagent",
		Status: StatusOK,
		Children: []*CallNode{
			{ID: "c1", Tool: "read", Arg: "a.go", Status: StatusOK},
			{ID: "c2", Tool: "write", Arg: "b.go", Status: StatusOK},
		},
	}

	t.Run("expanded", func(t *testing.T) {
		t.Parallel()
		expanded := map[string]bool{"p": true}
		out := RenderTree([]CallNode{parent}, styles, "", expanded, 60)

		for _, want := range []string{"subagent", "a.go", "b.go", "▾"} {
			if !strings.Contains(out, want) {
				t.Errorf("expanded tree: expected %q in output, got:\n%s", want, out)
			}
		}

		// Children lines should be indented (start with at least 2 spaces after newline).
		lines := strings.Split(out, "\n")
		if len(lines) < 3 {
			t.Fatalf("expanded tree: expected at least 3 lines, got %d:\n%s", len(lines), out)
		}
		// Parent is line 0, children are subsequent lines.
		parentLine := lines[0]
		if strings.HasPrefix(parentLine, "  ") {
			t.Errorf("parent line should not start with 2 spaces, got %q", parentLine)
		}
		for i, line := range lines[1:] {
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "  ") {
				t.Errorf("child line %d should start with at least 2 spaces, got %q", i+1, line)
			}
		}
	})

	t.Run("collapsed", func(t *testing.T) {
		t.Parallel()
		expanded := map[string]bool{}
		out := RenderTree([]CallNode{parent}, styles, "", expanded, 60)

		if !strings.Contains(out, "subagent") {
			t.Errorf("collapsed tree: expected 'subagent' in output, got:\n%s", out)
		}
		if !strings.Contains(out, "▸") {
			t.Errorf("collapsed tree: expected '▸' in output, got:\n%s", out)
		}
		if strings.Contains(out, "a.go") {
			t.Errorf("collapsed tree: 'a.go' should not be visible, got:\n%s", out)
		}
		if strings.Contains(out, "b.go") {
			t.Errorf("collapsed tree: 'b.go' should not be visible, got:\n%s", out)
		}
	})
}

func TestRenderTreeMaxDepth(t *testing.T) {
	t.Parallel()

	styles := NewStyles(DefaultTheme())

	// Build 4-level tree: root → c1 → c2 → c3.
	c3 := &CallNode{ID: "c3", Tool: "read", Arg: "deep.go", Status: StatusOK}
	c2 := &CallNode{ID: "c2", Tool: "read", Arg: "mid.go", Status: StatusOK, Children: []*CallNode{c3}}
	c1 := &CallNode{ID: "c1", Tool: "read", Arg: "child.go", Status: StatusOK, Children: []*CallNode{c2}}
	root := CallNode{ID: "root", Tool: "task", Arg: "root-agent", Status: StatusOK, Children: []*CallNode{c1}}

	expanded := map[string]bool{
		"root": true,
		"c1":   true,
		"c2":   true,
		"c3":   true,
	}

	out := RenderTree([]CallNode{root}, styles, "", expanded, 60)

	if strings.Contains(out, "deep.go") {
		t.Errorf("maxDepth=3: c3 (deep.go) should NOT be visible, got:\n%s", out)
	}
	if !strings.Contains(out, "mid.go") {
		t.Errorf("maxDepth=3: c2 (mid.go) SHOULD be visible, got:\n%s", out)
	}
}
