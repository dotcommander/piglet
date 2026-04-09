package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

// testHarness wires SDK output to a pipe for inspection.
// Tests using this harness can safely run in parallel — no global mutation.
type testHarness struct {
	ext    *Extension
	reader *bufio.Scanner
	pr     *os.File
	pw     *os.File
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	ext := New("test-ext", "1.0.0")
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	ext.rpcOut = pw // Direct SDK output to our pipe — no os.Stdout mutation
	t.Cleanup(func() {
		pw.Close()
		pr.Close()
	})
	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	return &testHarness{
		ext:    ext,
		reader: scanner,
		pr:     pr,
		pw:     pw,
	}
}

// readMessage reads one newline-delimited JSON message from captured stdout.
// It times out after 2 seconds to prevent hanging tests.
func (h *testHarness) readMessage(t *testing.T) rpcMessage {
	t.Helper()
	type result struct {
		msg rpcMessage
		err error
	}
	ch := make(chan result, 1)
	go func() {
		if !h.reader.Scan() {
			ch <- result{err: fmt.Errorf("scanner closed: %v", h.reader.Err())}
			return
		}
		var msg rpcMessage
		if err := json.Unmarshal(h.reader.Bytes(), &msg); err != nil {
			ch <- result{err: fmt.Errorf("unmarshal: %w", err)}
			return
		}
		ch <- result{msg: msg}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("readMessage: %v", r.err)
		}
		return r.msg
	case <-time.After(2 * time.Second):
		t.Fatal("readMessage: timed out waiting for SDK output")
		return rpcMessage{}
	}
}

// sendRequest builds a JSON-RPC request message with the given id, method, and params.
func sendRequest(id int, method string, params any) *rpcMessage {
	data, _ := json.Marshal(params)
	return &rpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  data,
	}
}

// sendNotif builds a JSON-RPC notification (no ID).
func sendNotif(method string, params any) *rpcMessage {
	data, _ := json.Marshal(params)
	return &rpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  data,
	}
}

// unmarshalResult unmarshals msg.Result into the provided value.
func unmarshalResult(t *testing.T, msg rpcMessage, v any) {
	t.Helper()
	if msg.Error != nil {
		t.Fatalf("unexpected error in response: code=%d msg=%s", msg.Error.Code, msg.Error.Message)
	}
	if err := json.Unmarshal(msg.Result, v); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
}

// skipNotifications reads and discards messages until the next response (has ID, no method).
func (h *testHarness) skipNotifications(t *testing.T) rpcMessage {
	t.Helper()
	for {
		msg := h.readMessage(t)
		if msg.ID != nil && msg.Method == "" {
			return msg
		}
		// It's a notification or registration — skip it.
	}
}

// ---------------------------------------------------------------------------
// 1. Initialize handshake
// ---------------------------------------------------------------------------

func TestInitialize(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ext.RegisterTool(ToolDef{
		Name:        "mytool",
		Description: "A test tool",
		Execute: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
			return TextResult("ok"), nil
		},
	})
	h.ext.RegisterCommand(CommandDef{
		Name:    "mycommand",
		Handler: func(_ context.Context, _ string) error { return nil },
	})
	h.ext.RegisterPromptSection(PromptSectionDef{Title: "Test", Content: "hello", Order: 10})

	req := sendRequest(1, "initialize", map[string]string{
		"protocolVersion": "3",
		"cwd":             "/tmp/test",
	})
	h.ext.handleMessage(req)

	// Collect registrations + the final initialize response.
	// We expect: register/tool, register/command, register/promptSection, then the response.
	seen := map[string]bool{}
	var initResp rpcMessage
	for range 4 {
		msg := h.readMessage(t)
		if msg.Method != "" {
			seen[msg.Method] = true
		} else if msg.ID != nil {
			initResp = msg
		}
	}

	if !seen["register/tool"] {
		t.Error("expected register/tool notification")
	}
	if !seen["register/command"] {
		t.Error("expected register/command notification")
	}
	if !seen["register/promptSection"] {
		t.Error("expected register/promptSection notification")
	}
	if initResp.ID == nil {
		t.Fatal("expected initialize response, got none")
	}
	var result struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	unmarshalResult(t, initResp, &result)
	if result.Name != "test-ext" {
		t.Errorf("name: got %q, want %q", result.Name, "test-ext")
	}
	if result.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", result.Version, "1.0.0")
	}
	if h.ext.CWD() != "/tmp/test" {
		t.Errorf("CWD: got %q, want %q", h.ext.CWD(), "/tmp/test")
	}
}

// ---------------------------------------------------------------------------
// 2. OnInit callback
// ---------------------------------------------------------------------------

func TestOnInit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	called := false
	h.ext.OnInit(func(e *Extension) {
		called = true
		e.RegisterTool(ToolDef{
			Name:        "lazy-tool",
			Description: "Registered in OnInit",
			Execute: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
				return TextResult("lazy"), nil
			},
		})
	})

	h.ext.handleMessage(sendRequest(1, "initialize", map[string]string{
		"protocolVersion": "3",
		"cwd":             "/tmp",
	}))

	// Drain: expect register/tool + response (2 messages).
	seen := map[string]bool{}
	for range 2 {
		msg := h.readMessage(t)
		if msg.Method != "" {
			seen[msg.Method] = true
		}
	}

	if !called {
		t.Error("OnInit callback was not called")
	}
	if !seen["register/tool"] {
		t.Error("expected register/tool after OnInit registered a tool")
	}
	if _, ok := h.ext.tools["lazy-tool"]; !ok {
		t.Error("lazy-tool not found in ext.tools after OnInit")
	}
}

// ---------------------------------------------------------------------------
// 3. Tool execution — success
// ---------------------------------------------------------------------------

func TestToolExecute(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ext.RegisterTool(ToolDef{
		Name:        "greet",
		Description: "Says hello",
		Execute: func(_ context.Context, _ map[string]any) (*ToolResult, error) {
			return TextResult("hello"), nil
		},
	})

	h.ext.handleMessage(sendRequest(10, "tool/execute", map[string]any{
		"callId": "c1",
		"name":   "greet",
		"args":   map[string]any{},
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	var result struct {
		Content []ContentBlock `json:"content"`
		IsError bool           `json:"isError"`
	}
	unmarshalResult(t, msg, &result)
	if result.IsError {
		t.Error("isError should be false")
	}
	if len(result.Content) == 0 {
		t.Fatal("content is empty")
	}
	if result.Content[0].Type != "text" {
		t.Errorf("content[0].type: got %q, want %q", result.Content[0].Type, "text")
	}
	if result.Content[0].Text != "hello" {
		t.Errorf("content[0].text: got %q, want %q", result.Content[0].Text, "hello")
	}
}

// ---------------------------------------------------------------------------
// 4. Tool execution — unknown tool
// ---------------------------------------------------------------------------

func TestToolExecuteUnknown(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.ext.handleMessage(sendRequest(11, "tool/execute", map[string]any{
		"callId": "c2",
		"name":   "no-such-tool",
		"args":   map[string]any{},
	}))

	msg := h.readMessage(t)
	if msg.Error == nil {
		t.Fatal("expected error response, got none")
	}
	if msg.Error.Code != -32602 {
		t.Errorf("error code: got %d, want -32602", msg.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// 5. Command execution
// ---------------------------------------------------------------------------

func TestCommandExecute(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ran := false
	h.ext.RegisterCommand(CommandDef{
		Name: "do-thing",
		Handler: func(_ context.Context, args string) error {
			ran = true
			return nil
		},
	})

	h.ext.handleMessage(sendRequest(20, "command/execute", map[string]string{
		"name": "do-thing",
		"args": "extra",
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	if !ran {
		t.Error("command handler was not called")
	}
}

// ---------------------------------------------------------------------------
// 6. Interceptor before — allow with modified args
// ---------------------------------------------------------------------------

func TestInterceptorBeforeAllow(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "modifier",
		Priority: 100,
		Before: func(_ context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			modified := map[string]any{"x": "modified"}
			return true, modified, nil
		},
	})

	h.ext.handleMessage(sendRequest(30, "interceptor/before", map[string]any{
		"toolName": "bash",
		"args":     map[string]any{"x": "original"},
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	var result struct {
		Allow bool           `json:"allow"`
		Args  map[string]any `json:"args"`
	}
	unmarshalResult(t, msg, &result)
	if !result.Allow {
		t.Error("allow should be true")
	}
	if result.Args["x"] != "modified" {
		t.Errorf("args[x]: got %v, want %q", result.Args["x"], "modified")
	}
}

// ---------------------------------------------------------------------------
// 7. Interceptor before — block
// ---------------------------------------------------------------------------

func TestInterceptorBeforeBlock(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "blocker",
		Priority: 100,
		Before: func(_ context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
			return false, nil, nil
		},
	})

	h.ext.handleMessage(sendRequest(31, "interceptor/before", map[string]any{
		"toolName": "bash",
		"args":     map[string]any{},
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	var result struct {
		Allow bool `json:"allow"`
	}
	unmarshalResult(t, msg, &result)
	if result.Allow {
		t.Error("allow should be false")
	}
}

// ---------------------------------------------------------------------------
// 8. Event dispatch — matched handler
// ---------------------------------------------------------------------------

func TestEventDispatch(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ext.RegisterEventHandler(EventHandlerDef{
		Name:   "turn-end-handler",
		Events: []string{"EventTurnEnd"},
		Handle: func(_ context.Context, eventType string, data json.RawMessage) *Action {
			return ActionNotify("turn ended")
		},
	})

	h.ext.handleMessage(sendRequest(40, "event/dispatch", map[string]any{
		"type": "EventTurnEnd",
		"data": json.RawMessage(`{}`),
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	var result struct {
		Action *wireActionResult `json:"action"`
	}
	unmarshalResult(t, msg, &result)
	if result.Action == nil {
		t.Fatal("expected action, got nil")
	}
	if result.Action.Type != "notify" {
		t.Errorf("action.type: got %q, want %q", result.Action.Type, "notify")
	}
}

// ---------------------------------------------------------------------------
// 9. Event dispatch — unmatched event type
// ---------------------------------------------------------------------------

func TestEventDispatchUnmatched(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ext.RegisterEventHandler(EventHandlerDef{
		Name:   "turn-end-handler",
		Events: []string{"EventTurnEnd"},
		Handle: func(_ context.Context, eventType string, data json.RawMessage) *Action {
			return ActionNotify("turn ended")
		},
	})

	h.ext.handleMessage(sendRequest(41, "event/dispatch", map[string]any{
		"type": "EventAgentStart",
		"data": json.RawMessage(`{}`),
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	var result struct {
		Action *wireActionResult `json:"action"`
	}
	unmarshalResult(t, msg, &result)
	if result.Action != nil {
		t.Errorf("expected nil action for unmatched event, got %+v", result.Action)
	}
}

// ---------------------------------------------------------------------------
// 10. Message hook
// ---------------------------------------------------------------------------

func TestMessageHook(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ext.RegisterMessageHook(MessageHookDef{
		Name:     "injector",
		Priority: 500,
		OnMessage: func(_ context.Context, msg string) (string, error) {
			return "injected context", nil
		},
	})

	h.ext.handleMessage(sendRequest(50, "messageHook/onMessage", map[string]string{
		"message": "hello world",
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	var result struct {
		Injection string `json:"injection"`
	}
	unmarshalResult(t, msg, &result)
	if result.Injection != "injected context" {
		t.Errorf("injection: got %q, want %q", result.Injection, "injected context")
	}
}

// ---------------------------------------------------------------------------
// 11. Unknown method
// ---------------------------------------------------------------------------

func TestUnknownMethod(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.ext.handleMessage(sendRequest(60, "no/such/method", nil))

	msg := h.readMessage(t)
	if msg.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if msg.Error.Code != -32601 {
		t.Errorf("error code: got %d, want -32601", msg.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// 12. Cancel request
// ---------------------------------------------------------------------------

func TestCancelRequest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctxCancelled := make(chan struct{})

	h.ext.RegisterTool(ToolDef{
		Name:        "slow-tool",
		Description: "Blocks until context cancelled",
		Execute: func(ctx context.Context, _ map[string]any) (*ToolResult, error) {
			<-ctx.Done()
			close(ctxCancelled)
			return nil, ctx.Err()
		},
	})

	// Launch the tool — runs in goroutine inside handleToolExecute.
	reqID := 70
	h.ext.handleMessage(sendRequest(reqID, "tool/execute", map[string]any{
		"callId": "c70",
		"name":   "slow-tool",
		"args":   map[string]any{},
	}))

	// Give the goroutine time to start and register its cancel func.
	time.Sleep(10 * time.Millisecond)

	// Send cancel notification.
	h.ext.handleMessage(sendNotif("$/cancelRequest", map[string]int{"id": reqID}))

	// Wait for context to be cancelled.
	select {
	case <-ctxCancelled:
		// success — context was cancelled
	case <-time.After(2 * time.Second):
		t.Fatal("context was not cancelled after $/cancelRequest")
	}

	// Drain the error response the tool sends after ctx.Err().
	msg := h.readMessage(t)
	if msg.ID == nil || *msg.ID != reqID {
		t.Errorf("response ID: got %v, want %d", msg.ID, reqID)
	}
}

// ---------------------------------------------------------------------------
// 13. Response routing — pending outgoing request
// ---------------------------------------------------------------------------

func TestResponseRouting(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Register a pending channel manually to simulate an outgoing request.
	respID := 99
	ch := make(chan *rpcMessage, 1)
	h.ext.pendingMu.Lock()
	h.ext.pending[respID] = ch
	h.ext.pendingMu.Unlock()

	// Deliver a response (has ID, no method).
	result, _ := json.Marshal(map[string]string{"text": "host reply"})
	resp := &rpcMessage{
		JSONRPC: "2.0",
		ID:      &respID,
		Result:  result,
	}
	h.ext.handleMessage(resp)

	// The channel should receive the message.
	select {
	case delivered := <-ch:
		if delivered.ID == nil || *delivered.ID != respID {
			t.Errorf("delivered ID: got %v, want %d", delivered.ID, respID)
		}
		var r struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(delivered.Result, &r); err != nil {
			t.Fatalf("unmarshal delivered result: %v", err)
		}
		if r.Text != "host reply" {
			t.Errorf("text: got %q, want %q", r.Text, "host reply")
		}
	case <-time.After(time.Second):
		t.Fatal("response not delivered to pending channel")
	}

	// Verify it was removed from the pending map.
	h.ext.pendingMu.Lock()
	_, still := h.ext.pending[respID]
	h.ext.pendingMu.Unlock()
	if still {
		t.Error("response should have been removed from pending map")
	}
}

// ---------------------------------------------------------------------------
// 14. Interceptor before — name discriminator targets only the named interceptor
// ---------------------------------------------------------------------------

func TestInterceptorBeforeNameFilter(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// "blocker" always blocks — if it runs, allow will be false.
	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "blocker",
		Priority: 2000,
		Before: func(_ context.Context, _ string, _ map[string]any) (bool, map[string]any, error) {
			return false, nil, nil
		},
	})
	// "passer" always allows.
	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "passer",
		Priority: 100,
		Before: func(_ context.Context, _ string, args map[string]any) (bool, map[string]any, error) {
			return true, args, nil
		},
	})

	// Target "passer" by name — "blocker" must NOT run.
	h.ext.handleMessage(sendRequest(100, "interceptor/before", map[string]any{
		"name":     "passer",
		"toolName": "bash",
		"args":     map[string]any{"command": "rm -rf /"},
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	var result struct {
		Allow bool `json:"allow"`
	}
	unmarshalResult(t, msg, &result)
	if !result.Allow {
		t.Error("allow should be true — blocker should not have run when name targets passer")
	}
}

// ---------------------------------------------------------------------------
// 15. Interceptor before — name discriminator blocks only the named interceptor
// ---------------------------------------------------------------------------

func TestInterceptorBeforeNameFilterBlock(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "blocker",
		Priority: 2000,
		Before: func(_ context.Context, _ string, _ map[string]any) (bool, map[string]any, error) {
			return false, nil, nil
		},
		Preview: func(_ context.Context, _ string, _ map[string]any) string {
			return "blocked by blocker"
		},
	})
	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "passer",
		Priority: 100,
		Before: func(_ context.Context, _ string, args map[string]any) (bool, map[string]any, error) {
			return true, args, nil
		},
	})

	// Target "blocker" by name — should block with preview.
	h.ext.handleMessage(sendRequest(101, "interceptor/before", map[string]any{
		"name":     "blocker",
		"toolName": "bash",
		"args":     map[string]any{},
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", msg.Error)
	}
	var result struct {
		Allow   bool   `json:"allow"`
		Preview string `json:"preview"`
	}
	unmarshalResult(t, msg, &result)
	if result.Allow {
		t.Error("allow should be false when blocker is targeted")
	}
	if result.Preview != "blocked by blocker" {
		t.Errorf("preview: got %q, want %q", result.Preview, "blocked by blocker")
	}
}

// ---------------------------------------------------------------------------
// 16. Interceptor before — block returns structured response, not JSON-RPC error
// ---------------------------------------------------------------------------

func TestInterceptorBeforeBlockIsNotRPCError(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Interceptor blocks without returning an error — the response must be
	// a structured {allow: false}, NOT a JSON-RPC error.
	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "clean-blocker",
		Priority: 100,
		Before: func(_ context.Context, _ string, _ map[string]any) (bool, map[string]any, error) {
			return false, nil, nil // allow=false, NO error
		},
		Preview: func(_ context.Context, _ string, _ map[string]any) string {
			return "reason for blocking"
		},
	})

	h.ext.handleMessage(sendRequest(102, "interceptor/before", map[string]any{
		"name":     "clean-blocker",
		"toolName": "bash",
		"args":     map[string]any{"command": "rm -rf /"},
	}))

	msg := h.readMessage(t)
	// The critical assertion: response must NOT be a JSON-RPC error.
	if msg.Error != nil {
		t.Fatalf("block must return structured {allow:false}, not JSON-RPC error: code=%d msg=%s",
			msg.Error.Code, msg.Error.Message)
	}
	var result struct {
		Allow   bool   `json:"allow"`
		Preview string `json:"preview"`
	}
	unmarshalResult(t, msg, &result)
	if result.Allow {
		t.Error("allow should be false")
	}
	if result.Preview != "reason for blocking" {
		t.Errorf("preview: got %q, want %q", result.Preview, "reason for blocking")
	}
}

// ---------------------------------------------------------------------------
// 17. Interceptor after — name discriminator
// ---------------------------------------------------------------------------

func TestInterceptorAfterNameFilter(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "mutator",
		Priority: 2000,
		After: func(_ context.Context, _ string, details any) (any, error) {
			return "mutated", nil
		},
	})
	h.ext.RegisterInterceptor(InterceptorDef{
		Name:     "identity",
		Priority: 100,
		After: func(_ context.Context, _ string, details any) (any, error) {
			return details, nil // pass through
		},
	})

	// Target "identity" — "mutator" must NOT run, details unchanged.
	h.ext.handleMessage(sendRequest(103, "interceptor/after", map[string]any{
		"name":     "identity",
		"toolName": "bash",
		"details":  "original",
	}))

	msg := h.readMessage(t)
	if msg.Error != nil {
		t.Fatalf("unexpected error: %v", msg.Error)
	}
	var result struct {
		Details string `json:"details"`
	}
	unmarshalResult(t, msg, &result)
	if result.Details != "original" {
		t.Errorf("details: got %q, want %q — mutator should not have run", result.Details, "original")
	}
}
