// Tests for cmd/sift. The CLI entry point is exercised via the
// canonical TestMain re-exec pattern: when the BE_SIFT_MAIN env var is
// set, the test binary acts as the sift binary itself (running main),
// otherwise it runs the normal test cases. This produces real
// statement coverage for main without requiring a separate build step.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envRunMain triggers main() execution inside the test binary.
const envRunMain = "BE_SIFT_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(envRunMain) == "1" {
		// Each re-exec is a fresh process; flag globals start clean.
		// Do not reset flag.CommandLine — doing so loses the package-level
		// flag.Usage indirection and replaces it with FlagSet.defaultUsage.
		main()
		return
	}
	os.Exit(m.Run())
}

// runSift re-execs the test binary in "act as sift" mode so main()
// runs under the test process and contributes to coverage.
func runSift(t *testing.T, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), envRunMain+"=1")
	cmd.Stdin = strings.NewReader(stdin)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("runSift: unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestSift_EmptyStdin_ExitsZeroNoOutput(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := runSift(t, "")
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestSift_SmallInput_PassesThrough(t *testing.T) {
	t.Parallel()
	// Input well below the default SizeThreshold (4096) should pass through.
	input := "hello world\nsecond line\n"
	stdout, _, code := runSift(t, input)
	require.Equal(t, 0, code)
	assert.Equal(t, input, stdout)
}

func TestSift_HelpFlag_PrintsUsage(t *testing.T) {
	t.Parallel()
	// `flag` uses ExitOnError + writes usage to stderr; with -h, exit is 0
	// in Go's stdlib (flag.ErrHelp short-circuits to 0).
	_, stderr, code := runSift(t, "", "-h")
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "Usage:")
	assert.Contains(t, stderr, "sift [flags]")
	assert.Contains(t, stderr, "Examples:")
}

func TestSift_UnknownFlag_ExitsNonZero(t *testing.T) {
	t.Parallel()
	_, stderr, code := runSift(t, "", "-no-such-flag")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "flag provided but not defined")
}

func TestSift_ThresholdOverride_CompressionTriggered(t *testing.T) {
	t.Parallel()
	// Force compression with a low threshold so blank-line collapsing fires.
	// Use a large blank-line run so the savings clearly exceed the ~45-byte
	// SIFT header that Compress prepends when it reduces output.
	var b strings.Builder
	b.WriteString("alpha\n")
	for i := 0; i < 200; i++ {
		b.WriteString("\n")
	}
	b.WriteString("omega\n")
	input := b.String()

	stdout, _, code := runSift(t, input, "-threshold", "1")
	require.Equal(t, 0, code)
	// With compression enabled, the output should be strictly shorter than
	// the input (blank-line collapse removes 199 of 200 blanks; the SIFT
	// header is shorter than what was removed).
	assert.Less(t, len(stdout), len(input))
	assert.Contains(t, stdout, "alpha")
	assert.Contains(t, stdout, "omega")
}

func TestSift_NoCollapseBlanks_DisablesBlankCollapse(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("alpha\n")
	for i := 0; i < 6; i++ {
		b.WriteString("\n")
	}
	b.WriteString("omega\n")
	input := b.String()

	// Force compression to engage via low threshold, but disable blank collapse.
	// With strip-trailing-whitespace still on (default), blanks remain as blanks.
	stdout, _, code := runSift(t, input, "-threshold", "1", "-no-collapse-blanks")
	require.Equal(t, 0, code)
	// The output should still contain alpha and omega.
	assert.Contains(t, stdout, "alpha")
	assert.Contains(t, stdout, "omega")
	// And the blank run should NOT have been collapsed to fewer lines than the
	// blank-collapse path would produce: count blank lines in output ≥ 6.
	blanks := 0
	for _, line := range strings.Split(stdout, "\n") {
		if line == "" {
			blanks++
		}
	}
	// Trailing newline contributes one empty split element; require at least 6 blanks.
	assert.GreaterOrEqual(t, blanks, 6)
}

func TestSift_NoCollapseRepeats_DisablesRepeatCollapse(t *testing.T) {
	t.Parallel()
	// 10 identical lines — default CollapseRepeatedLines=5 would collapse.
	var b strings.Builder
	for i := 0; i < 10; i++ {
		b.WriteString("repeat\n")
	}
	input := b.String()

	stdout, _, code := runSift(t, input, "-threshold", "1", "-no-collapse-repeats")
	require.Equal(t, 0, code)
	// With repeat-collapse disabled, all 10 lines should remain.
	assert.Equal(t, 10, strings.Count(stdout, "repeat"))
}

func TestSift_MaxSizeOverride_Truncates(t *testing.T) {
	t.Parallel()
	// Generate input clearly larger than the requested max-size.
	input := strings.Repeat("x", 8192)
	stdout, _, code := runSift(t, input, "-threshold", "1", "-max-size", "256")
	require.Equal(t, 0, code)
	// Output should be smaller than the original 8192-byte input.
	assert.Less(t, len(stdout), len(input))
}
