// MCP extension binary. Connects to configured MCP servers and exposes
// their tools as piglet tools via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/extensions/mcp"
	sdk "github.com/dotcommander/piglet/sdk"
)

func main() {
	e := sdk.New("mcp", "0.1.0")

	var mgr *mcp.Manager

	e.OnInit(func(ext *sdk.Extension) {
		start := time.Now()
		ext.Log("debug", "[mcp] OnInit start")

		cfg := mcp.LoadConfig()
		if len(cfg.Servers) == 0 {
			ext.Log("debug", fmt.Sprintf("[mcp] OnInit complete — no servers configured (%s)", time.Since(start)))
			return
		}

		ext.Log("debug", fmt.Sprintf("[mcp] NewManager start (%s)", time.Since(start)))
		mgr = mcp.NewManager()
		ext.Log("debug", fmt.Sprintf("[mcp] NewManager done (%s)", time.Since(start)))

		ctx := context.Background()
		ext.Log("debug", fmt.Sprintf("[mcp] StartAll start (%s)", time.Since(start)))
		errs := mgr.StartAll(ctx, cfg.Servers)
		ext.Log("debug", fmt.Sprintf("[mcp] StartAll done (%s)", time.Since(start)))
		for _, err := range errs {
			ext.Log("warn", "mcp: "+err.Error())
		}

		// Discover and register tools
		ext.Log("debug", fmt.Sprintf("[mcp] DiscoverTools start (%s)", time.Since(start)))
		tools := mgr.DiscoverTools(ctx)
		ext.Log("debug", fmt.Sprintf("[mcp] DiscoverTools done — %d tool(s) (%s)", len(tools), time.Since(start)))
		for _, t := range tools {
			registerMCPTool(ext, t)
		}

		if len(tools) > 0 {
			ext.Log("info", fmt.Sprintf("mcp: %d tool(s) from %d server(s)", len(tools), countServers(tools)))
		}

		// Prompt section with instructions
		if inst := mgr.Instructions(); inst != "" {
			ext.RegisterPromptSection(sdk.PromptSectionDef{
				Title:   "MCP Server Instructions",
				Content: inst,
				Order:   80,
			})
		}

		ext.Log("debug", fmt.Sprintf("[mcp] OnInit complete (%s)", time.Since(start)))
	})

	// /mcp command: show server status
	e.RegisterCommand(sdk.CommandDef{
		Name:        "mcp",
		Description: "Show MCP server connection status",
		Handler: func(ctx context.Context, _ string) error {
			if mgr == nil {
				if cfgDir, err := xdg.ExtensionDir("mcp"); err == nil {
					e.ShowMessage("No MCP servers configured. Add servers to " + cfgDir + "/mcp.yaml")
				} else {
					e.ShowMessage("No MCP servers configured. Add servers to your mcp.yaml config file.")
				}
				return nil
			}

			statuses := mgr.Status(ctx)
			if len(statuses) == 0 {
				e.ShowMessage("No MCP servers configured.")
				return nil
			}

			var b strings.Builder
			b.WriteString("MCP Servers:\n\n")
			for _, s := range statuses {
				if s.Error != "" {
					fmt.Fprintf(&b, "  %s: error — %s\n", s.Name, s.Error)
				} else {
					fmt.Fprintf(&b, "  %s: connected (%d tools)\n", s.Name, s.ToolCount)
				}
			}
			e.ShowMessage(b.String())
			return nil
		},
	})

	e.Run()
}

// registerMCPTool wraps a discovered MCP tool as a piglet tool.
func registerMCPTool(ext *sdk.Extension, t mcp.DiscoveredTool) {
	client := t.Client
	toolName := t.ToolName

	ext.RegisterTool(sdk.ToolDef{
		Name:        t.FullName,
		Description: fmt.Sprintf("[MCP:%s] %s", t.ServerName, t.Description),
		Parameters:  t.Schema,
		PromptHint:  fmt.Sprintf("MCP tool from %s server", t.ServerName),
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			r := mcp.CallAndExtract(ctx, client, toolName, args)
			if r.IsError {
				return sdk.ErrorResult(r.Text), nil
			}
			return sdk.TextResult(r.Text), nil
		},
	})
}

func countServers(tools []mcp.DiscoveredTool) int {
	seen := make(map[string]bool)
	for _, t := range tools {
		seen[t.ServerName] = true
	}
	return len(seen)
}
