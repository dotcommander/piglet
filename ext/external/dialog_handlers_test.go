package external

import (
	"bufio"
	"encoding/json"
	"io"
	"testing"

	"github.com/dotcommander/piglet/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDialogHost creates a Host with h.stdin wired to a pipe so that
// h.respond(...) writes are captured by the returned scanner.
func newDialogHost(t *testing.T) (*Host, *bufio.Scanner) {
	t.Helper()
	h := newTestHost()
	pr, pw := io.Pipe()
	h.stdin = pw
	t.Cleanup(func() {
		pw.Close()
		pr.Close()
	})
	scanner := bufio.NewScanner(pr)
	return h, scanner
}

// readResponse reads one JSON-RPC response from the scanner.
// Must be called from a goroutine that is NOT the one that will fire the
// callback: the pipe write in h.respond blocks until this reader consumes it.
func readResponse(t *testing.T, scanner *bufio.Scanner) Message {
	t.Helper()
	require.True(t, scanner.Scan(), "expected a response line; got none (scanner: %v)", scanner.Err())
	var msg Message
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &msg))
	return msg
}

// asyncReadResponse fires f() in a goroutine and concurrently reads one
// response. The pipe write inside f blocks until the scanner consumes it, so
// both sides must run concurrently.
func asyncReadResponse(t *testing.T, scanner *bufio.Scanner, f func()) Message {
	t.Helper()
	ch := make(chan Message, 1)
	go func() { ch <- readResponse(t, scanner) }()
	f()
	return <-ch
}

// ---------------------------------------------------------------------------
// handleHostAskUser — happy path
// ---------------------------------------------------------------------------

func TestHandleHostAskUserHappy(t *testing.T) {
	t.Parallel()

	h, scanner := newDialogHost(t)
	app := ext.NewApp("/tmp")
	h.SetApp(app)

	id := 42
	msg := &Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  MethodHostAskUser,
		Params:  mustMarshalParams(HostAskUserParams{Prompt: "pick", Choices: []string{"yes", "no"}}),
	}
	h.handleHostAskUser(msg)

	// Action should be enqueued; no response yet.
	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act, ok := actions[0].(ext.ActionAskUser)
	require.True(t, ok)
	assert.Equal(t, "pick", act.Prompt)
	assert.Equal(t, []string{"yes", "no"}, act.Choices)

	// Simulate user selecting "yes". asyncReadResponse is required because
	// h.respond's pipe write blocks until the reader goroutine consumes the data.
	resp := asyncReadResponse(t, scanner, func() {
		act.OnResolve(ext.AskUserResult{Selected: "yes"})
	})
	require.Nil(t, resp.Error, "unexpected error: %v", resp.Error)

	var result HostAskUserResult
	require.NoError(t, json.Unmarshal(resp.Result, &result))
	assert.Equal(t, "yes", result.Selected)
	assert.False(t, result.Cancelled)
}

// ---------------------------------------------------------------------------
// handleHostAskUser — cancel path
// ---------------------------------------------------------------------------

func TestHandleHostAskUserCancel(t *testing.T) {
	t.Parallel()

	h, scanner := newDialogHost(t)
	app := ext.NewApp("/tmp")
	h.SetApp(app)

	id := 43
	msg := &Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  MethodHostAskUser,
		Params:  mustMarshalParams(HostAskUserParams{Prompt: "confirm", Choices: []string{"ok", "cancel"}}),
	}
	h.handleHostAskUser(msg)

	actions := app.PendingActions()
	require.Len(t, actions, 1)
	act := actions[0].(ext.ActionAskUser)

	// Simulate user pressing Esc. asyncReadResponse is required — see happy path.
	resp := asyncReadResponse(t, scanner, func() {
		act.OnResolve(ext.AskUserResult{Cancelled: true})
	})
	require.Nil(t, resp.Error)

	var result HostAskUserResult
	require.NoError(t, json.Unmarshal(resp.Result, &result))
	assert.Empty(t, result.Selected)
	assert.True(t, result.Cancelled)
}

// ---------------------------------------------------------------------------
// handleHostAskUser — empty choices rejected
// ---------------------------------------------------------------------------

func TestHandleHostAskUserEmptyChoices(t *testing.T) {
	t.Parallel()

	h, scanner := newDialogHost(t)
	app := ext.NewApp("/tmp")
	h.SetApp(app)

	id := 44
	msg := &Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  MethodHostAskUser,
		Params:  mustMarshalParams(HostAskUserParams{Prompt: "empty", Choices: []string{}}),
	}
	// asyncReadResponse is required: h.respondError's pipe write blocks until
	// the reader goroutine consumes the data.
	resp := asyncReadResponse(t, scanner, func() {
		h.handleHostAskUser(msg)
	})

	// No action enqueued — rejected before reaching app.
	assert.Empty(t, app.PendingActions())
	require.NotNil(t, resp.Error)
	assert.Equal(t, -32602, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "choices must not be empty")
}

// ---------------------------------------------------------------------------
// Wire type round-trips
// ---------------------------------------------------------------------------

func TestHostAskUserParamsRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostAskUserParams{Prompt: "choose", Choices: []string{"a", "b", "c"}}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostAskUserParams
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "choose", decoded.Prompt)
	assert.Equal(t, []string{"a", "b", "c"}, decoded.Choices)
}

func TestHostAskUserResultRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostAskUserResult{Selected: "a", Cancelled: false}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostAskUserResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "a", decoded.Selected)
	assert.False(t, decoded.Cancelled)

	cancel := HostAskUserResult{Selected: "", Cancelled: true}
	data, err = json.Marshal(cancel)
	require.NoError(t, err)
	var decodedCancel HostAskUserResult
	require.NoError(t, json.Unmarshal(data, &decodedCancel))
	assert.Empty(t, decodedCancel.Selected)
	assert.True(t, decodedCancel.Cancelled)
}
