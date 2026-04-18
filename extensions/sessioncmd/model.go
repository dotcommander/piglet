package sessioncmd

import (
	"context"
	"fmt"

	"github.com/dotcommander/piglet/sdk"
)

func registerModel(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "model",
		Description: "Switch model",
		Immediate:   true,
		Handler: func(ctx context.Context, args string) error {
			return openModelPicker(ctx, e)
		},
	})
}

// openModelPicker shows the model selection picker and switches on selection.
// Shared between the slash command and the ctrl+p shortcut.
func openModelPicker(ctx context.Context, e *sdk.Extension) error {
	models, err := e.AvailableModels(ctx)
	if err != nil {
		e.ShowMessage("Failed to list models: " + err.Error())
		return nil
	}
	if len(models) == 0 {
		e.ShowMessage("No models available")
		return nil
	}
	items := make([]sdk.PickerItem, len(models))
	for i, m := range models {
		label := m.DisplayName
		if m.Current {
			label = fmt.Sprintf("%s (current)", m.DisplayName)
		}
		items[i] = sdk.PickerItem{
			ID:    m.Provider + "/" + m.ID,
			Label: label,
			Desc:  m.Provider,
		}
	}
	selected, err := e.ShowPicker(ctx, "Select Model", items)
	if err != nil || selected == "" {
		// Picker dismissed or timed out; silently return. Do NOT ShowMessage.
		return nil
	}
	if err := e.SwitchModel(ctx, selected, true); err != nil {
		e.ShowMessage("Failed to switch model: " + err.Error())
		return nil
	}
	// Find display name for confirmation message.
	displayName := selected
	for _, m := range models {
		if m.Provider+"/"+m.ID == selected {
			displayName = m.DisplayName
			break
		}
	}
	e.ShowMessage("Switched to " + displayName)
	return nil
}
