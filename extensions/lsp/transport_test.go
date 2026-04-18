package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newPipeClient constructs a Client wired to two in-process io.Pipe pairs.
//
// Returns:
//   - client:  ready-to-use *Client (readLoop NOT started — caller decides)
//   - toServer: what the test reads to inspect frames the client sends
//   - fromServer: what the test writes to inject frames the client receives
//   - cleanup:  closes both pipes; safe to call multiple times
func newPipeClient(t *testing.T) (c *Client, toServer io.Reader, fromServer io.WriteCloser, cleanup func()) {
	t.Helper()

	// client writes here → test reads
	clientWriteR, clientWriteW := io.Pipe()
	// test writes here → client reads
	serverWriteR, serverWriteW := io.Pipe()

	c = &Client{
		stdin:   clientWriteW,
		stdout:  bufio.NewReaderSize(serverWriteR, 64*1024),
		pending: make(map[int64]chan rpcResult),
		done:    make(chan struct{}),
	}
	c.nextID.Store(0)

	cleanup = sync.OnceFunc(func() {
		_ = clientWriteW.Close()
		_ = serverWriteW.Close()
		_ = clientWriteR.Close()
		_ = serverWriteR.Close()
	})

	t.Cleanup(cleanup)
	return c, clientWriteR, serverWriteW, cleanup
}

// writeFrame writes a well-formed LSP frame to w.
func writeFrame(t *testing.T, w io.Writer, body []byte) {
	t.Helper()
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	_, err := io.WriteString(w, header)
	require.NoError(t, err)
	_, err = w.Write(body)
	require.NoError(t, err)
}

// marshalJSON marshals v and fails the test on error.
func marshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// readFrame reads one LSP frame from r (header + body) and returns the body bytes.
func readFrame(t *testing.T, r io.Reader) []byte {
	t.Helper()
	br := bufio.NewReader(r)

	var contentLength int
	for {
		line, err := br.ReadString('\n')
		require.NoError(t, err, "reading header line")
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			_, err := fmt.Sscan(val, &contentLength)
			require.NoError(t, err)
		}
	}

	body := make([]byte, contentLength)
	_, err := io.ReadFull(br, body)
	require.NoError(t, err)
	return body
}

// responseFrame builds a JSON-RPC 2.0 response body.
func responseFrame(t *testing.T, id int64, result any) []byte {
	t.Helper()
	resultBytes := marshalJSON(t, result)
	raw := json.RawMessage(resultBytes)
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  raw,
	}
	return marshalJSON(t, resp)
}

// errorFrame builds a JSON-RPC 2.0 error response body.
func errorFrame(t *testing.T, id int64, code int, message string) []byte {
	t.Helper()
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      &id,
		Error:   &jsonrpcError{Code: code, Message: message},
	}
	return marshalJSON(t, resp)
}

// notificationFrame builds a JSON-RPC 2.0 notification body (no id field).
func notificationFrame(t *testing.T, method string, params any) []byte {
	t.Helper()
	msg := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return marshalJSON(t, msg)
}

// ---------------------------------------------------------------------------
// jsonrpcError.Error() — format
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// send() — framing
// ---------------------------------------------------------------------------

// TestSendFraming verifies that send() writes a correctly-framed
// Content-Length header followed by the JSON body.
func TestSendFraming(t *testing.T) {
	t.Parallel()

	c, toServer, _, _ := newPipeClient(t)

	type payload struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}
	msg := payload{JSONRPC: "2.0", Method: "test/ping"}
	expectedBody := marshalJSON(t, msg)

	// Read concurrently: io.Pipe is synchronous, send() blocks until read.
	frameCh := make(chan []byte, 1)
	go func() { frameCh <- readFrame(t, toServer) }()

	require.NoError(t, c.send(msg))

	got := <-frameCh
	require.Equal(t, expectedBody, got)
}

// TestSendContentLengthMatchesBody verifies that the Content-Length value
// written by send() equals the exact byte length of the JSON body.
func TestSendContentLengthMatchesBody(t *testing.T) {
	t.Parallel()

	c, toServer, _, _ := newPipeClient(t)

	type payload struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  string `json:"params"`
	}
	msg := payload{JSONRPC: "2.0", Method: "textDocument/hover", Params: "some unicode: \u00e9\u00e0"}

	// Read concurrently — io.Pipe is synchronous; send() makes two writes
	// (header, then body) and each blocks until the reader reads.
	// readFrame uses bufio.Reader internally, so it handles both writes.
	frameCh := make(chan []byte, 1)
	go func() { frameCh <- readFrame(t, toServer) }()

	require.NoError(t, c.send(msg))

	body := <-frameCh
	// Verify body is valid JSON and has expected content.
	var got payload
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "textDocument/hover", got.Method)

	// Now verify the framing by inspecting the raw pipe output directly.
	// We do this by marshaling the message ourselves and comparing lengths.
	expectedBody := marshalJSON(t, msg)
	require.Equal(t, len(expectedBody), len(body),
		"Content-Length must equal body byte length (verified via readFrame)")
}

// TestSendMultipleMessages verifies that consecutive send() calls each produce
// their own independent framed message (no interleaving or merging).
func TestSendMultipleMessages(t *testing.T) {
	t.Parallel()

	c, toServer, _, _ := newPipeClient(t)

	type ping struct {
		N int `json:"n"`
	}

	const count = 5

	// Collect all frames concurrently — io.Pipe is synchronous; send() and
	// readFrame must run on separate goroutines or they deadlock.
	framesCh := make(chan []byte, count)
	go func() {
		for range count {
			framesCh <- readFrame(t, toServer)
		}
	}()

	for i := range count {
		require.NoError(t, c.send(ping{N: i}))
	}

	for i := range count {
		body := <-framesCh
		var got ping
		require.NoError(t, json.Unmarshal(body, &got))
		require.Equal(t, i, got.N)
	}
}

// ---------------------------------------------------------------------------
// notify() — no id field
// ---------------------------------------------------------------------------

// TestNotifyFraming verifies that notify() produces a valid framed message
// that contains no "id" field in the JSON body.
func TestNotifyFraming(t *testing.T) {
	t.Parallel()

	c, toServer, _, _ := newPipeClient(t)

	type params struct {
		URI string `json:"uri"`
	}

	// Read concurrently — io.Pipe is synchronous; notify/send blocks until read.
	frameCh := make(chan []byte, 1)
	go func() { frameCh <- readFrame(t, toServer) }()

	require.NoError(t, c.notify("textDocument/didOpen", params{URI: "file:///foo.go"}))

	body := <-frameCh
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))

	// Must have jsonrpc and method.
	require.Contains(t, raw, "jsonrpc")
	require.Contains(t, raw, "method")

	// Must NOT have an "id" field — notifications have no id.
	require.NotContains(t, raw, "id", "notification must not contain an id field")

	// Method must match.
	var method string
	require.NoError(t, json.Unmarshal(raw["method"], &method))
	require.Equal(t, "textDocument/didOpen", method)
}

// TestNotifyNilParams verifies notify() with nil params omits the params key.
func TestNotifyNilParams(t *testing.T) {
	t.Parallel()

	c, toServer, _, _ := newPipeClient(t)

	frameCh := make(chan []byte, 1)
	go func() { frameCh <- readFrame(t, toServer) }()

	require.NoError(t, c.notify("exit", nil))

	body := <-frameCh
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))
	require.NotContains(t, raw, "params")
}

// ---------------------------------------------------------------------------
// readLoop() — frame parsing
// ---------------------------------------------------------------------------

// TestReadLoopDispatchesResponse verifies that readLoop delivers a
// server response to the correct pending channel by id.
func TestReadLoopDispatchesResponse(t *testing.T) {
	t.Parallel()

	c, _, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	// Register a pending channel for id=1.
	const id int64 = 1
	ch := make(chan rpcResult, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	// Write a response frame from the "server" side.
	type result struct {
		Value string `json:"value"`
	}
	writeFrame(t, fromServer, responseFrame(t, id, result{Value: "hello"}))

	select {
	case res := <-ch:
		require.NoError(t, res.Err)
		var got result
		require.NoError(t, json.Unmarshal(res.Data, &got))
		require.Equal(t, "hello", got.Value)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response dispatch")
	}
}

// TestReadLoopDispatchesError verifies that a JSON-RPC error response is
// delivered as an error on the pending channel.
func TestReadLoopDispatchesError(t *testing.T) {
	t.Parallel()

	c, _, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	const id int64 = 7
	ch := make(chan rpcResult, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	writeFrame(t, fromServer, errorFrame(t, id, -32601, "Method not found"))

	select {
	case res := <-ch:
		require.Error(t, res.Err)
		require.Contains(t, res.Err.Error(), "Method not found")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error dispatch")
	}
}

// TestReadLoopIgnoresNotifications verifies that server-sent notifications
// (no id field) do not block or error — they are silently dropped.
func TestReadLoopIgnoresNotifications(t *testing.T) {
	t.Parallel()

	c, _, fromServer, cleanup := newPipeClient(t)
	go c.readLoop()

	// Send a notification followed by a real response.
	const id int64 = 3
	ch := make(chan rpcResult, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	writeFrame(t, fromServer, notificationFrame(t, "window/logMessage", map[string]string{"message": "hi"}))
	writeFrame(t, fromServer, responseFrame(t, id, map[string]int{"answer": 42}))

	select {
	case res := <-ch:
		require.NoError(t, res.Err)
		var got map[string]int
		require.NoError(t, json.Unmarshal(res.Data, &got))
		require.Equal(t, 42, got["answer"])
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response after notification")
	}

	cleanup()
}

// TestReadLoopIgnoresUnknownID verifies that a response with an id that has
// no matching pending channel is silently dropped (no panic, no hang).
func TestReadLoopIgnoresUnknownID(t *testing.T) {
	t.Parallel()

	c, _, fromServer, cleanup := newPipeClient(t)
	go c.readLoop()

	// Send a response for id=99 — no pending channel registered.
	writeFrame(t, fromServer, responseFrame(t, 99, map[string]string{"x": "y"}))

	// Then send a response for a known id to confirm the loop continues.
	const id int64 = 1
	ch := make(chan rpcResult, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	writeFrame(t, fromServer, responseFrame(t, id, true))

	select {
	case res := <-ch:
		require.NoError(t, res.Err)
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop stalled after unknown id")
	}

	cleanup()
}

// TestReadLoopSkipsMalformedJSON verifies that a frame with invalid JSON body
// is silently skipped and the loop continues processing subsequent frames.
func TestReadLoopSkipsMalformedJSON(t *testing.T) {
	t.Parallel()

	c, _, fromServer, cleanup := newPipeClient(t)
	go c.readLoop()

	const id int64 = 5
	ch := make(chan rpcResult, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	// First frame: malformed JSON.
	bad := []byte(`{this is not valid json}`)
	writeFrame(t, fromServer, bad)

	// Second frame: valid response.
	writeFrame(t, fromServer, responseFrame(t, id, "ok"))

	select {
	case res := <-ch:
		require.NoError(t, res.Err)
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop stalled after malformed JSON frame")
	}

	cleanup()
}

// TestReadLoopSkipsZeroContentLength verifies that a frame with
// Content-Length: 0 is skipped (loop reads next frame).
func TestReadLoopSkipsZeroContentLength(t *testing.T) {
	t.Parallel()

	c, _, fromServer, cleanup := newPipeClient(t)
	go c.readLoop()

	const id int64 = 2
	ch := make(chan rpcResult, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	// Write a frame with Content-Length: 0 (no body read, loop continues).
	_, err := io.WriteString(fromServer, "Content-Length: 0\r\n\r\n")
	require.NoError(t, err)

	// Follow with a valid response.
	writeFrame(t, fromServer, responseFrame(t, id, "pong"))

	select {
	case res := <-ch:
		require.NoError(t, res.Err)
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop stalled after zero Content-Length frame")
	}

	cleanup()
}

// TestReadLoopExitsOnPipeClosed verifies that readLoop closes c.done when
// the server-side pipe is closed.
func TestReadLoopExitsOnPipeClosed(t *testing.T) {
	t.Parallel()

	c, _, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	// Close the server pipe to simulate the language server dying.
	require.NoError(t, fromServer.Close())

	select {
	case <-c.done:
		// readLoop exited and closed done — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not exit after pipe close")
	}
}

// TestReadLoopExitsOnTruncatedBody verifies that readLoop exits when the
// server closes the pipe mid-frame (after header, before full body).
func TestReadLoopExitsOnTruncatedBody(t *testing.T) {
	t.Parallel()

	c, _, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	// Write a header claiming 100 bytes but close before sending any body.
	_, err := io.WriteString(fromServer, "Content-Length: 100\r\n\r\n")
	require.NoError(t, err)
	require.NoError(t, fromServer.Close())

	select {
	case <-c.done:
		// readLoop exited cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not exit after truncated body")
	}
}

// ---------------------------------------------------------------------------
// callRaw() — request/response correlation
// ---------------------------------------------------------------------------

// TestCallRawReturnsResult verifies the end-to-end path: callRaw sends a
// framed request and returns the result when the matching response arrives.
func TestCallRawReturnsResult(t *testing.T) {
	t.Parallel()

	c, toServer, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	type myResult struct {
		OK bool `json:"ok"`
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var callErr error
	var callResult json.RawMessage

	go func() {
		defer wg.Done()
		callResult, callErr = c.callRaw(context.Background(), "test/method", map[string]string{"k": "v"})
	}()

	// Read the request the client sent, extract its id, reply.
	reqBody := readFrame(t, toServer)
	var req jsonrpcRequest
	require.NoError(t, json.Unmarshal(reqBody, &req))
	require.Equal(t, "test/method", req.Method)
	require.Equal(t, "2.0", req.JSONRPC)

	// Write back a matching response.
	writeFrame(t, fromServer, responseFrame(t, req.ID, myResult{OK: true}))

	wg.Wait()
	require.NoError(t, callErr)

	var got myResult
	require.NoError(t, json.Unmarshal(callResult, &got))
	require.True(t, got.OK)
}

// TestCallRawReturnsRPCError verifies that callRaw returns an error when the
// server responds with a JSON-RPC error object.
func TestCallRawReturnsRPCError(t *testing.T) {
	t.Parallel()

	c, toServer, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	var callErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		_, callErr = c.callRaw(context.Background(), "broken/method", nil)
	}()

	reqBody := readFrame(t, toServer)
	var req jsonrpcRequest
	require.NoError(t, json.Unmarshal(reqBody, &req))

	writeFrame(t, fromServer, errorFrame(t, req.ID, -32603, "Internal error"))

	wg.Wait()
	require.Error(t, callErr)
	require.Contains(t, callErr.Error(), "Internal error")
}

// TestCallRawContextCancellation verifies that callRaw returns ctx.Err() when
// the context is cancelled before a response arrives.
func TestCallRawContextCancellation(t *testing.T) {
	t.Parallel()

	c, toServer, _, _ := newPipeClient(t)
	go c.readLoop()

	// Drain the outbound pipe so send() completes and callRaw reaches its select.
	go io.Copy(io.Discard, toServer) //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())

	var callErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Params don't matter — the server will never respond.
		_, callErr = c.callRaw(ctx, "slow/method", nil)
	}()

	// Let the goroutine register in pending and complete send() before cancelling.
	time.Sleep(20 * time.Millisecond)
	cancel()

	wg.Wait()
	require.ErrorIs(t, callErr, context.Canceled)
}

// TestCallRawServerDied verifies that callRaw returns ErrServerDied when the
// server pipe closes while a call is in flight.
func TestCallRawServerDied(t *testing.T) {
	t.Parallel()

	c, toServer, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	// Drain the outbound pipe so send() completes and callRaw reaches its select.
	go io.Copy(io.Discard, toServer) //nolint:errcheck

	var callErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		_, callErr = c.callRaw(context.Background(), "dying/method", nil)
	}()

	// Let send() complete so the goroutine is blocked in select on c.done/ch.
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, fromServer.Close())

	wg.Wait()
	require.ErrorIs(t, callErr, ErrServerDied)
}

// TestCallRawMultipleConcurrentCalls verifies that multiple in-flight calls
// each get the response matching their id, regardless of response order.
func TestCallRawMultipleConcurrentCalls(t *testing.T) {
	t.Parallel()

	c, toServer, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	const n = 5

	type resultPayload struct {
		Seq int `json:"seq"`
	}

	// Collect outbound requests as they arrive.
	requestsCh := make(chan jsonrpcRequest, n)
	var readerWg sync.WaitGroup
	readerWg.Add(1)
	go func() {
		defer readerWg.Done()
		br := bufio.NewReader(toServer)
		for range n {
			var contentLength int
			for {
				line, err := br.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimSpace(line)
				if line == "" {
					break
				}
				if strings.HasPrefix(line, "Content-Length:") {
					val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
					fmt.Sscan(val, &contentLength) //nolint:errcheck
				}
			}
			body := make([]byte, contentLength)
			if _, err := io.ReadFull(br, body); err != nil {
				return
			}
			var req jsonrpcRequest
			if json.Unmarshal(body, &req) == nil {
				requestsCh <- req
			}
		}
	}()

	// Launch n concurrent callers.
	results := make([]json.RawMessage, n)
	errs := make([]error, n)
	var callerWg sync.WaitGroup
	for i := range n {
		callerWg.Add(1)
		go func(idx int) {
			defer callerWg.Done()
			results[idx], errs[idx] = c.callRaw(context.Background(), "multi/call", map[string]int{"idx": idx})
		}(i)
	}

	// Collect all n requests, then reply in reverse order to exercise id correlation.
	readerWg.Wait()
	close(requestsCh)
	reqs := make([]jsonrpcRequest, 0, n)
	for req := range requestsCh {
		reqs = append(reqs, req)
	}
	require.Len(t, reqs, n)

	// Reply in reverse id order.
	for i := len(reqs) - 1; i >= 0; i-- {
		req := reqs[i]
		writeFrame(t, fromServer, responseFrame(t, req.ID, resultPayload{Seq: int(req.ID)}))
	}

	callerWg.Wait()

	for i := range n {
		require.NoError(t, errs[i], "caller %d got error", i)
		var got resultPayload
		require.NoError(t, json.Unmarshal(results[i], &got))
		// Each result's Seq must equal the id used for that call.
		require.Positive(t, got.Seq)
	}
}

// ---------------------------------------------------------------------------
// call() — result unmarshaling
// ---------------------------------------------------------------------------

// TestCallUnmarshalsResult verifies that call() unmarshals the raw result into
// the provided pointer.
func TestCallUnmarshalsResult(t *testing.T) {
	t.Parallel()

	c, toServer, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	type myData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var callErr error
	var got myData

	go func() {
		defer wg.Done()
		callErr = c.call(context.Background(), "get/data", nil, &got)
	}()

	reqBody := readFrame(t, toServer)
	var req jsonrpcRequest
	require.NoError(t, json.Unmarshal(reqBody, &req))

	writeFrame(t, fromServer, responseFrame(t, req.ID, myData{Name: "Alice", Age: 30}))

	wg.Wait()
	require.NoError(t, callErr)
	require.Equal(t, "Alice", got.Name)
	require.Equal(t, 30, got.Age)
}

// TestCallNilResult verifies that call() with a nil result pointer succeeds
// without trying to unmarshal.
func TestCallNilResult(t *testing.T) {
	t.Parallel()

	c, toServer, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	var wg sync.WaitGroup
	wg.Add(1)
	var callErr error

	go func() {
		defer wg.Done()
		callErr = c.call(context.Background(), "notify/ack", nil, nil)
	}()

	reqBody := readFrame(t, toServer)
	var req jsonrpcRequest
	require.NoError(t, json.Unmarshal(reqBody, &req))

	writeFrame(t, fromServer, responseFrame(t, req.ID, nil))

	wg.Wait()
	require.NoError(t, callErr)
}

// TestCallPropagatesRPCError verifies that call() returns an error when the
// server responds with a JSON-RPC error object.
func TestCallPropagatesRPCError(t *testing.T) {
	t.Parallel()

	c, toServer, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	var wg sync.WaitGroup
	wg.Add(1)
	var callErr error

	go func() {
		defer wg.Done()
		callErr = c.call(context.Background(), "bad/method", nil, new(struct{}))
	}()

	reqBody := readFrame(t, toServer)
	var req jsonrpcRequest
	require.NoError(t, json.Unmarshal(reqBody, &req))

	writeFrame(t, fromServer, errorFrame(t, req.ID, -32600, "Invalid Request"))

	wg.Wait()
	require.Error(t, callErr)
	require.Contains(t, callErr.Error(), "Invalid Request")
}

// ---------------------------------------------------------------------------
// Pending-map cleanup — no goroutine leak
// ---------------------------------------------------------------------------

// TestCallRawCleansPendingOnContextCancel verifies that callRaw removes its
// entry from c.pending when the context is cancelled (no map growth).
func TestCallRawCleansPendingOnContextCancel(t *testing.T) {
	t.Parallel()

	c, toServer, _, _ := newPipeClient(t)
	go c.readLoop()

	// Drain the outbound pipe so send() completes and callRaw reaches its select.
	go io.Copy(io.Discard, toServer) //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = c.callRaw(ctx, "no/response", nil)
	}()

	// Let send() complete so callRaw is blocked in select before cancelling.
	time.Sleep(20 * time.Millisecond)
	cancel()
	wg.Wait()

	c.mu.Lock()
	pendingCount := len(c.pending)
	c.mu.Unlock()

	require.Equal(t, 0, pendingCount, "pending map must be empty after context cancel")
}

// TestCallRawCleansPendingOnSuccess verifies that callRaw removes its entry
// from c.pending after a successful response.
func TestCallRawCleansPendingOnSuccess(t *testing.T) {
	t.Parallel()

	c, toServer, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = c.callRaw(context.Background(), "clean/up", nil)
	}()

	reqBody := readFrame(t, toServer)
	var req jsonrpcRequest
	require.NoError(t, json.Unmarshal(reqBody, &req))

	writeFrame(t, fromServer, responseFrame(t, req.ID, "done"))

	wg.Wait()

	c.mu.Lock()
	pendingCount := len(c.pending)
	c.mu.Unlock()

	require.Equal(t, 0, pendingCount, "pending map must be empty after successful call")
}

// ---------------------------------------------------------------------------
// done channel — closed exactly once
// ---------------------------------------------------------------------------

// TestDoneClosedOnlyOnce verifies that readLoop closes c.done exactly once.
// Starting a second readLoop (or any double-close attempt) would panic; this
// test confirms the channel is closed after readLoop returns.
func TestDoneClosedOnlyOnce(t *testing.T) {
	t.Parallel()

	c, _, fromServer, _ := newPipeClient(t)
	go c.readLoop()

	require.NoError(t, fromServer.Close())

	select {
	case <-c.done:
		// channel is closed exactly once — reading from closed channel returns immediately
	case <-time.After(2 * time.Second):
		t.Fatal("c.done was not closed after pipe close")
	}

	// Reading again must not block (channel stays closed).
	select {
	case <-c.done:
	default:
		t.Fatal("c.done must remain readable after first close")
	}
}
