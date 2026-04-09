package sdk

import "context"

// ---------------------------------------------------------------------------
// Registration API
// ---------------------------------------------------------------------------

func (e *Extension) RegisterTool(t ToolDef) {
	if t.Name == "" {
		panic("sdk: RegisterTool called with empty Name")
	}
	if t.Execute == nil {
		panic("sdk: RegisterTool called with nil Execute function")
	}
	e.tools[t.Name] = &t
}

func (e *Extension) RegisterCommand(c CommandDef) {
	if c.Name == "" {
		panic("sdk: RegisterCommand called with empty Name")
	}
	if c.Handler == nil {
		panic("sdk: RegisterCommand called with nil Handler function")
	}
	e.commands[c.Name] = &c
}

func (e *Extension) RegisterPromptSection(s PromptSectionDef) {
	e.promptSections = append(e.promptSections, s)
}

func (e *Extension) RegisterInterceptor(i InterceptorDef) {
	if i.Name == "" {
		panic("sdk: RegisterInterceptor called with empty Name")
	}
	e.interceptors = append(e.interceptors, i)
}

func (e *Extension) RegisterEventHandler(h EventHandlerDef) {
	if h.Name == "" {
		panic("sdk: RegisterEventHandler called with empty Name")
	}
	if h.Handle == nil {
		panic("sdk: RegisterEventHandler called with nil Handle function")
	}
	e.eventHandlers = append(e.eventHandlers, h)
}

func (e *Extension) RegisterShortcut(s ShortcutDef) {
	if s.Key == "" {
		panic("sdk: RegisterShortcut called with empty Key")
	}
	if s.Handler == nil {
		panic("sdk: RegisterShortcut called with nil Handler function")
	}
	e.shortcuts[s.Key] = &s
}

func (e *Extension) RegisterMessageHook(h MessageHookDef) {
	if h.Name == "" {
		panic("sdk: RegisterMessageHook called with empty Name")
	}
	if h.OnMessage == nil {
		panic("sdk: RegisterMessageHook called with nil OnMessage function")
	}
	e.messageHooks = append(e.messageHooks, h)
}

func (e *Extension) RegisterCompactor(c CompactorDef) {
	if c.Name == "" {
		panic("sdk: RegisterCompactor called with empty Name")
	}
	if c.Compact == nil {
		panic("sdk: RegisterCompactor called with nil Compact function")
	}
	e.compactor = &c
}

func (e *Extension) RegisterInputTransformer(t InputTransformerDef) {
	if t.Name == "" {
		panic("sdk: RegisterInputTransformer called with empty Name")
	}
	if t.Transform == nil {
		panic("sdk: RegisterInputTransformer called with nil Transform")
	}
	e.inputTransformers = append(e.inputTransformers, t)
}

// RegisterProvider declares that this extension provides LLM streaming for the given API type.
func (e *Extension) RegisterProvider(api string) {
	e.sendNotification("register/provider", map[string]string{"api": api})
}

// OnProviderStream registers a handler for provider/stream requests from the host.
func (e *Extension) OnProviderStream(handler func(ctx context.Context, x *Extension, req ProviderStreamRequest) (*ProviderStreamResponse, error)) {
	e.providerStreamHandler = handler
}

// SendProviderDelta sends a streaming delta notification to the host.
func (e *Extension) SendProviderDelta(requestID int, deltaType string, index int, delta string, tool *ProviderToolCall) {
	e.sendNotification("provider/delta", ProviderDelta{
		RequestID: requestID,
		Type:      deltaType,
		Index:     index,
		Delta:     delta,
		Tool:      tool,
	})
}

// ---------------------------------------------------------------------------
// Initialization hooks
// ---------------------------------------------------------------------------

// OnInit sets a callback that runs after the host sends initialize (CWD is available)
// but before registrations are sent. Use this for lazy initialization that needs CWD.
func (e *Extension) OnInit(fn func(e *Extension)) {
	e.onInit = fn
}

// OnInitAppend chains an additional initialization function. Unlike OnInit which
// replaces the callback, OnInitAppend appends to the existing chain. Used by
// extension packs that compose multiple logical extensions into one binary.
func (e *Extension) OnInitAppend(fn func(e *Extension)) {
	prev := e.onInit
	if prev == nil {
		e.onInit = fn
		return
	}
	e.onInit = func(e *Extension) {
		prev(e)
		fn(e)
	}
}
