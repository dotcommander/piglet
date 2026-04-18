package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// request sends a JSON-RPC request to the host and waits for the response.
func (e *Extension) request(ctx context.Context, method string, params any) (*rpcMessage, error) {
	id := int(e.nextID.Add(1))

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	ch := make(chan *rpcMessage, 1)
	e.pendingMu.Lock()
	e.pending[id] = ch
	e.pendingMu.Unlock()

	e.write(&rpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  paramsJSON,
	})

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		e.pendingMu.Lock()
		delete(e.pending, id)
		e.pendingMu.Unlock()
		// Notify host to cancel work for this request
		e.sendNotification("$/cancelRequest", map[string]int{"id": id})
		// Drain any late-arriving response (50ms grace)
		t := time.NewTimer(50 * time.Millisecond)
		select {
		case resp := <-ch:
			t.Stop()
			return resp, nil
		case <-t.C:
			return nil, ctx.Err()
		}
	}
}
