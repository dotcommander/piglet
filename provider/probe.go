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

// ScanLocalServers probes well-known ports concurrently, returning the first
// responding server by preference order.
func ScanLocalServers() (ScanResult, error) {
	type hit struct {
		url    string
		models []ProbeResult
	}

	found := make([]hit, len(WellKnownPorts))
	ok := make([]bool, len(WellKnownPorts))
	var wg sync.WaitGroup

	// Probe well-known ports concurrently.
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
	State         string // "loaded", "not-loaded", or "" (unknown/not reported)
	ContextWindow int    // if discoverable, else 0
}

// bestResult returns the best chat-capable ProbeResult from a list.
// Prefers loaded models over unloaded, skips embedding-only models.
// Falls back to the first result if all models are embeddings.
// Returns a zero ProbeResult if results is empty.
func bestResult(results []ProbeResult) ProbeResult {
	// First pass: loaded + non-embedding (ideal).
	for _, r := range results {
		if r.State == "loaded" && !isEmbeddingModel(r.ModelID) {
			return r
		}
	}
	// Second pass: any non-embedding (state unknown or all unloaded).
	for _, r := range results {
		if !isEmbeddingModel(r.ModelID) {
			return r
		}
	}
	if len(results) > 0 {
		return results[0]
	}
	return ProbeResult{}
}

// BestModel returns the ID of the best chat-capable model from a list.
// Prefers loaded models over unloaded, skips embedding-only models.
// Returns "" if results is empty.
func BestModel(results []ProbeResult) string {
	return bestResult(results).ModelID
}

// ProbeServer contacts baseURL + "/v1/models" and returns the best
// available model, preferring loaded chat-capable models. For Ollama,
// it checks /api/ps for warm (in-memory) models first.
func ProbeServer(baseURL string) (ProbeResult, error) {
	results, err := ProbeModels(baseURL)
	if err != nil {
		return ProbeResult{}, err
	}

	// Ollama: check /api/ps for warm models before falling back to full list.
	if len(results) > 0 && results[0].ServerType == "ollama" {
		if warm := probeOllamaPS(baseURL); len(warm) > 0 {
			if r := bestResult(warm); r.ModelID != "" {
				return r, nil
			}
		}
	}

	return bestResult(results), nil
}

// probeOllamaPS calls Ollama's /api/ps to get currently loaded (warm) models.
func probeOllamaPS(baseURL string) []ProbeResult {
	client := &http.Client{Timeout: 3 * time.Second}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/ps"

	resp, err := client.Get(endpoint)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}

	results := make([]ProbeResult, len(body.Models))
	for i, m := range body.Models {
		results[i] = ProbeResult{
			ModelID:    m.Name,
			ServerType: "ollama",
			State:      "loaded",
		}
	}
	return results
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

	// Check owned_by field.
	switch strings.ToLower(ownedBy) {
	case "ollama", "ollama.org":
		return "ollama"
	case "llamacpp":
		return "llamacpp"
	case "vllm":
		return "vllm"
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
			ID      string                 `json:"id"`
			OwnedBy string                 `json:"owned_by"`
			State   string                 `json:"state"`  // LM Studio: "loaded" / "not-loaded"
			Status  struct{ Value string } `json:"status"` // llama.cpp router: "loaded" / "loading" / "unloaded"
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
		state := m.State
		if state == "" {
			state = m.Status.Value // llama.cpp router mode
		}
		results[i] = ProbeResult{
			ModelID:    m.ID,
			ServerType: serverType,
			State:      state,
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
	case "local", "lmstudio", "ollama", "llamacpp", "vllm", "mlx":
		return true
	}
	return false
}
