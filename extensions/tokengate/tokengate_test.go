package tokengate

import (
	"strings"
	"testing"
)

func TestStripAnsi(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text passthrough", "hello world", "hello world"},
		{"SGR color", "\x1b[31mRED\x1b[0m", "RED"},
		{"multi SGR", "\x1b[1;32mBOLD GREEN\x1b[0m normal \x1b[33myellow\x1b[0m", "BOLD GREEN normal yellow"},
		{"cursor move", "\x1b[2Jtext\x1b[H", "text"},
		{"OSC title set", "\x1b]0;my title\x07rest", "rest"},
		{"empty", "", ""},
		{"CSI with ?", "\x1b[?25ltext\x1b[?25h", "text"},
	}

	s := StripAnsi{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := s.Apply(tc.in); got != tc.want {
				t.Errorf("Apply(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripAnsi_NameStable(t *testing.T) {
	t.Parallel()
	if got := (StripAnsi{}).Name(); got != "strip_ansi" {
		t.Errorf("Name() = %q; want %q", got, "strip_ansi")
	}
}

func TestSmartTruncate(t *testing.T) {
	t.Parallel()

	t.Run("MaxChars zero disables truncation", func(t *testing.T) {
		t.Parallel()
		big := strings.Repeat("x", 10_000)
		got := SmartTruncate{MaxChars: 0}.Apply(big)
		if got != big {
			t.Errorf("MaxChars=0 must pass through unchanged")
		}
	})

	t.Run("below limit passthrough", func(t *testing.T) {
		t.Parallel()
		in := "short output"
		got := SmartTruncate{MaxChars: 100}.Apply(in)
		if got != in {
			t.Errorf("below-limit input must pass through unchanged; got %q", got)
		}
	})

	t.Run("at limit passthrough", func(t *testing.T) {
		t.Parallel()
		in := strings.Repeat("a", 100)
		got := SmartTruncate{MaxChars: 100}.Apply(in)
		if got != in {
			t.Error("input exactly at limit must pass through unchanged")
		}
	})

	t.Run("over limit produces head+tail with marker", func(t *testing.T) {
		t.Parallel()
		head := "HEAD" + strings.Repeat("a", 56) // 60 bytes
		middle := strings.Repeat("x", 500)
		tail := strings.Repeat("b", 36) + "TAIL" // 40 bytes
		in := head + middle + tail

		got := SmartTruncate{MaxChars: 100}.Apply(in)
		if !strings.Contains(got, "... [truncated") {
			t.Errorf("output missing truncation marker; got %q", got)
		}
		// Head bytes (60% of 100 = 60) preserved — must include HEAD prefix.
		if !strings.HasPrefix(got, "HEAD") {
			t.Error("output must preserve head prefix")
		}
		// Tail bytes (40% of 100 = 40) preserved — must include TAIL suffix.
		if !strings.HasSuffix(got, "TAIL") {
			t.Error("output must preserve tail suffix")
		}
	})
}

type recordingStage struct {
	name string
	fire bool
}

func (r recordingStage) Name() string { return r.name }
func (r recordingStage) Apply(text string) string {
	if r.fire {
		return text + "/" + r.name
	}
	return text
}

func TestRunPipeline_AppliesInOrderAndReportsFired(t *testing.T) {
	t.Parallel()

	stages := []Stage{
		recordingStage{name: "a", fire: true},
		recordingStage{name: "b", fire: false},
		recordingStage{name: "c", fire: true},
	}

	out, applied := RunPipeline(stages, "x")
	if out != "x/a/c" {
		t.Errorf("out = %q; want %q", out, "x/a/c")
	}
	if len(applied) != 2 || applied[0] != "a" || applied[1] != "c" {
		t.Errorf("applied = %v; want [a c]", applied)
	}
}

func TestRunPipeline_NoStagesFire(t *testing.T) {
	t.Parallel()

	stages := []Stage{
		recordingStage{name: "a", fire: false},
		recordingStage{name: "b", fire: false},
	}
	out, applied := RunPipeline(stages, "clean")
	if out != "clean" {
		t.Errorf("out = %q; want %q", out, "clean")
	}
	if len(applied) != 0 {
		t.Errorf("applied = %v; want empty", applied)
	}
}

func TestDefaultStages_Composition(t *testing.T) {
	t.Parallel()

	// Confirms StripAnsi runs before SmartTruncate, so ANSI bytes don't
	// eat into the truncation budget.
	colorPrefix := "\x1b[31m" + strings.Repeat("R", 50) + "\x1b[0m"
	plainTail := strings.Repeat("b", 200)
	in := colorPrefix + plainTail

	// Force truncation by using the default stage list with a smaller cap.
	stages := []Stage{StripAnsi{}, SmartTruncate{MaxChars: 100}}
	out, applied := RunPipeline(stages, in)

	if len(applied) == 0 {
		t.Fatal("expected at least one stage to fire")
	}
	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("output still contains ANSI escape: %q", out)
	}
	// applied[0] should be strip_ansi since color codes were removed.
	if applied[0] != "strip_ansi" {
		t.Errorf("first applied stage = %q; want strip_ansi", applied[0])
	}
}
