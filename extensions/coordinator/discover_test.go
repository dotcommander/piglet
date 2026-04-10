package coordinator

import (
	"testing"

	sdk "github.com/dotcommander/piglet/sdk"
	"github.com/stretchr/testify/assert"
)

func TestFormatCapabilities(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		result := FormatCapabilities(nil)
		assert.Contains(t, result, "No extension capabilities")
	})

	t.Run("with tools and commands", func(t *testing.T) {
		t.Parallel()
		caps := []Capability{
			{Extension: "memory", Tools: []string{"memory_set", "memory_get"}, Commands: []string{"memory"}},
			{Extension: "repomap", Tools: []string{"repo_map"}},
		}
		result := FormatCapabilities(caps)
		assert.Contains(t, result, "memory:")
		assert.Contains(t, result, "memory_set")
		assert.Contains(t, result, "repomap:")
		assert.Contains(t, result, "repo_map")
	})
}

func TestFilterCapabilities_SkipsSelf(t *testing.T) {
	t.Parallel()

	infos := []sdk.ExtInfo{
		{Name: extName, Tools: []string{"coordinate"}},
		{Name: "memory", Tools: []string{"memory_set", "memory_get"}},
		{Name: "safeguard", Interceptors: []string{"safeguard"}}, // no tools/commands
	}

	caps := filterCapabilities(infos)

	assert.Len(t, caps, 1, "should skip coordinator and toolless extensions")
	assert.Equal(t, "memory", caps[0].Extension)
}
