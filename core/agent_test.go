package core_test

import (
	"context"
	"fmt"
	"github.com/dotcommander/piglet/core"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock provider
// ---------------------------------------------------------------------------

// mockProvider returns canned responses. Each call to Stream pops the next response.
type mockProvider struct {
	mu        sync.Mutex
	responses []*core.AssistantMessage
	calls     int
}

func newMockProvider(responses ...*core.AssistantMessage) *mockProvider {
	return &mockProvider{responses: responses}
}

func (m *mockProvider) Stream(_ context.Context, req core.StreamRequest) <-chan core.StreamEvent {
	ch := make(chan core.StreamEvent, 2)
	m.mu.Lock()
	idx := m.calls
	m.calls++
	m.mu.Unlock()

	go func() {
		defer close(ch)
		var msg *core.AssistantMessage
		if idx < len(m.responses) {
			msg = m.responses[idx]
		} else {
			msg = textReply("default response")
		}
		ch <- core.StreamEvent{Type: core.StreamDone, Message: msg}
	}()
	return ch
}

func (m *mockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func textReply(text string) *core.AssistantMessage {
	return &core.AssistantMessage{
		Content:    []core.AssistantContent{core.TextContent{Text: text}},
		StopReason: core.StopReasonStop,
		Timestamp:  time.Now(),
	}
}

func toolCallReply(id, name string, args map[string]any) *core.AssistantMessage {
	return &core.AssistantMessage{
		Content: []core.AssistantContent{core.ToolCall{
			ID: id, Name: name, Arguments: args,
		}},
		StopReason: core.StopReasonTool,
		Timestamp:  time.Now(),
	}
}

func collectEvents(ch <-chan core.Event) []core.Event {
	var events []core.Event
	for evt := range ch {
		events = append(events, evt)
	}
	return events
}

func echoTool() core.Tool {
	return core.Tool{
		ToolSchema: core.ToolSchema{
			Name:        "echo",
			Description: "Echoes the input",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			text, _ := args["text"].(string)
			return &core.ToolResult{
				Content: []core.ContentBlock{core.TextContent{Text: text}},
			}, nil
		},
	}
}

func failTool() core.Tool {
	return core.Tool{
		ToolSchema: core.ToolSchema{Name: "fail", Description: "Always fails"},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			return nil, fmt.Errorf("tool failed")
		},
	}
}

func slowTool(delay time.Duration) core.Tool {
	return core.Tool{
		ToolSchema: core.ToolSchema{Name: "slow", Description: "Takes time"},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			select {
			case <-time.After(delay):
				return &core.ToolResult{Content: []core.ContentBlock{core.TextContent{Text: "done"}}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAgentBasicConversation(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(textReply("Hello!"))
	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		System:   "You are helpful.",
	})

	ctx := context.Background()
	events := collectEvents(ag.Start(ctx, "Hi"))

	// Should have: AgentStart, TurnStart, StreamDone, TurnEnd, AgentEnd
	var types []string
	for _, e := range events {
		types = append(types, fmt.Sprintf("%T", e))
	}
	assert.Contains(t, types, "core.EventAgentStart")
	assert.Contains(t, types, "core.EventAgentEnd")
	assert.Contains(t, types, "core.EventTurnStart")
	assert.Contains(t, types, "core.EventTurnEnd")

	msgs := ag.Messages()
	require.Len(t, msgs, 2) // user + assistant
	assert.IsType(t, &core.UserMessage{}, msgs[0])
	assert.IsType(t, &core.AssistantMessage{}, msgs[1])
}

func TestAgentToolExecution(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		toolCallReply("tc1", "echo", map[string]any{"text": "hello"}),
		textReply("Done!"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{echoTool()},
	})

	events := collectEvents(ag.Start(context.Background(), "echo hello"))

	var toolStarts, toolEnds int
	for _, e := range events {
		switch e.(type) {
		case core.EventToolStart:
			toolStarts++
		case core.EventToolEnd:
			toolEnds++
		}
	}
	assert.Equal(t, 1, toolStarts)
	assert.Equal(t, 1, toolEnds)

	msgs := ag.Messages()
	// user, assistant(tool_call), tool_result, assistant(text)
	require.Len(t, msgs, 4)
	assert.IsType(t, &core.ToolResultMessage{}, msgs[2])
}

func TestAgentToolError(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		toolCallReply("tc1", "fail", nil),
		textReply("Sorry, the tool failed."),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{failTool()},
	})

	events := collectEvents(ag.Start(context.Background(), "fail"))

	var errorResults int
	for _, e := range events {
		if te, ok := e.(core.EventToolEnd); ok && te.IsError {
			errorResults++
		}
	}
	assert.Equal(t, 1, errorResults)

	// Tool result should be marked as error
	msgs := ag.Messages()
	require.GreaterOrEqual(t, len(msgs), 3)
	tr, ok := msgs[2].(*core.ToolResultMessage)
	require.True(t, ok)
	assert.True(t, tr.IsError)
}

func TestAgentMaxTurns(t *testing.T) {
	t.Parallel()

	// Provider always returns tool calls → agent would loop forever without MaxTurns
	prov := newMockProvider(
		toolCallReply("tc1", "echo", map[string]any{"text": "1"}),
		toolCallReply("tc2", "echo", map[string]any{"text": "2"}),
		toolCallReply("tc3", "echo", map[string]any{"text": "3"}),
		textReply("unreachable"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{echoTool()},
		MaxTurns: 2,
	})

	events := collectEvents(ag.Start(context.Background(), "loop"))

	var maxTurnsEvents int
	for _, e := range events {
		if _, ok := e.(core.EventMaxTurns); ok {
			maxTurnsEvents++
		}
	}
	assert.Equal(t, 1, maxTurnsEvents)
	assert.LessOrEqual(t, prov.CallCount(), 3) // at most 2 turns + possible extra
}

func TestAgentMaxTurnsZeroUnlimited(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(textReply("done"))
	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		MaxTurns: 0, // unlimited
	})

	events := collectEvents(ag.Start(context.Background(), "hi"))

	for _, e := range events {
		_, isMaxTurns := e.(core.EventMaxTurns)
		assert.False(t, isMaxTurns, "should not emit MaxTurns with unlimited turns")
	}
}

func TestAgentStepModeApprove(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		toolCallReply("tc1", "echo", map[string]any{"text": "step"}),
		textReply("approved"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{echoTool()},
	})
	ag.SetStepMode(true)

	ch := ag.Start(context.Background(), "step test")

	// Wait for step wait event
	for evt := range ch {
		if _, ok := evt.(core.EventStepWait); ok {
			ag.StepRespond(core.StepApprove)
			break
		}
	}

	// Drain remaining events
	for range ch {
	}

	msgs := ag.Messages()
	require.GreaterOrEqual(t, len(msgs), 3)
	tr, ok := msgs[2].(*core.ToolResultMessage)
	require.True(t, ok)
	assert.False(t, tr.IsError) // was approved, not skipped
}

func TestAgentStepModeSkip(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		toolCallReply("tc1", "echo", map[string]any{"text": "skip me"}),
		textReply("skipped"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{echoTool()},
	})
	ag.SetStepMode(true)

	ch := ag.Start(context.Background(), "step test")

	for evt := range ch {
		if _, ok := evt.(core.EventStepWait); ok {
			ag.StepRespond(core.StepSkip)
			break
		}
	}
	for range ch {
	}

	msgs := ag.Messages()
	require.GreaterOrEqual(t, len(msgs), 3)
	tr, ok := msgs[2].(*core.ToolResultMessage)
	require.True(t, ok)
	assert.True(t, tr.IsError)
	assert.Equal(t, "Tool execution skipped by user", tr.Content[0].(core.TextContent).Text)
}

func TestAgentStepModeDisabled(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		toolCallReply("tc1", "echo", map[string]any{"text": "no step"}),
		textReply("done"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{echoTool()},
	})
	// Step mode NOT enabled

	events := collectEvents(ag.Start(context.Background(), "go"))

	for _, e := range events {
		_, isStepWait := e.(core.EventStepWait)
		assert.False(t, isStepWait, "should not emit StepWait when step mode is off")
	}
}

func TestAgentSteering(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		toolCallReply("tc1", "slow", nil),
		textReply("steered"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{slowTool(2 * time.Second)},
	})

	ch := ag.Start(context.Background(), "do slow thing")

	// Wait for tool to start then steer
	for evt := range ch {
		if _, ok := evt.(core.EventToolStart); ok {
			ag.Steer(&core.UserMessage{Content: "stop that", Timestamp: time.Now()})
			break
		}
	}
	for range ch {
	}

	// Steering message should appear in history
	msgs := ag.Messages()
	found := false
	for _, m := range msgs {
		if um, ok := m.(*core.UserMessage); ok && um.Content == "stop that" {
			found = true
		}
	}
	assert.True(t, found, "steering message should be in history")
}

func TestAgentFollowUp(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		textReply("first response"),
		textReply("follow-up response"),
	)

	ag := core.NewAgent(core.AgentConfig{Provider: prov})

	// Queue follow-up before starting
	ag.FollowUp(&core.UserMessage{Content: "follow up", Timestamp: time.Now()})

	events := collectEvents(ag.Start(context.Background(), "initial"))

	// Should have 2 AgentStart... nope, 1 start, but 2 turns
	var turnStarts int
	for _, e := range events {
		if _, ok := e.(core.EventTurnStart); ok {
			turnStarts++
		}
	}
	assert.Equal(t, 2, turnStarts)
	assert.Equal(t, 2, prov.CallCount())
}

func TestAgentCancellation(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(textReply("never"))
	// Override to block
	blockProv := &blockingProvider{}

	ag := core.NewAgent(core.AgentConfig{Provider: blockProv})

	ctx, cancel := context.WithCancel(context.Background())
	ch := ag.Start(ctx, "block")

	// Cancel immediately
	time.Sleep(10 * time.Millisecond)
	cancel()

	events := collectEvents(ch)
	_ = events // just ensure it terminates
	assert.False(t, ag.IsRunning())
	_ = prov // suppress unused
}

type blockingProvider struct{}

func (b *blockingProvider) Stream(ctx context.Context, _ core.StreamRequest) <-chan core.StreamEvent {
	ch := make(chan core.StreamEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch
}

func TestAgentUnknownTool(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		toolCallReply("tc1", "nonexistent", nil),
		textReply("recovered"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{echoTool()},
	})

	events := collectEvents(ag.Start(context.Background(), "call missing tool"))

	var errorEnds int
	for _, e := range events {
		if te, ok := e.(core.EventToolEnd); ok && te.IsError {
			errorEnds++
		}
	}
	assert.Equal(t, 1, errorEnds)
}

func TestAgentMultipleToolsParallel(t *testing.T) {
	t.Parallel()

	prov := newMockProvider(
		// Two tool calls in one message
		&core.AssistantMessage{
			Content: []core.AssistantContent{
				core.ToolCall{ID: "tc1", Name: "echo", Arguments: map[string]any{"text": "a"}},
				core.ToolCall{ID: "tc2", Name: "echo", Arguments: map[string]any{"text": "b"}},
			},
			StopReason: core.StopReasonTool,
			Timestamp:  time.Now(),
		},
		textReply("both done"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{echoTool()},
	})

	events := collectEvents(ag.Start(context.Background(), "do two things"))

	var toolEnds int
	for _, e := range events {
		if _, ok := e.(core.EventToolEnd); ok {
			toolEnds++
		}
	}
	assert.Equal(t, 2, toolEnds)

	// Should have: user, assistant(2 tools), 2 tool results, assistant(text) = 5
	msgs := ag.Messages()
	require.Len(t, msgs, 5)
}

func TestNewAgentDefaults(t *testing.T) {
	t.Parallel()

	ag := core.NewAgent(core.AgentConfig{
		Provider: newMockProvider(),
	})
	assert.NotNil(t, ag)
	assert.False(t, ag.IsRunning())
	assert.False(t, ag.StepMode())
	assert.Empty(t, ag.Messages())
}

func TestAgentPanicRecovery(t *testing.T) {
	t.Parallel()

	panicTool := core.Tool{
		ToolSchema: core.ToolSchema{
			Name:        "boom",
			Description: "Panics",
			Parameters:  map[string]any{"type": "object"},
		},
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			panic("kaboom")
		},
	}

	prov := newMockProvider(
		toolCallReply("tc1", "boom", map[string]any{}),
		textReply("recovered"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		Tools:    []core.Tool{panicTool},
	})

	events := collectEvents(ag.Start(context.Background(), "trigger panic"))

	// Agent should survive and emit a tool error, not crash
	var toolErrors int
	var turnEnds int
	for _, e := range events {
		if te, ok := e.(core.EventToolEnd); ok && te.IsError {
			toolErrors++
		}
		if _, ok := e.(core.EventTurnEnd); ok {
			turnEnds++
		}
	}
	assert.Equal(t, 1, toolErrors, "panicking tool should produce a tool error")
	// Agent should continue past the panic to the second LLM call
	assert.GreaterOrEqual(t, turnEnds, 2, "agent should continue after tool panic")
}

func TestAgentMaxMessages(t *testing.T) {
	t.Parallel()

	// 3 tool-call turns = 3*(assistant+tool_result) + 1 user = 7 messages, then final assistant = 8
	prov := newMockProvider(
		toolCallReply("tc1", "echo", map[string]any{"text": "a"}),
		toolCallReply("tc2", "echo", map[string]any{"text": "b"}),
		toolCallReply("tc3", "echo", map[string]any{"text": "c"}),
		textReply("done"),
	)

	ag := core.NewAgent(core.AgentConfig{
		Provider:    prov,
		Tools:       []core.Tool{echoTool()},
		MaxMessages: 5,
	})

	collectEvents(ag.Start(context.Background(), "test"))

	msgs := ag.Messages()
	assert.LessOrEqual(t, len(msgs), 5, "messages should be capped at MaxMessages")
	// First message (user prompt) should always be preserved
	if um, ok := msgs[0].(*core.UserMessage); ok {
		assert.Equal(t, "test", um.Content)
	} else {
		t.Error("first message should be the user prompt")
	}
}

func TestAgentAutoCompact(t *testing.T) {
	t.Parallel()

	withTokens := func(msg *core.AssistantMessage, input int) *core.AssistantMessage {
		msg.Usage = core.Usage{InputTokens: input, OutputTokens: 50}
		return msg
	}

	// 4 tool-call turns = 4*(assistant+tool_result) + 1 user = 9 messages → above keepRecent+1 guard
	// CompactAt checks the most recent assistant message's InputTokens (current context window size).
	prov := newMockProvider(
		withTokens(toolCallReply("tc1", "echo", map[string]any{"text": "a"}), 2000),
		withTokens(toolCallReply("tc2", "echo", map[string]any{"text": "b"}), 3000),
		withTokens(toolCallReply("tc3", "echo", map[string]any{"text": "c"}), 5000),
		withTokens(toolCallReply("tc4", "echo", map[string]any{"text": "d"}), 9000), // 9000 > 8000 threshold
		withTokens(textReply("final"), 1000),
	)

	var compactCalled int
	ag := core.NewAgent(core.AgentConfig{
		Provider:  prov,
		Tools:     []core.Tool{echoTool()},
		CompactAt: 8000,
		OnCompact: func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
			compactCalled++
			return core.CompactMessages(msgs, "Summary of earlier conversation."), nil
		},
	})

	events := collectEvents(ag.Start(context.Background(), "test"))

	assert.GreaterOrEqual(t, compactCalled, 1, "OnCompact should have been called")

	var compactEvents int
	for _, e := range events {
		if _, ok := e.(core.EventCompact); ok {
			compactEvents++
		}
	}
	assert.GreaterOrEqual(t, compactEvents, 1, "should emit EventCompact")
}
