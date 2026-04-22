package external

import (
	"github.com/dotcommander/piglet/ext"
)

// HostAskUserParams is the request for host/askUser.
type HostAskUserParams struct {
	Prompt  string   `json:"prompt"`
	Choices []string `json:"choices"`
}

// HostAskUserResult is the response for host/askUser.
// Cancelled=true indicates the user dismissed the dialog without selecting.
type HostAskUserResult struct {
	Selected  string `json:"selected"`
	Cancelled bool   `json:"cancelled"`
}

func (h *Host) handleHostAskUser(msg *Message) {
	var params HostAskUserParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}
	if len(params.Choices) == 0 {
		h.respondError(*msg.ID, -32602, "askUser: choices must not be empty")
		return
	}
	// Capture msg.ID by value — the closure fires on a different goroutine
	// after the TUI processes the action (same pattern as handleHostShowPicker).
	id := *msg.ID
	h.app.AskUser(params.Prompt, params.Choices, func(r ext.AskUserResult) {
		h.respond(id, HostAskUserResult{Selected: r.Selected, Cancelled: r.Cancelled})
	})
	// Do NOT call respond here — the callback sends the response on selection or
	// cancellation. The existing 5-minute hostRequestTimeout on the extension side
	// handles the "TUI crashed / user walked away" case: the orphaned callback fires
	// onto a dead response path and becomes a no-op, matching picker's contract.
}
