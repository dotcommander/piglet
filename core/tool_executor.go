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

type toolMeta struct {
	call     ToolCall
	behavior InterruptBehavior
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

	var safeMeta, unsafeMeta []toolMeta
	for _, tc := range calls {
		safe := true
		behavior := InterruptCancel
		if t, ok := toolMap[tc.Name]; ok {
			if t.ConcurrencySafe != nil {
				safe = t.ConcurrencySafe(tc.Arguments)
			}
			behavior = t.InterruptBehavior
		}
		m := toolMeta{call: tc, behavior: behavior}
		if safe {
			safeMeta = append(safeMeta, m)
		} else {
			unsafeMeta = append(unsafeMeta, m)
		}
	}

	// Execute safe calls in parallel.
	safeResults, safeSteer := a.executeParallelBatch(ctx, safeMeta, toolMap, stepMode, stepGate)

	// If an error or steering occurred, skip unsafe calls.
	if len(safeSteer) > 0 {
		return safeResults, safeSteer
	}
	for _, r := range safeResults {
		if r.IsError {
			return safeResults, nil
		}
	}

	// Execute unsafe calls sequentially.
	unsafeResults, unsafeSteer := a.executeSequentialBatch(ctx, unsafeMeta, toolMap, stepMode, stepGate)

	// Merge results.
	merged := make([]*ToolResultMessage, 0, len(safeResults)+len(unsafeResults))
	merged = append(merged, safeResults...)
	merged = append(merged, unsafeResults...)
	return merged, unsafeSteer
}

// executeParallelBatch runs tool calls concurrently with a semaphore and
// error-triggered cancellation of siblings.
func (a *Agent) executeParallelBatch(
	ctx context.Context,
	meta []toolMeta,
	toolMap map[string]*Tool,
	stepMode bool,
	stepGate chan StepAction,
) ([]*ToolResultMessage, []Message) {
	if len(meta) == 0 {
		return nil, nil
	}

	toolCtx, toolCancel := context.WithCancel(ctx)
	defer toolCancel()

	sem := make(chan struct{}, a.cfg.toolConcurrency())
	resultCh := make(chan toolExecResult, len(meta))

	for i, m := range meta {
		go a.executeOneToolWorker(toolCtx, i, m.call, toolMap, sem, resultCh, stepMode, stepGate)
	}

	results := make([]*ToolResultMessage, len(meta))
	var steering []Message
	received := 0

	allBlock := true
	for _, m := range meta {
		if m.behavior != InterruptBlock {
			allBlock = false
			break
		}
	}

drain:
	for received < len(meta) {
		select {
		case res := <-resultCh:
			results[res.index] = res.result
			received++

			if res.result != nil && res.result.IsError {
				toolCancel()
			}

			if len(res.steer) > 0 && steering == nil {
				steering = res.steer
				// Only cancel if not all tools are InterruptBlock.
				// If all are block-type, let them finish — steer is queued for after.
				if !allBlock {
					toolCancel()
				}
			}
		case <-toolCtx.Done():
			// Collect remaining — guard with parent ctx in case a worker is stuck
			for received < len(meta) {
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

	filtered := make([]*ToolResultMessage, 0, len(results))
	for _, r := range results {
		if r != nil {
			filtered = append(filtered, r)
		}
	}
	return filtered, steering
}

// executeSequentialBatch returns immediately on first error or steering message.
func (a *Agent) executeSequentialBatch(
	ctx context.Context,
	meta []toolMeta,
	toolMap map[string]*Tool,
	stepMode bool,
	stepGate chan StepAction,
) ([]*ToolResultMessage, []Message) {
	if len(meta) == 0 {
		return nil, nil
	}

	results := make([]*ToolResultMessage, 0, len(meta))

	for _, m := range meta {
		// Check context before starting next call.
		select {
		case <-ctx.Done():
			return results, nil
		default:
		}

		r, steer := a.executeOneTool(ctx, m.call, toolMap, stepMode, stepGate)
		if r != nil {
			results = append(results, r)
		}
		if steer != nil {
			return results, steer
		}
		if r != nil && r.IsError {
			return results, nil
		}
	}

	return results, nil
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
