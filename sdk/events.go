package sdk

import "encoding/json"

// ---------------------------------------------------------------------------
// Typed event payload structs
//
// Each struct mirrors the JSON shape emitted by the host when it calls
// json.Marshal on the corresponding core.Event* type. Field names are
// preserved as-is (no JSON tags on core structs), so "Messages", "ToolCallID",
// "IsError", etc. are the exact wire keys.
//
// Use DecodeEvent[T] to unmarshal a raw event payload into the typed struct.
// ---------------------------------------------------------------------------

// EventAgentStartPayload is the payload for EventAgentStart. No fields — empty object.
type EventAgentStartPayload struct{}

// EventAgentEndPayload is the payload for EventAgentEnd.
// Messages is kept as raw JSON because core.Message is a sealed interface
// requiring custom unmarshaling; extensions that need the messages can
// unmarshal further using the host's session methods.
type EventAgentEndPayload struct {
	Messages json.RawMessage `json:"Messages"`
}

// EventTurnStartPayload is the payload for EventTurnStart. No fields.
type EventTurnStartPayload struct{}

// EventTurnEndPayload is the payload for EventTurnEnd.
// Assistant and ToolResults use raw JSON for the same reason as EventAgentEndPayload.
type EventTurnEndPayload struct {
	Assistant   json.RawMessage `json:"Assistant"`   // *core.AssistantMessage or null
	ToolResults json.RawMessage `json:"ToolResults"` // []*core.ToolResultMessage or null
}

// EventStreamDeltaPayload is the payload for EventStreamDelta.
// Kind is one of "text", "thinking", or "toolcall".
type EventStreamDeltaPayload struct {
	Kind  string `json:"Kind"`
	Index int    `json:"Index"`
	Delta string `json:"Delta"`
}

// EventStreamDonePayload is the payload for EventStreamDone.
// Message uses raw JSON for the same reason as EventAgentEndPayload.
type EventStreamDonePayload struct {
	Message json.RawMessage `json:"Message"` // *core.AssistantMessage or null
}

// EventToolStartPayload is the payload for EventToolStart.
type EventToolStartPayload struct {
	ToolCallID string         `json:"ToolCallID"`
	ToolName   string         `json:"ToolName"`
	Args       map[string]any `json:"Args"`
}

// EventToolUpdatePayload is the payload for EventToolUpdate.
// Partial is kept raw because it is an opaque any value set by the tool.
type EventToolUpdatePayload struct {
	ToolCallID string          `json:"ToolCallID"`
	ToolName   string          `json:"ToolName"`
	Partial    json.RawMessage `json:"Partial"`
}

// EventToolEndPayload is the payload for EventToolEnd.
// Result is kept raw because it is an opaque any value set by the tool.
type EventToolEndPayload struct {
	ToolCallID string          `json:"ToolCallID"`
	ToolName   string          `json:"ToolName"`
	Result     json.RawMessage `json:"Result"`
	IsError    bool            `json:"IsError"`
}

// EventRetryPayload is the payload for EventRetry.
type EventRetryPayload struct {
	Attempt int    `json:"Attempt"`
	Max     int    `json:"Max"`
	DelayMs int    `json:"DelayMs"`
	Error   string `json:"Error"`
}

// EventMaxTurnsPayload is the payload for EventMaxTurns.
type EventMaxTurnsPayload struct {
	Count int `json:"Count"`
	Max   int `json:"Max"`
}

// EventStepWaitPayload is the payload for EventStepWait.
type EventStepWaitPayload struct {
	ToolCallID string         `json:"ToolCallID"`
	ToolName   string         `json:"ToolName"`
	Args       map[string]any `json:"Args"`
}

// EventCompactPayload is the payload for EventCompact.
type EventCompactPayload struct {
	Before          int `json:"Before"`
	After           int `json:"After"`
	TokensAtCompact int `json:"TokensAtCompact"`
}

// EventSessionLoadPayload is the payload for EventSessionLoad.
type EventSessionLoadPayload struct {
	MessageCount int `json:"MessageCount"`
}

// EventAgentInitPayload is the payload for EventAgentInit.
type EventAgentInitPayload struct {
	ToolCount int `json:"ToolCount"`
}

// EventPromptBuildPayload is the payload for EventPromptBuild.
type EventPromptBuildPayload struct {
	System string `json:"System"`
}

// EventMessagePrePayload is the payload for EventMessagePre.
type EventMessagePrePayload struct {
	Content string `json:"Content"`
}

// ---------------------------------------------------------------------------
// Generic decode helper
// ---------------------------------------------------------------------------

// DecodeEvent unmarshals a raw event payload into the typed struct T.
// Returns a zeroed T and the error on failure.
//
// Example:
//
//	e.RegisterEventHandler(sdk.EventHandlerDef{
//	    Name:   "my-handler",
//	    Events: []string{"EventTurnEnd"},
//	    Handle: func(ctx context.Context, eventType string, data json.RawMessage) *sdk.Action {
//	        p, err := sdk.DecodeEvent[sdk.EventTurnEndPayload](data)
//	        if err != nil { return nil }
//	        // use p.Assistant, p.ToolResults ...
//	        return nil
//	    },
//	})
func DecodeEvent[T any](raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, err
	}
	return v, nil
}
