package provider

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// WellKnownPort describes a default port for a local model server.
type WellKnownPort struct {
	Port int
	Name string
}

// WellKnownPorts lists default ports for common local model servers,
// ordered by preference (first match wins in ScanLocalServers).
var WellKnownPorts = []WellKnownPort{
	{1234, "LM Studio"},
	{11434, "Ollama"},
	{8080, "MLX/llama.cpp/LocalAI"},
	{8000, "vLLM"},
	{5000, "text-generation-webui"},
}

// ScanResult holds the outcome of ScanLocalServers.
type ScanResult struct {
	URL    string        // base URL of the responding server
	Models []ProbeResult // all models reported by that server
}

// ScanLocalServers probes well-known local model server ports concurrently
// and returns the first responding server (by preference order) together with
// its model list, avoiding a redundant probe call by the caller.
func ScanLocalServers() (ScanResult, error) {
	type hit struct {
		url    string
		models []ProbeResult
	}
	found := make([]hit, len(WellKnownPorts))
	ok := make([]bool, len(WellKnownPorts))
	var wg sync.WaitGroup
	for i, s := range WellKnownPorts {
		wg.Add(1)
		go func(idx, port int) {
			defer wg.Done()
			u := fmt.Sprintf("http://localhost:%d", port)
			if models, err := ProbeModels(u); err == nil {
				found[idx] = hit{url: u, models: models}
				ok[idx] = true
			}
		}(i, s.Port)
	}
	wg.Wait()

	for i, responding := range ok {
		if responding {
			return ScanResult{URL: found[i].url, Models: found[i].models}, nil
		}
	}

	parts := make([]string, len(WellKnownPorts))
	for i, s := range WellKnownPorts {
		parts[i] = fmt.Sprintf("%d (%s)", s.Port, s.Name)
	}
	return ScanResult{}, fmt.Errorf("no local model server found\nScanned: %s\nStart a server or use --port <n>", strings.Join(parts, ", "))
}

// ProbeResult holds the outcome of probing a local model server.
type ProbeResult struct {
	ModelID       string // discovered model ID (e.g. "qwen3.5-27b-mxfp8")
	ServerType    string // "lmstudio", "ollama", or "unknown"
	ContextWindow int    // if discoverable, else 0
}

// BestModel returns the ID of the best chat-capable model from a list,
// skipping embedding-only models. Returns "" if results is empty.
func BestModel(results []ProbeResult) string {
	for _, r := range results {
		if !isEmbeddingModel(r.ModelID) {
			return r.ModelID
		}
	}
	if len(results) > 0 {
		return results[0].ModelID
	}
	return ""
}

// ProbeServer contacts baseURL + "/v1/models" and returns the best
// available model, preferring chat-capable models over embedding models.
func ProbeServer(baseURL string) (ProbeResult, error) {
	results, err := ProbeModels(baseURL)
	if err != nil {
		return ProbeResult{}, err
	}
	for _, r := range results {
		if !isEmbeddingModel(r.ModelID) {
			return r, nil
		}
	}
	return results[0], nil
}

// isEmbeddingModel returns true if the model ID suggests an embedding-only model.
func isEmbeddingModel(id string) bool {
	lower := strings.ToLower(id)
	for _, p := range []string{
		"minilm", "e5-", "bge-", "gte-", "-embed",
		"embed-", "embedding", "instructor", "nomic-embed",
	} {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
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

// ProbeModels contacts baseURL + "/v1/models" and returns all available models.
func ProbeModels(baseURL string) ([]ProbeResult, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	baseURL = strings.TrimRight(baseURL, "/")
	endpoint := baseURL + "/v1/models"

	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("probe %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("probe %s: unexpected status %d", endpoint, resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("probe %s: decode response: %w", endpoint, err)
	}

	if len(body.Data) == 0 {
		return nil, fmt.Errorf("probe %s: no models in response", endpoint)
	}

	serverType := detectServerType(resp, body.Data[0].OwnedBy)

	results := make([]ProbeResult, len(body.Data))
	for i, m := range body.Data {
		results[i] = ProbeResult{
			ModelID:    m.ID,
			ServerType: serverType,
		}
	}
	return results, nil
}

// IsLoopbackURL reports whether rawURL points to a loopback or local address.
func IsLoopbackURL(rawURL string) bool {
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

// IsLocalProvider reports whether the provider name indicates a local server
// (auto-discovered or explicitly configured as local/lmstudio/ollama/vllm).
func IsLocalProvider(provider string) bool {
	switch strings.ToLower(provider) {
	case "local", "lmstudio", "ollama", "vllm", "mlx":
		return true
	}
	return false
}
