package webfetch

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const agentBrowserTimeout = 45 * time.Second

// AgentBrowserProvider fetches pages by shelling out to the agent-browser CLI.
// Handles Cloudflare, complex JS, interactive pages. Heaviest option.
type AgentBrowserProvider struct{}

func NewAgentBrowserProvider() *AgentBrowserProvider {
	// Only create if agent-browser is installed
	if _, err := exec.LookPath("agent-browser"); err != nil {
		return nil
	}
	return &AgentBrowserProvider{}
}

func (p *AgentBrowserProvider) Name() string { return "agent-browser" }

func (p *AgentBrowserProvider) Fetch(ctx context.Context, rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, agentBrowserTimeout)
	defer cancel()

	// Open the page
	openCmd := exec.CommandContext(ctx, "agent-browser", "open", rawURL)
	openOut, err := openCmd.CombinedOutput()
	if err != nil {
		return "", &HTTPError{
			URL:        rawURL,
			StatusCode: 0,
			Err:        fmt.Errorf("agent-browser open: %s: %w", strings.TrimSpace(string(openOut)), err),
		}
	}

	// Wait for network idle
	waitCmd := exec.CommandContext(ctx, "agent-browser", "wait", "load", "networkidle")
	_ = waitCmd.Run()

	// Extract page text
	textCmd := exec.CommandContext(ctx, "agent-browser", "get", "text", "body")
	textOut, err := textCmd.CombinedOutput()
	if err != nil {
		// Try to close before returning error
		closeCmd := exec.CommandContext(context.Background(), "agent-browser", "close")
		_ = closeCmd.Run()
		return "", &HTTPError{
			URL:        rawURL,
			StatusCode: 0,
			Err:        fmt.Errorf("agent-browser get text: %s: %w", strings.TrimSpace(string(textOut)), err),
		}
	}

	// Get title
	titleCmd := exec.CommandContext(ctx, "agent-browser", "get", "text", "title")
	titleOut, _ := titleCmd.CombinedOutput()
	title := strings.TrimSpace(string(titleOut))

	// Close the page
	closeCmd := exec.CommandContext(context.Background(), "agent-browser", "close")
	_ = closeCmd.Run()

	body := strings.TrimSpace(string(textOut))
	if len(body) == 0 {
		return "", &HTTPError{
			URL:        rawURL,
			StatusCode: 204,
			Err:        fmt.Errorf("agent-browser returned empty content"),
		}
	}

	content := buildFetchResult(title, rawURL, body)
	slog.Debug("agent-browser fetch completed", "url", rawURL, "bytes", len(content))
	return content, nil
}
