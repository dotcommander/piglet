package webfetch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	rodPageTimeout   = 30 * time.Second
	rodStableTimeout = 10 * time.Second
)

// RodProvider fetches JS-rendered pages using headless Chrome via CDP.
// Heavier than Colly but handles SPAs, dynamic content, and complex pages.
type RodProvider struct {
	mu      sync.Mutex
	browser *rod.Browser
}

func NewRodProvider() *RodProvider {
	return &RodProvider{}
}

func (p *RodProvider) Name() string { return "rod" }

// ensureBrowser lazily launches a shared browser instance.
func (p *RodProvider) ensureBrowser() (*rod.Browser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.browser != nil {
		return p.browser, nil
	}

	path, _ := launcher.LookPath()
	if path == "" {
		return nil, fmt.Errorf("no Chrome/Chromium found (set ROD_BROWSER or install Chrome)")
	}

	u := launcher.New().
		Bin(path).
		Headless(true).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage").
		MustLaunch()

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect to browser: %w", err)
	}

	p.browser = browser
	return browser, nil
}

// Close shuts down the browser if running.
func (p *RodProvider) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.browser != nil {
		_ = p.browser.Close()
		p.browser = nil
	}
}

func (p *RodProvider) Fetch(ctx context.Context, rawURL string) (string, error) {
	browser, err := p.ensureBrowser()
	if err != nil {
		return "", &HTTPError{URL: rawURL, StatusCode: 0, Err: err}
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return "", &HTTPError{URL: rawURL, StatusCode: 0, Err: fmt.Errorf("create page: %w", err)}
	}
	defer page.MustClose()

	// Set timeouts
	page = page.Timeout(rodPageTimeout)

	// Navigate
	if err := page.Navigate(rawURL); err != nil {
		return "", &HTTPError{URL: rawURL, StatusCode: 0, Err: fmt.Errorf("navigate: %w", err)}
	}

	// Wait for page to stabilize (DOM mutations stop)
	if err := page.WaitStable(rodStableTimeout); err != nil {
		slog.Debug("rod: page did not stabilize, proceeding anyway", "url", rawURL, "error", err)
	}

	// Extract title
	titleObj, err := page.Eval(`() => document.title`)
	title := ""
	if err == nil {
		title = titleObj.Value.Str()
	}

	// Remove noise elements before extracting text
	_, _ = page.Eval(`() => {
		const remove = ['script','style','noscript','nav','header','footer','iframe','svg'];
		remove.forEach(tag => {
			document.querySelectorAll(tag).forEach(el => el.remove());
		});
	}`)

	// Get text content
	textObj, err := page.Eval(`() => document.body ? document.body.innerText : ''`)
	if err != nil {
		return "", &HTTPError{URL: rawURL, StatusCode: 0, Err: fmt.Errorf("extract text: %w", err)}
	}

	body := strings.TrimSpace(textObj.Value.Str())

	if len(body) == 0 {
		return "", &HTTPError{
			URL:        rawURL,
			StatusCode: 204,
			Err:        fmt.Errorf("page rendered but no text content"),
		}
	}

	content := buildFetchResult(title, rawURL, body)
	slog.Debug("rod fetch completed", "url", rawURL, "bytes", len(content))
	return content, nil
}
