package webfetch

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

const (
	collyMinContentLen = 200 // bytes — below this, page probably needs JS
)

// CollyProvider fetches static HTML pages locally using Colly.
// Fast and free — no API key, no external service. Falls through if
// the page content is too short (likely JS-rendered).
type CollyProvider struct{}

func NewCollyProvider() *CollyProvider {
	return &CollyProvider{}
}

func (p *CollyProvider) Name() string { return "colly" }

func (p *CollyProvider) Fetch(ctx context.Context, rawURL string) (string, error) {
	var (
		body       string
		title      string
		fetchErr   error
		statusCode int
	)

	c := colly.NewCollector(
		colly.AllowURLRevisit(),
	)
	c.SetRequestTimeout(fetchTimeout)

	// Mimic a real browser
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
	})

	c.OnResponse(func(r *colly.Response) {
		statusCode = r.StatusCode
	})

	c.OnHTML("title", func(e *colly.HTMLElement) {
		title = strings.TrimSpace(e.Text)
	})

	c.OnHTML("body", func(e *colly.HTMLElement) {
		body = extractText(e)
	})

	c.OnError(func(r *colly.Response, err error) {
		if r != nil {
			statusCode = r.StatusCode
		}
		fetchErr = err
	})

	// Respect context cancellation
	done := make(chan struct{})
	go func() {
		defer close(done)
		fetchErr = c.Visit(rawURL)
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-done:
	}

	if fetchErr != nil {
		return "", &HTTPError{URL: rawURL, StatusCode: statusCode, Err: fetchErr}
	}

	if statusCode >= 400 {
		return "", &HTTPError{URL: rawURL, StatusCode: statusCode}
	}

	// If body is too short, this page probably needs JS rendering
	if len(strings.TrimSpace(body)) < collyMinContentLen {
		return "", &HTTPError{
			URL:        rawURL,
			StatusCode: http.StatusNoContent,
			Err:        fmt.Errorf("page content too short (%d bytes), likely needs JS rendering", len(body)),
		}
	}

	content := buildFetchResult(title, rawURL, body)
	slog.Debug("colly fetch completed", "url", rawURL, "bytes", len(content))
	return content, nil
}

// extractText pulls visible text from the body element, skipping script/style/nav.
func extractText(e *colly.HTMLElement) string {
	// Remove script, style, nav, header, footer elements from the DOM copy
	dom := e.DOM
	dom.Find("script, style, noscript, nav, header, footer, iframe, svg").Remove()

	text := strings.TrimSpace(dom.Text())

	// Collapse excessive whitespace
	lines := strings.Split(text, "\n")
	var cleaned []string
	blankCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blankCount++
			if blankCount <= 2 {
				cleaned = append(cleaned, "")
			}
			continue
		}
		blankCount = 0
		cleaned = append(cleaned, trimmed)
	}

	return strings.Join(cleaned, "\n")
}

// collyRateLimiter returns a collector with rate limiting for batch use.
// Not used in the single-fetch provider path but exported for callers
// doing multi-page crawls.
func CollyCollectorWithLimits(domains string, delay, randomDelay time.Duration, parallelism int) *colly.Collector {
	c := colly.NewCollector(
		colly.Async(true),
	)
	c.Limit(&colly.LimitRule{
		DomainGlob:  domains,
		Delay:       delay,
		RandomDelay: randomDelay,
		Parallelism: parallelism,
	})
	return c
}
