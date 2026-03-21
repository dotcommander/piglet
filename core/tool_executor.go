package core

import (
	"context"
	"fmt"
	"time"
)

type toolExecResult struct {
	index  int
	result *ToolResultMessage
	steer  []Message // steering messages that arrived during execution
}

func (a *Agent) executeTools(ctx context.Context, calls []ToolCall) ([]*ToolResultMessage, []Message) {
	a.mu.RLock()
	tools := a.cfg.Tools
	stepMode := a.stepMode
	stepGate := a.stepGate
	a.mu.RUnlock()

	toolCtx, toolCancel := context.WithCancel(ctx)
	defer toolCancel()

	sem := make(chan struct{}, a.cfg.toolConcurrency())
	resultCh := make(chan toolExecResult, len(calls))

	for i, tc := range calls {
		go a.executeOneToolWorker(toolCtx, i, tc, tools, sem, resultCh, stepMode, stepGate)
	}

	results := make([]*ToolResultMessage, len(calls))
	var steering []Message
	received := 0

	for received < len(calls) {
		select {
		case res := <-resultCh:
			results[res.index] = res.result
			received++

			if len(res.steer) > 0 && steering == nil {
				steering = res.steer
				toolCancel()
			}
		case <-toolCtx.Done():
			// Collect remaining
			for received < len(calls) {
				res := <-resultCh
				results[res.index] = res.result
				received++
			}
		}
	}

	// Filter nil results (cancelled tools)
	filtered := make([]*ToolResultMessage, 0, len(results))
	for _, r := range results {
		if r != nil {
			filtered = append(filtered, r)
		}
	}
	return filtered, steering
}

func (a *Agent) executeOneToolWorker(
	ctx context.Context,
	idx int,
	tc ToolCall,
	tools []Tool,
	sem chan struct{},
	resultCh chan<- toolExecResult,
	stepMode bool,
	stepGate chan StepAction,
) {
	// Acquire semaphore
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		resultCh <- toolExecResult{index: idx}
		return
	}

	// Step mode gate
	if stepMode && stepGate != nil {
		a.emit(EventStepWait{ToolCallID: tc.ID, ToolName: tc.Name, Args: tc.Arguments})
		select {
		case action := <-stepGate:
			switch action {
			case StepSkip:
				resultCh <- toolExecResult{
					index:  idx,
					result: skipResult(tc),
				}
				return
			case StepAbort:
				resultCh <- toolExecResult{index: idx}
				return
			case StepApprove:
				// continue
			}
		case <-ctx.Done():
			resultCh <- toolExecResult{index: idx}
			return
		}
	}

	// Find tool
	var tool *Tool
	for i := range tools {
		if tools[i].Name == tc.Name {
			tool = &tools[i]
			break
		}
	}

	if tool == nil {
		r := errorResult(tc, fmt.Sprintf("unknown tool: %s", tc.Name))
		a.emit(EventToolEnd{ToolCallID: tc.ID, ToolName: tc.Name, Result: r.Content, IsError: true})
		resultCh <- toolExecResult{index: idx, result: r}
		return
	}

	a.emit(EventToolStart{ToolCallID: tc.ID, ToolName: tc.Name, Args: tc.Arguments})

	result, err := safeExecute(ctx, tool, tc)

	if err != nil {
		r := errorResult(tc, err.Error())
		a.emit(EventToolEnd{ToolCallID: tc.ID, ToolName: tc.Name, Result: r.Content, IsError: true})
		resultCh <- toolExecResult{index: idx, result: r}
		return
	}

	tr := &ToolResultMessage{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    result.Content,
		Timestamp:  time.Now(),
	}
	a.emit(EventToolEnd{ToolCallID: tc.ID, ToolName: tc.Name, Result: result.Details})

	// Check for steering
	steer := a.dequeueSteer()

	resultCh <- toolExecResult{index: idx, result: tr, steer: steer}
}

// safeExecute wraps tool execution with panic recovery so one bad tool
// doesn't crash the agent.
func safeExecute(ctx context.Context, tool *Tool, tc ToolCall) (result *ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %s panicked: %v", tc.Name, r)
		}
	}()
	return tool.Execute(ctx, tc.ID, tc.Arguments)
}

func extractToolCalls(msg *AssistantMessage) []ToolCall {
	var calls []ToolCall
	for _, c := range msg.Content {
		if tc, ok := c.(ToolCall); ok {
			calls = append(calls, tc)
		}
	}
	return calls
}

func errorResult(tc ToolCall, errMsg string) *ToolResultMessage {
	return &ToolResultMessage{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    []ContentBlock{TextContent{Text: errMsg}},
		IsError:    true,
		Timestamp:  time.Now(),
	}
}

func skipResult(tc ToolCall) *ToolResultMessage {
	return &ToolResultMessage{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    []ContentBlock{TextContent{Text: "Tool execution skipped by user"}},
		IsError:    true,
		Timestamp:  time.Now(),
	}
}
