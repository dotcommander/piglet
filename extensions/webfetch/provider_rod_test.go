package webfetch_test

import (
	"testing"

	"github.com/go-rod/rod/lib/launcher"

	"github.com/dotcommander/piglet/extensions/webfetch"
	"github.com/stretchr/testify/assert"
)

func TestRodProvider_NoBrowser(t *testing.T) {
	t.Parallel()

	path, _ := launcher.LookPath()
	if path != "" {
		t.Skip("Chrome is available — this test is for systems without Chrome")
	}

	p := webfetch.NewRodProvider()
	assert.NotNil(t, p, "NewRodProvider always returns non-nil")

	// Fetch should fail gracefully when no browser is found
	_, err := p.Fetch(t.Context(), "http://example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Chrome")
}
