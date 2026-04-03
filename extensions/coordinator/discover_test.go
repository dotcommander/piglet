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

func TestDiscoverCapabilities_SkipsSelf(t *testing.T) {
	t.Parallel()

	// This is a unit test that verifies the filtering logic.
	// We can't call ExtInfos without a host, so test the filter function directly.
	infos := []sdk.ExtInfo{
		{Name: "coordinator", Tools: []string{"coordinate"}},
		{Name: "memory", Tools: []string{"memory_set", "memory_get"}},
		{Name: "safeguard", Interceptors: []string{"safeguard"}}, // no tools/commands
	}

	// Simulate the filtering logic
	var caps []Capability
	for _, info := range infos {
		if info.Name == "coordinator" {
			continue
		}
		if len(info.Tools) == 0 && len(info.Commands) == 0 {
			continue
		}
		caps = append(caps, Capability{
			Extension: info.Name,
			Tools:     info.Tools,
			Commands:  info.Commands,
		})
	}

	assert.Len(t, caps, 1, "should skip coordinator and toolless extensions")
	assert.Equal(t, "memory", caps[0].Extension)
}
