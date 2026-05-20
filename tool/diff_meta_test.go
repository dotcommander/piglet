package tool

import "testing"

func TestComputeDiffMeta(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		before  string
		after   string
		added   int
		removed int
		hunks   int
	}{
		{
			name:   "new file",
			before: "",
			after:  "line1\nline2\nline3",
			added:  3, removed: 0, hunks: 1,
		},
		{
			name:   "deleted file content",
			before: "line1\nline2",
			after:  "",
			added:  0, removed: 2, hunks: 1,
		},
		{
			name:   "no change",
			before: "same\ntext",
			after:  "same\ntext",
			added:  0, removed: 0, hunks: 0,
		},
		{
			name:   "single line replaced",
			before: "a\nb\nc",
			after:  "a\nB\nc",
			added:  1, removed: 1, hunks: 1,
		},
		{
			name:   "two separate hunks",
			before: "a\nb\nc\nd\ne",
			after:  "a\nX\nc\nd\nY",
			added:  2, removed: 2, hunks: 2,
		},
		{
			name:   "pure insertion mid-file",
			before: "a\nc",
			after:  "a\nb\nc",
			added:  1, removed: 0, hunks: 1,
		},
		{
			name:   "appended lines",
			before: "a\nb",
			after:  "a\nb\nc\nd",
			added:  2, removed: 0, hunks: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeDiffMeta(tc.before, tc.after)
			if got.Added != tc.added {
				t.Errorf("Added = %d, want %d", got.Added, tc.added)
			}
			if got.Removed != tc.removed {
				t.Errorf("Removed = %d, want %d", got.Removed, tc.removed)
			}
			if got.Files != 1 {
				t.Errorf("Files = %d, want 1", got.Files)
			}
			if got.Hunks != tc.hunks {
				t.Errorf("Hunks = %d, want %d", got.Hunks, tc.hunks)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	t.Parallel()

	if got := splitLines(""); got != nil {
		t.Errorf("splitLines(\"\") = %v, want nil", got)
	}
	if got := splitLines("one"); len(got) != 1 || got[0] != "one" {
		t.Errorf("splitLines(\"one\") = %v, want [one]", got)
	}
	if got := splitLines("a\nb"); len(got) != 2 {
		t.Errorf("splitLines(\"a\\nb\") len = %d, want 2", len(got))
	}
}
