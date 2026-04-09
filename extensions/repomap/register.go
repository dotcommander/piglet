package repomap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// codeChangingTools lists tool names that modify source code.
var codeChangingTools = map[string]bool{
	"write_file":    true,
	"edit_file":     true,
	"bash":          true,
	"notebook_edit": true,
	"multi_edit":    true,
}

// inventoryParams defines parameters for the repomap_inventory tool.
var inventoryParams = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"action": map[string]any{
			"type":        "string",
			"enum":        []string{"scan", "query"},
			"description": "scan: rebuild inventory from disk. query: filter existing inventory.",
		},
		"filter": map[string]any{
			"type":        "string",
			"description": "Filter expression for query (e.g. 'lines>100', 'path=internal/')",
		},
	},
	"required": []string{"action"},
}

// repomapToolParams is shared between repomap_show and repomap_refresh tools.
var repomapToolParams = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"verbose": map[string]any{
			"type":        "boolean",
			"description": "Show all symbols grouped by category (default: false)",
		},
		"detail": map[string]any{
			"type":        "boolean",
			"description": "Show all symbols with full signatures (default: false)",
		},
	},
}

// Register wires the repomap extension into a shared SDK extension.
func Register(e *sdk.Extension) {
	s := new(initState)

	e.OnInit(func(x *sdk.Extension) {
		handleOnInit(x, s)
	})

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "repomap-stale-check",
		Priority: 50,
		Events:   []string{"EventTurnEnd"},
		Handle: func(ctx context.Context, eventType string, data json.RawMessage) *sdk.Action {
			if s.rm == nil {
				return nil
			}

			if turnModifiedCode(data) {
				s.rm.Dirty()
			}

			if !s.rm.Stale() {
				return nil
			}
			go func() {
				buildCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := s.rm.Build(buildCtx); err != nil {
					if !errors.Is(err, ErrNotCodeProject) {
						s.extRef.Log("warn", "repomap rebuild failed: "+err.Error())
					}
				}
			}()
			return nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "repomap_refresh",
		Description: "Force rebuild the repository map after major file changes.",
		Deferred:    true,
		Parameters:  repomapToolParams,
		PromptHint:  "Rebuild the repository map after major file changes",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if s.rm == nil {
				return sdk.ErrorResult("repository map not initialized"), nil
			}
			if err := s.rm.Build(ctx); err != nil {
				return sdk.ErrorResult("rebuild failed: " + err.Error()), nil
			}
			s.setBuilt(true)
			return sdk.TextResult(formatRepomapOutput(s.rm, args)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "repomap_show",
		Description: "Show the current repository structure map with source code definitions.",
		Deferred:    true,
		Parameters:  repomapToolParams,
		PromptHint:  "Show the current repository structure map (default: source lines, verbose/detail for alternatives)",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if s.rm == nil {
				return sdk.TextResult("Repository map not initialized."), nil
			}
			out := formatRepomapOutput(s.rm, args)
			if out == "" {
				if !s.isBuilt() {
					return sdk.TextResult("Repository map is still building..."), nil
				}
				return sdk.TextResult("Repository map is empty (no source files found)."), nil
			}
			return sdk.TextResult(out), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "repomap_inventory",
		Description: "Scan repository files for metrics (lines, imports) and query the inventory.",
		Deferred:    true,
		Parameters:  inventoryParams,
		PromptHint:  "Query per-file metrics: lines, imports. Use 'scan' to build, 'query' to filter.",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if s.extRef == nil {
				return sdk.TextResult("repomap not initialized"), nil
			}
			action, _ := args["action"].(string)
			filter, _ := args["filter"].(string)

			switch action {
			case "scan":
				inv, err := ScanInventory(ctx, s.extRef.CWD())
				if err != nil {
					return sdk.ErrorResult("scan failed: " + err.Error()), nil
				}
				if err := PersistInventory(inv, repomapCacheDir()); err != nil {
					s.extRef.Log("warn", "inventory persist failed: "+err.Error())
				}
				header := fmt.Sprintf("Inventory: %d files (scanned %s)\n\n", len(inv.Files), inv.Scanned)
				if inv.Truncated {
					header = fmt.Sprintf("Inventory: %d files — truncated at cap %d (scanned %s)\n\n", len(inv.Files), inventoryFileCap, inv.Scanned)
				}
				return sdk.TextResult(formatInventoryTable(inv.Files, header)), nil
			case "query":
				out, err := QueryInventory(repomapCacheDir(), filter)
				if err != nil {
					return sdk.ErrorResult(err.Error()), nil
				}
				return sdk.TextResult(out), nil
			default:
				return sdk.ErrorResult("unknown action: " + action + " (expected 'scan' or 'query')"), nil
			}
		},
	})
}

// formatRepomapOutput returns the repomap in the requested format.
func formatRepomapOutput(rm *Map, args map[string]any) string {
	verbose, _ := args["verbose"].(bool)
	detail, _ := args["detail"].(bool)
	switch {
	case detail:
		return rm.StringDetail()
	case verbose:
		return rm.StringVerbose()
	default:
		return rm.StringLines()
	}
}
