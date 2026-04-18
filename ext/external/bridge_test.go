package external

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// actionResultToAction
// ---------------------------------------------------------------------------

func TestActionResultToActionNotify(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(struct{ Message string }{"hello from ext"})
	ar := &ActionResult{Type: "notify", Payload: payload}

	action := actionResultToAction(ar)
	require.NotNil(t, action)

	got, ok := action.(ext.ActionNotify)
	require.True(t, ok)
	assert.Equal(t, "hello from ext", got.Message)
}

func TestActionResultToActionShowMessage(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(struct{ Text string }{"displayed text"})
	ar := &ActionResult{Type: "showMessage", Payload: payload}

	action := actionResultToAction(ar)
	require.NotNil(t, action)

	got, ok := action.(ext.ActionShowMessage)
	require.True(t, ok)
	assert.Equal(t, "displayed text", got.Text)
}

func TestActionResultToActionSetSessionTitle(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(struct{ Title string }{"My Session"})
	ar := &ActionResult{Type: "setSessionTitle", Payload: payload}

	action := actionResultToAction(ar)
	require.NotNil(t, action)

	got, ok := action.(ext.ActionSetSessionTitle)
	require.True(t, ok)
	assert.Equal(t, "My Session", got.Title)
}

func TestActionResultToActionQuit(t *testing.T) {
	t.Parallel()

	ar := &ActionResult{Type: "quit"}
	action := actionResultToAction(ar)
	require.NotNil(t, action)

	_, ok := action.(ext.ActionQuit)
	assert.True(t, ok)
}

func TestActionResultToActionSetStatus(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(struct {
		Key  string
		Text string
	}{"model", "gpt-4o"})
	ar := &ActionResult{Type: "setStatus", Payload: payload}

	action := actionResultToAction(ar)
	require.NotNil(t, action)

	got, ok := action.(ext.ActionSetStatus)
	require.True(t, ok)
	assert.Equal(t, "model", got.Key)
	assert.Equal(t, "gpt-4o", got.Text)
}

func TestActionResultToActionAttachImage(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(struct {
		Data     string `json:"data"`
		MimeType string `json:"mimeType"`
	}{"base64data==", "image/png"})
	ar := &ActionResult{Type: "attachImage", Payload: payload}

	action := actionResultToAction(ar)
	require.NotNil(t, action)

	got, ok := action.(ext.ActionAttachImage)
	require.True(t, ok)
	img, ok := got.Image.(*core.ImageContent)
	require.True(t, ok)
	assert.Equal(t, "base64data==", img.Data)
	assert.Equal(t, "image/png", img.MimeType)
}

func TestActionResultToActionDetachImage(t *testing.T) {
	t.Parallel()

	ar := &ActionResult{Type: "detachImage"}
	action := actionResultToAction(ar)
	require.NotNil(t, action)

	_, ok := action.(ext.ActionDetachImage)
	assert.True(t, ok)
}

func TestActionResultToActionUnknownType(t *testing.T) {
	t.Parallel()

	ar := &ActionResult{Type: "unknownAction"}
	action := actionResultToAction(ar)
	assert.Nil(t, action)
}

func TestActionResultToActionNil(t *testing.T) {
	t.Parallel()

	action := actionResultToAction(nil)
	assert.Nil(t, action)
}

// ---------------------------------------------------------------------------
// wireToCore
// ---------------------------------------------------------------------------

func TestWireToCoreText(t *testing.T) {
	t.Parallel()

	blocks := []ContentBlock{
		{Type: "text", Text: "hello world"},
	}

	result := wireToCore(blocks)
	require.Len(t, result, 1)

	tc, ok := result[0].(core.TextContent)
	require.True(t, ok)
	assert.Equal(t, "hello world", tc.Text)
}

func TestWireToCoreImage(t *testing.T) {
	t.Parallel()

	blocks := []ContentBlock{
		{Type: "image", Data: "abc123", Mime: "image/jpeg"},
	}

	result := wireToCore(blocks)
	require.Len(t, result, 1)

	ic, ok := result[0].(core.ImageContent)
	require.True(t, ok)
	assert.Equal(t, "abc123", ic.Data)
	assert.Equal(t, "image/jpeg", ic.MimeType)
}

func TestWireToCoreMixed(t *testing.T) {
	t.Parallel()

	blocks := []ContentBlock{
		{Type: "text", Text: "first"},
		{Type: "image", Data: "imgdata", Mime: "image/png"},
		{Type: "text", Text: "second"},
	}

	result := wireToCore(blocks)
	require.Len(t, result, 3)

	tc1, ok := result[0].(core.TextContent)
	require.True(t, ok)
	assert.Equal(t, "first", tc1.Text)

	ic, ok := result[1].(core.ImageContent)
	require.True(t, ok)
	assert.Equal(t, "imgdata", ic.Data)

	tc2, ok := result[2].(core.TextContent)
	require.True(t, ok)
	assert.Equal(t, "second", tc2.Text)
}

func TestWireToCoreEmpty(t *testing.T) {
	t.Parallel()

	result := wireToCore(nil)
	assert.Empty(t, result)
}

func TestWireToCoreUnknownTypeDefaultsToText(t *testing.T) {
	t.Parallel()

	blocks := []ContentBlock{
		{Type: "video", Text: "fallback text"},
	}

	result := wireToCore(blocks)
	require.Len(t, result, 1)

	_, ok := result[0].(core.TextContent)
	assert.True(t, ok, "unknown type should produce TextContent")
}

// ---------------------------------------------------------------------------
// coreToWire
// ---------------------------------------------------------------------------

func TestCoreToWireText(t *testing.T) {
	t.Parallel()

	blocks := []core.ContentBlock{
		core.TextContent{Text: "hello"},
	}

	result := coreToWire(blocks)
	require.Len(t, result, 1)
	assert.Equal(t, "text", result[0].Type)
	assert.Equal(t, "hello", result[0].Text)
}

func TestCoreToWireImage(t *testing.T) {
	t.Parallel()

	blocks := []core.ContentBlock{
		core.ImageContent{Data: "imgdata==", MimeType: "image/png"},
	}

	result := coreToWire(blocks)
	require.Len(t, result, 1)
	assert.Equal(t, "image", result[0].Type)
	assert.Equal(t, "imgdata==", result[0].Data)
	assert.Equal(t, "image/png", result[0].Mime)
}

func TestCoreToWireMixed(t *testing.T) {
	t.Parallel()

	blocks := []core.ContentBlock{
		core.TextContent{Text: "alpha"},
		core.ImageContent{Data: "beta", MimeType: "image/gif"},
	}

	result := coreToWire(blocks)
	require.Len(t, result, 2)
	assert.Equal(t, "text", result[0].Type)
	assert.Equal(t, "alpha", result[0].Text)
	assert.Equal(t, "image", result[1].Type)
	assert.Equal(t, "beta", result[1].Data)
}

func TestCoreToWireEmpty(t *testing.T) {
	t.Parallel()

	result := coreToWire(nil)
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// eventTypeName
// ---------------------------------------------------------------------------

func TestEventTypeNameValue(t *testing.T) {
	t.Parallel()

	evt := core.EventAgentEnd{}
	assert.Equal(t, "EventAgentEnd", eventTypeName(evt))
}

func TestEventTypeNamePointer(t *testing.T) {
	t.Parallel()

	// EventTurnEnd is a struct; wrap behind an interface via pointer
	evt := &core.EventTurnEnd{}
	assert.Equal(t, "EventTurnEnd", eventTypeName(evt))
}

// ---------------------------------------------------------------------------
// proxyEventFilter
// ---------------------------------------------------------------------------

func TestProxyEventFilterNilEventsAcceptsAll(t *testing.T) {
	t.Parallel()

	filter := proxyEventFilter(nil)
	assert.Nil(t, filter, "nil events should return nil filter (accept all)")
}

func TestProxyEventFilterEmptyEventsAcceptsAll(t *testing.T) {
	t.Parallel()

	filter := proxyEventFilter([]string{})
	assert.Nil(t, filter, "empty events should return nil filter (accept all)")
}

func TestProxyEventFilterMatchesEventType(t *testing.T) {
	t.Parallel()

	filter := proxyEventFilter([]string{"EventAgentEnd", "EventTurnEnd"})
	require.NotNil(t, filter)

	assert.True(t, filter(core.EventAgentEnd{}))
	assert.True(t, filter(core.EventTurnEnd{}))
}

func TestProxyEventFilterRejectsOtherEventTypes(t *testing.T) {
	t.Parallel()

	filter := proxyEventFilter([]string{"EventAgentEnd"})
	require.NotNil(t, filter)

	assert.False(t, filter(core.EventTurnEnd{}))
	assert.False(t, filter(core.EventToolStart{}))
}

// ---------------------------------------------------------------------------
// Protocol JSON round-trips for all registration param types
// ---------------------------------------------------------------------------

func TestRegisterToolParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := RegisterToolParams{
		Name:        "my_tool",
		Description: "Does things",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		PromptHint:  "call this when you need X",
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded RegisterToolParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Description, decoded.Description)
	assert.Equal(t, orig.PromptHint, decoded.PromptHint)
}

func TestRegisterCommandParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := RegisterCommandParams{
		Name:        "my_cmd",
		Description: "A command",
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded RegisterCommandParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Description, decoded.Description)
}

func TestRegisterPromptSectionParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := RegisterPromptSectionParams{
		Title:   "Context",
		Content: "some injected context",
		Order:   42,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded RegisterPromptSectionParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Title, decoded.Title)
	assert.Equal(t, orig.Content, decoded.Content)
	assert.Equal(t, orig.Order, decoded.Order)
}

func TestRegisterInterceptorParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := RegisterInterceptorParams{
		Name:     "safeguard",
		Priority: 2000,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded RegisterInterceptorParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Priority, decoded.Priority)
}

func TestRegisterEventHandlerParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := RegisterEventHandlerParams{
		Name:     "autotitle",
		Priority: 10,
		Events:   []string{"EventAgentEnd"},
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded RegisterEventHandlerParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Priority, decoded.Priority)
	assert.Equal(t, orig.Events, decoded.Events)
}

func TestRegisterShortcutParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := RegisterShortcutParams{
		Key:         "ctrl+v",
		Description: "Paste image",
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded RegisterShortcutParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Key, decoded.Key)
	assert.Equal(t, orig.Description, decoded.Description)
}

func TestRegisterMessageHookParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := RegisterMessageHookParams{
		Name:     "skill-hook",
		Priority: 500,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded RegisterMessageHookParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Priority, decoded.Priority)
}

func TestRegisterCompactorParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := RegisterCompactorParams{
		Name:      "memory-compact",
		Threshold: 100,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded RegisterCompactorParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Threshold, decoded.Threshold)
}

// ---------------------------------------------------------------------------
// Execution param/result round-trips
// ---------------------------------------------------------------------------

func TestToolExecuteParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := ToolExecuteParams{
		CallID: "call-99",
		Name:   "bash",
		Args:   map[string]any{"command": "echo hi"},
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded ToolExecuteParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.CallID, decoded.CallID)
	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, "echo hi", decoded.Args["command"])
}

func TestToolExecuteResultRoundTrip(t *testing.T) {
	t.Parallel()

	orig := ToolExecuteResult{
		Content: []ContentBlock{
			{Type: "text", Text: "output"},
		},
		IsError: false,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded ToolExecuteResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Len(t, decoded.Content, 1)
	assert.Equal(t, "output", decoded.Content[0].Text)
	assert.False(t, decoded.IsError)
}

func TestToolExecuteResultIsErrorRoundTrip(t *testing.T) {
	t.Parallel()

	orig := ToolExecuteResult{
		Content: []ContentBlock{{Type: "text", Text: "something failed"}},
		IsError: true,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded ToolExecuteResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.True(t, decoded.IsError)
}

func TestCommandExecuteParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := CommandExecuteParams{
		Name: "status",
		Args: "--verbose",
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded CommandExecuteParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Args, decoded.Args)
}

func TestInterceptorBeforeParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := InterceptorBeforeParams{
		ToolName: "bash",
		Args:     map[string]any{"command": "rm -rf /"},
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded InterceptorBeforeParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.ToolName, decoded.ToolName)
	assert.Equal(t, "rm -rf /", decoded.Args["command"])
}

func TestInterceptorBeforeResultAllowWithModifiedArgs(t *testing.T) {
	t.Parallel()

	orig := InterceptorBeforeResult{
		Allow: true,
		Args:  map[string]any{"command": "echo safe"},
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded InterceptorBeforeResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.True(t, decoded.Allow)
	assert.Equal(t, "echo safe", decoded.Args["command"])
}

func TestInterceptorBeforeResultDeny(t *testing.T) {
	t.Parallel()

	orig := InterceptorBeforeResult{Allow: false}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded InterceptorBeforeResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.False(t, decoded.Allow)
}

func TestEventDispatchParamsRoundTrip(t *testing.T) {
	t.Parallel()

	payload, _ := json.Marshal(map[string]string{"turns": "3"})
	orig := EventDispatchParams{
		Type: "EventAgentEnd",
		Data: payload,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded EventDispatchParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Type, decoded.Type)
	assert.NotEmpty(t, decoded.Data)
}

func TestShortcutHandleParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := ShortcutHandleParams{Key: "ctrl+g"}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded ShortcutHandleParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Key, decoded.Key)
}

func TestMessageHookParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := MessageHookParams{Message: "user said something"}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded MessageHookParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Message, decoded.Message)
}

func TestMessageHookResultRoundTrip(t *testing.T) {
	t.Parallel()

	orig := MessageHookResult{Injection: "extra context here"}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded MessageHookResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Injection, decoded.Injection)
}

func TestMessageHookResultEmptyInjectionOmitted(t *testing.T) {
	t.Parallel()

	orig := MessageHookResult{Injection: ""}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	// Empty injection should be omitempty — field absent from JSON
	assert.NotContains(t, string(data), "injection")
}

func TestCompactMessageRoundTrip(t *testing.T) {
	t.Parallel()

	inner, _ := json.Marshal(map[string]string{"content": "hello"})
	orig := CompactMessage{
		Type: "user",
		Data: inner,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded CompactMessage
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Type, decoded.Type)
	assert.Equal(t, []byte(orig.Data), []byte(decoded.Data))
}

func TestHostListToolsParamFilterConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		filter string
		want   string
	}{
		{FilterAll, "all"},
		{FilterBackgroundSafe, "background_safe"},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			t.Parallel()

			p := HostListToolsParams{Filter: tt.filter}
			data, err := json.Marshal(p)
			require.NoError(t, err)

			var decoded HostListToolsParams
			require.NoError(t, json.Unmarshal(data, &decoded))

			assert.Equal(t, tt.want, decoded.Filter)
		})
	}
}

func TestHostChatParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := HostChatParams{
		System: "You are helpful.",
		Messages: []ChatMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
		Model:     "small",
		MaxTokens: 1024,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded HostChatParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.System, decoded.System)
	assert.Equal(t, orig.Model, decoded.Model)
	assert.Equal(t, orig.MaxTokens, decoded.MaxTokens)
	require.Len(t, decoded.Messages, 2)
	assert.Equal(t, "user", decoded.Messages[0].Role)
	assert.Equal(t, "hello", decoded.Messages[0].Content)
}

func TestHostChatResultRoundTrip(t *testing.T) {
	t.Parallel()

	orig := HostChatResult{
		Text:  "The answer is 42.",
		Usage: HostTokenUsage{Input: 10, Output: 5},
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded HostChatResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Text, decoded.Text)
	assert.Equal(t, 10, decoded.Usage.Input)
	assert.Equal(t, 5, decoded.Usage.Output)
}

func TestHostAgentRunParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := HostAgentRunParams{
		System:   "Be concise.",
		Task:     "List the files in /tmp",
		Tools:    "background_safe",
		Model:    "default",
		MaxTurns: 5,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded HostAgentRunParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.System, decoded.System)
	assert.Equal(t, orig.Task, decoded.Task)
	assert.Equal(t, orig.Tools, decoded.Tools)
	assert.Equal(t, orig.MaxTurns, decoded.MaxTurns)
}

func TestCancelParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := CancelParams{ID: 7}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded CancelParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.ID, decoded.ID)
}

func TestInitializeParamsRoundTrip(t *testing.T) {
	t.Parallel()

	orig := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		CWD:             "/home/user/project",
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded InitializeParams
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, ProtocolVersion, decoded.ProtocolVersion)
	assert.Equal(t, orig.CWD, decoded.CWD)
}

func TestInitializeResultRoundTrip(t *testing.T) {
	t.Parallel()

	orig := InitializeResult{
		Name:    "my-ext",
		Version: "2.3.1",
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded InitializeResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, orig.Name, decoded.Name)
	assert.Equal(t, orig.Version, decoded.Version)
}

// ---------------------------------------------------------------------------
// Method name constants — verify they match expected wire strings
// ---------------------------------------------------------------------------

func TestMethodNameConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"initialize", MethodInitialize, "initialize"},
		{"shutdown", MethodShutdown, "shutdown"},
		{"cancel", MethodCancelRequest, "$/cancelRequest"},
		{"registerTool", MethodRegisterTool, "register/tool"},
		{"registerCommand", MethodRegisterCommand, "register/command"},
		{"registerPromptSection", MethodRegisterPromptSection, "register/promptSection"},
		{"registerInterceptor", MethodRegisterInterceptor, "register/interceptor"},
		{"registerEventHandler", MethodRegisterEventHandler, "register/eventHandler"},
		{"registerShortcut", MethodRegisterShortcut, "register/shortcut"},
		{"registerMessageHook", MethodRegisterMessageHook, "register/messageHook"},
		{"registerCompactor", MethodRegisterCompactor, "register/compactor"},
		{"toolExecute", MethodToolExecute, "tool/execute"},
		{"commandExecute", MethodCommandExecute, "command/execute"},
		{"interceptorBefore", MethodInterceptorBefore, "interceptor/before"},
		{"interceptorAfter", MethodInterceptorAfter, "interceptor/after"},
		{"eventDispatch", MethodEventDispatch, "event/dispatch"},
		{"shortcutHandle", MethodShortcutHandle, "shortcut/handle"},
		{"messageHookOnMessage", MethodMessageHookOnMessage, "messageHook/onMessage"},
		{"compactExecute", MethodCompactExecute, "compact/execute"},
		{"hostListTools", MethodHostListTools, "host/listTools"},
		{"hostExecuteTool", MethodHostExecuteTool, "host/executeTool"},
		{"hostConfigGet", MethodHostConfigGet, "host/config.get"},
		{"hostConfigReadExt", MethodHostConfigReadExt, "host/config.readExtension"},
		{"hostAuthGetKey", MethodHostAuthGetKey, "host/auth.getKey"},
		{"hostChat", MethodHostChat, "host/chat"},
		{"hostAgentRun", MethodHostAgentRun, "host/agent.run"},
		{"hostSessionEntryInfos", MethodHostSessionEntryInfos, "host/sessionEntryInfos"},
		{"hostSessionFullTree", MethodHostSessionFullTree, "host/sessionFullTree"},
		{"hostSessionTitle", MethodHostSessionTitle, "host/sessionTitle"},
		{"hostShowPicker", MethodHostShowPicker, "host/showPicker"},
		{"notify", MethodNotify, "notify"},
		{"log", MethodLog, "log"},
		{"showMessage", MethodShowMessage, "showMessage"},
		{"sendMessage", MethodSendMessage, "sendMessage"},
		{"steer", MethodSteer, "steer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}

// ---------------------------------------------------------------------------
// handleMessage dispatch — test registration collection via handleMessage
// ---------------------------------------------------------------------------

func newTestHost() *Host {
	m := &Manifest{Name: "test", Runtime: "bun", Entry: "index.ts", Dir: "/tmp"}
	return NewHost(m, "/tmp")
}

func mustMarshalParams(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func TestHandleMessageRegisterTool(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterTool,
		Params:  mustMarshalParams(RegisterToolParams{Name: "tool1", Description: "desc"}),
	}
	h.handleMessage(msg)

	require.Len(t, h.tools, 1)
	assert.Equal(t, "tool1", h.tools[0].Name)
}

func TestHandleMessageRegisterCommand(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterCommand,
		Params:  mustMarshalParams(RegisterCommandParams{Name: "cmd1", Description: "a cmd"}),
	}
	h.handleMessage(msg)

	require.Len(t, h.commands, 1)
	assert.Equal(t, "cmd1", h.commands[0].Name)
}

func TestHandleMessageRegisterPromptSection(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterPromptSection,
		Params:  mustMarshalParams(RegisterPromptSectionParams{Title: "T", Content: "C", Order: 10}),
	}
	h.handleMessage(msg)

	require.Len(t, h.promptSections, 1)
	assert.Equal(t, "T", h.promptSections[0].Title)
	assert.Equal(t, 10, h.promptSections[0].Order)
}

func TestHandleMessageRegisterInterceptor(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterInterceptor,
		Params:  mustMarshalParams(RegisterInterceptorParams{Name: "guard", Priority: 1000}),
	}
	h.handleMessage(msg)

	require.Len(t, h.interceptors, 1)
	assert.Equal(t, "guard", h.interceptors[0].Name)
	assert.Equal(t, 1000, h.interceptors[0].Priority)
}

func TestHandleMessageRegisterEventHandler(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterEventHandler,
		Params:  mustMarshalParams(RegisterEventHandlerParams{Name: "titler", Events: []string{"EventAgentEnd"}}),
	}
	h.handleMessage(msg)

	require.Len(t, h.eventHandlers, 1)
	assert.Equal(t, "titler", h.eventHandlers[0].Name)
	assert.Equal(t, []string{"EventAgentEnd"}, h.eventHandlers[0].Events)
}

func TestHandleMessageRegisterShortcut(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterShortcut,
		Params:  mustMarshalParams(RegisterShortcutParams{Key: "ctrl+v", Description: "paste"}),
	}
	h.handleMessage(msg)

	require.Len(t, h.shortcuts, 1)
	assert.Equal(t, "ctrl+v", h.shortcuts[0].Key)
}

func TestHandleMessageRegisterMessageHook(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterMessageHook,
		Params:  mustMarshalParams(RegisterMessageHookParams{Name: "hook1", Priority: 500}),
	}
	h.handleMessage(msg)

	require.Len(t, h.messageHooks, 1)
	assert.Equal(t, "hook1", h.messageHooks[0].Name)
}

func TestHandleMessageRegisterCompactor(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterCompactor,
		Params:  mustMarshalParams(RegisterCompactorParams{Name: "compact1", Threshold: 80}),
	}
	h.handleMessage(msg)

	require.NotNil(t, h.compactor)
	assert.Equal(t, "compact1", h.compactor.Name)
	assert.Equal(t, 80, h.compactor.Threshold)
}

func TestHandleMessageResponseRoutedToPending(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	id := 42
	ch := make(chan *Message, 1)

	h.pendingMu.Lock()
	h.pending[id] = ch
	h.pendingMu.Unlock()

	result, _ := json.Marshal(ToolExecuteResult{
		Content: []ContentBlock{{Type: "text", Text: "done"}},
	})
	msg := &Message{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  result,
	}
	h.handleMessage(msg)

	select {
	case resp := <-ch:
		require.NotNil(t, resp)
		assert.Equal(t, &id, resp.ID)
	default:
		t.Fatal("expected response to be routed to pending channel")
	}

	// Pending entry should be removed
	h.pendingMu.Lock()
	_, stillPending := h.pending[id]
	h.pendingMu.Unlock()
	assert.False(t, stillPending)
}

func TestHandleMessageUnknownNotificationNocrash(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  "unknown/method",
		Params:  json.RawMessage(`{}`),
	}
	// Should not panic
	assert.NotPanics(t, func() { h.handleMessage(msg) })
}

func TestHandleMessageBadJSONParamsSkipped(t *testing.T) {
	t.Parallel()

	h := newTestHost()
	msg := &Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterTool,
		Params:  json.RawMessage(`not-valid-json`),
	}
	// Bad JSON — tool should NOT be registered, no panic
	assert.NotPanics(t, func() { h.handleMessage(msg) })
	assert.Empty(t, h.tools)
}

func TestHostSessionEntryInfosResultRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostSessionEntryInfosResult{
		Entries: []WireEntryInfo{
			{ID: "e1", ParentID: "", Type: "user", Timestamp: "2026-04-18T16:00:00Z", Children: 2},
			{ID: "e2", ParentID: "e1", Type: "assistant", Timestamp: "2026-04-18T16:00:05Z", Children: 0},
		},
	}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostSessionEntryInfosResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Entries, 2)
	assert.Equal(t, "e1", decoded.Entries[0].ID)
	assert.Equal(t, "e1", decoded.Entries[1].ParentID)
	assert.Equal(t, 2, decoded.Entries[0].Children)
}

func TestHostSessionFullTreeResultRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostSessionFullTreeResult{
		Nodes: []WireTreeNode{
			{ID: "n1", Type: "user", Timestamp: "2026-04-18T16:00:00Z", Depth: 0, OnActivePath: true, Label: "start"},
		},
	}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostSessionFullTreeResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Nodes, 1)
	assert.True(t, decoded.Nodes[0].OnActivePath)
	assert.Equal(t, "start", decoded.Nodes[0].Label)
}

func TestHostSessionTitleResultRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostSessionTitleResult{Title: "my session"}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostSessionTitleResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "my session", decoded.Title)
}

func TestHostShowPickerParamsRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostShowPickerParams{
		Title: "Pick one",
		Items: []WirePickerItem{
			{ID: "a", Label: "Alpha", Desc: "first"},
			{ID: "b", Label: "Beta"},
		},
	}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostShowPickerParams
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "Pick one", decoded.Title)
	require.Len(t, decoded.Items, 2)
	assert.Equal(t, "a", decoded.Items[0].ID)
	assert.Equal(t, "first", decoded.Items[0].Desc)
	assert.Empty(t, decoded.Items[1].Desc)
}

func TestHostShowPickerResultRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostShowPickerResult{Selected: "a"}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostShowPickerResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "a", decoded.Selected)
}

func TestNewSessionMethodsUseHostPrefix(t *testing.T) {
	t.Parallel()
	methods := []string{
		MethodHostSessionEntryInfos,
		MethodHostSessionFullTree,
		MethodHostSessionTitle,
		MethodHostShowPicker,
	}
	for _, m := range methods {
		assert.NotEmpty(t, m)
		assert.True(t, strings.HasPrefix(m, "host/"), "method %q must use host/ prefix", m)
	}
}

func TestHostAvailableModelsResultRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostAvailableModelsResult{
		Models: []WireModelInfo{
			{
				ID:            "claude-opus-4-5",
				API:           "anthropic",
				DisplayName:   "anthropic/Claude Opus 4.5",
				Provider:      "anthropic",
				ContextWindow: 200000,
				MaxTokens:     32000,
				Reasoning:     true,
				CostInput:     15.0,
				CostOutput:    75.0,
				Current:       true,
			},
			{
				ID:          "claude-3-5-haiku",
				API:         "anthropic",
				DisplayName: "anthropic/claude-3-5-haiku",
				Provider:    "anthropic",
				Current:     false,
			},
		},
	}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostAvailableModelsResult
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Models, 2)
	assert.Equal(t, "claude-opus-4-5", decoded.Models[0].ID)
	assert.Equal(t, "anthropic", decoded.Models[0].API)
	assert.Equal(t, "anthropic/Claude Opus 4.5", decoded.Models[0].DisplayName)
	assert.True(t, decoded.Models[0].Reasoning)
	assert.True(t, decoded.Models[0].Current)
	assert.Equal(t, 200000, decoded.Models[0].ContextWindow)
	assert.Equal(t, 75.0, decoded.Models[0].CostOutput)
	assert.False(t, decoded.Models[1].Current)
}

func TestHostSwitchModelParamsRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostSwitchModelParams{
		ModelID:        "anthropic/claude-opus-4-5",
		PersistDefault: true,
	}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostSwitchModelParams
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "anthropic/claude-opus-4-5", decoded.ModelID)
	assert.True(t, decoded.PersistDefault)
}

func TestHostSwitchModelParamsNoPersistRoundTrip(t *testing.T) {
	t.Parallel()
	orig := HostSwitchModelParams{ModelID: "anthropic/claude-3-5-haiku"}
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var decoded HostSwitchModelParams
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "anthropic/claude-3-5-haiku", decoded.ModelID)
	assert.False(t, decoded.PersistDefault)
}

func TestNewModelMethodsUseHostPrefix(t *testing.T) {
	t.Parallel()
	methods := []string{
		MethodHostAvailableModels,
		MethodHostSwitchModel,
	}
	for _, m := range methods {
		assert.NotEmpty(t, m)
		assert.True(t, strings.HasPrefix(m, "host/"), "method %q must use host/ prefix", m)
	}
}

// ---------------------------------------------------------------------------
// logWriter
// ---------------------------------------------------------------------------

func TestLogWriterWriteReturnsLen(t *testing.T) {
	t.Parallel()

	w := &logWriter{name: "test-ext"}
	msg := []byte("some log line\n")
	n, err := w.Write(msg)

	assert.NoError(t, err)
	assert.Equal(t, len(msg), n)
}

// ---------------------------------------------------------------------------
// NewHost state
// ---------------------------------------------------------------------------

func TestNewHostInitialState(t *testing.T) {
	t.Parallel()

	m := &Manifest{Name: "ext-x", Runtime: "bun", Entry: "index.ts", Dir: "/ext"}
	h := NewHost(m, "/workspace")

	assert.Equal(t, "ext-x", h.Name())
	assert.Empty(t, h.Tools())
	assert.Empty(t, h.Commands())
	assert.Empty(t, h.PromptSections())
	assert.Empty(t, h.Interceptors())
	assert.Empty(t, h.EventHandlers())
	assert.Empty(t, h.Shortcuts())
	assert.Empty(t, h.MessageHooks())
	assert.Nil(t, h.Compactor())
}
