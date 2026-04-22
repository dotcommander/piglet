package sdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// AskUser — selection round-trip
// ---------------------------------------------------------------------------

func TestAskUserSelection(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type callResult struct {
		selected  string
		cancelled bool
		err       error
	}
	done := make(chan callResult, 1)

	go func() {
		sel, cancelled, err := h.ext.AskUser(ctx, "pick one", []string{"alpha", "beta"})
		done <- callResult{sel, cancelled, err}
	}()

	// Read the outgoing request the SDK sent to the host.
	// AskUser is a host call — no prior registration traffic, read directly.
	req := h.readMessage(t)
	require.NotNil(t, req.ID)
	assert.Equal(t, "host/askUser", req.Method)

	var params struct {
		Prompt  string   `json:"prompt"`
		Choices []string `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(req.Params, &params))
	assert.Equal(t, "pick one", params.Prompt)
	assert.Equal(t, []string{"alpha", "beta"}, params.Choices)

	// Simulate host responding: user selected "alpha".
	result, _ := json.Marshal(map[string]any{"selected": "alpha", "cancelled": false})
	h.ext.handleMessage(&rpcMessage{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	})

	r := <-done
	require.NoError(t, r.err)
	assert.Equal(t, "alpha", r.selected)
	assert.False(t, r.cancelled)
}

// ---------------------------------------------------------------------------
// AskUser — cancellation round-trip
// ---------------------------------------------------------------------------

func TestAskUserCancellation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct {
		selected  string
		cancelled bool
		err       error
	}, 1)

	go func() {
		sel, cancelled, err := h.ext.AskUser(ctx, "confirm", []string{"ok", "cancel"})
		done <- struct {
			selected  string
			cancelled bool
			err       error
		}{sel, cancelled, err}
	}()

	req := h.readMessage(t)
	require.Equal(t, "host/askUser", req.Method)

	// Simulate host responding: user pressed Esc.
	result, _ := json.Marshal(map[string]any{"selected": "", "cancelled": true})
	h.ext.handleMessage(&rpcMessage{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	})

	r := <-done
	require.NoError(t, r.err)
	assert.Empty(t, r.selected)
	assert.True(t, r.cancelled)
}

// ---------------------------------------------------------------------------
// AskUser — empty choices rejected client-side
// ---------------------------------------------------------------------------

func TestAskUserEmptyChoicesRejected(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, _, err := h.ext.AskUser(context.Background(), "oops", []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "choices must not be empty")
}
