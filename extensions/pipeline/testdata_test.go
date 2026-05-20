package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadDirTestdataPipelines exercises the committed sample fixtures in
// testdata/pipelines/ so the `pipeline list` example advertised in the docs
// has a verifiable, in-tree target. Add or remove fixtures by editing
// testdata/pipelines/ and updating the expected names below.
func TestLoadDirTestdataPipelines(t *testing.T) {
	t.Parallel()

	pipes, err := LoadDir("testdata/pipelines")
	require.NoError(t, err)
	require.NotEmpty(t, pipes, "expected sample pipelines under testdata/pipelines/")

	names := make(map[string]bool, len(pipes))
	for _, p := range pipes {
		names[p.Name] = true
		// Each fixture must validate cleanly so it can serve as a usage example.
		require.NoError(t, p.Validate(nil), "fixture %q failed validation", p.Name)
	}
	for _, want := range []string{"hello", "build-and-test", "each-loop"} {
		assert.True(t, names[want], "expected testdata fixture %q in LoadDir result", want)
	}
}
