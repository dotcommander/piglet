package provider

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProbeResult holds the outcome of probing a local model server.
type ProbeResult struct {
	ModelID       string // discovered model ID (e.g. "qwen3.5-27b-mxfp8")
	ServerType    string // "lmstudio", "ollama", or "unknown"
	ContextWindow int    // if discoverable, else 0
}

// ProbeServer contacts baseURL + "/v1/models" and returns the first
// available model along with the detected server type.
func ProbeServer(baseURL string) (ProbeResult, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	baseURL = strings.TrimRight(baseURL, "/")
	endpoint := baseURL + "/v1/models"

	resp, err := client.Get(endpoint)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("probe %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ProbeResult{}, fmt.Errorf("probe %s: unexpected status %d", endpoint, resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ProbeResult{}, fmt.Errorf("probe %s: decode response: %w", endpoint, err)
	}

	if len(body.Data) == 0 {
		return ProbeResult{}, fmt.Errorf("probe %s: no models in response", endpoint)
	}

	first := body.Data[0]
	result := ProbeResult{
		ModelID:       first.ID,
		ContextWindow: 0,
	}

	result.ServerType = detectServerType(resp, first.OwnedBy)

	return result, nil
}

// detectServerType inspects response headers and the owned_by field to
// identify the server software.
func detectServerType(resp *http.Response, ownedBy string) string {
	// Check response headers for LM Studio.
	for _, v := range resp.Header.Values("Server") {
		if strings.Contains(strings.ToLower(v), "lmstudio") {
			return "lmstudio"
		}
	}

	// Check owned_by field for Ollama.
	switch strings.ToLower(ownedBy) {
	case "ollama", "ollama.org":
		return "ollama"
	}

	return "unknown"
}

// isLoopbackURL reports whether rawURL points to a loopback or local address.
func isLoopbackURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := u.Hostname()
	if host == "" {
		return true
	}

	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Handle bare IPs that aren't the common loopback literals.
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return true
	}

	return false
}
