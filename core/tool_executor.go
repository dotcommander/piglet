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

	toolMap := make(map[string]*Tool, len(tools))
	for i := range tools {
		toolMap[tools[i].Name] = &tools[i]
	}

	toolCtx, toolCancel := context.WithCancel(ctx)
	defer toolCancel()

	concurrency := a.cfg.toolConcurrency()
	if stepMode {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	resultCh := make(chan toolExecResult, len(calls))

	for i, tc := range calls {
		go a.executeOneToolWorker(toolCtx, i, tc, toolMap, sem, resultCh, stepMode, stepGate)
	}

	results := make([]*ToolResultMessage, len(calls))
	var steering []Message
	received := 0

drain:
	for received < len(calls) {
		select {
		case res := <-resultCh:
			results[res.index] = res.result
			received++

			if res.result != nil && res.result.IsError {
				toolCancel()
			}

			if len(res.steer) > 0 && steering == nil {
				steering = res.steer
				toolCancel()
			}
		case <-toolCtx.Done():
			// Collect remaining — guard with parent ctx in case a worker is stuck.
			for received < len(calls) {
				select {
				case res := <-resultCh:
					results[res.index] = res.result
					received++
				case <-ctx.Done():
					break drain
				}
			}
		}
	}

	for i, r := range results {
		if r == nil {
			results[i] = errorResult(calls[i], "tool execution cancelled")
		}
	}
	return results, steering
}

// executeOneTool handles step-mode gate, tool lookup, execution, and event
// emission. Returns nil result for abort/context-cancel, skipResult for
// StepSkip, or the real result (with steer) on success/error.
func (a *Agent) executeOneTool(
	ctx context.Context,
	tc ToolCall,
	toolMap map[string]*Tool,
	stepMode bool,
	stepGate chan StepAction,
) (*ToolResultMessage, []Message) {
	if stepMode && stepGate != nil {
		a.emit(EventStepWait{ToolCallID: tc.ID, ToolName: tc.Name, Args: tc.Arguments})
		select {
		case action := <-stepGate:
			switch action {
			case StepSkip:
				return skipResult(tc), nil
			case StepAbort:
				return nil, nil
			case StepApprove:
				// continue
			}
		case <-ctx.Done():
			return nil, nil
		}
	}

	tool, ok := toolMap[tc.Name]
	if !ok {
		r := errorResult(tc, fmt.Sprintf("unknown tool: %s", tc.Name))
		a.emit(EventToolEnd{ToolCallID: tc.ID, ToolName: tc.Name, Result: r.Content, IsError: true})
		return r, nil
	}

	a.emit(EventToolStart{ToolCallID: tc.ID, ToolName: tc.Name, Args: tc.Arguments})

	result, err := safeExecute(ctx, tool, tc)
	if err != nil {
		r := errorResult(tc, err.Error())
		a.emit(EventToolEnd{ToolCallID: tc.ID, ToolName: tc.Name, Result: r.Content, IsError: true})
		return r, nil
	}

	tr := &ToolResultMessage{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    result.Content,
		Timestamp:  time.Now(),
	}
	a.emit(EventToolEnd{ToolCallID: tc.ID, ToolName: tc.Name, Result: result.Details})

	return tr, a.dequeueSteer()
}

func (a *Agent) executeOneToolWorker(
	ctx context.Context,
	idx int,
	tc ToolCall,
	toolMap map[string]*Tool,
	sem chan struct{},
	resultCh chan<- toolExecResult,
	stepMode bool,
	stepGate chan StepAction,
) {
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		resultCh <- toolExecResult{index: idx}
		return
	}

	r, steer := a.executeOneTool(ctx, tc, toolMap, stepMode, stepGate)
	resultCh <- toolExecResult{index: idx, result: r, steer: steer}
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
