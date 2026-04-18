package external

import (
	"encoding/json"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

func (h *Host) handleHostExtInfos(msg *Message) {
	if h.app == nil {
		h.respond(*msg.ID, HostExtInfosResult{})
		return
	}
	infos := h.app.ExtInfos()
	wires := make([]WireExtInfo, len(infos))
	for i, info := range infos {
		wires[i] = WireExtInfo{
			Name:          info.Name,
			Version:       info.Version,
			Kind:          info.Kind,
			Runtime:       info.Runtime,
			Tools:         info.Tools,
			Commands:      info.Commands,
			Interceptors:  info.Interceptors,
			EventHandlers: info.EventHandlers,
			Shortcuts:     info.Shortcuts,
			MessageHooks:  info.MessageHooks,
			Compactor:     info.Compactor,
		}
	}
	h.respond(*msg.ID, HostExtInfosResult{Extensions: wires})
}

func (h *Host) handleHostExtensionsDir(msg *Message) {
	dir, err := ExtensionsDir()
	if err != nil {
		h.respondError(*msg.ID, -32603, "extensions dir: "+err.Error())
		return
	}
	h.respond(*msg.ID, HostExtensionsDirResult{Path: dir})
}

func (h *Host) handleHostUndoSnapshots(msg *Message) {
	if !h.requireUndoSnapshots(msg) {
		return
	}
	snapshots, err := h.undoSnapshotsFn()
	if err != nil {
		h.respondError(*msg.ID, -32603, "undo snapshots: "+err.Error())
		return
	}
	sizes := make(map[string]int, len(snapshots))
	for path, data := range snapshots {
		sizes[path] = len(data)
	}
	h.respond(*msg.ID, HostUndoSnapshotsResult{Snapshots: sizes})
}

func (h *Host) handleHostUndoRestore(msg *Message) {
	var params HostUndoRestoreParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireUndoSnapshots(msg) {
		return
	}
	snapshots, err := h.undoSnapshotsFn()
	if err != nil {
		h.respondError(*msg.ID, -32603, "undo snapshots: "+err.Error())
		return
	}
	data, ok := snapshots[params.Path]
	if !ok {
		h.respondError(*msg.ID, -32604, "no snapshot for path: "+params.Path)
		return
	}
	if err := config.AtomicWrite(params.Path, data, 0600); err != nil {
		h.respondError(*msg.ID, -32603, "restore file: "+err.Error())
		return
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostPublish(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostPublishParams) (struct{}, error) {
		app.Publish(p.Topic, p.Data)
		return struct{}{}, nil
	})
}

func (h *Host) handleHostSubscribe(msg *Message) {
	var params HostSubscribeParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}

	// Generate subscription ID
	subID := int(h.nextID.Add(1))

	// Subscribe on the event bus and forward events as notifications to the extension
	unsub := h.app.Subscribe(params.Topic, func(data any) {
		dataJSON, _ := json.Marshal(data)
		h.sendNotification(MethodHostEventBusEvent, EventBusEventParams{
			Topic:          params.Topic,
			SubscriptionID: subID,
			Data:           dataJSON,
		})
	})

	// Track unsubscribe function for cleanup
	h.subsMu.Lock()
	h.subscriptions[subID] = unsub
	h.subsMu.Unlock()

	h.respond(*msg.ID, HostSubscribeResult{SubscriptionID: subID})
}

func (h *Host) handleHostActivateTool(msg *Message) {
	var params HostActivateToolParams
	if !h.decodeParams(msg, &params) {
		return
	}
	if !h.requireApp(msg) {
		return
	}
	if !h.app.ActivateTool(params.Name) {
		h.respondError(*msg.ID, -32604, "tool not found or not deferred: "+params.Name)
		return
	}
	h.respond(*msg.ID, struct{}{})
}

func (h *Host) handleHostSetToolFilter(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostSetToolFilterParams) (struct{}, error) {
		app.SetToolFilterByName(p.Names)
		return struct{}{}, nil
	})
}
