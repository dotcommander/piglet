package external

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/provider"
)

// compactWireToCore deserializes wire CompactMessages back to core.Message slice.
func compactWireToCore(wire []CompactMessage) []core.Message {
	out := make([]core.Message, 0, len(wire))
	for _, cm := range wire {
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
	return out
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
	unmarshal := func(dst any) bool {
		if err := json.Unmarshal(ar.Payload, dst); err != nil {
			slog.Debug("malformed action payload", "type", ar.Type, "err", err)
			return false
		}
		return true
	}
	switch ar.Type {
	case "notify":
		var p struct{ Message string }
		if !unmarshal(&p) {
			return nil
		}
		return ext.ActionNotify{Message: p.Message}
	case "showMessage":
		var p struct{ Text string }
		if !unmarshal(&p) {
			return nil
		}
		return ext.ActionShowMessage{Text: p.Text}
	case "setSessionTitle":
		var p struct{ Title string }
		if !unmarshal(&p) {
			return nil
		}
		return ext.ActionSetSessionTitle{Title: p.Title}
	case "quit":
		return ext.ActionQuit{}
	case "setStatus":
		var p struct {
			Key  string
			Text string
		}
		if !unmarshal(&p) {
			return nil
		}
		return ext.ActionSetStatus{Key: p.Key, Text: p.Text}
	case "attachImage":
		var p struct {
			Data     string `json:"data"`
			MimeType string `json:"mimeType"`
		}
		if !unmarshal(&p) {
			return nil
		}
		return ext.ActionAttachImage{Image: &core.ImageContent{Data: p.Data, MimeType: p.MimeType}}
	case "detachImage":
		return ext.ActionDetachImage{}
	case "sendMessage":
		var p struct{ Content string }
		if !unmarshal(&p) {
			return nil
		}
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

// makeProviderResolver returns a providerResolverFn that loads config and auth at call time.
func makeProviderResolver() providerResolverFn {
	return func(model string) (core.StreamProvider, error) {
		auth, err := config.NewAuthDefault()
		if err != nil {
			return nil, fmt.Errorf("load auth: %w", err)
		}
		settings, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		modelQuery := model
		switch model {
		case "", "default":
			modelQuery = settings.ResolveDefaultModel()
		case "small":
			modelQuery = settings.ResolveSmallModel()
		}
		if modelQuery == "" {
			return nil, fmt.Errorf("no model configured")
		}
		registry := provider.NewRegistry()
		resolved, ok := registry.Resolve(modelQuery)
		if !ok {
			return nil, fmt.Errorf("unknown model: %s", modelQuery)
		}
		return registry.Create(resolved, func() string {
			return auth.GetAPIKey(resolved.Provider)
		})
	}
}
