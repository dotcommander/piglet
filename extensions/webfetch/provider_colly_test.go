package webfetch_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/extensions/webfetch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollyProvider_StaticHTML(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Test Page</title></head>
<body>
<nav>Navigation links</nav>
<main>
<h1>Hello World</h1>
<p>This is a test page with enough content to pass the minimum threshold.
Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor
incididunt ut labore et dolore magna aliqua.</p>
</main>
<footer>Footer content</footer>
</body></html>`)
	}))
	t.Cleanup(srv.Close)

	p := webfetch.NewCollyProvider()
	content, err := p.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Contains(t, content, "Hello World")
	assert.Contains(t, content, "Lorem ipsum")
	assert.Contains(t, content, "Title: Test Page")
	// nav and footer should be stripped
	assert.NotContains(t, content, "Navigation links")
	assert.NotContains(t, content, "Footer content")
}

func TestCollyProvider_TooShort(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><p>Short</p></body></html>`)
	}))
	t.Cleanup(srv.Close)

	p := webfetch.NewCollyProvider()
	_, err := p.Fetch(context.Background(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestCollyProvider_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	p := webfetch.NewCollyProvider()
	_, err := p.Fetch(context.Background(), srv.URL)
	require.Error(t, err)
}

func TestCollyProvider_ScriptStripping(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Scripts Test</title></head>
<body>
<script>var x = "should not appear";</script>
<style>.hidden { display: none; }</style>
<div>
<p>This is the actual visible content that should be extracted from the page.
It needs to be long enough to pass the minimum content length threshold check.</p>
<p>Additional paragraph with more content to ensure we exceed the minimum bytes requirement.</p>
</div>
<noscript>Enable JavaScript</noscript>
</body></html>`)
	}))
	t.Cleanup(srv.Close)

	p := webfetch.NewCollyProvider()
	content, err := p.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Contains(t, content, "actual visible content")
	assert.NotContains(t, content, "should not appear")
	assert.NotContains(t, content, "display: none")
	assert.NotContains(t, content, "Enable JavaScript")
}

func TestCollyProvider_Truncation(t *testing.T) {
	t.Parallel()

	bigContent := strings.Repeat("x", 110*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head><title>Big</title></head><body><p>%s</p></body></html>`, bigContent)
	}))
	t.Cleanup(srv.Close)

	p := webfetch.NewCollyProvider()
	content, err := p.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Contains(t, content, "[Content truncated at 100KB]")
}

func TestCollyProvider_ContextCancel(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := webfetch.NewCollyProvider()
	_, err := p.Fetch(ctx, srv.URL)
	require.Error(t, err)
}
