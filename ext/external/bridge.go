package external

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"sync"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// LoadAll discovers and starts all external extensions, registering their
// tools, commands, and prompt sections with the given ext.App.
// Returns the number of loaded extensions and a cleanup function that stops
// all extension processes.
func LoadAll(ctx context.Context, app *ext.App) (loaded int, cleanup func(), err error) {
	extDir, err := ExtensionsDir()
	if err != nil {
		return 0, func() {}, nil // non-fatal
	}

	manifests, err := DiscoverExtensions(extDir)
	if err != nil {
		return 0, func() {}, nil // non-fatal
	}

	if len(manifests) == 0 {
		return 0, func() {}, nil
	}

	// Start all extensions concurrently (each blocks on handshake)
	type result struct {
		host *Host
		err  error
	}
	results := make([]result, len(manifests))
	var wg sync.WaitGroup
	for i, m := range manifests {
		wg.Add(1)
		go func(i int, m *Manifest) {
			defer wg.Done()
			h := NewHost(m, app.CWD())
			if err := h.Start(ctx); err != nil {
				results[i] = result{err: err}
				return
			}
			results[i] = result{host: h}
		}(i, m)
	}
	wg.Wait()

	var hosts []*Host
	for i, r := range results {
		if r.err != nil {
			slog.Warn("failed to start extension", "name", manifests[i].Name, "err", r.err)
			continue
		}
		r.host.SetApp(app)
		hosts = append(hosts, r.host)
		bridge(app, r.host)
	}

	return len(hosts), func() {
		for _, h := range hosts {
			h.Stop()
		}
	}, nil
}

// bridge wires a single host's registrations into ext.App.
func bridge(app *ext.App, h *Host) {
	tools := h.Tools()
	commands := h.Commands()

	// Record extension metadata
	info := ext.ExtInfo{
		Name:    h.Name(),
		Kind:    "external",
		Runtime: h.manifest.Runtime,
		Version: h.manifest.Version,
	}
	for _, t := range tools {
		info.Tools = append(info.Tools, t.Name)
	}
	for _, c := range commands {
		info.Commands = append(info.Commands, c.Name)
	}
	// Register tools
	for _, t := range tools {
		app.RegisterTool(&ext.ToolDef{
			ToolSchema: core.ToolSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
			PromptHint: t.PromptHint,
			Execute:    proxyToolExecute(h, t.Name),
		})
	}

	// Register commands
	for _, c := range commands {
		app.RegisterCommand(&ext.Command{
			Name:        c.Name,
			Description: c.Description,
			Handler:     proxyCommandExecute(h, c.Name),
		})
	}

	// Register prompt sections
	for _, ps := range h.PromptSections() {
		app.RegisterPromptSection(ext.PromptSection{
			Title:   ps.Title,
			Content: ps.Content,
			Order:   ps.Order,
		})
	}

	// Register interceptors
	for _, ic := range h.Interceptors() {
		app.RegisterInterceptor(ext.Interceptor{
			Name:     ic.Name,
			Priority: ic.Priority,
			Before:   proxyInterceptorBefore(h),
			After:    proxyInterceptorAfter(h),
		})
		info.Interceptors = append(info.Interceptors, ic.Name)
	}

	// Register event handlers
	for _, eh := range h.EventHandlers() {
		app.RegisterEventHandler(ext.EventHandler{
			Name:     eh.Name,
			Priority: eh.Priority,
			Filter:   proxyEventFilter(eh.Events),
			Handle:   proxyEventHandle(h),
		})
		info.EventHandlers = append(info.EventHandlers, eh.Name)
	}

	// Register shortcuts
	for _, sc := range h.Shortcuts() {
		app.RegisterShortcut(&ext.Shortcut{
			Key:         sc.Key,
			Description: sc.Description,
			Handler:     proxyShortcutHandle(h, sc.Key),
		})
		info.Shortcuts = append(info.Shortcuts, sc.Key)
	}

	// Register message hooks
	for _, mh := range h.MessageHooks() {
		app.RegisterMessageHook(ext.MessageHook{
			Name:      mh.Name,
			Priority:  mh.Priority,
			OnMessage: proxyMessageHook(h),
		})
		info.MessageHooks = append(info.MessageHooks, mh.Name)
	}

	// Register compactor
	if cp := h.Compactor(); cp != nil {
		app.RegisterCompactor(ext.Compactor{
			Name:      cp.Name,
			Threshold: cp.Threshold,
			Compact:   proxyCompactExecute(h),
		})
		info.Compactor = cp.Name
	}

	// Register extension metadata after all fields are populated
	app.RegisterExtInfo(info)
}

// proxyToolExecute returns a ToolExecuteFn that proxies to the extension process.
func proxyToolExecute(h *Host, toolName string) core.ToolExecuteFn {
	return func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
		result, err := h.ExecuteTool(ctx, id, toolName, args)
		if err != nil {
			return nil, fmt.Errorf("ext %s tool %s: %w", h.Name(), toolName, err)
		}

		return &core.ToolResult{Content: wireToCore(result.Content)}, nil
	}
}

// proxyCommandExecute returns a command handler that proxies to the extension.
func proxyCommandExecute(h *Host, cmdName string) func(args string, app *ext.App) error {
	return func(args string, app *ext.App) error {
		return h.ExecuteCommand(context.TODO(), cmdName, args)
	}
}

// proxyInterceptorBefore returns a Before function that proxies to the extension.
func proxyInterceptorBefore(h *Host) func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
	return func(ctx context.Context, toolName string, args map[string]any) (bool, map[string]any, error) {
		return h.InterceptBefore(ctx, toolName, args)
	}
}

// proxyInterceptorAfter returns an After function that proxies to the extension.
func proxyInterceptorAfter(h *Host) func(ctx context.Context, toolName string, details any) (any, error) {
	return func(ctx context.Context, toolName string, details any) (any, error) {
		return h.InterceptAfter(ctx, toolName, details)
	}
}

// proxyEventFilter returns a Filter function that checks event type names.
// nil events slice means accept all events.
func proxyEventFilter(events []string) func(core.Event) bool {
	if len(events) == 0 {
		return nil // nil = accept all
	}
	return func(evt core.Event) bool {
		typeName := eventTypeName(evt)
		return slices.Contains(events, typeName)
	}
}

// proxyEventHandle returns a Handle function that dispatches events to the extension.
// Wraps in ActionRunAsync since extension calls may be slow (e.g. LLM calls for autotitle).
func proxyEventHandle(h *Host) func(ctx context.Context, evt core.Event) ext.Action {
	return func(ctx context.Context, evt core.Event) ext.Action {
		typeName := eventTypeName(evt)
		data, _ := json.Marshal(evt)

		return ext.ActionRunAsync{Fn: func() ext.Action {
			ar, err := h.DispatchEvent(ctx, typeName, data)
			if err != nil {
				slog.Debug("event dispatch error", "ext", h.Name(), "err", err)
				return nil
			}
			return actionResultToAction(ar)
		}}
	}
}

// proxyShortcutHandle returns a Handler function that proxies to the extension.
func proxyShortcutHandle(h *Host, key string) func(app *ext.App) (ext.Action, error) {
	return func(app *ext.App) (ext.Action, error) {
		ar, err := h.HandleShortcut(context.TODO(), key)
		if err != nil {
			return nil, fmt.Errorf("ext %s shortcut %s: %w", h.Name(), key, err)
		}
		return actionResultToAction(ar), nil
	}
}

// proxyMessageHook returns an OnMessage function that proxies to the extension.
func proxyMessageHook(h *Host) func(ctx context.Context, msg string) (string, error) {
	return func(ctx context.Context, msg string) (string, error) {
		return h.OnMessage(ctx, msg)
	}
}

// proxyCompactExecute returns a Compact function that proxies to the extension.
func proxyCompactExecute(h *Host) func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
	return func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		// Serialize messages with type discriminator
		wire := make([]CompactMessage, 0, len(msgs))
		for _, m := range msgs {
			var msgType string
			switch m.(type) {
			case *core.UserMessage:
				msgType = "user"
			case *core.AssistantMessage:
				msgType = "assistant"
			case *core.ToolResultMessage:
				msgType = "tool_result"
			default:
				continue
			}
			data, err := json.Marshal(m)
			if err != nil {
				continue
			}
			wire = append(wire, CompactMessage{Type: msgType, Data: data})
		}

		result, err := h.ExecuteCompact(ctx, wire)
		if err != nil {
			return nil, err
		}

		// Deserialize back to core.Message
		out := make([]core.Message, 0, len(result))
		for _, cm := range result {
			switch cm.Type {
			case "user":
				var msg core.UserMessage
				if json.Unmarshal(cm.Data, &msg) == nil {
					out = append(out, &msg)
				}
			case "assistant":
				var msg core.AssistantMessage
				if json.Unmarshal(cm.Data, &msg) == nil {
					out = append(out, &msg)
				}
			case "tool_result":
				var msg core.ToolResultMessage
				if json.Unmarshal(cm.Data, &msg) == nil {
					out = append(out, &msg)
				}
			}
		}
		return out, nil
	}
}

// eventTypeName returns the Go type name of a core.Event (e.g. "EventAgentEnd").
func eventTypeName(evt core.Event) string {
	t := reflect.TypeOf(evt)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// actionResultToAction converts a wire ActionResult to an ext.Action.
// Returns nil if the ActionResult is nil or has an unknown type.
func actionResultToAction(ar *ActionResult) ext.Action {
	if ar == nil {
		return nil
	}
	switch ar.Type {
	case "notify":
		var p struct{ Message string }
		_ = json.Unmarshal(ar.Payload, &p)
		return ext.ActionNotify{Message: p.Message}
	case "showMessage":
		var p struct{ Text string }
		_ = json.Unmarshal(ar.Payload, &p)
		return ext.ActionShowMessage{Text: p.Text}
	case "setSessionTitle":
		var p struct{ Title string }
		_ = json.Unmarshal(ar.Payload, &p)
		return ext.ActionSetSessionTitle{Title: p.Title}
	case "quit":
		return ext.ActionQuit{}
	case "setStatus":
		var p struct {
			Key  string
			Text string
		}
		_ = json.Unmarshal(ar.Payload, &p)
		return ext.ActionSetStatus{Key: p.Key, Text: p.Text}
	case "attachImage":
		var p struct {
			Data     string `json:"data"`
			MimeType string `json:"mimeType"`
		}
		_ = json.Unmarshal(ar.Payload, &p)
		return ext.ActionAttachImage{Image: &core.ImageContent{Data: p.Data, MimeType: p.MimeType}}
	case "detachImage":
		return ext.ActionDetachImage{}
	case "sendMessage":
		var p struct{ Content string }
		_ = json.Unmarshal(ar.Payload, &p)
		return ext.ActionSendMessage{Content: p.Content}
	default:
		slog.Debug("unknown action type from extension", "type", ar.Type)
		return nil
	}
}

// wireToCore converts wire ContentBlocks to core.ContentBlocks.
func wireToCore(blocks []ContentBlock) []core.ContentBlock {
	out := make([]core.ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "image":
			out = append(out, core.ImageContent{Data: b.Data, MimeType: b.Mime})
		default:
			out = append(out, core.TextContent{Text: b.Text})
		}
	}
	return out
}

// coreToWire converts core.ContentBlocks to wire ContentBlocks.
func coreToWire(blocks []core.ContentBlock) []ContentBlock {
	out := make([]ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		switch cb := b.(type) {
		case core.ImageContent:
			out = append(out, ContentBlock{Type: "image", Data: cb.Data, Mime: cb.MimeType})
		case core.TextContent:
			out = append(out, ContentBlock{Type: "text", Text: cb.Text})
		default:
			out = append(out, ContentBlock{Type: "text", Text: fmt.Sprintf("%v", cb)})
		}
	}
	return out
}
