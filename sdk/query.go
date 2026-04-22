package sdk

import "context"

// ---------------------------------------------------------------------------
// Host registry query methods (read-only)
// ---------------------------------------------------------------------------

// CommandInfo describes a registered slash command.
type CommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Commands returns all commands currently registered with the host, sorted by name.
func (e *Extension) Commands(ctx context.Context) ([]CommandInfo, error) {
	type resp struct {
		Commands []CommandInfo `json:"commands"`
	}
	r, err := hostCall[resp](e, ctx, "host/commands", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Commands, nil
}

// ToolDefInfo describes a registered tool (name + description only; schema omitted).
type ToolDefInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ToolDefs returns all tool definitions currently registered with the host, sorted by name.
func (e *Extension) ToolDefs(ctx context.Context) ([]ToolDefInfo, error) {
	type resp struct {
		Tools []ToolDefInfo `json:"tools"`
	}
	r, err := hostCall[resp](e, ctx, "host/toolDefs", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Tools, nil
}

// ShortcutInfo describes a registered keyboard shortcut.
type ShortcutInfo struct {
	Description string `json:"description"`
}

// Shortcuts returns all shortcuts currently registered with the host.
// The returned map key is the keybind string (e.g. "ctrl+g").
func (e *Extension) Shortcuts(ctx context.Context) (map[string]ShortcutInfo, error) {
	type resp struct {
		Shortcuts map[string]ShortcutInfo `json:"shortcuts"`
	}
	r, err := hostCall[resp](e, ctx, "host/shortcuts", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Shortcuts, nil
}

// PromptSectionInfo describes a registered prompt section (title + token budget hint).
type PromptSectionInfo struct {
	Title     string `json:"title"`
	TokenHint int    `json:"tokenHint,omitempty"`
}

// PromptSections returns all prompt sections currently registered with the host.
// Content is omitted; use this for token-budget accounting or display.
func (e *Extension) PromptSections(ctx context.Context) ([]PromptSectionInfo, error) {
	type resp struct {
		Sections []PromptSectionInfo `json:"sections"`
	}
	r, err := hostCall[resp](e, ctx, "host/promptSections", struct{}{})
	if err != nil {
		return nil, err
	}
	return r.Sections, nil
}
