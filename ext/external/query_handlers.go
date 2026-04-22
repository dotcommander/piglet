package external

import (
	"cmp"
	"slices"
)

// handleHostCommands returns all registered commands sorted by name.
func (h *Host) handleHostCommands(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	cmds := h.app.Commands()
	infos := make([]HostCommandInfo, 0, len(cmds))
	for _, c := range cmds {
		infos = append(infos, HostCommandInfo{Name: c.Name, Description: c.Description})
	}
	slices.SortFunc(infos, func(a, b HostCommandInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})
	h.respond(*msg.ID, HostCommandsResult{Commands: infos})
}

// handleHostToolDefs returns all registered tool definitions (name + description).
func (h *Host) handleHostToolDefs(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	defs := h.app.ToolDefs()
	infos := make([]HostToolDefInfo, len(defs))
	for i, d := range defs {
		infos[i] = HostToolDefInfo{Name: d.Name, Description: d.Description}
	}
	h.respond(*msg.ID, HostToolDefsResult{Tools: infos})
}

// handleHostShortcuts returns all registered shortcuts keyed by keybind string.
func (h *Host) handleHostShortcuts(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	shortcuts := h.app.Shortcuts()
	wire := make(map[string]HostShortcutInfo, len(shortcuts))
	for key, s := range shortcuts {
		wire[key] = HostShortcutInfo{Description: s.Description}
	}
	h.respond(*msg.ID, HostShortcutsResult{Shortcuts: wire})
}

// handleHostPromptSections returns all registered prompt sections (title + tokenHint only).
func (h *Host) handleHostPromptSections(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	sections := h.app.PromptSections()
	infos := make([]HostPromptSectionInfo, len(sections))
	for i, ps := range sections {
		infos[i] = HostPromptSectionInfo{Title: ps.Title, TokenHint: ps.TokenHint}
	}
	h.respond(*msg.ID, HostPromptSectionsResult{Sections: infos})
}
