package webfetch

import "testing"

func TestIsLLMRefusal(t *testing.T) {
	t.Parallel()

	refusals := []string{
		"I am unable to access URLs, so I cannot provide a summary.",
		"I cannot access the URL you provided.",
		"I'm unable to access external websites.",
		"I can't access URLs or browse the internet.",
		"I do not have the ability to access external URLs.",
		"I don't have access to external URLs.",
		"As an AI, I cannot browse the web.",
		"I cannot browse the internet to retrieve content.",
		"I'm not able to access websites directly.",
		"I cannot visit URLs or access web content.",
		"I cannot fetch external content.",
		"I cannot open URLs.",
		"I cannot retrieve content from the web.",
		"I'm not able to browse websites.",
		// Case variations
		"I AM UNABLE TO ACCESS URLs.",
		"  I am unable to access the given URL.  ",
	}

	for _, s := range refusals {
		if !isLLMRefusal(s) {
			t.Errorf("expected refusal for %q", s)
		}
	}

	legitimate := []string{
		"# Hello World\n\nThis is actual page content.",
		"Herman Melville - Moby-Dick",
		"The Go Programming Language",
		"200 OK — page loaded successfully",
		"", // empty is not a refusal (handled separately)
		"Unable to find the page (this is page content, not LLM text)",
	}

	for _, s := range legitimate {
		if isLLMRefusal(s) {
			t.Errorf("false positive refusal for %q", s)
		}
	}
}

func TestIsRecoverable_SoftFail(t *testing.T) {
	t.Parallel()

	// 204 (empty content) should be recoverable — triggers fallthrough to next provider
	err204 := &HTTPError{URL: "http://example.com", StatusCode: 204}
	if !isRecoverable(err204) {
		t.Error("HTTP 204 should be recoverable (soft fail)")
	}

	// 451 should be recoverable — often a proxy/reader issue
	err451 := &HTTPError{URL: "http://example.com", StatusCode: 451}
	if !isRecoverable(err451) {
		t.Error("HTTP 451 should be recoverable")
	}

	// 404 should NOT be recoverable
	err404 := &HTTPError{URL: "http://example.com", StatusCode: 404}
	if isRecoverable(err404) {
		t.Error("HTTP 404 should not be recoverable")
	}

	// 429 should be recoverable (rate limit)
	err429 := &HTTPError{URL: "http://example.com", StatusCode: 429}
	if !isRecoverable(err429) {
		t.Error("HTTP 429 should be recoverable")
	}

	// 500 should be recoverable
	err500 := &HTTPError{URL: "http://example.com", StatusCode: 500}
	if !isRecoverable(err500) {
		t.Error("HTTP 500 should be recoverable")
	}

	// nil error is not recoverable
	if isRecoverable(nil) {
		t.Error("nil error should not be recoverable")
	}
}
