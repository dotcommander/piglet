package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (a *Agent) streamWithRetry(ctx context.Context) (*AssistantMessage, error) {
	maxRetries := a.cfg.maxRetries()
	for attempt := range maxRetries {
		msg, err := a.streamOnce(ctx)
		if err == nil {
			return msg, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt >= maxRetries-1 {
			return nil, err
		}

		delay := retryDelay(attempt)
		a.emit(EventRetry{
			Attempt: attempt + 1,
			Max:     maxRetries,
			DelayMs: int(delay.Milliseconds()),
			Error:   err.Error(),
		})

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("unreachable")
}

func (a *Agent) streamOnce(ctx context.Context) (*AssistantMessage, error) {
	a.mu.Lock()
	msgs := make([]Message, len(a.messages))
	copy(msgs, a.messages)
	tools := make([]ToolSchema, len(a.cfg.Tools))
	for i, t := range a.cfg.Tools {
		tools[i] = t.ToolSchema
	}
	system := a.cfg.System
	turnCtx := a.turnContext
	a.turnContext = nil
	opts := a.cfg.Options
	model := a.cfg.Model
	a.mu.Unlock()

	// Append ephemeral turn context to system prompt
	if len(turnCtx) > 0 {
		var b strings.Builder
		b.WriteString(system)
		for _, ctx := range turnCtx {
			b.WriteString("\n\n")
			b.WriteString(ctx)
		}
		system = b.String()
	}

	if opts.APIKeyFunc != nil {
		// Validate key is available
		key := opts.APIKeyFunc(model.Provider)
		if key == "" {
			return nil, fmt.Errorf("no API key for provider %q", model.Provider)
		}
	}

	ch := a.cfg.Provider.Stream(ctx, StreamRequest{
		System:   system,
		Messages: msgs,
		Tools:    tools,
		Options:  opts,
	})

	var final *AssistantMessage
	for evt := range ch {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch evt.Type {
		case StreamTextDelta, StreamThinkingDelta, StreamToolCallDelta:
			kind := "text"
			if evt.Type == StreamThinkingDelta {
				kind = "thinking"
			} else if evt.Type == StreamToolCallDelta {
				kind = "toolcall"
			}
			a.emit(EventStreamDelta{Kind: kind, Index: evt.Index, Delta: evt.Delta})
		case StreamDone:
			final = evt.Message
			a.emit(EventStreamDone{Message: evt.Message})
		case StreamError:
			if evt.Error != nil {
				return nil, evt.Error
			}
			return nil, fmt.Errorf("stream error")
		}
	}

	if final == nil {
		return nil, fmt.Errorf("stream ended without a final message")
	}
	return final, nil
}

func retryDelay(attempt int) time.Duration {
	d := RetryBaseDelay * (1 << max(attempt, 0))
	if d > RetryMaxDelay {
		d = RetryMaxDelay
	}
	return d
}
