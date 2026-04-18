package lsp

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestJSONRPCErrorFormat verifies the Error() string format for jsonrpcError.
func TestJSONRPCErrorFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		code    int
		message string
		want    string
	}{
		{"method not found", -32601, "Method not found", "LSP error -32601: Method not found"},
		{"parse error", -32700, "Parse error", "LSP error -32700: Parse error"},
		{"zero code", 0, "ok", "LSP error 0: ok"},
		{"custom positive", 1001, "custom error", "LSP error 1001: custom error"},
		{"empty message", -32603, "", "LSP error -32603: "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &jsonrpcError{Code: tc.code, Message: tc.message}
			require.Equal(t, tc.want, e.Error())
		})
	}
}

// TestJSONRPCErrorImplementsError verifies that *jsonrpcError satisfies the
// error interface (compile-time assertion surfaced as a runtime check).
func TestJSONRPCErrorImplementsError(t *testing.T) {
	t.Parallel()

	var e error = &jsonrpcError{Code: -32600, Message: "Invalid Request"}
	require.NotEmpty(t, e.Error())
}

// TestErrServerDiedSentinel verifies that ErrServerDied is a distinct sentinel
// that can be detected with errors.Is.
func TestErrServerDiedSentinel(t *testing.T) {
	t.Parallel()

	// Direct match
	require.True(t, errors.Is(ErrServerDied, ErrServerDied))

	// Wrapped match
	wrapped := fmt.Errorf("read loop: %w", ErrServerDied)
	require.True(t, errors.Is(wrapped, ErrServerDied))

	// Unrelated error should not match
	other := errors.New("something else")
	require.False(t, errors.Is(other, ErrServerDied))
}
