package sdk

import (
	"encoding/json"
	"slices"
)

// handleEventDispatch dispatches an agent lifecycle event to registered handlers.
func (e *Extension) handleEventDispatch(msg *rpcMessage) {
	var params struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	var resultAction *wireActionResult
	for _, eh := range e.eventHandlers {
		if len(eh.Events) > 0 && !slices.Contains(eh.Events, params.Type) {
			continue
		}
		if eh.Handle == nil {
			continue
		}
		action := eh.Handle(ctx, params.Type, params.Data)
		if action != nil {
			payload, _ := json.Marshal(action.Payload)
			resultAction = &wireActionResult{Type: action.Type, Payload: payload}
			break // first action wins
		}
	}

	e.sendResponse(*msg.ID, map[string]any{"action": resultAction})
}

// handleShortcutHandle dispatches a keyboard shortcut to its registered handler.
func (e *Extension) handleShortcutHandle(msg *rpcMessage) {
	var params struct {
		Key string `json:"key"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	// Match the specific shortcut by key
	sc, ok := e.shortcuts[params.Key]
	if !ok || sc.Handler == nil {
		e.sendResponse(*msg.ID, map[string]any{"action": nil})
		return
	}

	action, err := sc.Handler(ctx)
	if err != nil {
		e.sendError(*msg.ID, -32603, err.Error())
		return
	}
	var resultAction *wireActionResult
	if action != nil {
		payload, _ := json.Marshal(action.Payload)
		resultAction = &wireActionResult{Type: action.Type, Payload: payload}
	}

	e.sendResponse(*msg.ID, map[string]any{"action": resultAction})
}

// handleMessageHook runs all message hooks and returns concatenated injection text.
func (e *Extension) handleMessageHook(msg *rpcMessage) {
	var params struct {
		Message string `json:"message"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	var injection string
	for _, mh := range e.messageHooks {
		if mh.OnMessage == nil {
			continue
		}
		extra, err := mh.OnMessage(ctx, params.Message)
		if err != nil {
			e.sendError(*msg.ID, -32603, err.Error())
			return
		}
		if extra != "" {
			if injection != "" {
				injection += "\n"
			}
			injection += extra
		}
	}

	e.sendResponse(*msg.ID, map[string]any{"injection": injection})
}

// handleInputTransform runs the input transformer chain and returns the final output.
func (e *Extension) handleInputTransform(msg *rpcMessage) {
	var params struct {
		Input string `json:"input"`
	}
	if !e.unmarshalParams(msg, &params) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	current := params.Input
	for _, it := range e.inputTransformers {
		if it.Transform == nil {
			continue
		}
		output, handled, err := it.Transform(ctx, current)
		if err != nil {
			e.sendError(*msg.ID, -32603, err.Error())
			return
		}
		if handled {
			e.sendResponse(*msg.ID, map[string]any{"output": output, "handled": true})
			return
		}
		current = output
	}
	e.sendResponse(*msg.ID, map[string]any{"output": current, "handled": false})
}

// handleCompactExecute dispatches a compaction request to the registered compactor.
func (e *Extension) handleCompactExecute(msg *rpcMessage) {
	if e.compactor == nil || e.compactor.Compact == nil {
		e.sendError(*msg.ID, -32603, "no compactor registered")
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	result, err := e.compactor.Compact(ctx, msg.Params)
	if err != nil {
		e.sendError(*msg.ID, -32603, err.Error())
		return
	}

	e.sendResponse(*msg.ID, result)
}

// handleProviderStream dispatches a provider streaming request to the registered handler.
func (e *Extension) handleProviderStream(msg *rpcMessage) {
	if e.providerStreamHandler == nil {
		e.sendError(*msg.ID, -32603, "no provider stream handler registered")
		return
	}

	var req ProviderStreamRequest
	if !e.unmarshalParams(msg, &req) {
		return
	}

	ctx, cleanup := e.requestCtx(*msg.ID)
	defer cleanup()

	resp, err := e.providerStreamHandler(ctx, e, req)
	if err != nil {
		e.sendError(*msg.ID, -32603, err.Error())
		return
	}

	e.sendResponse(*msg.ID, resp)
}
