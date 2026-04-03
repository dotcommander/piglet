package webfetch_test

import (
	"os/exec"
	"testing"

	"github.com/dotcommander/piglet/extensions/webfetch"
	"github.com/stretchr/testify/assert"
)

func TestAgentBrowserProvider_Creation(t *testing.T) {
	t.Parallel()

	p := webfetch.NewAgentBrowserProvider()

	_, err := exec.LookPath("agent-browser")
	if err != nil {
		assert.Nil(t, p, "should return nil when agent-browser is not installed")
	} else {
		assert.NotNil(t, p, "should return non-nil when agent-browser is installed")
		assert.Equal(t, "agent-browser", p.Name())
	}
}
