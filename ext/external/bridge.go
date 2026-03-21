package external

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// LoadAll discovers and starts all external extensions, registering their
// tools, commands, and prompt sections with the given ext.App.
// Returns a cleanup function that stops all extension processes.
func LoadAll(ctx context.Context, app *ext.App) (cleanup func(), err error) {
	extDir, err := ExtensionsDir()
	if err != nil {
		return func() {}, nil // non-fatal
	}

	manifests, err := DiscoverExtensions(extDir)
	if err != nil {
		return func() {}, nil // non-fatal
	}

	if len(manifests) == 0 {
		return func() {}, nil
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

	return func() {
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
	app.RegisterExtInfo(info)
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

}

// proxyToolExecute returns a ToolExecuteFn that proxies to the extension process.
func proxyToolExecute(h *Host, toolName string) core.ToolExecuteFn {
	return func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
		result, err := h.ExecuteTool(ctx, id, toolName, args)
		if err != nil {
			return nil, fmt.Errorf("ext %s tool %s: %w", h.Name(), toolName, err)
		}

		blocks := make([]core.ContentBlock, 0, len(result.Content))
		for _, b := range result.Content {
			switch b.Type {
			case "image":
				blocks = append(blocks, core.ImageContent{Data: b.Data, MimeType: b.Mime})
			default:
				blocks = append(blocks, core.TextContent{Text: b.Text})
			}
		}

		return &core.ToolResult{Content: blocks}, nil
	}
}

// proxyCommandExecute returns a command handler that proxies to the extension.
func proxyCommandExecute(h *Host, cmdName string) func(args string, app *ext.App) error {
	return func(args string, app *ext.App) error {
		ctx := context.Background()
		return h.ExecuteCommand(ctx, cmdName, args)
	}
}
