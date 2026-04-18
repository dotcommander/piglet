package external

import (
	"fmt"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
)

// ---------------------------------------------------------------------------
// Host model service: extension → host (request/response)
// Added for T6a — supports /model command extraction (T6b).
// ---------------------------------------------------------------------------

// WireModelInfo is the wire representation of a configured model.
type WireModelInfo struct {
	ID            string  `json:"id"`
	API           string  `json:"api"`
	DisplayName   string  `json:"displayName"`
	Provider      string  `json:"provider"`
	ContextWindow int     `json:"contextWindow,omitempty"`
	MaxTokens     int     `json:"maxTokens,omitempty"`
	Reasoning     bool    `json:"reasoning,omitempty"`
	CostInput     float64 `json:"costInput,omitempty"`
	CostOutput    float64 `json:"costOutput,omitempty"`
	Current       bool    `json:"current"`
}

// HostAvailableModelsResult is the host's response to host/availableModels.
type HostAvailableModelsResult struct {
	Models []WireModelInfo `json:"models"`
}

// HostSwitchModelParams is the extension's request to switch the active model.
type HostSwitchModelParams struct {
	ModelID        string `json:"modelId"`
	PersistDefault bool   `json:"persistDefault,omitempty"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (h *Host) handleHostAvailableModels(msg *Message) {
	if !h.requireApp(msg) {
		return
	}
	models := h.app.AvailableModels()
	currentID := h.app.CurrentModelID()

	wire := make([]WireModelInfo, len(models))
	for i, m := range models {
		key := m.Provider + "/" + m.ID
		wire[i] = WireModelInfo{
			ID:            m.ID,
			API:           string(m.API),
			DisplayName:   m.DisplayName(),
			Provider:      m.Provider,
			ContextWindow: m.ContextWindow,
			MaxTokens:     m.MaxTokens,
			Reasoning:     m.Reasoning,
			CostInput:     m.Cost.Input,
			CostOutput:    m.Cost.Output,
			Current:       key == currentID,
		}
	}
	h.respond(*msg.ID, HostAvailableModelsResult{Models: wire})
}

func (h *Host) handleHostSwitchModel(msg *Message) {
	handleAppCall(h, msg, func(app *ext.App, p HostSwitchModelParams) (struct{}, error) {
		if p.ModelID == "" {
			return struct{}{}, fmt.Errorf("modelId is required")
		}
		if err := app.SwitchModel(p.ModelID); err != nil {
			return struct{}{}, fmt.Errorf("switch model: %w", err)
		}
		if p.PersistDefault {
			cfg, err := config.Load()
			if err != nil {
				return struct{}{}, fmt.Errorf("load config: %w", err)
			}
			cfg.DefaultModel = p.ModelID
			if err := config.Save(cfg); err != nil {
				return struct{}{}, fmt.Errorf("save config: %w", err)
			}
		}
		return struct{}{}, nil
	})
}
