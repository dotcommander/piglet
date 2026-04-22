package external

import (
	"context"
	"encoding/json"
	"log/slog"
)

// handleNotification dispatches a notification (no ID) from the extension.
func (h *Host) handleNotification(msg *Message) {
	switch msg.Method {
	case MethodRegisterTool:
		var p RegisterToolParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.tools = append(h.tools, p)
		}
	case MethodRegisterCommand:
		var p RegisterCommandParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.commands = append(h.commands, p)
		}
	case MethodRegisterPromptSection:
		var p RegisterPromptSectionParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.promptSections = append(h.promptSections, p)
		}
	case MethodRegisterInterceptor:
		var p RegisterInterceptorParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.interceptors = append(h.interceptors, p)
		}
	case MethodRegisterEventHandler:
		var p RegisterEventHandlerParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.eventHandlers = append(h.eventHandlers, p)
		}
	case MethodRegisterShortcut:
		var p RegisterShortcutParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.shortcuts = append(h.shortcuts, p)
		}
	case MethodRegisterMessageHook:
		var p RegisterMessageHookParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.messageHooks = append(h.messageHooks, p)
		}
	case MethodRegisterCompactor:
		var p RegisterCompactorParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.compactor = &p
		}
	case MethodRegisterInputTransformer:
		var p RegisterInputTransformerParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.inputTransformers = append(h.inputTransformers, p)
		}
	case MethodRegisterProvider:
		var p RegisterProviderParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.providers = append(h.providers, p)
		}
	case MethodSetWidget:
		var p SetWidgetParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.SetWidget(p.Key, p.Placement, p.Content)
		}
	case MethodShowOverlay:
		var p ShowOverlayParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.ShowOverlay(p.Key, p.Title, p.Content, p.Anchor, p.Width)
		}
	case MethodCloseOverlay:
		var p CloseOverlayParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.CloseOverlay(p.Key)
		}
	case MethodNotify:
		var p NotifyParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.NotifyWithLevel(p.Message, p.Level)
		}
	case MethodShowMessage:
		var p ShowMessageParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.ShowMessage(p.Text)
		}
	case MethodSendMessage:
		var p SendMessageParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.SendMessage(p.Content)
		}
	case MethodSetInputText:
		var p SetInputTextParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.SetInputText(p.Text)
		}
	case MethodSteer:
		var p SteerParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.Steer(p.Content)
		}
	case MethodAbortWithMarker:
		var p AbortWithMarkerParams
		if json.Unmarshal(msg.Params, &p) == nil && h.app != nil {
			h.app.AbortWithMarker(p.Reason)
		}
	case MethodAbort:
		if h.app != nil {
			h.app.Abort()
		}
	case MethodLog:
		var p LogParams
		if json.Unmarshal(msg.Params, &p) == nil {
			var level slog.Level
			_ = level.UnmarshalText([]byte(p.Level))
			slog.Log(context.Background(), level, p.Message, "ext", h.manifest.Name)
		}
	case MethodProviderDelta:
		var p ProviderDeltaParams
		if json.Unmarshal(msg.Params, &p) == nil {
			h.deltaMu.Lock()
			ch, ok := h.deltaChans[p.RequestID]
			h.deltaMu.Unlock()
			if ok {
				select {
				case ch <- p:
				default:
					slog.Debug("provider delta dropped (channel full)", "ext", h.manifest.Name, "requestID", p.RequestID)
				}
			}
		}
	}
}
