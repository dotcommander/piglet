package webfetch

import (
	"context"
	"io"
	"net/http"
)

// httpGet performs a GET request with body-size limiting and error handling.
func httpGet(ctx context.Context, client *http.Client, rawURL string, headers map[string]string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", &HTTPError{URL: rawURL, StatusCode: 0, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", &HTTPError{URL: rawURL, StatusCode: resp.StatusCode}
	}

	limited := io.LimitReader(resp.Body, maxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}

	content := string(data)
	if len(data) > maxBodyBytes {
		content = content[:maxBodyBytes] + truncationNote
	}

	return content, nil
}
