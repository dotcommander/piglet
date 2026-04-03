package webfetch_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/extensions/webfetch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultReaderBase = "https://r.jina.ai/"
	defaultSearchBase = "https://s.jina.ai/"
)

// makeSearchServer creates an httptest.Server that returns the given items as
// a Jina-formatted JSON response.
func makeSearchServer(t *testing.T, items []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Equal(t, "piglet/1.0", r.Header.Get("User-Agent"))
		payload := map[string]any{"data": items}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(payload))
	}))
}

// ---- Fetch ------------------------------------------------------------------

func TestFetch_ReaderMode(t *testing.T) {
	t.Parallel()

	const body = "# Hello\n\nClean markdown content."

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In reader mode the original URL is appended to the reader base.
		assert.Contains(t, r.RequestURI, "example.com")
		assert.Equal(t, "piglet/1.0", r.Header.Get("User-Agent"))
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	client := webfetch.NewForTest(srv.URL+"/", defaultSearchBase)
	got, err := client.Fetch(context.Background(), "http://example.com/page", false)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestFetch_RawMode(t *testing.T) {
	t.Parallel()

	const rawHTML = "<html><body>raw content</body></html>"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/page", r.URL.Path)
		fmt.Fprint(w, rawHTML)
	}))
	t.Cleanup(srv.Close)

	// In raw mode the target URL is fetched directly, so we use a dummy reader base.
	client := webfetch.NewForTest(defaultReaderBase, defaultSearchBase)
	got, err := client.Fetch(context.Background(), srv.URL+"/page", true)
	require.NoError(t, err)
	assert.Equal(t, rawHTML, got)
}

func TestFetch_Truncation(t *testing.T) {
	t.Parallel()

	bigBody := strings.Repeat("x", 100*1024+500)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, bigBody)
	}))
	t.Cleanup(srv.Close)

	client := webfetch.NewForTest(defaultReaderBase, defaultSearchBase)
	got, err := client.Fetch(context.Background(), srv.URL+"/", true)
	require.NoError(t, err)
	assert.True(t, len(got) <= 100*1024+len("\n\n[Content truncated at 100KB]"),
		"response must be capped at 100KB plus truncation note")
	assert.Contains(t, got, "[Content truncated at 100KB]")
}

func TestFetch_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client := webfetch.NewForTest(defaultReaderBase, defaultSearchBase)
	_, err := client.Fetch(context.Background(), srv.URL+"/missing", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetch_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := webfetch.NewForTest(defaultReaderBase, defaultSearchBase)
	_, err := client.Fetch(ctx, srv.URL+"/slow", true)
	require.Error(t, err)
}

// ---- Search -----------------------------------------------------------------

func TestSearch_Parsing(t *testing.T) {
	t.Parallel()

	items := []map[string]any{
		{"title": "Go Blog", "url": "https://go.dev/blog", "description": "Official Go blog"},
		{"title": "Pkg Docs", "url": "https://pkg.go.dev", "description": "Go package docs"},
	}
	srv := makeSearchServer(t, items)
	t.Cleanup(srv.Close)

	client := webfetch.NewForTest(defaultReaderBase, srv.URL+"/")
	results, err := client.Search(context.Background(), "golang", 5)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "Go Blog", results[0].Title)
	assert.Equal(t, "https://go.dev/blog", results[0].URL)
	assert.Equal(t, "Official Go blog", results[0].Description)
}

func TestSearch_Limit(t *testing.T) {
	t.Parallel()

	items := []map[string]any{
		{"title": "A", "url": "https://a.com", "description": "a"},
		{"title": "B", "url": "https://b.com", "description": "b"},
		{"title": "C", "url": "https://c.com", "description": "c"},
	}
	srv := makeSearchServer(t, items)
	t.Cleanup(srv.Close)

	client := webfetch.NewForTest(defaultReaderBase, srv.URL+"/")
	results, err := client.Search(context.Background(), "test", 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSearch_DefaultLimit(t *testing.T) {
	t.Parallel()

	// 10 items, default limit is 5.
	items := make([]map[string]any, 10)
	for i := range items {
		items[i] = map[string]any{
			"title":       fmt.Sprintf("Result %d", i),
			"url":         fmt.Sprintf("https://example.com/%d", i),
			"description": fmt.Sprintf("desc %d", i),
		}
	}
	srv := makeSearchServer(t, items)
	t.Cleanup(srv.Close)

	client := webfetch.NewForTest(defaultReaderBase, srv.URL+"/")
	results, err := client.Search(context.Background(), "test", 0) // 0 → default 5
	require.NoError(t, err)
	assert.Len(t, results, 5)
}

func TestSearch_FallbackToContent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		payload := map[string]any{
			"data": []map[string]any{{
				"title":       "No Desc",
				"url":         "https://example.com",
				"description": "",
				"content":     strings.Repeat("long content ", 50),
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(payload))
	}))
	t.Cleanup(srv.Close)

	client := webfetch.NewForTest(defaultReaderBase, srv.URL+"/")
	results, err := client.Search(context.Background(), "test", 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotEmpty(t, results[0].Description)
	// "…" is a multi-byte rune, so cap is 200 chars + 3 bytes = 203 bytes max.
	assert.True(t, len(results[0].Description) <= 203,
		"description snippet should be capped at ~200 chars, got %d", len(results[0].Description))
}

func TestSearch_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	// Both reader and search point to the error server so fetch fallback also fails.
	client := webfetch.NewForTest(srv.URL+"/", srv.URL+"/")
	_, err := client.Search(context.Background(), "search-http-error-test", 5)
	require.Error(t, err)
}

func TestSearch_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "not json at all")
	}))
	t.Cleanup(srv.Close)

	// Reader also points to the bad server so fetch fallback returns non-JSON too,
	// but that's valid text content — the fallback succeeds with raw text.
	// So this test now verifies graceful degradation: search fails, fetch fallback returns content.
	client := webfetch.NewForTest(srv.URL+"/", srv.URL+"/")
	results, err := client.Search(context.Background(), "search-invalid-json-test", 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Description, "not json at all")
}

// ---- FormatResults ----------------------------------------------------------

func TestFormatResults_Empty(t *testing.T) {
	t.Parallel()

	out := webfetch.FormatResults(nil)
	assert.Equal(t, "No results found.", out)
}

func TestFormatResults_Items(t *testing.T) {
	t.Parallel()

	results := []webfetch.SearchResult{
		{Title: "Go", URL: "https://go.dev", Description: "The Go language"},
	}
	out := webfetch.FormatResults(results)
	assert.Contains(t, out, "**Go**")
	assert.Contains(t, out, "https://go.dev")
	assert.Contains(t, out, "The Go language")
}
