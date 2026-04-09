package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dotcommander/piglet/core"
)

// ---------------------------------------------------------------------------
// proxyStreamProvider
// ---------------------------------------------------------------------------

// proxyStreamProvider implements core.StreamProvider by delegating LLM streaming
// to an external extension process via JSON-RPC.
type proxyStreamProvider struct {
	host  *Host
	model core.Model
}

func (p *proxyStreamProvider) Stream(ctx context.Context, req core.StreamRequest) <-chan core.StreamEvent {
	ch := make(chan core.StreamEvent, 64)
	go p.stream(ctx, req, ch)
	return ch
}

// marshalStreamRequest marshals all serializable fields of a StreamRequest into
// a ProviderStreamParams ready for wire transport. Returns (params, true) on
// success, or emits a StreamError on ch and returns (_, false) on failure.
// requestID must already be allocated and registered in deltaChans before calling.
func (p *proxyStreamProvider) marshalStreamRequest(
	req core.StreamRequest,
	requestID int,
	ch chan core.StreamEvent,
) (ProviderStreamParams, bool) {
	failf := func(label string, err error) (ProviderStreamParams, bool) {
		p.host.releaseDeltaChan(requestID)
		ch <- core.StreamEvent{Type: core.StreamError, Error: fmt.Errorf("marshal %s: %w", label, err)}
		return ProviderStreamParams{}, false
	}

	modelJSON, err := json.Marshal(p.model)
	if err != nil {
		return failf("model", err)
	}
	messagesJSON, err := json.Marshal(req.Messages)
	if err != nil {
		return failf("messages", err)
	}

	var toolsJSON json.RawMessage
	if len(req.Tools) > 0 {
		toolsJSON, err = json.Marshal(req.Tools)
		if err != nil {
			return failf("tools", err)
		}
	}

	// Strip APIKeyFunc (not serializable) before marshalling options.
	type wireOptions struct {
		Temperature *float64           `json:"temperature,omitempty"`
		MaxTokens   *int               `json:"maxTokens,omitempty"`
		Thinking    core.ThinkingLevel `json:"thinking,omitempty"`
		Headers     map[string]string  `json:"headers,omitempty"`
	}
	optJSON, err := json.Marshal(wireOptions{
		Temperature: req.Options.Temperature,
		MaxTokens:   req.Options.MaxTokens,
		Thinking:    req.Options.Thinking,
		Headers:     req.Options.Headers,
	})
	if err != nil {
		return failf("options", err)
	}

	return ProviderStreamParams{
		RequestID: requestID,
		Model:     modelJSON,
		System:    req.System,
		Messages:  messagesJSON,
		Tools:     toolsJSON,
		Options:   optJSON,
	}, true
}

func (p *proxyStreamProvider) stream(ctx context.Context, req core.StreamRequest, ch chan core.StreamEvent) {
	defer close(ch)

	select {
	case <-p.host.Closed():
		ch <- core.StreamEvent{Type: core.StreamError, Error: fmt.Errorf("extension %s stopped", p.host.Name())}
		return
	default:
	}

	requestID := int(p.host.nextID.Add(1))

	// Register delta channel BEFORE sending the request so no deltas are lost.
	deltaCh := make(chan ProviderDeltaParams, 256)
	p.host.deltaMu.Lock()
	p.host.deltaChans[requestID] = deltaCh
	p.host.deltaMu.Unlock()

	params, ok := p.marshalStreamRequest(req, requestID, ch)
	if !ok {
		return
	}

	// Send the request in a goroutine — it blocks until the extension responds.
	type streamResponse struct {
		msg *Message
		err error
	}
	respCh := make(chan streamResponse, 1)
	go func() {
		resp, err := p.host.request(ctx, MethodProviderStream, params)
		respCh <- streamResponse{resp, err}
	}()

	// Forward deltas while waiting for the final response.
	for {
		select {
		case delta, ok := <-deltaCh:
			if !ok {
				return
			}
			ch <- deltaToStreamEvent(delta)

		case resp := <-respCh:
			p.host.releaseDeltaChan(requestID)
			drainDeltas(deltaCh, ch)
			if resp.err != nil {
				ch <- core.StreamEvent{Type: core.StreamError, Error: resp.err}
				return
			}
			if resp.msg.Error != nil {
				ch <- core.StreamEvent{Type: core.StreamError, Error: errors.New(resp.msg.Error.Message)}
				return
			}
			var result ProviderStreamResult
			if err := json.Unmarshal(resp.msg.Result, &result); err != nil {
				ch <- core.StreamEvent{Type: core.StreamError, Error: fmt.Errorf("unmarshal provider/stream result: %w", err)}
				return
			}
			if result.Error != "" {
				ch <- core.StreamEvent{Type: core.StreamError, Error: errors.New(result.Error)}
				return
			}
			var msg core.AssistantMessage
			if err := json.Unmarshal(result.Message, &msg); err != nil {
				ch <- core.StreamEvent{Type: core.StreamError, Error: fmt.Errorf("unmarshal assistant message: %w", err)}
				return
			}
			ch <- core.StreamEvent{Type: core.StreamDone, Message: &msg}
			return

		case <-ctx.Done():
			p.host.releaseDeltaChan(requestID)
			ch <- core.StreamEvent{Type: core.StreamError, Error: ctx.Err()}
			return

		case <-p.host.closed:
			p.host.releaseDeltaChan(requestID)
			ch <- core.StreamEvent{Type: core.StreamError, Error: fmt.Errorf("extension %s crashed", p.host.manifest.Name)}
			return
		}
	}
}

// drainDeltas flushes any remaining buffered deltas from deltaCh into ch.
// Called after releaseDeltaChan to avoid losing events that arrived concurrently.
func drainDeltas(deltaCh <-chan ProviderDeltaParams, ch chan<- core.StreamEvent) {
	for {
		select {
		case d := <-deltaCh:
			ch <- deltaToStreamEvent(d)
		default:
			return
		}
	}
}

// deltaToStreamEvent converts a wire ProviderDeltaParams to a core.StreamEvent.
func deltaToStreamEvent(d ProviderDeltaParams) core.StreamEvent {
	evt := core.StreamEvent{
		Type:  d.Type,
		Index: d.Index,
		Delta: d.Delta,
	}
	if d.Tool != nil {
		var args map[string]any
		if err := json.Unmarshal([]byte(d.Tool.Arguments), &args); err != nil {
			slog.Warn("unmarshal tool arguments in stream delta", "tool", d.Tool.Name, "error", err)
		}
		evt.Tool = &core.ToolCall{
			ID:        d.Tool.ID,
			Name:      d.Tool.Name,
			Arguments: args,
		}
	}
	return evt
}
