package external

import (
	"encoding/json"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// hostRequestTimeout caps how long an inbound extension request (tool call, chat,
// agent run) may run before cancellation.
const hostRequestTimeout = 5 * time.Minute

// decodeParams unmarshals msg.Params into dst, responding with error on failure.
func (h *Host) decodeParams(msg *Message, dst any) bool {
	if err := json.Unmarshal(msg.Params, dst); err != nil {
		h.respondError(*msg.ID, -32602, "invalid params: "+err.Error())
		return false
	}
	return true
}

// requireApp checks h.app is bound, responding with error if nil.
func (h *Host) requireApp(msg *Message) bool {
	if h.app == nil {
		h.respondError(*msg.ID, -32603, "host app not available")
		return false
	}
	return true
}

// handleAppCall decodes params, requires app, calls fn, and responds.
// fn receives the decoded params and the bound app.
// If fn returns an error, responds with error. Otherwise responds with fn's result.
func handleAppCall[P any, R any](h *Host, msg *Message, fn func(*ext.App, P) (R, error)) {
	var params P
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}
	result, err := fn(h.app, params)
	if err != nil {
		h.respondError(*msg.ID, -32603, err.Error())
		return
	}
	h.respond(*msg.ID, result)
}

// resolveProvider resolves a StreamProvider for the given model, responding
// with error on failure. Returns (provider, true) on success.
func (h *Host) resolveProvider(msg *Message, model string) (core.StreamProvider, bool) {
	if h.resolveProviderFn == nil {
		h.respondError(*msg.ID, -32603, "provider resolver not available")
		return nil, false
	}
	prov, err := h.resolveProviderFn(model)
	if err != nil {
		h.respondError(*msg.ID, -32603, "resolve provider: "+err.Error())
		return nil, false
	}
	return prov, true
}

// requireUndoSnapshots checks h.undoSnapshotsFn is set, responding with error if nil.
func (h *Host) requireUndoSnapshots(msg *Message) bool {
	if h.undoSnapshotsFn == nil {
		h.respondError(*msg.ID, -32603, "undo snapshots not available")
		return false
	}
	return true
}
