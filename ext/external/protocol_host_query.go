package external

// ---------------------------------------------------------------------------
// Host query service: extension → host (read-only registry queries)
// ---------------------------------------------------------------------------

// HostCommandInfo is the wire representation of one registered command.
type HostCommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// HostCommandsResult is the host's response to host/commands.
type HostCommandsResult struct {
	Commands []HostCommandInfo `json:"commands"`
}

// HostToolDefInfo is the wire representation of one registered tool definition
// (name + description only — input schema is omitted for lightweight /help use).
type HostToolDefInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// HostToolDefsResult is the host's response to host/toolDefs.
type HostToolDefsResult struct {
	Tools []HostToolDefInfo `json:"tools"`
}

// HostShortcutInfo is the wire representation of one registered shortcut.
type HostShortcutInfo struct {
	Description string `json:"description"`
}

// HostShortcutsResult is the host's response to host/shortcuts.
// Key is the keybind string (e.g. "ctrl+g").
type HostShortcutsResult struct {
	Shortcuts map[string]HostShortcutInfo `json:"shortcuts"`
}

// HostPromptSectionInfo is the wire representation of one registered prompt section
// (title + token hint only — Content is omitted; consumers care about budget, not text).
type HostPromptSectionInfo struct {
	Title     string `json:"title"`
	TokenHint int    `json:"tokenHint,omitzero"`
}

// HostPromptSectionsResult is the host's response to host/promptSections.
type HostPromptSectionsResult struct {
	Sections []HostPromptSectionInfo `json:"sections"`
}
